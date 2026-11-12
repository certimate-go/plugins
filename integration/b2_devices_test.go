package integration

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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
	"software.sslmate.com/src/go-pkcs12"
)

var b2DeviceProviders = []string{"dokploy", "huaweiibmc", "proxmoxbs", "proxmoxve", "ratpanel", "ratpanel-console", "synologydsm"}

type b2DeviceFixture struct {
	t                          *testing.T
	mu                         sync.Mutex
	provider                   string
	mode                       string
	cert, key                  string
	storedCert, storedKey      string
	writes, restarts, bindings int
	waitEntered                chan struct{}
	sites                      map[string]bool
}

func TestB2DevicesDeploymentBatch(t *testing.T) {
	dir := os.Getenv("CERTIMATE_B2_DEVICES_BINARIES")
	if dir == "" {
		dir = t.TempDir()
		root, err := filepath.Abs("..")
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range b2DeviceProviders {
			dest := filepath.Join(dir, name)
			if err := os.MkdirAll(dest, 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("go", "build", "-p", "2", "-o", filepath.Join(dest, name), "./"+name)
			cmd.Dir = root
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build %s: %v\n%s", name, err, out)
			}
			data, err := os.ReadFile(filepath.Join(root, name, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			m, err := plugin.ParseManifest(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{"manifest.json", m.Icon} {
				data, err := os.ReadFile(filepath.Join(root, name, file))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dest, file), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := plugin.PluginConfig{PluginDir: dir, StartTimeout: 15 * time.Second, DeployTimeout: 15 * time.Second}
	discovered, failures := plugin.Discover(ctx, cfg)
	if len(failures) != 0 || len(discovered) != 7 {
		t.Fatalf("discovery=%d failures=%v", len(discovered), failures)
	}
	manager := plugin.NewManager(cfg, slog.New(slog.DiscardHandler))
	for _, dp := range discovered {
		t.Run(dp.Manifest.ProviderType, func(t *testing.T) {
			provider := dp.Manifest.ProviderType
			meta, schema, err := manager.Bootstrap(ctx, dp)
			if err != nil {
				t.Fatal(err)
			}
			validateSchema(t, schema, provider, strings.TrimSuffix(provider, "-console"), meta)
			if meta.ProviderType != provider || meta.DeployCategory != dp.Manifest.DeployCategory || meta.ProtocolVersion != plugin.ProtocolVersion || meta.AccessProviderType != dp.Manifest.AccessProviderType || meta.DeployDisplayNameKey != dp.Manifest.DisplayNameKey {
				t.Fatal("manifest and metadata differ")
			}
			if info, err := os.Stat(filepath.Join(dp.Dir, dp.Manifest.Icon)); err != nil || info.Size() == 0 {
				t.Fatal("plugin icon missing")
			}
			fixture := &b2DeviceFixture{t: t, provider: provider, waitEntered: make(chan struct{}, 1)}
			server := httptest.NewServer(http.HandlerFunc(fixture.serve))
			defer server.Close()
			access := map[string]any{"serverUrl": server.URL, "host": server.URL, "username": "fixture-user", "password": "fixture-password", "apiKey": "fixture-api-key", "apiToken": "fixture-token", "apiTokenSecret": "fixture-secret", "accessTokenId": 1, "accessToken": "fixture-access"}
			extended := map[string]any{}
			switch provider {
			case "huaweiibmc", "proxmoxbs", "proxmoxve":
				extended = map[string]any{"autoRestart": true, "nodeName": "node-a"}
			case "ratpanel":
				extended = map[string]any{"deployTarget": "website", "siteNames": "site-a;site-b"}
			case "synologydsm":
				extended = map[string]any{"certificateIdOrDesc": "existing", "isDefault": true}
			}
			encode := func(v any) string {
				b, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				return string(b)
			}
			req := plugin.DeployRequest{AccessConfigJSON: encode(access), ExtendedConfigJSON: encode(extended), LogLevel: "DEBUG"}
			req.CertificatePEM, req.PrivateKeyPEM, _ = makeB2DeviceCertificate(t)
			deploy := func(ctx context.Context, req *plugin.DeployRequest) (*plugin.DeployResult, error) {
				var logs bytes.Buffer
				result, err := manager.Deploy(ctx, dp, req, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
				for _, secret := range []string{"fixture-password", "fixture-secret", "fixture-access", strings.Split(req.PrivateKeyPEM, "\n")[1]} {
					if strings.Contains(logs.String(), secret) || err != nil && strings.Contains(err.Error(), secret) {
						t.Error("secret leaked in deployment logs or errors")
					}
				}
				return result, err
			}
			if os.Getenv("CERTIMATE_B1_CORE_BINARY") != "" {
				deploy = newB1WorkflowDeployer(t, dir, provider)
			}
			reset := func(mode string) {
				fixture.mu.Lock()
				fixture.mode = mode
				fixture.cert = req.CertificatePEM
				fixture.key = req.PrivateKeyPEM
				fixture.writes = 0
				fixture.restarts = 0
				fixture.bindings = 0
				fixture.sites = map[string]bool{}
				fixture.mu.Unlock()
			}
			assertStored := func(t *testing.T) {
				t.Helper()
				fixture.mu.Lock()
				defer fixture.mu.Unlock()
				if fixture.storedCert != req.CertificatePEM || fixture.storedKey != req.PrivateKeyPEM {
					t.Fatal("target certificate/key differ from deployment input")
				}
				if provider == "ratpanel" && strings.Contains(req.ExtendedConfigJSON, `"website"`) && (!fixture.sites["site-a"] || !fixture.sites["site-b"]) {
					t.Fatal("not all requested websites received the certificate")
				}
				if provider == "huaweiibmc" && fixture.restarts == 0 {
					t.Fatal("requested iBMC restart missing")
				}
				if provider == "synologydsm" && fixture.bindings == 0 {
					t.Fatal("DSM service was not bound to default certificate")
				}
				if fixture.writes == 0 && provider != "dokploy" {
					t.Fatal("no target update")
				}
			}
			run := func(t *testing.T, wantError bool) {
				t.Helper()
				result, err := deploy(ctx, &req)
				if wantError {
					if err == nil {
						t.Fatal("expected deployment failure")
					}
				} else if err != nil || result == nil {
					t.Fatalf("deployment failed: %v", err)
				}
			}
			t.Run("deploy_repeat_renew", func(t *testing.T) {
				reset("")
				run(t, false)
				assertStored(t)
				writes := fixture.writes
				run(t, false)
				assertStored(t)
				if provider == "dokploy" && fixture.writes != writes {
					t.Fatal("duplicate certificate uploaded")
				}
				req.CertificatePEM, req.PrivateKeyPEM, _ = makeB2DeviceCertificate(t)
				reset("")
				run(t, false)
				assertStored(t)
			})
			for _, mode := range []string{"auth", "write"} {
				t.Run(mode+"_failure_recovery", func(t *testing.T) {
					req.CertificatePEM, req.PrivateKeyPEM, _ = makeB2DeviceCertificate(t)
					reset(mode)
					run(t, true)
					fixture.mu.Lock()
					n := fixture.writes
					fixture.mu.Unlock()
					if n != 0 {
						t.Fatal("failure mutated target")
					}
					reset("")
					run(t, false)
					assertStored(t)
				})
			}
			if provider == "ratpanel" {
				t.Run("certificate_target_numeric_compatibility", func(t *testing.T) {
					for _, id := range []any{"42", 42} {
						req.ExtendedConfigJSON = encode(map[string]any{"deployTarget": "certificate", "certificateId": id})
						access["accessTokenId"] = 1
						if _, stringID := id.(string); stringID {
							access["accessTokenId"] = "1"
						}
						req.AccessConfigJSON = encode(access)
						reset("")
						run(t, false)
						assertStored(t)
					}
				})
			}
			t.Run("cancellation_recovery", func(t *testing.T) {
				reset("wait")
				short, stop := context.WithCancel(ctx)
				defer stop()
				done := make(chan error, 1)
				go func() { _, err := manager.Deploy(short, dp, &req, slog.New(slog.DiscardHandler)); done <- err }()
				select {
				case <-fixture.waitEntered:
				case <-time.After(10 * time.Second):
					t.Fatal("deployment never reached target before cancellation")
				}
				stop()
				select {
				case err := <-done:
					if err == nil {
						t.Fatal("canceled deployment succeeded")
					}
				case <-time.After(5 * time.Second):
					t.Fatal("cancellation did not propagate")
				}

				reset("")
				run(t, false)
				assertStored(t)
			})
		})
	}
}

func (f *b2DeviceFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	mode := f.mode
	f.mu.Unlock()
	if mode == "wait" {
		select {
		case f.waitEntered <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
			http.Error(w, "timeout", 504)
			return
		}
	}
	if mode == "auth" {
		http.Error(w, "unauthorized", 401)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	reply := func(v any) {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			f.t.Error(err)
		}
	}
	failWrite := func() bool {
		if mode == "write" {
			http.Error(w, "fixture rejected write fixture-password", 503)
			return true
		}
		return false
	}
	save := func(cert, key string) {
		if cert != f.cert || key != f.key {
			f.t.Error("target received incorrect certificate or private key")
		}
		f.storedCert = cert
		f.storedKey = key
		f.writes++
	}
	body, _ := io.ReadAll(r.Body)
	decode := func() map[string]any {
		var v map[string]any
		if err := json.Unmarshal(body, &v); err != nil {
			f.t.Error(err)
		}
		return v
	}
	switch f.provider {
	case "dokploy":
		if r.Header.Get("X-Api-Key") != "fixture-api-key" {
			f.t.Error("wrong Dokploy credential")
		}
		switch r.URL.Path {
		case "/api/certificates.all":
			if f.storedCert == "" {
				reply([]any{})
			} else {
				reply([]any{map[string]any{"certificateId": "cert-1", "name": "existing", "certificateData": f.storedCert, "privateKey": f.storedKey}})
			}
		case "/api/user.get":
			reply(map[string]any{"organizationId": "org-a"})
		case "/api/certificates.create":
			if failWrite() {
				return
			}
			v := decode()
			if v["organizationId"] != "org-a" {
				f.t.Error("wrong organization")
			}
			save(fmt.Sprint(v["certificateData"]), fmt.Sprint(v["privateKey"]))
			reply(map[string]any{"certificateId": "cert-1", "name": "existing"})
		default:
			f.t.Errorf("unexpected Dokploy path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	case "proxmoxbs", "proxmoxve":
		sep, prefix := ":", "PBS"
		if f.provider == "proxmoxve" {
			sep, prefix = "=", "PVE"
		}
		if r.Header.Get("Authorization") != prefix+"APIToken=fixture-token"+sep+"fixture-secret" {
			f.t.Error("wrong Proxmox credential")
		}
		if r.URL.Path != "/api2/json/nodes/node-a/certificates/custom" || r.Method != "POST" {
			f.t.Error("wrong Proxmox target")
		}
		if failWrite() {
			return
		}
		v := decode()
		if v["restart"] != true || v["force"] != true {
			f.t.Error("force/restart lost")
		}
		save(fmt.Sprint(v["certificates"]), fmt.Sprint(v["key"]))
		f.restarts++
		if f.provider == "proxmoxbs" {
			reply(map[string]any{"data": []any{}})
		} else {
			reply(map[string]any{"data": map[string]any{}})
		}
	case "ratpanel", "ratpanel-console":
		sum := func(v string) string { x := sha256.Sum256([]byte(v)); return hex.EncodeToString(x[:]) }
		canonical := fmt.Sprintf("%s\n%s\n%s\n%s", r.Method, r.URL.Path, r.URL.Query().Encode(), sum(string(body)))
		mac := hmac.New(sha256.New, []byte("fixture-access"))
		mac.Write([]byte("HMAC-SHA256\n" + r.Header.Get("X-Timestamp") + "\n" + sum(canonical)))
		if r.Header.Get("Authorization") != "HMAC-SHA256 Credential=1, Signature="+hex.EncodeToString(mac.Sum(nil)) {
			f.t.Error("invalid RatPanel signature")
		}
		if failWrite() {
			return
		}
		v := decode()
		switch r.URL.Path {
		case "/api/website/cert":
			if f.provider == "ratpanel-console" {
				f.t.Error("console plugin selected website target")
			}
			f.sites[fmt.Sprint(v["name"])] = true
			if v["name"] != "site-a" && v["name"] != "site-b" {
				f.t.Error("wrong site")
			}
		case "/api/cert/cert/42":
			if v["id"] != float64(42) {
				f.t.Error("wrong cert ID")
			}
		case "/api/setting/cert":
			if f.provider != "ratpanel-console" {
				f.t.Error("website plugin selected console target")
			}
		default:
			f.t.Error("wrong RatPanel target")
		}
		save(fmt.Sprint(v["cert"]), fmt.Sprint(v["key"]))
		reply(map[string]any{"msg": "success"})
	case "huaweiibmc":
		switch {
		case r.URL.Path == "/redfish/v1/SessionService/Sessions":
			v := decode()
			if v["UserName"] != "fixture-user" || v["Password"] != "fixture-password" {
				f.t.Error("wrong iBMC credential")
			}
			w.Header().Set("X-Auth-Token", "fixture-session")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			reply(map[string]any{})
		case r.Method == "DELETE":
			reply(map[string]any{})
		case r.URL.Path == "/redfish/v1/Managers":
			reply(map[string]any{"Members": []any{map[string]any{"@odata.id": "/redfish/v1/Managers/1"}}})
		case strings.HasSuffix(r.URL.Path, "HttpsCert.ImportCustomCertificate"):
			if failWrite() {
				return
			}
			v := decode()
			pfx, err := base64.StdEncoding.DecodeString(fmt.Sprint(v["Certificate"]))
			if err != nil {
				f.t.Error(err)
				return
			}
			key, cert, chain, err := pkcs12.DecodeChain(pfx, fmt.Sprint(v["Password"]))
			if err != nil {
				f.t.Error(err)
				return
			}
			expected, err := x509.ParseCertificate(mustPEMDER(f.cert))
			if err != nil {
				f.t.Error(err)
				return
			}
			if !bytes.Equal(cert.Raw, expected.Raw) {
				f.t.Error("iBMC PFX certificate mismatch")
			}
			keyDER, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil || !bytes.Equal(keyDER, mustPEMDER(f.key)) {
				f.t.Error("iBMC PFX key mismatch")
			}
			var bundle strings.Builder
			bundle.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
			for _, ca := range chain {
				bundle.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}))
			}
			save(bundle.String(), f.key)
			reply(map[string]any{})
		case strings.HasSuffix(r.URL.Path, "Manager.Reset"):
			if decode()["ResetType"] != "ForceRestart" {
				f.t.Error("wrong reset type")
			}
			f.restarts++
			reply(map[string]any{})
		default:
			f.t.Errorf("unexpected iBMC path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	case "synologydsm":
		r.Body = io.NopCloser(bytes.NewReader(body))
		_ = r.ParseMultipartForm(1 << 20)
		_ = r.ParseForm()
		switch r.Form.Get("method") {
		case "query":
			reply(map[string]any{"success": true, "data": map[string]any{"SYNO.API.Auth": map[string]any{"path": "auth.cgi", "maxVersion": 7}}})
		case "login":
			if r.Form.Get("account") != "fixture-user" || r.Form.Get("passwd") != "fixture-password" {
				f.t.Error("wrong DSM credential")
			}
			reply(map[string]any{"success": true, "data": map[string]any{"sid": "fixture-sid", "synotoken": "fixture-syno"}})
		case "logout":
			reply(map[string]any{"success": true})
		case "list":
			reply(map[string]any{"success": true, "data": map[string]any{"certificates": []any{map[string]any{"id": "existing", "desc": "existing", "is_default": true, "services": []any{}}, map[string]any{"id": "old", "services": []any{map[string]any{"service": "DSM", "subscriber": "system"}}}}}})
		case "import":
			if failWrite() {
				return
			}
			read := func(name string) string {
				file, _, err := r.FormFile(name)
				if err != nil {
					f.t.Error(err)
					return ""
				}
				defer file.Close()
				b, _ := io.ReadAll(file)
				return string(b)
			}
			if r.FormValue("id") != "existing" || r.FormValue("as_default") != "true" {
				f.t.Error("DSM certificate target/default lost")
			}
			save(read("cert")+read("inter_cert"), read("key"))
			reply(map[string]any{"success": true, "data": map[string]any{}})
		case "set":
			if !strings.Contains(r.Form.Get("settings"), `"id":"existing"`) {
				f.t.Error("DSM service binding lost")
			}
			f.bindings++
			reply(map[string]any{"success": true})
		default:
			f.t.Errorf("unexpected DSM request %s %s", r.URL.Path, r.Form.Get("method"))
			http.NotFound(w, r)
		}
	}
}

func mustPEMDER(value string) []byte {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil
	}
	return block.Bytes
}

func makeB2DeviceCertificate(t *testing.T) (string, string, *x509.Certificate) {
	t.Helper()
	caPEM, caKeyPEM, ca := makeCertificate(t, []string{"ca.fixture.example.test"}, true)
	_, keyPEM, leaf := makeCertificate(t, []string{"fixture.example.test"}, false)
	caKey, err := x509.ParsePKCS8PrivateKey(mustPEMDER(caKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.ParsePKCS8PrivateKey(mustPEMDER(keyPEM))
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, key.(crypto.Signer).Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) + caPEM, keyPEM, leaf
}
