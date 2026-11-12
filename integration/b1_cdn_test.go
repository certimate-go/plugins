package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

var b1CDNProviders = []string{"asiaispcdn", "cdnfly", "lecdn"}

type b1CDNFixture struct {
	expectedID              string
	requestStarted          chan struct{}
	t                       *testing.T
	mu                      sync.Mutex
	mode                    string
	certPEM, keyPEM         string
	tlsURL                  string
	url                     string
	uploads, updates, calls int
	certificates            map[string][2]string
	boundID                 string
	boundCert               string
	clientID                any
}

func TestB1CDNDeploymentBatch(t *testing.T) {
	if runtime.GOOS != "linux" {
		runB1CDNBatchInLinux(t)
		return
	}
	binaryDir := os.Getenv("CERTIMATE_B1_CDN_E2E_BINARIES")
	if binaryDir == "" {
		binaryDir = t.TempDir()
		buildB1CDNBatch(t, binaryDir, runtime.GOARCH)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := plugin.PluginConfig{PluginDir: binaryDir, StartTimeout: 15 * time.Second, DeployTimeout: 15 * time.Second}
	discovered, failures := plugin.Discover(ctx, cfg)
	if len(failures) != 0 || len(discovered) != 3 {
		t.Fatalf("discovery count=%d failures=%v", len(discovered), failures)
	}
	byName := map[string]*plugin.DiscoveredPlugin{}
	for _, dp := range discovered {
		byName[dp.Manifest.ProviderType] = dp
	}
	manager := plugin.NewManager(cfg, slog.New(slog.DiscardHandler))
	fixture := newB1CDNFixture(t)
	for _, provider := range b1CDNProviders {
		t.Run(provider, func(t *testing.T) {
			dp := byName[provider]
			meta, schema, err := manager.Bootstrap(ctx, dp)
			if err != nil {
				t.Fatal(err)
			}
			validateSchema(t, schema, provider, provider, meta)
			if meta.ProviderType != provider || meta.AccessProviderType != provider || meta.DeployCategory != "cdn" || meta.ProtocolVersion != plugin.ProtocolVersion || dp.Manifest.DisplayNameKey != meta.DeployDisplayNameKey {
				t.Fatalf("unexpected metadata %+v", meta)
			}
			req := plugin.DeployRequest{CertificatePEM: fixture.certPEM, PrivateKeyPEM: fixture.keyPEM, LogLevel: "DEBUG"}
			switch provider {
			case "asiaispcdn":
				req.AccessConfigJSON = `{"accessKeyId":"fixture-key","accessKeySecret":"fixture-secret"}`
				req.ExtendedConfigJSON = `{"domainMatchPattern":"exact","domain":"cdn.example.com"}`
			case "cdnfly":
				req.AccessConfigJSON = fmt.Sprintf(`{"serverUrl":%q,"apiKey":"fixture-key","apiSecret":"fixture-secret"}`, fixture.url)
				req.ExtendedConfigJSON = `{"deployTarget":"website","siteId":"42","certificateId":""}`
			case "lecdn":
				req.AccessConfigJSON = fmt.Sprintf(`{"serverUrl":%q,"apiVersion":"v4","apiRole":"client","authMethod":"apikey","apiKey":"fixture-secret"}`, fixture.url)
				req.ExtendedConfigJSON = `{"deployTarget":"certificate","certificateId":"42","clientId":""}`
			}
			var workflowDeploy func(context.Context, *plugin.DeployRequest) (*plugin.DeployResult, error)
			if os.Getenv("CERTIMATE_B1_CORE_BINARY") != "" {
				workflowDeploy = newB1WorkflowDeployer(t, binaryDir, provider)
			}
			deploy := func(t *testing.T, request plugin.DeployRequest, wantError bool) {
				t.Helper()
				var logs bytes.Buffer
				var result *plugin.DeployResult
				var err error
				if workflowDeploy != nil {
					result, err = workflowDeploy(ctx, &request)
				} else {
					result, err = manager.Deploy(ctx, dp, &request, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
				}
				for _, secret := range []string{strings.Split(request.PrivateKeyPEM, "\n")[1], base64.StdEncoding.EncodeToString([]byte(request.PrivateKeyPEM)), "fixture-secret", "fixture-session"} {
					if strings.Contains(logs.String(), secret) || err != nil && strings.Contains(err.Error(), secret) {
						t.Fatal("secret leaked in logs or error")
					}
				}
				if wantError {
					if err == nil {
						t.Fatal("expected failure")
					}
					return
				}
				if err != nil || result == nil {
					t.Fatalf("deployment result=%v error=%v logs=%s", result, err, logs.String())
				}
			}
			t.Run("deploy_repeat_renew", func(t *testing.T) {
				fixture.reset()
				deploy(t, req, false)
				fixture.assertBound(t, req.CertificatePEM)
				deploy(t, req, false)
				fixture.assertBound(t, req.CertificatePEM)
				fixture.mu.Lock()
				uploads := fixture.uploads
				fixture.mu.Unlock()
				if provider == "asiaispcdn" && uploads != 1 {
					t.Fatalf("repeat uploaded %d certificates", uploads)
				}
				renewed := req
				renewed.CertificatePEM, renewed.PrivateKeyPEM, _ = makeCertificate(t, []string{"cdn.example.com"}, false)
				fixture.setExpected(renewed.CertificatePEM, renewed.PrivateKeyPEM)
				deploy(t, renewed, false)
				fixture.assertBound(t, renewed.CertificatePEM)
				fixture.setExpected(req.CertificatePEM, req.PrivateKeyPEM)
			})
			for _, mode := range []string{"auth", "failure", "bind-failure"} {
				t.Run(mode+"_recovery", func(t *testing.T) {
					fixture.reset()
					fixture.setMode(mode)
					deploy(t, req, true)
					fixture.mu.Lock()
					bound := fixture.boundCert
					calls := fixture.calls
					fixture.mu.Unlock()
					if bound != "" || calls == 0 {
						t.Fatalf("failure effects bound=%t calls=%d", bound != "", calls)
					}
					fixture.setMode("")
					deploy(t, req, false)
					fixture.assertBound(t, req.CertificatePEM)
				})
			}

			t.Run("cancel_then_recovery", func(t *testing.T) {
				fixture.reset()
				fixture.setMode("hang")
				cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				completed := make(chan error, 1)
				go func() { _, err := manager.Deploy(cancelCtx, dp, &req, slog.New(slog.DiscardHandler)); completed <- err }()
				select {
				case <-fixture.requestStarted:
					cancel()
				case <-cancelCtx.Done():
					t.Fatal("cancellation request never reached API")
				}
				if err := <-completed; err == nil {
					t.Fatal("canceled deployment succeeded")
				}

				fixture.setMode("")
				deploy(t, req, false)
				fixture.assertBound(t, req.CertificatePEM)
			})
			if provider != "asiaispcdn" {
				t.Run("tls_requires_opt_in", func(t *testing.T) {
					fixture.reset()
					request := req
					request.AccessConfigJSON = strings.ReplaceAll(request.AccessConfigJSON, fixture.url, fixture.tlsURL)
					deploy(t, request, true)
					request.AccessConfigJSON = strings.TrimSuffix(request.AccessConfigJSON, "}") + `,"allowInsecureConnections":true}`
					deploy(t, request, false)
					fixture.assertBound(t, req.CertificatePEM)
				})
			}
			if provider == "asiaispcdn" {
				for _, pattern := range []string{"wildcard", "certsan"} {
					t.Run(pattern, func(t *testing.T) {
						fixture.reset()
						request := req
						request.ExtendedConfigJSON = fmt.Sprintf(`{"domainMatchPattern":%q,"domain":"*.example.com"}`, pattern)
						deploy(t, request, false)
						fixture.assertBound(t, req.CertificatePEM)
					})
				}
			}
			if provider == "cdnfly" {
				t.Run("numeric_id_preserves_legacy_failure", func(t *testing.T) {
					fixture.reset()
					request := req
					request.ExtendedConfigJSON = `{"deployTarget":"website","siteId":42}`
					deploy(t, request, true)
					fixture.mu.Lock()
					calls := fixture.calls
					fixture.mu.Unlock()
					if calls != 0 {
						t.Fatal("numeric legacy site ID reached API")
					}
				})

				for _, target := range []string{"website", "certificate"} {
					for _, id := range []string{`"42"`, `"9007199254740993"`} {
						t.Run(target+id, func(t *testing.T) {
							fixture.reset()
							request := req
							request.ExtendedConfigJSON = fmt.Sprintf(`{"deployTarget":%q,"siteId":%s,"certificateId":%s}`, target, id, id)
							fixture.mu.Lock()
							fixture.expectedID = strings.Trim(id, `"`)
							fixture.mu.Unlock()
							deploy(t, request, false)
							fixture.assertBound(t, req.CertificatePEM)
						})
					}
				}
			}
			if provider == "lecdn" {
				for _, version := range []string{"v3", "v4"} {
					for _, role := range []string{"client", "master"} {
						for _, method := range []string{"password", "apikey"} {
							t.Run(version+"_"+role+"_"+method, func(t *testing.T) {
								fixture.reset()
								request := req
								request.AccessConfigJSON = fmt.Sprintf(`{"serverUrl":%q,"apiVersion":%q,"apiRole":%q,"authMethod":%q,"username":"fixture-user","password":"fixture-secret","apiKey":"fixture-secret"}`, fixture.url, version, role, method)
								request.ExtendedConfigJSON = `{"deployTarget":"certificate","certificateId":42,"clientId":"7"}`
								deploy(t, request, false)
								fixture.assertBound(t, req.CertificatePEM)
								fixture.mu.Lock()
								clientID := fixture.clientID
								fixture.mu.Unlock()
								if version == "v3" && role == "master" && clientID != json.Number("7") {
									t.Fatalf("client ID lost: %v", clientID)
								}
							})
						}
					}
				}
			}
		})
	}
}

func (f *b1CDNFixture) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = ""
	f.requestStarted = make(chan struct{})
	f.uploads = 0
	f.updates = 0
	f.calls = 0
	f.certificates = map[string][2]string{}
	f.expectedID = "42"
	f.boundID = ""
	f.boundCert = ""
	f.clientID = nil
}
func (f *b1CDNFixture) setMode(mode string) { f.mu.Lock(); defer f.mu.Unlock(); f.mode = mode }
func (f *b1CDNFixture) setExpected(cert, key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.certPEM = cert
	f.keyPEM = key
}

func (f *b1CDNFixture) assertBound(t *testing.T, cert string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.boundCert != cert || f.updates == 0 {
		t.Fatalf("target does not have expected certificate: updates=%d", f.updates)
	}
}

func newB1CDNFixture(t *testing.T) *b1CDNFixture {
	cert, key, _ := makeCertificate(t, []string{"cdn.example.com"}, false)
	f := &b1CDNFixture{t: t, certPEM: cert, keyPEM: key}
	f.reset()
	server := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(server.Close)
	f.url = server.URL
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(f.handle))
	t.Cleanup(tlsServer.Close)
	f.tlsURL = tlsServer.URL
	ca, caKey, _ := makeCertificate(t, []string{"api.asia-isp.com"}, true)
	pair, err := tls.X509KeyPair([]byte(ca), []byte(caKey))
	if err != nil {
		t.Fatal(err)
	}
	cloud := httptest.NewUnstartedServer(http.HandlerFunc(f.handle))
	cloud.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	cloud.StartTLS()
	t.Cleanup(cloud.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "api.asia-isp.com:443" {
			http.Error(w, "unexpected destination", 502)
			return
		}
		upstream, err := net.DialTimeout("tcp", cloud.Listener.Addr().String(), 5*time.Second)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstream.Close()
			return
		}
		defer client.Close()
		defer upstream.Close()
		if _, err = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if buffer.Flush() != nil {
			return
		}
		done := make(chan struct{})
		go func() { io.Copy(upstream, buffer); upstream.Close(); close(done) }()
		io.Copy(client, upstream)
		client.Close()
		<-done
	}))
	t.Cleanup(proxy.Close)
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, []byte(ca), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("SSL_CERT_FILE", caPath)
	return f
}

func (f *b1CDNFixture) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.mode == "hang" {
		io.Copy(io.Discard, r.Body)
		close(f.requestStarted)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if f.mode == "auth" {
		http.Error(w, `{"code":401,"message":"authentication failed"}`, 401)
		return
	}
	if f.mode == "failure" || f.mode == "bind-failure" && r.Method == http.MethodPut {
		http.Error(w, `{"code":500,"message":"fixture failure"}`, 500)
		return
	}
	body := map[string]any{}
	var rawBody []byte
	if r.Method != http.MethodGet {
		rawBody, _ = io.ReadAll(r.Body)
		d := json.NewDecoder(bytes.NewReader(rawBody))
		d.UseNumber()
		if err := d.Decode(&body); err != nil {
			f.t.Error(err)
			return
		}
	}
	check := func(label string, got, want any) {
		if !reflect.DeepEqual(got, want) {
			f.t.Errorf("%s %s differs", r.URL.Path, label)
		}
	}
	respond := func(value any) {
		if err := json.NewEncoder(w).Encode(value); err != nil {
			f.t.Error(err)
		}
	}
	if r.Host == "api.asia-isp.com" {
		check("access key", r.Header.Get("accessKeyId"), "fixture-key")
		signatureInput := fmt.Sprintf("accessKeySecret=fixture-secret&method=%s&nonce=%s&queryString=%s&timestamp=%s&uri=%s", r.Method, r.Header.Get("nonce"), r.URL.Query().Encode(), r.Header.Get("timestamp"), r.URL.Path)
		if r.Method != http.MethodGet && r.Method != http.MethodDelete {
			signatureInput = fmt.Sprintf("accessKeySecret=fixture-secret&body=%s&method=%s&nonce=%s&queryString=%s&timestamp=%s&uri=%s", rawBody, r.Method, r.Header.Get("nonce"), r.URL.Query().Encode(), r.Header.Get("timestamp"), r.URL.Path)
		}
		signer := hmac.New(sha1.New, []byte("fixture-secret"))
		signer.Write([]byte(signatureInput))
		check("signature", r.Header.Get("signature"), base64.RawURLEncoding.EncodeToString(signer.Sum(nil)))
		switch r.URL.Query().Get("action") {
		case "certificateQueryList":
			rows := []any{}
			for id := range f.certificates {
				rows = append(rows, map[string]any{"certId": id, "name": "fixture"})
			}
			respond(map[string]any{"code": 0, "data": rows})
		case "certificateQuery":
			id := r.URL.Query().Get("certId")
			cert := f.certificates[id]
			respond(map[string]any{"code": 0, "data": map[string]any{"certId": id, "publicKey": cert[0], "privateKey": cert[1]}})
		case "certificateUpload":
			check("certificate", body["publicKey"], f.certPEM)
			check("key", body["privateKey"], f.keyPEM)
			f.uploads++
			id := fmt.Sprint(f.uploads)
			f.certificates[id] = [2]string{f.certPEM, f.keyPEM}
			respond(map[string]any{"code": 0, "data": id})
		case "domainQueryList":
			domain := r.URL.Query().Get("domain")
			if domain != "" {
				check("domain", domain, "cdn.example.com")
			}
			bound := json.Number("0")
			if f.boundID != "" {
				bound = json.Number(f.boundID)
			}
			respond(map[string]any{"code": 0, "data": []any{map[string]any{"domain": "cdn.example.com", "domainStatus": 1, "certId": bound}}})
		case "domainModify":
			check("domain", body["domain"], "cdn.example.com")
			check("protocol", body["protocol"], "https")
			id := fmt.Sprint(body["certId"])
			stored, ok := f.certificates[id]
			if !ok {
				f.t.Error("bound missing certificate")
			}
			f.boundID = id
			f.boundCert = stored[0]
			f.updates++
			respond(map[string]any{"code": 0, "data": 1})
		default:
			f.t.Errorf("unexpected action %s", r.URL.RawQuery)
		}
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/") {
		if strings.HasPrefix(r.URL.Path, "/v1/sites/") || strings.HasPrefix(r.URL.Path, "/v1/certs/") {
			parts := strings.Split(r.URL.Path, "/")
			check("target ID", parts[len(parts)-1], f.expectedID)
		}
		check("API key", r.Header.Get("API-Key"), "fixture-key")
		check("API secret", r.Header.Get("API-Secret"), "fixture-secret")
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/sites/") && r.Method == http.MethodGet:
			respond(map[string]any{"code": 0, "data": map[string]any{"id": 42, "https_listen": `{"port":443,"http2":true,"cert":"old"}`}})
		case r.URL.Path == "/v1/certs":
			check("certificate", body["cert"], f.certPEM)
			check("key", body["key"], f.keyPEM)
			f.uploads++
			id := fmt.Sprint(f.uploads)
			f.certificates[id] = [2]string{f.certPEM, f.keyPEM}
			respond(map[string]any{"code": 0, "data": id})
		case strings.HasPrefix(r.URL.Path, "/v1/sites/") && r.Method == http.MethodPut:
			listen, ok := body["https_listen"].(map[string]any)
			if !ok {
				f.t.Error("missing https settings")
				return
			}
			check("port", listen["port"], json.Number("443"))
			check("http2", listen["http2"], true)
			id := fmt.Sprint(listen["cert"])
			f.boundCert = f.certificates[id][0]
			f.updates++
			respond(map[string]any{"code": 0})
		case strings.HasPrefix(r.URL.Path, "/v1/certs/") && r.Method == http.MethodPut:
			check("certificate", body["cert"], f.certPEM)
			check("key", body["key"], f.keyPEM)
			f.boundCert = f.certPEM
			f.updates++
			respond(map[string]any{"code": 0})
		default:
			f.t.Errorf("unexpected CDNfly request %s %s", r.Method, r.URL.Path)
		}
		return
	}
	switch r.URL.Path {
	case "/prod-api/login", "/prod-api/auth/login":
		check("password", body["password"], "fixture-secret")
		respond(map[string]any{"code": 200, "data": map[string]any{"token": "fixture-session"}})
	case "/prod-api/certificate/42":
		auth := r.Header.Get("Authorization")
		if auth != "fixture-secret" && auth != "Bearer fixture-session" {
			f.t.Error("missing LeCDN authorization")
		}
		check("certificate", body["ssl_pem"], base64.StdEncoding.EncodeToString([]byte(f.certPEM)))
		check("key", body["ssl_key"], base64.StdEncoding.EncodeToString([]byte(f.keyPEM)))
		check("auto renewal", body["auto_renewal"], false)
		f.clientID = body["client_id"]
		f.boundCert = f.certPEM
		f.updates++
		respond(map[string]any{"code": 200})
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}
}

func buildB1CDNBatch(t *testing.T, dir, arch string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range b1CDNProviders {
		out := filepath.Join(dir, provider)
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(out, provider), "./"+provider)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("build %s: %v\n%s", provider, err, output)
		}
		manifestBytes, err := os.ReadFile(filepath.Join(root, provider, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := plugin.ParseManifest(manifestBytes)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"manifest.json", manifest.Icon} {
			data, err := os.ReadFile(filepath.Join(root, provider, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func runB1CDNBatchInLinux(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	archOutput, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.Architecture}}").CombinedOutput()
	if err != nil {
		t.Fatalf("real plugin TLS tests require Linux or Docker: %v\n%s", err, archOutput)
	}
	arch := strings.TrimSpace(string(archOutput))
	switch arch {
	case "aarch64":
		arch = "arm64"
	case "x86_64":
		arch = "amd64"
	}
	if arch != "amd64" && arch != "arm64" {
		t.Fatalf("unsupported Docker architecture %q", arch)
	}
	dir := t.TempDir()
	buildB1CDNBatch(t, dir, arch)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "b1-cdn-batch.test"), "./integration")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Linux tests: %v\n%s", err, output)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	if binary := os.Getenv("CERTIMATE_B1_CORE_BINARY"); binary != "" {
		data, err := os.ReadFile(binary)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "core-app"), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd = exec.CommandContext(ctx, "docker", "run", "--rm", "--network=none", "--mount", "type=bind,src="+dir+",dst=/fixtures,readonly", "-e", "CERTIMATE_B1_CDN_E2E_BINARIES=/fixtures", image, "/fixtures/b1-cdn-batch.test", "-test.run=^TestB1CDNDeploymentBatch$", "-test.v", "-test.timeout=5m")
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") != "" {
		for i, arg := range cmd.Args {
			if arg == image {
				cmd.Args = append(cmd.Args[:i], append([]string{"-e", "CERTIMATE_B1_CORE_BINARY=/fixtures/core-app", "-e", "CERTIMATE_B1_BASELINE=" + os.Getenv("CERTIMATE_B1_BASELINE")}, cmd.Args[i:]...)...)
				break
			}
		}
	}
	output, err := cmd.CombinedOutput()
	t.Logf("Linux plugin test output:\n%s", output)
	if err != nil {
		t.Fatalf("Linux end-to-end tests: %v", err)
	}
}
