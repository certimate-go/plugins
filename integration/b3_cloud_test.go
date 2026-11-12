package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

var b3CloudProviders = []string{"gcore-cdn", "googlecloud-certificatemanager", "linode-los", "netlify"}

type b3CloudFixture struct {
	barrier                                  workflowRequestBarrier
	mu                                       sync.Mutex
	t                                        *testing.T
	provider, mode, cert, key, name          string
	writes, deletes, bindings, certificateID int
	blockedRequest                           chan struct{}
}

func TestB3CloudDeploymentBatch(t *testing.T) {
	if runtime.GOOS != "linux" {
		runB3CloudInLinux(t)
		return
	}
	dir := os.Getenv("CERTIMATE_B3_CLOUD_BINARIES")
	if dir == "" {
		dir = t.TempDir()
		buildB3Cloud(t, dir, runtime.GOARCH)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	cfg := plugin.PluginConfig{PluginDir: dir, StartTimeout: 15 * time.Second, DeployTimeout: 15 * time.Second}
	discovered, failures := plugin.Discover(ctx, cfg)
	if len(failures) != 0 || len(discovered) != len(b3CloudProviders) {
		t.Fatalf("discovery count=%d failures=%v", len(discovered), failures)
	}
	manager := plugin.NewManager(cfg, slog.New(slog.DiscardHandler))
	for _, dp := range discovered {
		t.Run(dp.Manifest.ProviderType, func(t *testing.T) {
			provider := dp.Manifest.ProviderType
			fixture := newB3CloudFixture(t, provider)
			meta, schema, err := manager.Bootstrap(ctx, dp)
			if err != nil {
				t.Fatal(err)
			}
			if meta.ProviderType != provider || meta.AccessProviderType != strings.Split(provider, "-")[0] || len(schema.AccessSchemaJSON) != 0 {
				t.Fatal("shared built-in access metadata changed")
			}
			var envelope struct {
				Provider string `json:"provider"`
				Category string `json:"category"`
			}
			if err := json.Unmarshal(schema.DeploySchemaJSON, &envelope); err != nil || envelope.Provider != provider || envelope.Category != "deploy" {
				t.Fatal("deploy schema missing")
			}
			if schema.I18n["en"][meta.DeployDisplayNameKey] == "" || schema.I18n["zh"][meta.DeployDisplayNameKey] == "" {
				t.Fatal("display translations missing")
			}
			access := map[string]any{"apiToken": "fixture-cloud-token", "accessToken": "fixture-cloud-token"}
			extended := map[string]any{}
			switch provider {
			case "gcore-cdn":
				extended = map[string]any{"resourceId": "17", "certificateId": "41"}
			case "linode-los":
				extended = map[string]any{"regionId": "us-east-1", "bucket": "cloud.example.com"}
			case "netlify":
				extended = map[string]any{"deployTarget": "website", "siteId": "fixture-site"}
			case "googlecloud-certificatemanager":
				key, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatal(err)
				}
				keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
				sa, _ := json.Marshal(map[string]string{"type": "service_account", "project_id": "fixture-project", "private_key_id": "fixture-id", "private_key": keyPEM, "client_email": "fixture@fixture-project.iam.gserviceaccount.com", "token_uri": "https://oauth2.googleapis.com/token"})
				access = map[string]any{"projectId": "fixture-project", "serviceAccountKey": string(sa)}
			}
			rawAccess, _ := json.Marshal(access)
			rawExtended, _ := json.Marshal(extended)
			req := &plugin.DeployRequest{AccessConfigJSON: string(rawAccess), ExtendedConfigJSON: string(rawExtended), LogLevel: "DEBUG"}
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			deploy := func(ctx context.Context, req *plugin.DeployRequest) (*plugin.DeployResult, error) {
				return manager.Deploy(ctx, dp, req, logger)
			}
			if os.Getenv("CERTIMATE_B1_CORE_BINARY") != "" {
				deploy = newB1WorkflowDeployer(t, dir, provider, workflowLifecycleTarget{
					barrier: &fixture.barrier,
					verify: func(request *plugin.DeployRequest) {
						fixture.mu.Lock()
						defer fixture.mu.Unlock()
						if strings.TrimSpace(fixture.cert) != strings.TrimSpace(request.CertificatePEM) || strings.TrimSpace(fixture.key) != strings.TrimSpace(request.PrivateKeyPEM) {
							t.Fatal("lifecycle deployment did not write the fresh certificate and private key to the target")
						}
					},
				})
			}
			for _, step := range []string{"initial", "repeat", "renew", "auth-failure", "recovery", "write-failure", "recovery"} {
				t.Run(step, func(t *testing.T) {
					if step == "initial" || step == "renew" || step == "write-failure" {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{"cloud.example.com"}, false)
						issuer, _, _ := makeCertificate(t, []string{"issuer.example.com"}, true)
						req.CertificatePEM += issuer
					}
					fixture.mu.Lock()
					fixture.mode = step
					before := fixture.writes
					fixture.mu.Unlock()
					_, err := deploy(ctx, req)
					fixture.mu.Lock()
					defer fixture.mu.Unlock()
					if step == "auth-failure" || step == "write-failure" {
						if err == nil {
							t.Fatal("remote error reported success")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if fixture.cert != req.CertificatePEM || fixture.key != req.PrivateKeyPEM {
						t.Fatal("target certificate chain/private key mismatch")
					}
					if provider == "googlecloud-certificatemanager" && step == "repeat" && fixture.writes != before {
						t.Fatal("identical certificate was uploaded twice")
					}
					if provider != "googlecloud-certificatemanager" && fixture.writes <= before {
						t.Fatal("target upload was not executed")
					}
					if provider == "gcore-cdn" && fixture.bindings == 0 {
						t.Fatal("CDN did not bind uploaded certificate")
					}
					if provider == "linode-los" && step == "renew" && fixture.deletes == 0 {
						t.Fatal("existing storage certificate was not replaced")
					}
					if strings.Contains(logs.String(), req.PrivateKeyPEM) || strings.Contains(logs.String(), "fixture-cloud-token") {
						t.Fatal("secret leaked into plugin logs")
					}
				})
			}
			if os.Getenv("CERTIMATE_B1_BASELINE") != "1" {
				t.Run("cancel-and-recover", func(t *testing.T) {
					entered := make(chan struct{})
					fixture.mu.Lock()
					fixture.mode = "timeout"
					fixture.blockedRequest = entered
					fixture.mu.Unlock()
					cancelCtx, stop := context.WithCancel(ctx)
					defer stop()
					completed := make(chan error, 1)
					go func() { _, err := manager.Deploy(cancelCtx, dp, req, logger); completed <- err }()
					select {
					case <-entered:
						stop()
					case err := <-completed:
						t.Fatalf("deployment ended before the target was blocked: %v", err)
					case <-time.After(cfg.DeployTimeout):
						t.Fatal("deployment did not reach the blocked target")
					}
					select {
					case err := <-completed:
						if err == nil {
							t.Fatal("canceled deployment reported success")
						}
					case <-time.After(cfg.DeployTimeout):
						t.Fatal("deployment did not stop after cancellation")
					}
					fixture.mu.Lock()
					fixture.mode = "recovery"
					fixture.mu.Unlock()
					if _, err := manager.Deploy(ctx, dp, req, logger); err != nil {
						t.Fatal(err)
					}
					fixture.mu.Lock()
					defer fixture.mu.Unlock()
					if fixture.cert != req.CertificatePEM || fixture.key != req.PrivateKeyPEM {
						t.Fatal("canceled plugin did not recover")
					}
				})
			}
			if provider == "gcore-cdn" {
				t.Run("upload-then-bind-failure", func(t *testing.T) {
					fixture.mu.Lock()
					fixture.mode = "bind-failure"
					before := fixture.writes
					fixture.mu.Unlock()
					if _, err := deploy(ctx, req); err == nil {
						t.Fatal("resource binding failure reported success")
					}
					fixture.mu.Lock()
					after := fixture.writes
					fixture.mode = "recovery"
					fixture.mu.Unlock()
					if after <= before {
						t.Fatal("fixture did not upload before binding failed")
					}
					if _, err := deploy(ctx, req); err != nil {
						t.Fatal(err)
					}
				})
				t.Run("create-certificate", func(t *testing.T) {
					req.ExtendedConfigJSON = `{"resourceId":17,"certificateId":0}`
					fixture.mu.Lock()
					fixture.mode = "initial"
					fixture.mu.Unlock()
					if _, err := deploy(ctx, req); err != nil {
						t.Fatal(err)
					}
					fixture.mu.Lock()
					defer fixture.mu.Unlock()
					if fixture.certificateID != 42 || fixture.cert != req.CertificatePEM || fixture.key != req.PrivateKeyPEM {
						t.Fatal("new certificate was not bound")
					}
				})
			}
		})
	}
}

func (f *b3CloudFixture) serve(w http.ResponseWriter, r *http.Request) {
	if err := f.barrier.waitHTTP(r); err != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if f.mode == "timeout" {
		entered := f.blockedRequest
		f.blockedRequest = nil
		f.mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		if entered != nil {
			close(entered)
		}
		<-r.Context().Done()
		f.mu.Lock()
		return
	}
	if r.URL.Path == "/token" {
		io.WriteString(w, `{"access_token":"fixture-oauth-token","token_type":"Bearer","expires_in":3600}`)
		return
	}
	if f.mode == "auth-failure" {
		w.WriteHeader(403)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "fixture access denied fixture-cloud-token " + f.key, "error": map[string]any{"code": 403, "message": "fixture access denied fixture-cloud-token " + f.key}})
		return
	}
	if f.mode == "write-failure" && r.Method != "GET" {
		w.WriteHeader(400)
		io.WriteString(w, `{"message":"fixture write denied"}`)
		return
	}
	if f.mode == "bind-failure" && r.URL.Path == "/cdn/resources/17" && r.Method == "PUT" {
		w.WriteHeader(400)
		io.WriteString(w, `{"message":"fixture resource binding denied"}`)
		return
	}
	var body map[string]any
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	write := func(v any) {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			f.t.Error(err)
		}
	}
	switch f.provider {
	case "gcore-cdn":
		if r.Header.Get("Authorization") != "APIKey fixture-cloud-token" {
			f.t.Error("Gcore authorization mismatch")
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/cdn/sslData"):
			if r.Method == "POST" || r.Method == "PATCH" {
				f.cert, _ = body["sslCertificate"].(string)
				f.key, _ = body["sslPrivateKey"].(string)
				f.writes++
				if r.Method == "POST" {
					f.certificateID = 42
				} else {
					f.certificateID = 41
				}
			}
			write(map[string]any{"id": f.certificateID, "name": "fixture-certificate"})
		case r.URL.Path == "/cdn/resources/17":
			if r.Method == "PUT" {
				if body["sslData"] != float64(f.certificateID) || body["sslEnabled"] != true || body["originGroup"] != float64(7) {
					f.t.Error("Gcore resource binding mismatch")
				}
				f.bindings++
			}
			write(map[string]any{"id": 17, "description": "fixture-resource", "active": true, "originGroup": 7, "originProtocol": "HTTPS", "secondaryHostnames": []string{"cloud.example.com"}})
		default:
			f.t.Errorf("unexpected Gcore request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	case "googlecloud-certificatemanager":
		if r.Header.Get("Authorization") != "Bearer fixture-oauth-token" {
			f.t.Error("Google authorization mismatch")
		}
		if r.URL.Path != "/v1/projects/fixture-project/locations/global/certificates" {
			f.t.Errorf("unexpected Google path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.Method == "GET" {
			items := []any{}
			if f.cert != "" {
				items = append(items, map[string]any{"name": f.name, "pemCertificate": f.cert})
			}
			write(map[string]any{"certificates": items})
			return
		}
		self, ok := body["selfManaged"].(map[string]any)
		if !ok {
			f.t.Error("Google selfManaged certificate missing")
			w.WriteHeader(400)
			return
		}
		f.cert, _ = self["pemCertificate"].(string)
		f.key, _ = self["pemPrivateKey"].(string)
		f.name = "projects/fixture-project/locations/global/certificates/" + r.URL.Query().Get("certificateId")
		f.writes++
		write(map[string]any{"name": "projects/fixture-project/locations/global/operations/fixture-operation", "done": true, "response": map[string]any{"name": f.name}})
	case "linode-los":
		if r.Header.Get("Authorization") != "Bearer fixture-cloud-token" {
			f.t.Error("Linode authorization mismatch")
		}
		if r.URL.Path != "/v4/object-storage/buckets/us-east-1/cloud.example.com/ssl" {
			f.t.Errorf("unexpected Linode path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.Method == "DELETE" {
			f.cert = ""
			f.key = ""
			f.deletes++
		}
		if r.Method == "POST" {
			f.cert, _ = body["certificate"].(string)
			f.key, _ = body["private_key"].(string)
			f.writes++
		}
		write(map[string]any{"ssl": f.cert != ""})
	case "netlify":
		if r.Header.Get("Authorization") != "Bearer fixture-cloud-token" {
			f.t.Error("Netlify authorization mismatch")
		}
		if r.URL.Path != "/api/v1/sites/fixture-site/ssl" || r.Method != "POST" {
			f.t.Errorf("unexpected Netlify request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		q := r.URL.Query()
		f.cert = q.Get("certificate") + q.Get("ca_certificates")
		f.key = q.Get("key")
		f.writes++
		write(map[string]any{"state": "custom", "domains": []string{"cloud.example.com"}})
	}
}

func newB3CloudFixture(t *testing.T, provider string) *b3CloudFixture {
	t.Helper()
	f := &b3CloudFixture{t: t, provider: provider, certificateID: 41}
	hosts := []string{"api.gcore.com", "certificatemanager.googleapis.com", "oauth2.googleapis.com", "api.linode.com", "api.netlify.com"}
	ca, key, _ := makeCertificate(t, hosts, true)
	pair, err := tls.X509KeyPair([]byte(ca), []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	target := httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	target.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	target.StartTLS()
	t.Cleanup(target.Close)
	allowed := map[string]bool{}
	for _, host := range hosts {
		allowed[host+":443"] = true
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || !allowed[r.Host] {
			http.Error(w, "unexpected proxy destination", 502)
			return
		}
		upstream, err := net.DialTimeout("tcp", target.Listener.Addr().String(), 5*time.Second)
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
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte(ca), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("SSL_CERT_FILE", path)
	return f
}

func buildB3Cloud(t *testing.T, dir, arch string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range b3CloudProviders {
		out := filepath.Join(dir, provider)
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("go", "build", "-o", filepath.Join(out, provider), "./"+provider)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", provider, err, output)
		}
		raw, err := os.ReadFile(filepath.Join(root, provider, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Icon string `json:"icon"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"manifest.json", m.Icon} {
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

func runB3CloudInLinux(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.Architecture}}").CombinedOutput()
	if err != nil {
		t.Fatalf("Docker is required for TLS fixtures: %v\n%s", err, output)
	}
	arch := strings.TrimSpace(string(output))
	if arch == "aarch64" {
		arch = "arm64"
	}
	if arch == "x86_64" {
		arch = "amd64"
	}
	dir := t.TempDir()
	buildB3Cloud(t, dir, arch)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "b3-cloud.test"), "./integration")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build tests: %v\n%s", err, output)
	}
	args := []string{"run", "--rm", "--network=none", "--mount", "type=bind,src=" + dir + ",dst=/fixtures,readonly", "-e", "CERTIMATE_B3_CLOUD_BINARIES=/fixtures"}
	if binary := os.Getenv("CERTIMATE_B1_CORE_BINARY"); binary != "" {
		abs, err := filepath.Abs(binary)
		if err != nil {
			t.Fatal(err)
		}
		args = append(args, "--mount", "type=bind,src="+abs+",dst=/core,readonly", "-e", "CERTIMATE_B1_CORE_BINARY=/core", "-e", "CERTIMATE_B1_BASELINE="+os.Getenv("CERTIMATE_B1_BASELINE"))
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	args = append(args, image, "/fixtures/b3-cloud.test", "-test.run=^TestB3CloudDeploymentBatch$", "-test.v", "-test.timeout=10m")
	cmd = exec.CommandContext(ctx, "docker", args...)
	output, err = cmd.CombinedOutput()
	t.Logf("Linux workflow output:\n%s", output)
	if err != nil {
		t.Fatalf("Linux workflow verification: %v", err)
	}
}
