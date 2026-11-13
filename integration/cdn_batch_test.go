package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

var cdnProviders = []string{"cachefly", "axisnow", "flexcdn"}

type cdnMode string

const (
	cdnNormal         cdnMode = "normal"
	cdnFailure        cdnMode = "failure"
	cdnUploadFailure  cdnMode = "upload-failure"
	cdnReuse          cdnMode = "reuse"
	cdnPaginatedReuse cdnMode = "paginated-reuse"
	cdnAuthFailure    cdnMode = "auth-failure"
)

type cdnState struct {
	mode    cdnMode
	calls   int
	uploads int
	updates int
	pages   []string
	roles   []string
	ids     []json.Number
}

type cdnFixture struct {
	t          *testing.T
	mu         sync.Mutex
	state      cdnState
	certPEM    string
	keyPEM     string
	cert       *x509.Certificate
	flexURL    string
	flexTLSURL string
}

func TestCDNDeploymentBatch(t *testing.T) {
	if runtime.GOOS != "linux" {
		runCDNBatchInLinux(t)
		return
	}
	binaryDir := os.Getenv("CERTIMATE_CDN_E2E_BINARIES")
	if binaryDir == "" {
		binaryDir = t.TempDir()
		buildCDNBatch(t, binaryDir, runtime.GOARCH)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := plugin.PluginConfig{PluginDir: binaryDir, StartTimeout: 15 * time.Second, DeployTimeout: 15 * time.Second}
	discovered, failures := plugin.Discover(ctx, cfg)
	if len(failures) != 0 || len(discovered) != len(cdnProviders) {
		t.Fatalf("discover three plugins: count=%d failures=%v", len(discovered), failures)
	}
	byName := make(map[string]*plugin.DiscoveredPlugin)
	for _, dp := range discovered {
		byName[dp.Manifest.ProviderType] = dp
	}
	manager := plugin.NewManager(cfg, slog.New(slog.DiscardHandler))
	fixture := newCDNFixture(t)
	for _, provider := range cdnProviders {
		t.Run(provider, func(t *testing.T) {
			dp := byName[provider]
			if dp == nil {
				t.Fatal("plugin absent from discovery")
			}
			meta, schema, err := manager.Bootstrap(ctx, dp)
			if err != nil {
				t.Fatal(err)
			}
			if meta.ProviderType != provider || meta.AccessProviderType != provider || meta.DeployCategory != "cdn" || meta.ProtocolVersion != plugin.ProtocolVersion {
				t.Fatalf("unexpected metadata: %+v", meta)
			}
			if dp.Manifest.AccessProviderType != provider || dp.Manifest.DeployCategory != meta.DeployCategory || dp.Manifest.ProtocolVersion != meta.ProtocolVersion || dp.Manifest.DisplayNameKey != meta.DeployDisplayNameKey {
				t.Fatalf("manifest differs from metadata: %+v", dp.Manifest)
			}
			if info, err := os.Stat(filepath.Join(dp.Dir, dp.Manifest.Icon)); err != nil || info.IsDir() || info.Size() == 0 {
				t.Fatalf("packaged icon unavailable: %v", err)
			}
			validateSchema(t, schema, provider, provider, meta)
			if provider != "flexcdn" {
				var form struct {
					Schema struct{ Columns []json.RawMessage } `json:"schema"`
				}
				if err := json.Unmarshal(schema.DeploySchemaJSON, &form); err != nil || len(form.Schema.Columns) != 0 {
					t.Fatalf("deployment form must be empty: %s", schema.DeploySchemaJSON)
				}
			}
			req := plugin.DeployRequest{AccessConfigJSON: `{"apiToken":"fixture-token"}`, ExtendedConfigJSON: `{}`, CertificatePEM: fixture.certPEM, PrivateKeyPEM: fixture.keyPEM, LogLevel: "DEBUG"}
			if provider == "flexcdn" {
				req.AccessConfigJSON = fixture.flexAccess("user", fixture.flexURL, false)
				req.ExtendedConfigJSON = `{"deployTarget":"certificate","certificateId":42}`
			}
			deploy := func(t *testing.T, request plugin.DeployRequest, wantError string) {
				t.Helper()
				var logs bytes.Buffer
				result, err := manager.Deploy(ctx, dp, &request, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
				for _, secret := range []string{fixture.keyPEM, strings.Split(fixture.keyPEM, "\n")[1], base64.StdEncoding.EncodeToString([]byte(fixture.keyPEM)), "fixture-token", "fixture-secret", "fixture-session"} {
					if strings.Contains(logs.String(), secret) || err != nil && strings.Contains(err.Error(), secret) {
						t.Fatal("secret leaked in host logs or error")
					}
				}
				if wantError != "" {
					if err == nil || !strings.Contains(err.Error(), wantError) {
						t.Fatalf("expected error containing %q, got result=%+v err=%v", wantError, result, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("deployment failed: %v\n%s", err, logs.String())
				}
				if result == nil || logs.Len() == 0 {
					t.Fatal("missing deployment result or streamed logs")
				}
			}
			t.Run("invalid_json", func(t *testing.T) {
				fixture.reset(cdnNormal)
				bad := req
				bad.AccessConfigJSON = "{"
				deploy(t, bad, "invalid access config")
				bad = req
				bad.ExtendedConfigJSON = "{"
				deploy(t, bad, "invalid deploy config")
				if fixture.snapshot().calls != 0 {
					t.Fatal("invalid JSON reached API")
				}
			})
			t.Run("deploy", func(t *testing.T) {
				fixture.reset(cdnNormal)
				deploy(t, req, "")
				fixture.assertEffects(t, provider, 1)
			})
			t.Run("api_failure_then_recovery", func(t *testing.T) {
				fixture.reset(cdnFailure)
				deploy(t, req, "failed")
				fixture.assertEffects(t, provider, 0)
				if fixture.snapshot().calls == 0 {
					t.Fatal("failure never reached API")
				}
				fixture.reset(cdnNormal)
				deploy(t, req, "")
				fixture.assertEffects(t, provider, 1)
			})
			if provider == "axisnow" {
				for _, mode := range []cdnMode{cdnReuse, cdnPaginatedReuse} {
					t.Run(string(mode), func(t *testing.T) {
						fixture.reset(mode)
						deploy(t, req, "")
						fixture.assertEffects(t, provider, 0)
						wantPages := []string{"1"}
						if mode == cdnPaginatedReuse {
							wantPages = append(wantPages, "2")
						}
						if !reflect.DeepEqual(fixture.snapshot().pages, wantPages) {
							t.Fatalf("unexpected pagination: %v", fixture.snapshot().pages)
						}
					})
				}
				t.Run("upload_failure_then_recovery", func(t *testing.T) {
					fixture.reset(cdnUploadFailure)
					deploy(t, req, "failed")
					fixture.assertEffects(t, provider, 0)
					if fixture.snapshot().calls != 2 {
						t.Fatalf("expected list and failed upload, got %+v", fixture.snapshot())
					}
					fixture.reset(cdnNormal)
					deploy(t, req, "")
					fixture.assertEffects(t, provider, 1)
				})
			}
			if provider == "flexcdn" {
				for _, role := range []string{"user", "admin"} {
					for _, id := range []string{`42`, `"42"`, `"9007199254740993"`} {
						t.Run(role+"_id_"+id, func(t *testing.T) {
							fixture.reset(cdnNormal)
							request := req
							request.AccessConfigJSON = fixture.flexAccess(role, fixture.flexURL, false)
							request.ExtendedConfigJSON = fmt.Sprintf(`{"deployTarget":"certificate","certificateId":%s}`, id)
							deploy(t, request, "")
							fixture.assertEffects(t, provider, 1)
							state := fixture.snapshot()
							if !reflect.DeepEqual(state.roles, []string{role}) || !reflect.DeepEqual(state.ids, []json.Number{json.Number(strings.Trim(id, `"`))}) {
								t.Fatalf("role or ID changed: %+v", state)
							}
						})
					}
				}
				for _, config := range []string{`{"deployTarget":"certificate"}`, `{"deployTarget":"certificate","certificateId":0}`, `{"deployTarget":"certificate","certificateId":-1}`, `{"deployTarget":"certificate","certificateId":1.5}`, `{"deployTarget":"certificate","certificateId":"bad"}`, `{"deployTarget":"domain","certificateId":42}`} {
					t.Run("invalid_"+config, func(t *testing.T) {
						fixture.reset(cdnNormal)
						request := req
						request.ExtendedConfigJSON = config
						wantError := "config"
						if strings.Contains(config, `"domain"`) {
							wantError = "unsupported deploy target"
						}
						deploy(t, request, wantError)
						if fixture.snapshot().calls != 0 {
							t.Fatal("invalid config reached API")
						}
					})
				}
				t.Run("invalid_certificate", func(t *testing.T) {
					fixture.reset(cdnNormal)
					request := req
					request.CertificatePEM = "not a certificate"
					deploy(t, request, "PEM decode did not yield a valid block")
					if fixture.snapshot().calls != 0 {
						t.Fatal("invalid certificate reached API")
					}
				})
				t.Run("auth_failure_then_recovery", func(t *testing.T) {
					fixture.reset(cdnAuthFailure)
					deploy(t, req, "failed")
					fixture.assertEffects(t, provider, 0)
					if fixture.snapshot().calls != 1 {
						t.Fatal("authentication failure did not stop update")
					}
					fixture.reset(cdnNormal)
					deploy(t, req, "")
					fixture.assertEffects(t, provider, 1)
				})
				t.Run("tls_requires_explicit_opt_in", func(t *testing.T) {
					fixture.reset(cdnNormal)
					request := req
					request.AccessConfigJSON = fixture.flexAccess("user", fixture.flexTLSURL, false)
					deploy(t, request, "signed by unknown authority")
					if fixture.snapshot().calls != 0 {
						t.Fatal("untrusted TLS connection reached API")
					}
					request.AccessConfigJSON = fixture.flexAccess("user", fixture.flexTLSURL, true)
					deploy(t, request, "")
					fixture.assertEffects(t, provider, 1)
				})
			}
		})
	}
}

func (f *cdnFixture) flexAccess(role, url string, insecure bool) string {
	data, err := json.Marshal(map[string]any{"serverUrl": url, "apiRole": role, "accessKeyId": "fixture-key", "accessKey": "fixture-secret", "allowInsecureConnections": insecure})
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

func (f *cdnFixture) reset(mode cdnMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = cdnState{mode: mode}
}

func (f *cdnFixture) snapshot() cdnState {
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.state
	state.pages = append([]string(nil), state.pages...)
	state.roles = append([]string(nil), state.roles...)
	state.ids = append([]json.Number(nil), state.ids...)
	return state
}

func (f *cdnFixture) assertEffects(t *testing.T, provider string, count int) {
	t.Helper()
	state := f.snapshot()
	uploads, updates := count, 0
	if provider == "flexcdn" {
		uploads, updates = 0, count
	}
	if state.uploads != uploads || state.updates != updates {
		t.Fatalf("unexpected provider effects: %+v, want uploads=%d updates=%d", state, uploads, updates)
	}
}

func newCDNFixture(t *testing.T) *cdnFixture {
	t.Helper()
	certPEM, keyPEM, cert := makeCertificate(t, []string{"cdn.example.com"}, false)
	f := &cdnFixture{t: t, certPEM: certPEM, keyPEM: keyPEM, cert: cert}
	f.reset(cdnNormal)
	hosts := []string{"api.cachefly.com", "api.axisnow.io"}
	caPEM, caKeyPEM, _ := makeCertificate(t, hosts, true)
	serverCert, err := tls.X509KeyPair([]byte(caPEM), []byte(caKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	cloud := httptest.NewUnstartedServer(http.HandlerFunc(f.handle))
	cloud.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}, MinVersion: tls.VersionTLS12}
	cloud.StartTLS()
	t.Cleanup(cloud.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "api.cachefly.com:443" && r.Host != "api.axisnow.io:443" {
			http.Error(w, "unexpected proxy destination", http.StatusBadGateway)
			return
		}
		upstream, err := net.DialTimeout("tcp", cloud.Listener.Addr().String(), 5*time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstream.Close()
			return
		}
		defer client.Close()
		defer upstream.Close()
		if _, err := buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if err := buffer.Flush(); err != nil {
			return
		}
		done := make(chan struct{})
		go func() { io.Copy(upstream, buffer); upstream.Close(); close(done) }()
		io.Copy(client, upstream)
		client.Close()
		<-done
	}))
	t.Cleanup(proxy.Close)
	caPath := filepath.Join(t.TempDir(), "cdn-ca.pem")
	if err := os.WriteFile(caPath, []byte(caPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("SSL_CERT_FILE", caPath)
	flex := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(flex.Close)
	f.flexURL = flex.URL
	flexTLS := httptest.NewUnstartedServer(http.HandlerFunc(f.handle))
	flexTLS.Config.ErrorLog = log.New(io.Discard, "", 0)
	flexTLS.StartTLS()
	t.Cleanup(flexTLS.Close)
	f.flexTLSURL = flexTLS.URL
	return f
}

func (f *cdnFixture) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.calls++
	w.Header().Set("Content-Type", "application/json")
	if f.state.mode == cdnFailure || f.state.mode == cdnUploadFailure && r.Method == http.MethodPost {
		http.Error(w, `{"message":"fixture API failure"}`, http.StatusInternalServerError)
		return
	}
	body := make(map[string]any)
	if r.Method != http.MethodGet {
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&body); err != nil {
			f.t.Errorf("invalid API request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	}
	check := func(label string, got, want any) {
		if !reflect.DeepEqual(got, want) {
			f.t.Errorf("%s %s: %s differs from expected payload", r.Host, r.URL.Path, label)
		}
	}
	respond := func(value any) {
		if err := json.NewEncoder(w).Encode(value); err != nil {
			f.t.Errorf("fixture response: %v", err)
		}
	}
	switch r.Host {
	case "api.cachefly.com":
		check("method", r.Method, http.MethodPost)
		check("path", r.URL.Path, "/api/2.5/certificates")
		check("authorization", r.Header.Get("X-CF-Authorization"), "Bearer fixture-token")
		check("certificate", body["certificate"], f.certPEM)
		check("key", body["certificateKey"], f.keyPEM)
		f.state.uploads++
		respond(map[string]any{"_id": "cachefly-cert", "subjectCommonName": "cdn.example.com"})
	case "api.axisnow.io":
		check("path", r.URL.Path, "/client/v1/certificates")
		check("authorization", r.Header.Get("Authorization"), "Bearer fixture-token")
		if r.Method == http.MethodGet {
			page := r.URL.Query().Get("page")
			f.state.pages = append(f.state.pages, page)
			check("per_page", r.URL.Query().Get("per_page"), "100")
			certs := []any{}
			if f.state.mode == cdnPaginatedReuse && page == "1" {
				for i := 0; i < 100; i++ {
					certs = append(certs, map[string]any{"uuid": fmt.Sprint(i), "certificate": "unrelated"})
				}
			} else if f.state.mode == cdnReuse || f.state.mode == cdnPaginatedReuse && page == "2" {
				certs = append(certs, map[string]any{"uuid": "existing-cert", "name": "existing", "certificate": f.certPEM})
			}
			respond(map[string]any{"success": true, "result": certs, "result_info": map[string]any{"total_count": 101}})
		} else if r.Method == http.MethodPost {
			check("type", body["type"], "upload")
			check("certificate", body["certificate"], f.certPEM)
			check("key", body["private_key"], f.keyPEM)
			f.state.uploads++
			respond(map[string]any{"success": true, "result": map[string]any{"uuid": "new-cert", "name": body["name"]}})
		} else {
			f.t.Errorf("unexpected AxisNow method %s", r.Method)
		}
	default:
		check("method", r.Method, http.MethodPost)
		switch r.URL.Path {
		case "/APIAccessTokenService/getAPIAccessToken":
			check("accessKeyId", body["accessKeyId"], "fixture-key")
			check("accessKey", body["accessKey"], "fixture-secret")
			role, _ := body["type"].(string)
			if role != "user" && role != "admin" {
				f.t.Errorf("invalid FlexCDN role %q", role)
			}
			f.state.roles = append(f.state.roles, role)
			if f.state.mode == cdnAuthFailure {
				respond(map[string]any{"code": 401, "message": "fixture authentication failure"})
				return
			}
			respond(map[string]any{"code": 200, "data": map[string]any{"token": "fixture-session", "expiresAt": time.Now().Add(time.Hour).Unix()}})
		case "/SSLCertService/updateSSLCert":
			check("token", r.Header.Get("X-Cloud-Access-Token"), "fixture-session")
			check("certData", body["certData"], base64.StdEncoding.EncodeToString([]byte(f.certPEM)))
			check("keyData", body["keyData"], base64.StdEncoding.EncodeToString([]byte(f.keyPEM)))
			check("isOn", body["isOn"], true)
			check("isCA", body["isCA"], false)
			check("serverName", body["serverName"], f.cert.Subject.CommonName)
			check("timeBeginAt", body["timeBeginAt"], json.Number(fmt.Sprint(f.cert.NotBefore.Unix())))
			check("timeEndAt", body["timeEndAt"], json.Number(fmt.Sprint(f.cert.NotAfter.Unix())))
			check("dnsNames", body["dnsNames"], []any{"cdn.example.com"})
			id, ok := body["sslCertId"].(json.Number)
			if !ok {
				f.t.Error("certificate ID must be an integer")
			}
			f.state.ids = append(f.state.ids, id)
			f.state.updates++
			respond(map[string]any{"code": 200})
		default:
			f.t.Errorf("unexpected FlexCDN path %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusBadRequest)
		}
	}
}

func buildCDNBatch(t *testing.T, dir, arch string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range cdnProviders {
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

func runCDNBatchInLinux(t *testing.T) {
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
	buildCDNBatch(t, dir, arch)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "cdn-batch.test"), "./integration")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Linux tests: %v\n%s", err, output)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	cmd = exec.CommandContext(ctx, "docker", "run", "--rm", "--network=none", "--mount", "type=bind,src="+dir+",dst=/fixtures,readonly", "-e", "CERTIMATE_CDN_E2E_BINARIES=/fixtures", image, "/fixtures/cdn-batch.test", "-test.run=^TestCDNDeploymentBatch$", "-test.v", "-test.timeout=5m")
	output, err := cmd.CombinedOutput()
	t.Logf("Linux plugin test output:\n%s", output)
	if err != nil {
		t.Fatalf("Linux end-to-end tests: %v", err)
	}
}
