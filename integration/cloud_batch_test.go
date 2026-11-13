package integration

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

type cloudBatchEntry struct {
	provider string
	access   string
	category string
	config   string
	uploads  int
	replaces int
	bindings map[string]string
}

var cloudBatch = []cloudBatchEntry{
	{"ksyun-cdn", "ksyun", "cdn", `{"deployTarget":"domain","domainMatchPattern":"exact","domain":"cdn.example.com"}`, 1, 0, map[string]string{"domain-1": "uploaded"}},
	{"ksyun-kcm", "ksyun", "ssl", `{}`, 1, 0, nil},
	{"ksyun-slb", "ksyun", "loadbalance", `{"region":"cn-beijing-6","deployTarget":"certificate","certificateId":"lb-existing"}`, 1, 1, map[string]string{"lb-existing": "cert-1"}},
	{"wangsu-cdn", "wangsu", "cdn", `{"domainMatchPattern":"exact","domains":"cdn.example.com;static.example.com"}`, 1, 0, map[string]string{"cdn.example.com": "101", "static.example.com": "101"}},
	{"wangsu-cdnpro", "wangsu", "cdn", `{"domain":"cdn.example.com"}`, 1, 0, map[string]string{"production": "pro-cert/1"}},
	{"wangsu-certificate", "wangsu", "ssl", `{}`, 1, 0, nil},
	{"baishan-cdn", "baishan", "cdn", `{"deployTarget":"domain","domainMatchPattern":"exact","domain":"cdn.example.com"}`, 1, 0, map[string]string{"cdn.example.com": "101"}},
}

type cloudBatchMode string

const (
	cloudNormal    cloudBatchMode = "normal"
	cloudFail      cloudBatchMode = "fail"
	cloudReuse     cloudBatchMode = "reuse"
	cloudDuplicate cloudBatchMode = "duplicate"
	cloudPollFail  cloudBatchMode = "poll-fail"
	cloudPending   cloudBatchMode = "pending"
)

type cloudBatchState struct {
	mode        cloudBatchMode
	calls       int
	uploads     int
	replaces    int
	attempts    int
	polls       int
	bindings    map[string]string
	taskTarget  string
	taskCert    string
	taskVersion int
}

type cloudBatchFixture struct {
	t       *testing.T
	mu      sync.Mutex
	state   cloudBatchState
	certPEM string
	keyPEM  string
	cert    *x509.Certificate
}

func TestCloudDeploymentBatch(t *testing.T) {
	if runtime.GOOS != "linux" {
		runCloudBatchInLinux(t)
		return
	}
	binaryDir := os.Getenv("CERTIMATE_CLOUD_E2E_BINARIES")
	if binaryDir == "" {
		binaryDir = t.TempDir()
		buildCloudBatch(t, binaryDir, runtime.GOARCH)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	cfg := plugin.PluginConfig{PluginDir: binaryDir, StartTimeout: 15 * time.Second, DeployTimeout: 45 * time.Second}
	discovered, failures := plugin.Discover(ctx, cfg)
	if len(failures) != 0 || len(discovered) != len(cloudBatch) {
		t.Fatalf("discover seven packaged plugins: found=%d failures=%v", len(discovered), failures)
	}
	byName := make(map[string]*plugin.DiscoveredPlugin)
	for _, dp := range discovered {
		byName[dp.Manifest.ProviderType] = dp
	}
	manager := plugin.NewManager(cfg, slog.New(slog.DiscardHandler))
	fixture := newCloudBatchFixture(t)
	accessSchemas := make(map[string][]byte)
	for _, entry := range cloudBatch {
		t.Run(entry.provider, func(t *testing.T) {
			dp := byName[entry.provider]
			if dp == nil {
				t.Fatal("plugin absent from discovery")
			}
			meta, schema, err := manager.Bootstrap(ctx, dp)
			if err != nil {
				t.Fatal(err)
			}
			if meta.ProviderType != entry.provider || meta.AccessProviderType != entry.access || meta.DeployCategory != entry.category || meta.ProtocolVersion != plugin.ProtocolVersion {
				t.Fatalf("unexpected metadata: %+v", meta)
			}
			if dp.Manifest.AccessProviderType != meta.AccessProviderType || dp.Manifest.DeployCategory != meta.DeployCategory || dp.Manifest.ProtocolVersion != meta.ProtocolVersion || dp.Manifest.DisplayNameKey != meta.DeployDisplayNameKey {
				t.Fatalf("manifest differs from plugin metadata: %+v", dp.Manifest)
			}
			if info, err := os.Stat(filepath.Join(dp.Dir, dp.Manifest.Icon)); err != nil || info.IsDir() || info.Size() == 0 {
				t.Fatalf("packaged plugin icon unavailable: %v", err)
			}
			validateSchema(t, schema, entry.provider, entry.access, meta)
			if len(schema.AccessSchemaJSON) > 0 {
				if previous := accessSchemas[entry.access]; previous != nil && !bytes.Equal(previous, schema.AccessSchemaJSON) {
					t.Fatal("sibling access schemas differ")
				}
				accessSchemas[entry.access] = schema.AccessSchemaJSON
			} else if accessSchemas[entry.access] == nil {
				t.Fatal("no plugin in the group supplies the access schema")
			}
			if entry.provider == "ksyun-kcm" {
				var empty struct {
					Schema struct{ Columns []json.RawMessage } `json:"schema"`
				}
				if err := json.Unmarshal(schema.DeploySchemaJSON, &empty); err != nil || len(empty.Schema.Columns) != 0 {
					t.Fatalf("KCM deployment form must be empty: %s", schema.DeploySchemaJSON)
				}
			}
			access := map[string]string{
				"ksyun":   `{"accessKeyId":"fixture-key","secretAccessKey":"fixture-secret"}`,
				"wangsu":  `{"accessKeyId":"fixture-key","accessKeySecret":"fixture-secret","apiKey":"fixture-api-key"}`,
				"baishan": `{"apiToken":"fixture-token"}`,
			}[entry.access]
			req := plugin.DeployRequest{AccessConfigJSON: access, ExtendedConfigJSON: entry.config, CertificatePEM: fixture.certPEM, PrivateKeyPEM: fixture.keyPEM, LogLevel: "DEBUG"}
			deploy := func(t *testing.T, request plugin.DeployRequest, wantError string) {
				t.Helper()
				var logs bytes.Buffer
				result, err := manager.Deploy(ctx, dp, &request, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
				for _, secret := range []string{fixture.keyPEM, strings.Split(fixture.keyPEM, "\n")[1], "fixture-secret", "fixture-api-key", "fixture-token"} {
					if strings.Contains(logs.String(), secret) || (err != nil && strings.Contains(err.Error(), secret)) {
						t.Fatal("unredacted secret in host logs or error")
					}
				}
				if wantError != "" {
					if err == nil || !strings.Contains(err.Error(), wantError) {
						t.Fatalf("want error containing %q, got result=%+v err=%v", wantError, result, err)
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
				fixture.reset(cloudNormal)
				bad := req
				bad.AccessConfigJSON = "{"
				deploy(t, bad, "invalid access config")
				bad = req
				bad.ExtendedConfigJSON = "{"
				deploy(t, bad, "invalid deploy config")
				if fixture.snapshot().calls != 0 {
					t.Fatal("invalid JSON reached cloud API")
				}
			})
			t.Run("deploy", func(t *testing.T) {
				fixture.reset(cloudNormal)
				deploy(t, req, "")
				fixture.assertEffects(t, entry.uploads, entry.replaces, entry.bindings)
			})
			t.Run("api_failure_then_recovery", func(t *testing.T) {
				fixture.reset(cloudFail)
				deploy(t, req, "failed")
				fixture.assertEffects(t, 0, 0, nil)
				if fixture.snapshot().calls == 0 {
					t.Fatal("failure did not reach API")
				}
				fixture.reset(cloudNormal)
				deploy(t, req, "")
				fixture.assertEffects(t, entry.uploads, entry.replaces, entry.bindings)
			})
			switch entry.provider {
			case "ksyun-cdn":
				for _, pattern := range []string{"exact", "wildcard", "certsan"} {
					t.Run(pattern+"_without_domain", func(t *testing.T) {
						fixture.reset(cloudNormal)
						request := req
						request.ExtendedConfigJSON = fmt.Sprintf(`{"deployTarget":"domain","domainMatchPattern":%q}`, pattern)
						if pattern == "certsan" {
							deploy(t, request, "")
							fixture.assertEffects(t, 1, 0, entry.bindings)
						} else {
							deploy(t, request, "config `[REDACTED]` is required")
							fixture.assertEffects(t, 0, 0, nil)
						}
					})
				}
				for _, pattern := range []string{"wildcard", "certsan"} {
					t.Run(pattern+"_matching", func(t *testing.T) {
						fixture.reset(cloudNormal)
						request := req
						request.ExtendedConfigJSON = fmt.Sprintf(`{"deployTarget":"domain","domainMatchPattern":%q,"domain":"*.example.com"}`, pattern)
						deploy(t, request, "")
						fixture.assertEffects(t, 1, 0, entry.bindings)
					})
				}
				t.Run("no_matching_domain", func(t *testing.T) {
					fixture.reset(cloudNormal)
					request := req
					request.ExtendedConfigJSON = `{"deployTarget":"domain","domainMatchPattern":"exact","domain":"missing.example.com"}`
					deploy(t, request, "could not find [REDACTED]")
					fixture.assertEffects(t, 0, 0, nil)
				})
				t.Run("replace_certificate", func(t *testing.T) {
					fixture.reset(cloudNormal)
					request := req
					request.ExtendedConfigJSON = `{"deployTarget":"certificate","certificateId":"cdn-existing"}`
					deploy(t, request, "")
					fixture.assertEffects(t, 0, 1, map[string]string{"cdn-existing": "uploaded"})
				})
			case "ksyun-kcm", "ksyun-slb", "wangsu-cdn", "wangsu-certificate":
				t.Run("reuse_identical_certificate", func(t *testing.T) {
					fixture.reset(cloudReuse)
					deploy(t, req, "")
					fixture.assertEffects(t, 0, entry.replaces, entry.bindings)
				})
				if entry.access == "ksyun" {
					t.Run("duplicate_upload_recovery", func(t *testing.T) {
						fixture.reset(cloudDuplicate)
						deploy(t, req, "")
						fixture.assertEffects(t, 0, entry.replaces, entry.bindings)
						if fixture.snapshot().attempts != 1 {
							t.Fatal("duplicate upload was not attempted exactly once")
						}
					})
				}
				if entry.provider == "wangsu-certificate" {
					t.Run("replace_certificate", func(t *testing.T) {
						fixture.reset(cloudNormal)
						request := req
						request.ExtendedConfigJSON = `{"certificateId":"202"}`
						deploy(t, request, "")
						fixture.assertEffects(t, 0, 1, map[string]string{"202": "uploaded"})
					})
				}
			case "wangsu-cdnpro":
				t.Run("update_certificate_in_staging", func(t *testing.T) {
					fixture.reset(cloudNormal)
					request := req
					request.ExtendedConfigJSON = `{"domain":"cdn.example.com","certificateId":"pro-existing","environment":"staging","webhookId":"hook-1"}`
					deploy(t, request, "")
					fixture.assertEffects(t, 0, 1, map[string]string{"staging": "pro-existing/5"})
				})
				t.Run("pending_task_completes", func(t *testing.T) {
					fixture.reset(cloudPending)
					deploy(t, req, "")
					fixture.assertEffects(t, 1, 0, entry.bindings)
					if fixture.snapshot().polls != 2 {
						t.Fatal("pending deployment did not poll until completion")
					}
				})
				t.Run("failed_task_then_recovery", func(t *testing.T) {
					fixture.reset(cloudPollFail)
					deploy(t, req, "unexpected deployment task status")
					fixture.assertEffects(t, 1, 0, nil)
					fixture.reset(cloudNormal)
					deploy(t, req, "")
					fixture.assertEffects(t, 1, 0, entry.bindings)
				})
			case "baishan-cdn":
				t.Run("duplicate_upload_recovery", func(t *testing.T) {
					fixture.reset(cloudDuplicate)
					deploy(t, req, "")
					fixture.assertEffects(t, 0, 0, entry.bindings)
				})
				t.Run("replace_certificate", func(t *testing.T) {
					fixture.reset(cloudNormal)
					request := req
					request.ExtendedConfigJSON = `{"deployTarget":"certificate","certificateId":"202"}`
					deploy(t, request, "")
					fixture.assertEffects(t, 0, 1, map[string]string{"202": "uploaded"})
				})
			}
		})
	}
}

func buildCloudBatch(t *testing.T, dir, arch string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range cloudBatch {
		out := filepath.Join(dir, entry.provider)
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(out, entry.provider), "./"+entry.provider)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("build %s: %v\n%s", entry.provider, err, output)
		}
		manifestBytes, err := os.ReadFile(filepath.Join(root, entry.provider, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := plugin.ParseManifest(manifestBytes)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"manifest.json", manifest.Icon} {
			data, err := os.ReadFile(filepath.Join(root, entry.provider, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func runCloudBatchInLinux(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
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
	buildCloudBatch(t, dir, arch)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "cloud-batch.test"), "./integration")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Linux tests: %v\n%s", err, output)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	cmd = exec.CommandContext(ctx, "docker", "run", "--rm", "--network=none", "--mount", "type=bind,src="+dir+",dst=/fixtures,readonly", "-e", "CERTIMATE_CLOUD_E2E_BINARIES=/fixtures", image, "/fixtures/cloud-batch.test", "-test.run=^TestCloudDeploymentBatch$", "-test.v", "-test.timeout=9m")
	output, err := cmd.CombinedOutput()
	t.Logf("Linux plugin test output:\n%s", output)
	if err != nil {
		t.Fatalf("Linux end-to-end tests: %v", err)
	}
}

func newCloudBatchFixture(t *testing.T) *cloudBatchFixture {
	t.Helper()
	certPEM, keyPEM, cert := makeCertificate(t, []string{"cdn.example.com"}, false)
	fixture := &cloudBatchFixture{t: t, certPEM: certPEM, keyPEM: keyPEM, cert: cert}
	fixture.reset(cloudNormal)
	hosts := []string{"cdn.api.ksyun.com", "kcm.api.ksyun.com", "open.chinanetcenter.com", "cdn.api.baishan.com"}
	caPEM, caKeyPEM, _ := makeCertificate(t, hosts, true)
	serverCert, err := tls.X509KeyPair([]byte(caPEM), []byte(caKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	cloud := httptest.NewUnstartedServer(http.HandlerFunc(fixture.handle))
	cloud.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}, MinVersion: tls.VersionTLS12}
	cloud.StartTLS()
	t.Cleanup(cloud.Close)
	allowed := make(map[string]bool)
	for _, host := range hosts {
		allowed[host+":443"] = true
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || !allowed[r.Host] {
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
	caPath := filepath.Join(t.TempDir(), "cloud-batch-ca.pem")
	if err := os.WriteFile(caPath, []byte(caPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("SSL_CERT_FILE", caPath)
	return fixture
}

func (f *cloudBatchFixture) reset(mode cloudBatchMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = cloudBatchState{mode: mode, bindings: make(map[string]string)}
}

func (f *cloudBatchFixture) snapshot() cloudBatchState {
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.state
	state.bindings = make(map[string]string)
	for k, v := range f.state.bindings {
		state.bindings[k] = v
	}
	return state
}

func (f *cloudBatchFixture) assertEffects(t *testing.T, uploads, replaces int, bindings map[string]string) {
	t.Helper()
	state := f.snapshot()
	if bindings == nil {
		bindings = map[string]string{}
	}
	if state.uploads != uploads || state.replaces != replaces || !reflect.DeepEqual(state.bindings, bindings) {
		t.Fatalf("unexpected cloud effects: uploads=%d replacements=%d bindings=%v, want uploads=%d replacements=%d bindings=%v", state.uploads, state.replaces, state.bindings, uploads, replaces, bindings)
	}
}

func (f *cloudBatchFixture) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.calls++
	w.Header().Set("Content-Type", "application/json")
	if f.state.mode == cloudFail {
		http.Error(w, `{"error":"fixture API failure"}`, http.StatusInternalServerError)
		return
	}
	body := make(map[string]any)
	if r.Method != http.MethodGet {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
	if r.Host == "open.chinanetcenter.com" && r.Header.Get("Authorization") == "" {
		f.t.Error("cloud request missing authorization")
	}
	if strings.HasSuffix(r.Host, ".api.ksyun.com") {
		accessKey, signature := r.URL.Query().Get("Accesskey"), r.URL.Query().Get("Signature")
		if r.Method != http.MethodGet {
			accessKey, _ = body["Accesskey"].(string)
			signature, _ = body["Signature"].(string)
		}
		check("access key", accessKey, "fixture-key")
		if signature == "" {
			f.t.Error("Ksyun request missing signature")
		}
	}
	switch r.Host {
	case "cdn.api.ksyun.com":
		switch r.URL.Path {
		case "/2019-06-01/GetCdnDomains":
			respond(map[string]any{"Domains": []any{map[string]any{"DomainId": "domain-1", "DomainName": "cdn.example.com", "DomainStatus": "online"}, map[string]any{"DomainId": "unrelated", "DomainName": "unrelated.test", "DomainStatus": "online"}, map[string]any{"DomainId": "disabled", "DomainName": "hidden.example.com", "DomainStatus": "offline"}}})
		case "/2016-09-01/cert/ConfigCertificate":
			check("Enable", body["Enable"], "on")
			check("DomainIds", body["DomainIds"], "domain-1")
			check("certificate", body["ServerCertificate"], f.certPEM)
			check("private key", body["PrivateKey"], f.keyPEM)
			f.state.uploads++
			f.state.bindings[fmt.Sprint(body["DomainIds"])] = "uploaded"
			respond(map[string]any{"CertificateId": "cdn-cert"})
		case "/2016-09-01/cert/SetCertificate":
			check("CertificateId", body["CertificateId"], "cdn-existing")
			check("certificate", body["ServerCertificate"], f.certPEM)
			check("private key", body["PrivateKey"], f.keyPEM)
			f.state.replaces++
			f.state.bindings[fmt.Sprint(body["CertificateId"])] = "uploaded"
			respond(map[string]any{"CertificateId": "cdn-existing"})
		default:
			f.t.Errorf("unexpected Ksyun CDN path %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusBadRequest)
		}
	case "kcm.api.ksyun.com":
		action := r.URL.Query().Get("Action")
		if value, ok := body["Action"].(string); ok {
			action = value
		}
		switch action {
		case "ListUserCertificates":
			certs := []any{}
			if f.state.mode == cloudReuse || (f.state.mode == cloudDuplicate && f.state.attempts > 0) {
				fingerprint := sha1.Sum(f.cert.Raw)
				certs = append(certs, map[string]any{"CertID": "cert-1", "CertName": "fixture", "Domains": f.cert.DNSNames, "CA": f.cert.Issuer.CommonName, "FingerPrint": hex.EncodeToString(fingerprint[:])})
			}
			respond(map[string]any{"Success": true, "Ret": map[string]any{"Certs": certs}})
		case "UploadCertificate":
			check("certificate", body["CertFile"], f.certPEM)
			check("private key", body["CertKey"], f.keyPEM)
			f.state.attempts++
			if f.state.mode == cloudDuplicate {
				respond(map[string]any{"Error": map[string]string{"Code": "Duplicate", "Message": "重复的证书文件"}})
				return
			}
			f.state.uploads++
			respond(map[string]any{"Success": true, "Ret": map[string]any{"CertID": "cert-1", "CertName": "fixture"}})
		case "ModifyCertificate":
			check("Region", body["Region"], "cn-beijing-6")
			check("CertificateId", body["CertificateId"], "lb-existing")
			check("SslCertificateId", body["SslCertificateId"], "cert-1")
			check("query CertificateId", r.URL.Query().Get("CertificateId"), "lb-existing")
			f.state.replaces++
			f.state.bindings[fmt.Sprint(body["CertificateId"])] = fmt.Sprint(body["SslCertificateId"])
			respond(map[string]any{"Certificate": map[string]any{"CertificateId": "lb-existing"}})
		default:
			f.t.Errorf("unexpected KCM action %s", action)
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	case "open.chinanetcenter.com":
		f.handleWangsu(w, r, body, check, respond)
	case "cdn.api.baishan.com":
		check("token", r.URL.Query().Get("token"), "fixture-token")
		switch {
		case r.URL.Path == "/v2/domain/certificate":
			check("certificate", body["certificate"], f.certPEM)
			check("private key", body["key"], f.keyPEM)
			if id, ok := body["cert_id"].(string); ok {
				check("certificate ID", id, "202")
				f.state.replaces++
				f.state.bindings[id] = "uploaded"
			} else if f.state.mode == cloudDuplicate {
				respond(map[string]any{"code": 400699, "message": "this certificate is exists 101"})
				return
			} else {
				f.state.uploads++
			}
			respond(map[string]any{"code": 0, "data": map[string]any{"cert_id": 101}})
		case r.URL.Path == "/v2/domain/config" && r.Method == http.MethodGet:
			check("domains", r.URL.Query().Get("domains"), "cdn.example.com")
			respond(map[string]any{"code": 0, "data": []any{map[string]any{"domain": "cdn.example.com", "config": map[string]any{"https": map[string]any{"cert_id": 77, "force_https": "off", "http2": "on", "ocsp": "off"}}}}})
		case r.URL.Path == "/v2/domain/config" && r.Method == http.MethodPost:
			check("domains", body["domains"], "cdn.example.com")
			check("HTTPS settings", body["config"], map[string]any{"https": map[string]any{"cert_id": float64(101), "force_https": "off", "http2": "on", "ocsp": "off"}})
			f.state.bindings[fmt.Sprint(body["domains"])] = "101"
			respond(map[string]any{"code": 0})
		default:
			f.t.Errorf("unexpected Baishan request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected path", http.StatusBadRequest)
		}
	default:
		f.t.Errorf("unexpected cloud host %s", r.Host)
		http.Error(w, "unexpected host", http.StatusBadRequest)
	}
}

func (f *cloudBatchFixture) handleWangsu(w http.ResponseWriter, r *http.Request, body map[string]any, check func(string, any, any), respond func(any)) {
	switch {
	case r.URL.Path == "/api/ssl/certificate":
		certs := []any{}
		if f.state.mode == cloudReuse {
			zone := time.FixedZone("CST", 8*60*60)
			certs = append(certs, map[string]any{"certificate-id": "101", "name": "fixture", "certificate-serial": f.cert.SerialNumber.Text(16), "certificate-validity-from": f.cert.NotBefore.In(zone).Format(time.DateTime), "certificate-validity-to": f.cert.NotAfter.In(zone).Format(time.DateTime)})
		}
		respond(map[string]any{"ssl-certificates": certs})
	case r.URL.Path == "/api/certificate" && r.Method == http.MethodPost:
		check("certificate", body["certificate"], f.certPEM)
		check("private key", body["privateKey"], f.keyPEM)
		f.state.uploads++
		w.Header().Set("Location", "https://open.chinanetcenter.com/api/certificate/101")
		respond(map[string]any{})
	case r.URL.Path == "/api/certificate/202" && r.Method == http.MethodPut:
		check("certificate", body["certificate"], f.certPEM)
		check("private key", body["privateKey"], f.keyPEM)
		f.state.replaces++
		f.state.bindings["202"] = "uploaded"
		respond(map[string]any{})
	case r.URL.Path == "/api/config/certificate/batch" && r.Method == http.MethodPut:
		check("certificate ID", body["certificateId"], float64(101))
		check("domains", body["domainNames"], []any{"cdn.example.com", "static.example.com"})
		if names, ok := body["domainNames"].([]any); ok {
			for _, name := range names {
				f.state.bindings[fmt.Sprint(name)] = "101"
			}
		}
		respond(map[string]any{})
	case r.URL.Path == "/cdn/hostnames/cdn.example.com":
		respond(map[string]any{"hostname": "cdn.example.com"})
	case r.URL.Path == "/cdn/certificates" && r.Method == http.MethodPost,
		r.URL.Path == "/cdn/certificates/pro-existing" && r.Method == http.MethodPatch:
		version, ok := body["newVersion"].(map[string]any)
		if !ok {
			f.t.Error("CDN Pro certificate version missing")
			http.Error(w, "missing version", http.StatusBadRequest)
			return
		}
		check("certificate", version["certificate"], f.certPEM)
		encrypted, ok := version["privateKey"].(string)
		if !ok {
			f.t.Error("CDN Pro encrypted key missing")
		} else if plaintext, err := decryptCloudBatchKey(encrypted, r.Header.Get("X-CNC-Timestamp")); err != nil {
			f.t.Errorf("CDN Pro private key decryption failed: %v", err)
		} else {
			check("decrypted private key", plaintext, f.keyPEM)
		}
		if r.Method == http.MethodPost {
			f.state.uploads++
			w.Header().Set("Location", "https://open.chinanetcenter.com/cdn/certificates/pro-cert")
		} else {
			f.state.replaces++
			w.Header().Set("Location", "https://open.chinanetcenter.com/cdn/certificates/pro-existing/versions/5")
		}
		respond(map[string]any{})
	case r.URL.Path == "/cdn/deploymentTasks" && r.Method == http.MethodPost:
		target, _ := body["target"].(string)
		if target != "production" && target != "staging" {
			f.t.Errorf("unexpected deployment environment %q", target)
		}
		wantCert, wantVersion := "pro-cert", float64(1)
		if target == "staging" {
			wantCert, wantVersion = "pro-existing", float64(5)
			check("webhook", body["webhook"], "hook-1")
		}
		check("deployment actions", body["actions"], []any{map[string]any{"action": "deploy_cert", "certificateId": wantCert, "version": wantVersion}})
		f.state.taskTarget = target
		f.state.taskCert = wantCert
		f.state.taskVersion = int(wantVersion)
		w.Header().Set("Location", "https://open.chinanetcenter.com/cdn/deploymentTasks/task-1")
		respond(map[string]any{})
	case r.URL.Path == "/cdn/deploymentTasks/task-1" && r.Method == http.MethodGet:
		f.state.polls++
		if f.state.mode == cloudPollFail {
			respond(map[string]any{"status": "failed"})
			return
		}
		if f.state.mode == cloudPending && f.state.polls == 1 {
			respond(map[string]any{"status": "running"})
			return
		}
		f.state.bindings[f.state.taskTarget] = fmt.Sprintf("%s/%d", f.state.taskCert, f.state.taskVersion)
		respond(map[string]any{"status": "succeeded"})
	default:
		f.t.Errorf("unexpected Wangsu request %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected path", http.StatusBadRequest)
	}
}

func decryptCloudBatchKey(encoded, timestamp string) (string, error) {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return "", err
	}
	digest := hmac.New(sha256.New, []byte("fixture-api-key"))
	digest.Write([]byte(time.Unix(seconds, 0).UTC().Format(http.TimeFormat)))
	keyAndIV := digest.Sum(nil)
	block, err := aes.NewCipher(keyAndIV[16:])
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(ciphertext) == 0 || len(ciphertext)%block.BlockSize() != 0 {
		return "", fmt.Errorf("invalid ciphertext size")
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, keyAndIV[:16]).CryptBlocks(plaintext, ciphertext)
	padding := int(plaintext[len(plaintext)-1])
	if padding < 1 || padding > block.BlockSize() || padding > len(plaintext) || !bytes.Equal(plaintext[len(plaintext)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return "", fmt.Errorf("invalid padding")
	}
	return string(plaintext[:len(plaintext)-padding]), nil
}
