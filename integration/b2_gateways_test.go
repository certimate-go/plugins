package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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

var b2GatewayProviders = []string{"apisix", "kong", "nginxproxymanager", "goedge", "safeline", "samwaf", "samwaf-console"}

func TestB2GatewayDeploymentBatch(t *testing.T) {
	if runtime.GOOS != "linux" && os.Getenv("CERTIMATE_B2_NATIVE") != "1" {
		runB2GatewaysInLinux(t)
		return
	}
	dir := os.Getenv("CERTIMATE_B2_GATEWAY_BINARIES")
	if dir == "" {
		dir = t.TempDir()
		buildB2Gateways(t, dir, runtime.GOARCH)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := plugin.PluginConfig{PluginDir: dir, StartTimeout: 15 * time.Second, DeployTimeout: 15 * time.Second}
	discovered, failures := plugin.Discover(ctx, cfg)
	if len(failures) != 0 || len(discovered) != len(b2GatewayProviders) {
		t.Fatalf("discovery count=%d failures=%v", len(discovered), failures)
	}
	manager := plugin.NewManager(cfg, slog.New(slog.DiscardHandler))
	for _, dp := range discovered {
		t.Run(dp.Manifest.ProviderType, func(t *testing.T) {
			provider := dp.Manifest.ProviderType
			fixture := &b2GatewayFixture{t: t, provider: provider}
			server := httptest.NewServer(http.HandlerFunc(fixture.serve))
			t.Cleanup(server.Close)
			meta, schema, err := manager.Bootstrap(ctx, dp)
			if err != nil {
				t.Fatal(err)
			}
			validateSchema(t, schema, provider, strings.Split(provider, "-")[0], meta)
			if meta.ProviderType != dp.Manifest.ProviderType || meta.AccessProviderType != dp.Manifest.AccessProviderType || meta.DeployCategory != dp.Manifest.DeployCategory {
				t.Fatal("metadata mismatch")
			}
			access := map[string]any{"allowInsecureConnections": "true", "serverUrl": server.URL, "apiKey": "fixture-key", "apiToken": "fixture-token", "apiRole": "user", "accessKeyId": "fixture-id", "accessKey": "fixture-key", "authMethod": "token"}
			extended := map[string]any{"deployTarget": "certificate", "certificateId": "42"}
			if provider == "kong" {
				extended["workspace"] = "team"
			}
			if strings.HasPrefix(provider, "samwaf") {
				extended["certificateId"] = "fcde7d63-1e83-422c-bdae-7486c5b9327b"
			}
			if provider == "samwaf-console" {
				extended = map[string]any{"autoRestart": true}
			}
			accessJSON, _ := json.Marshal(access)
			extendedJSON, _ := json.Marshal(extended)
			req := &plugin.DeployRequest{AccessConfigJSON: string(accessJSON), ExtendedConfigJSON: string(extendedJSON), LogLevel: "DEBUG"}
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			deploy := func(ctx context.Context, req *plugin.DeployRequest) (*plugin.DeployResult, error) {
				return manager.Deploy(ctx, dp, req, logger)
			}
			if os.Getenv("CERTIMATE_B1_CORE_BINARY") != "" {
				deploy = newB1WorkflowDeployer(t, dir, provider)
			}
			for _, step := range []string{"initial", "repeat", "renew", "auth-failure", "recovery"} {
				t.Run(step, func(t *testing.T) {
					if step == "initial" || step == "renew" {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{"gateway.example.com"}, false)
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
							t.Fatal("auth failure reported success")
						}
						if fixture.writes != before {
							t.Fatal("failure changed remote certificate")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if fixture.writes <= before || fixture.cert != req.CertificatePEM || fixture.key != req.PrivateKeyPEM {
						t.Fatal("remote certificate/private key mismatch")
					}
					if provider == "samwaf-console" && (!fixture.enabled || fixture.restarts == 0) {
						t.Fatal("console not enabled/restarted")
					}
					if provider == "nginxproxymanager" && fixture.reloads == 0 {
						t.Fatal("NPM reload missing")
					}
					if strings.Contains(logs.String(), req.PrivateKeyPEM) || strings.Contains(logs.String(), "fixture-key") || strings.Contains(logs.String(), "fixture-token") {
						t.Fatal("secret leaked into logs")
					}
				})
			}
			if provider == "samwaf-console" || provider == "nginxproxymanager" {
				t.Run("upload-then-bind-failure", func(t *testing.T) {
					fixture.mu.Lock()
					fixture.mode = "bind-failure"
					before := fixture.writes
					fixture.mu.Unlock()
					if _, err := deploy(ctx, req); err == nil {
						t.Fatal("binding failure reported success")
					}
					fixture.mu.Lock()
					if fixture.writes <= before {
						t.Error("fixture did not reach upload before bind failure")
					}
					fixture.mode = "recovery"
					fixture.mu.Unlock()
					if _, err := deploy(ctx, req); err != nil {
						t.Fatal(err)
					}
				})
			}

			if provider == "nginxproxymanager" {
				access["authMethod"] = "password"
				access["username"] = "fixture-user"
				access["password"] = "fixture-password"
				accessJSON, _ = json.Marshal(access)
				req.AccessConfigJSON = string(accessJSON)
				for _, hostType := range []string{"proxy", "redirection", "stream", "dead"} {
					t.Run("host-"+hostType, func(t *testing.T) {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{"gateway.example.com"}, false)
						issuer, _, _ := makeCertificate(t, []string{"issuer.example.com"}, true)
						req.CertificatePEM += issuer
						for _, pattern := range []string{"specified", "certsan"} {
							values, _ := json.Marshal(map[string]any{"deployTarget": "host", "hostType": hostType, "hostId": "7", "hostMatchPattern": pattern})
							req.ExtendedConfigJSON = string(values)
							if _, err := deploy(ctx, req); err != nil {
								t.Fatal(err)
							}
							fixture.mu.Lock()
							writes := fixture.writes
							if fixture.cert != req.CertificatePEM || fixture.key != req.PrivateKeyPEM || fixture.bindings[hostType] != fixture.nextCert {
								t.Error("host did not bind uploaded certificate")
							}
							fixture.mu.Unlock()
							if _, err := deploy(ctx, req); err != nil {
								t.Fatal(err)
							}
							fixture.mu.Lock()
							if fixture.writes != writes {
								t.Error("same certificate uploaded again")
							}
							fixture.mu.Unlock()
						}
					})
				}
				values, _ := json.Marshal(map[string]any{"deployTarget": "certificate", "certificateId": "42"})
				req.ExtendedConfigJSON = string(values)
			}

			if provider == "samwaf-console" {
				fixture.mu.Lock()
				before := fixture.restarts
				fixture.mu.Unlock()
				req.ExtendedConfigJSON = `{"autoRestart":false}`
				if _, err := deploy(ctx, req); err != nil {
					t.Fatal(err)
				}
				fixture.mu.Lock()
				if fixture.restarts != before {
					t.Error("disabled restart still restarted manager")
				}
				fixture.mu.Unlock()
			}

			if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
				fixture.mu.Lock()
				fixture.mode = "timeout"
				fixture.mu.Unlock()
				timeoutCtx, stop := context.WithTimeout(ctx, 500*time.Millisecond)
				defer stop()
				if _, err := deploy(timeoutCtx, req); err == nil {
					t.Fatal("timeout reported success")
				}
				fixture.mu.Lock()
				fixture.mode = "recovery"
				fixture.mu.Unlock()
				if _, err := deploy(ctx, req); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

type b2GatewayFixture struct {
	t                         *testing.T
	mu                        sync.Mutex
	provider, mode, cert, key string
	writes, restarts, reloads int
	enabled                   bool
	nextCert                  int
	certificates              []map[string]any
	bindings                  map[string]int
}

func (f *b2GatewayFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	mode := f.mode
	echo := "fixture-key fixture-token fixture-password " + f.key
	f.mu.Unlock()
	if mode == "timeout" {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if mode == "bind-failure" && (r.URL.Path == "/api/settings/default-site" && r.Method == "PUT" || r.URL.Path == "/api/v1/vipconfig/updateSslEnable") {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "binding failed " + echo})
		return
	}

	if mode == "auth-failure" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized " + echo})
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(v any) {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			f.t.Error(err)
		}
	}
	body := map[string]any{}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") && r.Body != nil {
		data, _ := io.ReadAll(r.Body)
		if len(data) > 0 {
			if err := json.Unmarshal(data, &body); err != nil {
				f.t.Error(err)
			}
		}
	}
	record := func(cert, key string) { f.cert = cert; f.key = key; f.writes++ }
	check := func(ok bool, message string) {
		if !ok {
			f.t.Error(message)
		}
	}
	switch f.provider {
	case "apisix":
		check(r.Method == "PUT" && r.URL.Path == "/apisix/admin/ssls/42", "APISIX wrong resource/method")
		check(r.Header.Get("X-Api-Key") == "fixture-key", "APISIX missing auth")
		check(body["id"] == "42" && body["type"] == "server" && body["status"] == float64(1), "APISIX config mismatch")
		record(fmt.Sprint(body["cert"]), fmt.Sprint(body["key"]))
		reply(map[string]any{})
	case "kong":
		check(r.Method == "PUT" && r.URL.Path == "/team/certificates/42", "Kong wrong resource/workspace")
		check(r.Header.Get("Kong-Admin-Token") == "fixture-token", "Kong missing auth")
		record(fmt.Sprint(body["cert"]), fmt.Sprint(body["key"]))
		reply(map[string]any{})
	case "goedge":
		if r.URL.Path == "/APIAccessTokenService/getAPIAccessToken" {
			check(body["accessKey"] == "fixture-key" && body["type"] == "user", "GoEdge auth mismatch")
			reply(map[string]any{"code": 200, "data": map[string]any{"token": "edge-token", "expiresAt": time.Now().Add(time.Hour).Unix()}})
			return
		}
		check(r.URL.Path == "/SSLCertService/updateSSLCert" && body["sslCertId"] == float64(42), "GoEdge wrong resource")
		check(r.Header.Get("X-Edge-Access-Token") == "edge-token", "GoEdge missing token")
		cert, e := base64.StdEncoding.DecodeString(fmt.Sprint(body["certData"]))
		if e != nil {
			f.t.Error(e)
		}
		key, e := base64.StdEncoding.DecodeString(fmt.Sprint(body["keyData"]))
		if e != nil {
			f.t.Error(e)
		}
		record(string(cert), string(key))
		reply(map[string]any{"code": 200})
	case "safeline":
		check(r.URL.Path == "/api/open/cert" && body["id"] == float64(42), "SafeLine wrong resource")
		check(r.Header.Get("X-SLCE-API-TOKEN") == "fixture-token", "SafeLine missing auth")
		manual, ok := body["manual"].(map[string]any)
		if !ok {
			f.t.Error("missing manual certificate")
			return
		}
		record(fmt.Sprint(manual["crt"]), fmt.Sprint(manual["key"]))
		reply(map[string]any{})
	case "samwaf", "samwaf-console":
		check(r.Header.Get("X-API-Key") == "fixture-key", "SamWAF missing auth")
		switch r.URL.Path {
		case "/api/v1/sslconfig/detail":
			check(r.URL.Query().Get("id") == "fcde7d63-1e83-422c-bdae-7486c5b9327b", "SamWAF wrong lookup")
			reply(map[string]any{"code": 0, "data": map[string]any{"id": r.URL.Query().Get("id")}})
			return
		case "/api/v1/sslconfig/edit":
			check(body["id"] == "fcde7d63-1e83-422c-bdae-7486c5b9327b", "SamWAF wrong resource")
			record(fmt.Sprint(body["cert_content"]), fmt.Sprint(body["key_content"]))
		case "/api/v1/vipconfig/uploadSslCert":
			record(fmt.Sprint(body["cert_content"]), fmt.Sprint(body["key_content"]))
		case "/api/v1/vipconfig/updateSslEnable":
			f.enabled = body["ssl_enable"] == true
		case "/api/v1/vipconfig/restartManager":
			f.restarts++
		default:
			f.t.Errorf("unexpected SamWAF path %s", r.URL.Path)
		}
		reply(map[string]any{"code": 0})
	case "nginxproxymanager":
		if r.URL.Path == "/api/tokens" {
			check(body["identity"] == "fixture-user" && body["secret"] == "fixture-password", "NPM password auth mismatch")
			reply(map[string]any{"token": "fixture-token"})
			return
		}
		check(r.Header.Get("Authorization") == "Bearer fixture-token", "NPM missing auth")
		switch r.URL.Path {
		case "/api/nginx/certificates":
			if r.Method == "GET" {
				if f.certificates == nil {
					reply([]any{})
				} else {
					reply(f.certificates)
				}
				return
			}
			f.nextCert++
			reply(map[string]any{"id": f.nextCert, "nice_name": "fixture"})
			return
		case "/api/nginx/proxy-hosts", "/api/nginx/redirection-hosts", "/api/nginx/streams", "/api/nginx/dead-hosts":
			hostType := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/nginx/"), "-hosts")
			if hostType == "streams" {
				hostType = "stream"
			}
			reply([]any{map[string]any{"id": 7, "domain_names": []string{"gateway.example.com"}, "certificate_id": f.bindings[hostType]}, map[string]any{"id": 8, "domain_names": []string{"other.example.com"}, "certificate_id": 99}})
			return
		default:
			if strings.HasPrefix(r.URL.Path, "/api/nginx/") && strings.HasSuffix(r.URL.Path, "/7") {
				hostType := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/nginx/"), "/7"), "-hosts")
				if hostType == "streams" {
					hostType = "stream"
				}
				if f.bindings == nil {
					f.bindings = map[string]int{}
				}
				id, ok := body["certificate_id"].(float64)
				check(ok, "missing host certificate ID")
				f.bindings[hostType] = int(id)
				reply(map[string]any{})
				return
			}
			if !strings.HasSuffix(r.URL.Path, "/upload") {
				f.t.Errorf("unexpected NPM path %s", r.URL.Path)
				w.WriteHeader(404)
				return
			}

			check(r.Method == "POST", "NPM upload method")
			read := func(name string) string {
				file, _, err := r.FormFile(name)
				if err != nil {
					f.t.Error(err)
					return ""
				}
				defer file.Close()
				data, err := io.ReadAll(file)
				if err != nil {
					f.t.Error(err)
				}
				return string(data)
			}
			cert, issuer, key := read("certificate"), read("intermediate_certificate"), read("certificate_key")
			record(cert+issuer, key)
			if r.URL.Path != "/api/nginx/certificates/42/upload" {
				check(r.URL.Path == fmt.Sprintf("/api/nginx/certificates/%d/upload", f.nextCert), "NPM wrong certificate upload ID")
				f.certificates = append(f.certificates, map[string]any{"id": f.nextCert, "meta": map[string]any{"certificate": cert, "certificate_key": key, "intermediate_certificate": issuer}})
			}
			reply(map[string]any{})
		case "/api/settings/default-site":
			if r.Method == "PUT" {
				check(body["value"] == "404", "NPM default site changed")
				f.reloads++
			}
			reply(map[string]any{"value": "404"})
		}
	}
}

func buildB2Gateways(t *testing.T, dir, arch string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range b2GatewayProviders {
		out := filepath.Join(dir, provider)
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(out, provider), "./"+provider)
		cmd.Dir = root
		targetOS := "linux"
		if os.Getenv("CERTIMATE_B2_NATIVE") == "1" {
			targetOS = runtime.GOOS
		}
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

func runB2GatewaysInLinux(t *testing.T) {
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
	buildB2Gateways(t, dir, arch)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "b2-gateways.test"), "./integration")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Linux tests: %v\n%s", err, output)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	args := []string{"run", "--rm", "--network=none", "--mount", "type=bind,src=" + dir + ",dst=/fixtures,readonly", "-e", "CERTIMATE_B2_GATEWAY_BINARIES=/fixtures"}
	if coreBinary := os.Getenv("CERTIMATE_B1_CORE_BINARY"); coreBinary != "" {
		data, err := os.ReadFile(coreBinary)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "core-app"), data, 0o755); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-e", "CERTIMATE_B1_CORE_BINARY=/fixtures/core-app")
	}
	if os.Getenv("CERTIMATE_B1_BASELINE") == "1" {
		args = append(args, "-e", "CERTIMATE_B1_BASELINE=1")
	}
	args = append(args, image, "/fixtures/b2-gateways.test", "-test.run=^TestB2GatewayDeploymentBatch$", "-test.v", "-test.timeout=5m")
	cmd = exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	t.Logf("Linux plugin test output:\n%s", output)
	if err != nil {
		t.Fatalf("Linux end-to-end tests: %v", err)
	}
}
