package integration

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

var b2PanelProviders = []string{"1panel", "1panel-console", "baotapanel", "baotapanel-console", "baotapanelgo", "baotapanelgo-console", "baotawaf", "baotawaf-console"}

type b2PanelFixture struct {
	mu                        sync.Mutex
	mode                      string
	cert, key                 string
	uploadedCert, uploadedKey string
	boundCert, boundKey       string
	uploads, bindings         int
	started, cancelled        chan struct{}
}

func TestB2PanelsDeploymentBatch(t *testing.T) {
	dir := os.Getenv("CERTIMATE_B2_PANELS_BINARIES")
	if dir == "" {
		dir = t.TempDir()
		buildB2Panels(t, dir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	config := plugin.PluginConfig{PluginDir: dir, StartTimeout: 15 * time.Second, DeployTimeout: 20 * time.Second}
	discovered, failures := plugin.Discover(ctx, config)
	if len(failures) != 0 || len(discovered) != len(b2PanelProviders) {
		t.Fatalf("discovery count=%d failures=%v", len(discovered), failures)
	}
	manager := plugin.NewManager(config, slog.New(slog.DiscardHandler))
	for _, dp := range discovered {
		t.Run(dp.Manifest.ProviderType, func(t *testing.T) {
			provider := dp.Manifest.ProviderType
			family := strings.TrimSuffix(provider, "-console")
			meta, schema, err := manager.Bootstrap(ctx, dp)
			if err != nil {
				t.Fatal(err)
			}
			validateSchema(t, schema, provider, family, meta)
			if meta.ProviderType != provider || meta.AccessProviderType != family || meta.ProtocolVersion != plugin.ProtocolVersion || meta.DeployCategory != dp.Manifest.DeployCategory {
				t.Fatal("manifest and metadata differ")
			}
			f := &b2PanelFixture{}
			server := httptest.NewServer(http.HandlerFunc(f.serve))
			t.Cleanup(server.Close)
			cert, key, _ := makeCertificate(t, []string{"app.example.com"}, false)
			req := plugin.DeployRequest{LogLevel: "DEBUG", CertificatePEM: cert, PrivateKeyPEM: key}
			access := map[string]any{"allowInsecureConnections": "true", "serverUrl": server.URL, "apiKey": "panel-fixture-secret", "apiVersion": "v2"}
			raw, _ := json.Marshal(access)
			req.AccessConfigJSON = string(raw)
			configs := []string{`{}`}
			switch provider {
			case "1panel":
				configs = []string{`{"deployTarget":"website","websiteMatchPattern":"specified","websiteId":"42","nodeName":"fixture-node"}`, `{"deployTarget":"certificate","certificateId":73,"nodeName":"fixture-node"}`, `{"deployTarget":"website","websiteMatchPattern":"certsan","nodeName":"fixture-node"}`, `{"deployTarget":"website","websiteId":42}`}
			case "1panel-console", "baotapanel-console":
				configs = []string{`{"autoRestart":true}`}
			case "baotapanel":
				configs = []string{`{"siteType":"any","siteNames":"app.example.com"}`, `{"siteType":"php","siteNames":"app.example.com"}`, `{"siteType":"proxy","siteNames":"app.example.com"}`}
			case "baotapanelgo":
				configs = []string{`{"siteType":"php","siteNames":"app.example.com"}`, `{"siteType":"aspx","siteNames":"app.example.com"}`}
			case "baotawaf":
				configs = []string{`{"siteNames":"app.example.com","sitePort":8443}`}
			}
			var logs bytes.Buffer
			run := func(ctx context.Context, request *plugin.DeployRequest) (*plugin.DeployResult, error) {
				return manager.Deploy(ctx, dp, request, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			}
			if os.Getenv("CERTIMATE_B1_CORE_BINARY") != "" {
				run = newB1WorkflowDeployer(t, dir, provider)
			}
			deploy := func(t *testing.T, request plugin.DeployRequest, wantError bool) {
				t.Helper()
				logs.Reset()
				res, err := run(ctx, &request)
				if wantError {
					if err == nil {
						t.Fatal("injected failure succeeded")
					}
				} else if err != nil {
					t.Fatal(err)
				} else if res == nil {
					t.Fatal("missing deployment result")
				}
				for _, secret := range []string{"panel-fixture-secret", request.PrivateKeyPEM, "BEGIN PRIVATE KEY"} {
					if strings.Contains(logs.String(), secret) {
						t.Fatal("deployment logs contain secret")
					}
					if os.Getenv("CERTIMATE_B1_BASELINE") != "1" && err != nil && strings.Contains(err.Error(), secret) {
						t.Fatal("deployment error contains secret")
					}
				}
			}
			for i, extended := range configs {
				t.Run(fmt.Sprintf("config_%d", i), func(t *testing.T) {
					req.ExtendedConfigJSON = extended
					f.reset("normal", req)
					deploy(t, req, false)
					f.assertBound(t, req)
					deploy(t, req, false)
					f.assertBound(t, req)
					if provider == "1panel" && i == 0 {
						f.mu.Lock()
						n := f.uploads
						f.mu.Unlock()
						if n != 1 {
							t.Fatalf("same certificate uploaded %d times", n)
						}
					}
					renew := req
					renew.CertificatePEM, renew.PrivateKeyPEM, _ = makeCertificate(t, []string{"app.example.com"}, false)
					f.expect(renew)
					deploy(t, renew, false)
					f.assertBound(t, renew)
					for _, mode := range []string{"auth-failure", "bind-failure", "api-failure", "malformed-response"} {
						f.reset(mode, req)
						deploy(t, req, true)
						f.assertUnbound(t)
						f.reset("normal", req)
						deploy(t, req, false)
						f.assertBound(t, req)
					}
				})
			}
			if provider == "baotapanel" {
				t.Run("v2_fallback", func(t *testing.T) {
					req.ExtendedConfigJSON = configs[0]
					f.reset("v1-unavailable", req)
					deploy(t, req, false)
					f.assertBound(t, req)
				})
			}
			if family == "1panel" {
				t.Run("v1_compatibility", func(t *testing.T) {
					access["apiVersion"] = "v1"
					data, _ := json.Marshal(access)
					req.AccessConfigJSON = string(data)
					req.ExtendedConfigJSON = configs[0]
					f.reset("normal", req)
					deploy(t, req, false)
					f.assertBound(t, req)
				})
			}
			t.Run("cancel_then_recover", func(t *testing.T) {
				f.reset("wait", req)
				cancelCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				result := make(chan error, 1)
				go func() { _, err := manager.Deploy(cancelCtx, dp, &req, slog.New(slog.DiscardHandler)); result <- err }()
				select {
				case <-f.started:
					cancel()
				case <-cancelCtx.Done():
					t.Fatal("request never reached fixture")
				}
				select {
				case err := <-result:
					if err == nil {
						t.Fatal("cancelled deploy succeeded")
					}
				case <-time.After(5 * time.Second):
					t.Fatal("plugin call did not cancel")
				}
				select {
				case <-f.cancelled:
				case <-time.After(5 * time.Second):
					t.Fatal("HTTP request did not cancel")
				}
				f.assertUnbound(t)
				f.reset("normal", req)
				deploy(t, req, false)
				f.assertBound(t, req)
			})
		})
	}
}

func (f *b2PanelFixture) reset(mode string, req plugin.DeployRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
	f.cert = req.CertificatePEM
	f.key = req.PrivateKeyPEM
	f.uploadedCert = ""
	f.uploadedKey = ""
	f.boundCert = ""
	f.boundKey = ""
	f.uploads = 0
	f.bindings = 0
	f.started = make(chan struct{}, 1)
	f.cancelled = make(chan struct{}, 1)
}

func (f *b2PanelFixture) expect(req plugin.DeployRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cert = req.CertificatePEM
	f.key = req.PrivateKeyPEM
}

func (f *b2PanelFixture) assertBound(t *testing.T, req plugin.DeployRequest) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.boundCert != req.CertificatePEM || f.boundKey != req.PrivateKeyPEM || f.bindings == 0 {
		t.Fatal("target certificate or private key does not match deployment")
	}
}

func (f *b2PanelFixture) assertUnbound(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.bindings != 0 {
		t.Fatal("failed deployment updated target")
	}
}

func (f *b2PanelFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	mode, started, cancelled, privateKey := f.mode, f.started, f.cancelled, f.key
	f.mu.Unlock()
	if mode == "wait" {
		_, _ = io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		select {
		case <-r.Context().Done():
			cancelled <- struct{}{}
		case <-time.After(15 * time.Second):
		}
		return
	}
	if mode == "auth-failure" {
		http.Error(w, "authentication failed: panel-fixture-secret\n"+privateKey, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if mode == "api-failure" {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": false, "code": 500, "msg": "panel-fixture-secret\n" + privateKey, "message": "panel-fixture-secret\n" + privateKey})
		return
	}
	if mode == "malformed-response" {
		encoded, _ := json.Marshal(privateKey)
		_, _ = fmt.Fprintf(w, "{\"echo\":%s,\"message\":\"panel-fixture-secret\"", encoded)
		return
	}
	body := map[string]any{}
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid JSON", 400)
				return
			}
		}
	} else {
		_ = r.ParseForm()
		for k, v := range r.PostForm {
			if len(v) > 0 {
				body[k] = v[0]
			}
		}
	}
	text := func(key string) string { s, _ := body[key].(string); return s }
	md5sum := func(s string) string { sum := md5.Sum([]byte(s)); return hex.EncodeToString(sum[:]) }
	if strings.HasPrefix(r.URL.Path, "/api/v") {
		timestamp := r.Header.Get("1Panel-Timestamp")
		if timestamp == "" || r.Header.Get("1Panel-Token") != md5sum("1panelpanel-fixture-secret"+timestamp) {
			http.Error(w, "bad 1panel signature", 401)
			return
		}
	} else if strings.HasPrefix(r.URL.Path, "/api/") {
		timestamp := r.Header.Get("waf_request_time")
		if timestamp == "" || r.Header.Get("waf_request_token") != md5sum(timestamp+md5sum("panel-fixture-secret")) {
			http.Error(w, "bad waf signature", 401)
			return
		}
	} else if text("request_time") == "" || text("request_token") != md5sum(text("request_time")+md5sum("panel-fixture-secret")) {
		http.Error(w, "bad panel signature", 401)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	respond := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	success := map[string]any{"code": 200, "status": true}
	if strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/api/v") {
		success["code"] = 0
	}
	bind := func(cert, key string) bool {
		if mode == "bind-failure" {
			http.Error(w, "binding failed: panel-fixture-secret\n"+f.key, 500)
			return false
		}
		if cert != f.cert || key != f.key {
			http.Error(w, "certificate payload mismatch", 400)
			return false
		}
		f.boundCert = cert
		f.boundKey = key
		f.bindings++
		return true
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/websites/ssl/search"):
		items := []any{}
		if f.uploadedCert != "" {
			items = append(items, map[string]any{"id": 73, "pem": f.uploadedCert, "privateKey": f.uploadedKey})
		}
		success["data"] = map[string]any{"items": items, "total": len(items)}
	case strings.HasSuffix(r.URL.Path, "/websites/search"):
		success["data"] = map[string]any{"items": []any{map[string]any{"id": 42, "primaryDomain": "app.example.com"}, map[string]any{"id": 99, "primaryDomain": "unrelated.example.org"}}, "total": 2}
	case strings.HasSuffix(r.URL.Path, "/websites/42"):
		success["data"] = map[string]any{"id": 42, "domains": []any{map[string]any{"domain": "app.example.com", "ssl": true}}}
	case strings.HasSuffix(r.URL.Path, "/websites/ssl/upload"):
		if text("certificate") != f.cert || text("privateKey") != f.key {
			http.Error(w, "upload mismatch", 400)
			return
		}
		f.uploadedCert = text("certificate")
		f.uploadedKey = text("privateKey")
		f.uploads++
		if body["sslID"] == float64(73) {
			if !bind(f.uploadedCert, f.uploadedKey) {
				return
			}
		}
	case strings.HasSuffix(r.URL.Path, "/websites/42/https"):
		if r.Method == http.MethodGet {
			success["data"] = map[string]any{"enable": false, "websiteSSLId": 73, "httpConfig": "HTTPSOnly", "hsts": true, "SSLProtocol": []string{"TLSv1.2"}}
		} else {
			if body["websiteSSLId"] != float64(73) || text("httpConfig") != "HTTPSOnly" || body["hsts"] != true {
				http.Error(w, "binding options changed", 400)
				return
			}
			if !bind(f.uploadedCert, f.uploadedKey) {
				return
			}
		}
	case strings.HasSuffix(r.URL.Path, "/settings/ssl/update"):
		if text("autoRestart") != "true" {
			http.Error(w, "restart flag changed", 400)
			return
		}
		if !bind(text("cert"), text("key")) {
			return
		}
	case r.URL.Path == "/ssl/cert/save_cert":
		if mode == "v1-unavailable" {
			http.Error(w, "v1 unavailable", 404)
			return
		}
		f.uploadedCert = text("csr")
		f.uploadedKey = text("key")
		f.uploads++
		success["ssl_hash"] = "fixture-hash"
	case r.URL.Path == "/ssl" && r.URL.Query().Get("action") == "SetBatchCertToSite":
		var batch []struct {
			SiteName string `json:"siteName"`
			SSLHash  string `json:"ssl_hash"`
		}
		if json.Unmarshal([]byte(text("BatchInfo")), &batch) != nil || len(batch) != 1 || batch[0].SiteName != "app.example.com" || batch[0].SSLHash != "fixture-hash" {
			http.Error(w, "wrong site binding", 400)
			return
		}
		if !bind(f.uploadedCert, f.uploadedKey) {
			return
		}
	case r.URL.Path == "/v2/ssl_domain":
		success["status"] = 0
		if mode != "v1-unavailable" {
			http.Error(w, "fallback unavailable in this scenario", 500)
			return
		}
		switch r.URL.Query().Get("action") {
		case "upload_cert":
			if text("cert") != f.cert || text("key") != f.key {
				http.Error(w, "upload mismatch", 400)
				return
			}
			f.uploadedCert = text("cert")
			f.uploadedKey = text("key")
			f.uploads++
			success["message"] = map[string]any{"hash": "fixture-hash"}
		case "cert_deploy_sites":
			var domains []string
			if json.Unmarshal([]byte(text("domains")), &domains) != nil || len(domains) != 1 || domains[0] != "app.example.com" || text("hash") != "fixture-hash" || text("append") != "1" {
				http.Error(w, "wrong fallback binding", 400)
				return
			}
			if !bind(f.uploadedCert, f.uploadedKey) {
				return
			}
		default:
			http.Error(w, "unexpected fallback action", 400)
			return
		}
	case r.URL.Path == "/site" || r.URL.Path == "/mod/proxy/com/set_ssl":
		site := text("siteName")
		if site == "" {
			site = text("site_name")
		}
		if site != "app.example.com" {
			http.Error(w, "wrong site", 400)
			return
		}
		if !bind(text("csr"), text("key")) {
			return
		}
	case r.URL.Path == "/config":
		if !bind(text("certPem"), text("privateKey")) {
			return
		}
	case r.URL.Path == "/system":
		if text("name") != "nginx" || text("type") != "restart" {
			http.Error(w, "wrong service", 400)
			return
		}
	case r.URL.Path == "/panel/get_config":
		success["site"] = map[string]any{"webserver": "nginx"}
	case r.URL.Path == "/site/get_project_list" || r.URL.Path == "/datalist/get_data_list":
		success["data"] = []any{map[string]any{"id": 42, "name": "app.example.com"}}
	case r.URL.Path == "/site/set_site_ssl":
		if text("siteid") != "42" || text("status") != "true" {
			http.Error(w, "wrong site", 400)
			return
		}
		if !bind(text("cert"), text("key")) {
			return
		}
	case r.URL.Path == "/config/set_panel_ssl":
		if text("ssl_status") != "1" {
			http.Error(w, "SSL not enabled", 400)
			return
		}
		if !bind(text("ssl_pem"), text("ssl_key")) {
			return
		}
	case r.URL.Path == "/api/wafmastersite/get_site_list":
		success["res"] = map[string]any{"list": []any{map[string]any{"site_id": "site42", "site_name": "app.example.com"}}, "total": 1}
	case r.URL.Path == "/api/wafmastersite/modify_site":
		if text("site_id") != "site42" || text("types") != "openCert" {
			http.Error(w, "wrong site", 400)
			return
		}
		server, _ := body["server"].(map[string]any)
		ssl, _ := server["ssl"].(map[string]any)
		ports, _ := server["listen_ssl_port"].([]any)
		if len(ports) != 1 || ports[0] != "8443" || ssl["is_ssl"] != float64(1) {
			http.Error(w, "wrong SSL options", 400)
			return
		}
		cert, _ := ssl["full_chain"].(string)
		key, _ := ssl["private_key"].(string)
		if !bind(cert, key) {
			return
		}
	case r.URL.Path == "/api/config/set_cert":
		if !bind(text("certContent"), text("keyContent")) {
			return
		}
	default:
		http.Error(w, "unexpected endpoint "+r.URL.Path, 404)
		return
	}
	respond(success)
}

func buildB2Panels(t *testing.T, dir string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range b2PanelProviders {
		name := strings.Replace(provider, "1panel", "panel1", 1)
		out := filepath.Join(dir, name)
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("go", "build", "-o", filepath.Join(out, name), "./"+name)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, output)
		}
		for _, file := range []string{"manifest.json", map[bool]string{true: "1panel.svg", false: "baota.svg"}[strings.HasPrefix(provider, "1panel")]} {
			data, err := os.ReadFile(filepath.Join(root, name, file))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, file), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
