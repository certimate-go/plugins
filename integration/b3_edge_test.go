package integration

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/tls"
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
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

var b3EdgeProviders = []string{"bunny-cdn", "cloudflare-ssl", "cpanel", "digitalocean-certificate"}

func TestB3EdgeDeploymentBatch(t *testing.T) {
	if runtime.GOOS != "linux" {
		runB3EdgeInLinux(t)
		return
	}
	dir := os.Getenv("CERTIMATE_B3_EDGE_BINARIES")
	if dir == "" {
		dir = t.TempDir()
		buildB3Edge(t, dir, runtime.GOARCH)
	}
	fixture := &b3EdgeFixture{t: t}
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)
	newB3EdgeProxy(t, fixture)
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	cfg := plugin.PluginConfig{PluginDir: dir, StartTimeout: 15 * time.Second, DeployTimeout: 15 * time.Second}
	discovered, failures := plugin.Discover(ctx, cfg)
	if len(failures) != 0 || len(discovered) != len(b3EdgeProviders) {
		t.Fatalf("discovery count=%d failures=%v", len(discovered), failures)
	}
	manager := plugin.NewManager(cfg, slog.New(slog.DiscardHandler))
	for _, dp := range discovered {
		t.Run(dp.Manifest.ProviderType, func(t *testing.T) {
			provider := dp.Manifest.ProviderType
			fixture.mu.Lock()
			fixture.provider, fixture.mode, fixture.cert, fixture.key, fixture.resource, fixture.environment, fixture.method = provider, "", "", "", "", "", ""
			fixture.writes = 0
			fixture.mu.Unlock()
			meta, schema, err := manager.Bootstrap(ctx, dp)
			if err != nil {
				t.Fatal(err)
			}
			if meta.ProviderType != provider || meta.AccessProviderType != dp.Manifest.AccessProviderType || meta.DeployCategory != dp.Manifest.DeployCategory {
				t.Fatal("metadata mismatch")
			}
			if len(schema.AccessSchemaJSON) != 0 {
				t.Fatal("plugin redefines built-in DNS access schema")
			}
			var envelope struct {
				SchemaVersion, Provider, Category string
				Schema                            struct{ Columns []json.RawMessage }
			}
			if err := json.Unmarshal(schema.DeploySchemaJSON, &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.SchemaVersion != "form/v1" || envelope.Provider != provider || envelope.Category != "deploy" {
				t.Fatal("deploy schema envelope mismatch")
			}
			if schema.I18n["en"][meta.DeployDisplayNameKey] == "" || schema.I18n["zh"][meta.DeployDisplayNameKey] == "" {
				t.Fatal("missing display translation")
			}
			access := map[string]any{"apiKey": "fixture-key", "apiToken": "fixture-token", "accessToken": "fixture-token", "username": "fixture-user", "serverUrl": server.URL, "allowInsecureConnections": "true"}
			extended := map[string]any{}
			switch provider {
			case "bunny-cdn":
				extended = map[string]any{"pullZoneId": "42", "hostname": "edge.example.com"}
			case "cloudflare-ssl":
				extended = map[string]any{"zoneId": "zone-42", "environment": "staging"}
			case "cpanel":
				extended = map[string]any{"deployTarget": "website", "domain": "edge.example.com"}
			}
			accessJSON, _ := json.Marshal(access)
			extendedJSON, _ := json.Marshal(extended)
			req := &plugin.DeployRequest{AccessConfigJSON: string(accessJSON), ExtendedConfigJSON: string(extendedJSON), LogLevel: "DEBUG"}
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			deploy := func(ctx context.Context, req *plugin.DeployRequest) (*plugin.DeployResult, error) {
				return manager.Deploy(ctx, dp, req, logger)
			}
			workflow := os.Getenv("CERTIMATE_B1_CORE_BINARY") != ""
			if workflow {
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
			for _, step := range []string{"initial", "repeat", "renew", "auth-failure", "recovery"} {
				t.Run(step, func(t *testing.T) {
					if step == "initial" || step == "renew" {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{"edge.example.com"}, false)
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
					if step == "auth-failure" {
						if err == nil {
							t.Fatal("authentication failure reported success")
						}
						if fixture.writes != before {
							t.Fatal("failed authentication changed certificate")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if fixture.cert != req.CertificatePEM || fixture.key != req.PrivateKeyPEM {
						t.Fatal("target certificate chain/private key mismatch")
					}
					if provider == "digitalocean-certificate" && (step == "repeat" || step == "recovery") {
						if fixture.writes != before {
							t.Fatal("identical certificate uploaded again")
						}
					} else if fixture.writes <= before {
						t.Fatal("target was not updated")
					}
					switch provider {
					case "bunny-cdn":
						if fixture.resource != "42/edge.example.com" {
							t.Fatal("wrong Bunny resource")
						}
					case "cloudflare-ssl":
						if fixture.resource != "zone-42" || fixture.environment != "staging" {
							t.Fatal("wrong Cloudflare zone/environment")
						}
					case "cpanel":
						if fixture.resource != "edge.example.com" {
							t.Fatal("wrong cPanel domain")
						}
					}
					for _, secret := range []string{req.PrivateKeyPEM, base64.StdEncoding.EncodeToString([]byte(req.PrivateKeyPEM)), "fixture-key", "fixture-token"} {
						if strings.Contains(logs.String(), secret) {
							t.Fatal("secret leaked into plugin logs")
						}
					}
				})
			}
			if provider == "bunny-cdn" {
				req.ExtendedConfigJSON = `{"pullZoneId":42,"hostname":"edge.example.com"}`
				if _, err := deploy(ctx, req); err == nil {
					t.Fatal("numeric pull zone identifier changed historical GetString behavior")
				}
				req.ExtendedConfigJSON = string(extendedJSON)
			}
			if provider == "cloudflare-ssl" {
				extended["certificateId"] = "cert-42"
				extended["environment"] = "production"
				data, _ := json.Marshal(extended)
				req.ExtendedConfigJSON = string(data)
				if _, err := deploy(ctx, req); err != nil {
					t.Fatal(err)
				}
				fixture.mu.Lock()
				if fixture.resource != "zone-42/cert-42" || fixture.method != http.MethodPatch || fixture.environment != "production" {
					t.Error("Cloudflare did not update specified certificate")
				}
				fixture.mu.Unlock()
			}
			if !workflow {
				entered := make(chan struct{})
				fixture.mu.Lock()
				fixture.mode = "timeout"
				fixture.blockedRequest = entered
				fixture.mu.Unlock()
				cancelCtx, stop := context.WithCancel(ctx)
				defer stop()
				completed := make(chan error, 1)
				go func() { _, err := deploy(cancelCtx, req); completed <- err }()
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
				if _, err := deploy(ctx, req); err != nil {
					t.Fatal(err)
				}
				fixture.mu.Lock()
				recovered := fixture.cert == req.CertificatePEM && fixture.key == req.PrivateKeyPEM
				fixture.mu.Unlock()
				if !recovered {
					t.Fatal("canceled plugin did not recover target certificate")
				}
				t.Logf("CANCELLATION PASS %s after target received request", provider)
			}
		})
	}
}

type b3EdgeFixture struct {
	barrier                                                  workflowRequestBarrier
	t                                                        *testing.T
	mu                                                       sync.Mutex
	provider, mode, cert, key, resource, environment, method string
	writes                                                   int
	blockedRequest                                           chan struct{}
}

func (f *b3EdgeFixture) serve(w http.ResponseWriter, r *http.Request) {
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
	if f.mode == "auth-failure" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "fixture-token fixture-key", "errors": []map[string]any{{"code": 1000, "message": "fixture-token"}}})
		return
	}
	if r.Header.Get("AccessKey") != "fixture-key" && r.Header.Get("Authorization") != "Bearer fixture-token" && r.Header.Get("Authorization") != "cpanel fixture-user:fixture-token" {
		f.t.Error("missing expected authentication")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var body map[string]string
	if r.Method != http.MethodGet {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Error(err)
			w.WriteHeader(400)
			return
		}
	}
	switch f.provider {
	case "bunny-cdn":
		if r.URL.Path != "/pullzone/42/addCertificate" || r.Method != http.MethodPost {
			f.t.Errorf("unexpected Bunny request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		cert, err := base64.StdEncoding.DecodeString(body["Certificate"])
		if err != nil {
			f.t.Error(err)
		}
		key, err := base64.StdEncoding.DecodeString(body["CertificateKey"])
		if err != nil {
			f.t.Error(err)
		}
		f.cert, f.key, f.resource = string(cert), string(key), "42/"+body["Hostname"]
		f.writes++
		fmt.Fprint(w, `{}`)
	case "cloudflare-ssl":
		if r.URL.Path != "/client/v4/zones/zone-42/custom_certificates" && r.URL.Path != "/client/v4/zones/zone-42/custom_certificates/cert-42" {
			f.t.Errorf("unexpected Cloudflare path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		f.cert, f.key, f.environment, f.method = body["certificate"], body["private_key"], body["deploy"], r.Method
		f.resource = "zone-42"
		if strings.HasSuffix(r.URL.Path, "/cert-42") {
			f.resource += "/cert-42"
		}
		if body["bundle_method"] != "ubiquitous" {
			f.t.Error("Cloudflare bundle method changed")
		}
		f.writes++
		fmt.Fprint(w, `{"success":true,"result":{"id":"cert-42"}}`)
	case "cpanel":
		if r.URL.Path != "/execute/SSL/install_ssl" || r.Method != http.MethodGet {
			f.t.Errorf("unexpected cPanel request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		q := r.URL.Query()
		f.cert, f.key, f.resource = q.Get("cert")+q.Get("cabundle"), q.Get("key"), q.Get("domain")
		f.writes++
		fmt.Fprint(w, `{"status":1,"data":{"domain":"edge.example.com"}}`)
	case "digitalocean-certificate":
		if r.URL.Path != "/v2/certificates" {
			f.t.Errorf("unexpected DigitalOcean path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.Method == http.MethodGet {
			certs := []map[string]any{}
			if f.cert != "" {
				certificate, err := xcert.ParseCertificateFromPEM(f.cert)
				if err != nil {
					f.t.Error(err)
					w.WriteHeader(500)
					return
				}
				sum := sha1.Sum(certificate.Raw)
				certs = append(certs, map[string]any{"id": "cert-42", "name": "fixture", "dns_names": certificate.DNSNames, "not_after": certificate.NotAfter.Format(time.RFC3339), "sha1_fingerprint": hex.EncodeToString(sum[:])})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"certificates": certs})
			return
		}
		if body["type"] != "custom" || !strings.HasPrefix(body["name"], "certimate-") {
			f.t.Error("DigitalOcean certificate metadata changed")
		}
		f.cert, f.key = body["leaf_certificate"]+body["certificate_chain"], body["private_key"]
		f.writes++
		fmt.Fprint(w, `{"certificate":{"id":"cert-42","name":"fixture"}}`)
	default:
		f.t.Errorf("unknown fixture provider %s", f.provider)
		w.WriteHeader(404)
	}
}

func newB3EdgeProxy(t *testing.T, f *b3EdgeFixture) {
	t.Helper()
	hosts := []string{"api.bunny.net", "api.cloudflare.com", "api.digitalocean.com"}
	ca, key, _ := makeCertificate(t, hosts, true)
	pair, err := tls.X509KeyPair([]byte(ca), []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	cloud := httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	cloud.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	cloud.StartTLS()
	t.Cleanup(cloud.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || (r.Host != "api.bunny.net:443" && r.Host != "api.cloudflare.com:443" && r.Host != "api.digitalocean.com:443") {
			http.Error(w, "unexpected destination", 502)
			return
		}
		upstream, err := net.DialTimeout("tcp", cloud.Listener.Addr().String(), 5*time.Second)
		if err != nil {
			http.Error(w, "fixture unavailable", 502)
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
}

func buildB3Edge(t *testing.T, dir, arch string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range b3EdgeProviders {
		out := filepath.Join(dir, provider)
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(out, provider), "./"+provider)
		cmd.Dir = root
		targetOS := "linux"
		cmd.Env = append(os.Environ(), "GOOS="+targetOS, "GOARCH="+arch, "CGO_ENABLED=0")
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

func runB3EdgeInLinux(t *testing.T) {
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
	buildB3Edge(t, dir, arch)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "b3-edge.test"), "./integration")
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
	cmd = exec.CommandContext(ctx, "docker", "run", "--rm", "--network=none", "--mount", "type=bind,src="+dir+",dst=/fixtures,readonly", "-e", "CERTIMATE_B3_EDGE_BINARIES=/fixtures", image, "/fixtures/b3-edge.test", "-test.run=^TestB3EdgeDeploymentBatch$", "-test.v", "-test.timeout=8m")
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
