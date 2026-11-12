package integration

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

var b1AppsProviders = []string{"mohua-mvh", "unicloud-webhost", "flyio"}

type b1AppsFixture struct {
	t               *testing.T
	mu              sync.Mutex
	mode            string
	calls, updates  int
	certPEM, keyPEM string
	rsaKey          *rsa.PrivateKey
	requestStarted  chan struct{}
	requestCanceled chan struct{}
}

func TestB1AppsDeploymentBatch(t *testing.T) {
	if runtime.GOOS != "linux" {
		runB1AppsBatchInLinux(t)
		return
	}
	dir := os.Getenv("CERTIMATE_B1_APPS_E2E_BINARIES")
	if dir == "" {
		dir = t.TempDir()
		buildB1AppsBatch(t, dir, runtime.GOARCH)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := plugin.PluginConfig{PluginDir: dir, StartTimeout: 15 * time.Second, DeployTimeout: 15 * time.Second}
	discovered, failures := plugin.Discover(ctx, cfg)
	if len(failures) != 0 || len(discovered) != len(b1AppsProviders) {
		t.Fatalf("discovery count=%d failures=%v", len(discovered), failures)
	}
	byName := map[string]*plugin.DiscoveredPlugin{}
	for _, dp := range discovered {
		byName[dp.Manifest.ProviderType] = dp
	}
	manager := plugin.NewManager(cfg, slog.New(slog.DiscardHandler))
	fixture := newB1AppsFixture(t)
	for _, provider := range b1AppsProviders {
		t.Run(provider, func(t *testing.T) {
			dp := byName[provider]
			meta, schema, err := manager.Bootstrap(ctx, dp)
			if err != nil {
				t.Fatal(err)
			}
			access := strings.Split(provider, "-")[0]
			if meta.ProviderType != provider || meta.AccessProviderType != access || meta.DeployCategory != "website" || meta.ProtocolVersion != plugin.ProtocolVersion {
				t.Fatalf("wrong metadata: %+v", meta)
			}
			if dp.Manifest.DisplayNameKey != meta.DeployDisplayNameKey || dp.Manifest.AccessProviderType != access || dp.Manifest.DeployCategory != meta.DeployCategory || dp.Manifest.ProtocolVersion != meta.ProtocolVersion {
				t.Fatal("manifest differs from metadata")
			}
			if info, e := os.Stat(filepath.Join(dp.Dir, dp.Manifest.Icon)); e != nil || info.Size() == 0 {
				t.Fatalf("missing icon: %v", e)
			}
			validateSchema(t, schema, provider, access, meta)
			req := plugin.DeployRequest{CertificatePEM: fixture.certPEM, PrivateKeyPEM: fixture.keyPEM, LogLevel: "DEBUG"}
			switch provider {
			case "mohua-mvh":
				req.AccessConfigJSON = `{"username":"fixture-user","apiPassword":"fixture-password"}`
				req.ExtendedConfigJSON = `{"hostId":"42","domainId":"73"}`
			case "unicloud-webhost":
				req.AccessConfigJSON = `{"username":"fixture-user","password":"fixture-password"}`
				req.ExtendedConfigJSON = `{"spaceProvider":"tencent","spaceId":"fixture-space","domain":"app.example.com"}`
			case "flyio":
				req.AccessConfigJSON = `{"apiToken":"fixture-token"}`
				req.ExtendedConfigJSON = `{"appName":"fixture-app","domain":"*.example.com"}`
			}
			var workflowDeploy func(context.Context, *plugin.DeployRequest) (*plugin.DeployResult, error)
			if os.Getenv("CERTIMATE_B1_CORE_BINARY") != "" {
				workflowDeploy = newB1WorkflowDeployer(t, dir, provider)
			}
			deploy := func(t *testing.T, request plugin.DeployRequest, wantErr string) {
				t.Helper()
				var logs bytes.Buffer
				var result *plugin.DeployResult
				var e error
				if workflowDeploy != nil && json.Valid([]byte(request.AccessConfigJSON)) && json.Valid([]byte(request.ExtendedConfigJSON)) {
					result, e = workflowDeploy(ctx, &request)
				} else {
					result, e = manager.Deploy(ctx, dp, &request, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
				}
				for _, secret := range []string{"fixture-token", "fixture-password", "fixture-jwt", "fixture-console", strings.Split(request.PrivateKeyPEM, "\n")[1]} {
					if strings.Contains(logs.String(), secret) || e != nil && strings.Contains(e.Error(), secret) {
						t.Fatal("secret leaked in host logs or errors")
					}
				}
				if wantErr != "" {
					if e == nil || !strings.Contains(e.Error(), wantErr) {
						t.Fatalf("expected %q, got %v", wantErr, e)
					}
					return
				}
				if e != nil || result == nil {
					t.Fatalf("deploy failed: %v", e)
				}
			}
			t.Run("invalid_json", func(t *testing.T) {
				fixture.reset("normal", req)
				bad := req
				bad.AccessConfigJSON = "{"
				deploy(t, bad, "invalid access config")
				bad = req
				bad.ExtendedConfigJSON = "{"
				deploy(t, bad, "invalid deploy config")
				fixture.assertCounts(t, 0, 0)
			})
			t.Run("deploy_repeat_renew", func(t *testing.T) {
				fixture.reset("normal", req)
				deploy(t, req, "")
				deploy(t, req, "")
				fixture.assertUpdates(t, 2)
				renew := req
				renew.CertificatePEM, renew.PrivateKeyPEM, _ = makeCertificate(t, []string{"app.example.com", "*.example.com"}, false)
				fixture.reset("normal", renew)
				deploy(t, renew, "")
				fixture.assertUpdates(t, 1)
			})
			for _, mode := range []string{"auth-failure", "update-failure"} {
				t.Run(mode+"_recovery", func(t *testing.T) {
					fixture.reset(mode, req)
					deploy(t, req, "failed")
					fixture.assertUpdates(t, 0)
					fixture.reset("normal", req)
					deploy(t, req, "")
					fixture.assertUpdates(t, 1)
				})
			}
			if provider == "mohua-mvh" {
				t.Run("numeric_saved_domain_id", func(t *testing.T) {
					fixture.reset("normal", req)
					numeric := req
					numeric.ExtendedConfigJSON = `{"hostId":"42","domainId":73}`
					deploy(t, numeric, "")
					fixture.assertUpdates(t, 1)
					fixture.reset("normal", req)
					numeric.ExtendedConfigJSON = `{"hostId":42,"domainId":73}`
					deploy(t, numeric, "config `hostId` is required")
					fixture.assertCounts(t, 0, 0)
				})
			}
			t.Run("cancel_then_recover", func(t *testing.T) {
				fixture.reset("wait", req)
				cancelCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				completed := make(chan error, 1)
				go func() {
					_, err := manager.Deploy(cancelCtx, dp, &req, slog.New(slog.DiscardHandler))
					completed <- err
				}()
				select {
				case <-fixture.requestStarted:
					cancel()
				case <-cancelCtx.Done():
					t.Fatal("cancellation request never reached API")
				}
				select {
				case err := <-completed:
					if err == nil {
						t.Fatal("cancelled deployment succeeded")
					}
				case <-time.After(5 * time.Second):
					t.Fatal("cancelled plugin call did not return")
				}
				select {
				case <-fixture.requestCanceled:
				case <-time.After(5 * time.Second):
					t.Fatal("cancellation did not reach the API request")
				}
				fixture.assertUpdates(t, 0)
				fixture.reset("normal", req)
				deploy(t, req, "")
				fixture.assertUpdates(t, 1)
			})
		})
	}
}

func (f *b1AppsFixture) reset(mode string, req plugin.DeployRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
	f.requestStarted = make(chan struct{}, 1)
	f.requestCanceled = make(chan struct{}, 1)
	f.calls = 0
	f.updates = 0
	f.certPEM = req.CertificatePEM
	f.keyPEM = req.PrivateKeyPEM
}

func (f *b1AppsFixture) assertCounts(t *testing.T, calls, updates int) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls != calls || f.updates != updates {
		t.Fatalf("calls=%d updates=%d, expected %d/%d", f.calls, f.updates, calls, updates)
	}
}

func (f *b1AppsFixture) assertUpdates(t *testing.T, updates int) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == 0 || f.updates != updates {
		t.Fatalf("calls=%d updates=%d, expected updates=%d", f.calls, f.updates, updates)
	}
}

func (f *b1AppsFixture) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	mode := f.mode
	started, canceled := f.requestStarted, f.requestCanceled
	f.calls++
	f.mu.Unlock()
	if mode == "wait" {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
			select {
			case canceled <- struct{}{}:
			default:
			}
		case <-time.After(10 * time.Second):
		}
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		f.t.Error(err)
		http.Error(w, "bad JSON", 400)
		return
	}
	check := func(label string, got, want any) {
		if !reflect.DeepEqual(got, want) {
			f.t.Errorf("%s %s: %s mismatch", r.Host, r.URL.Path, label)
		}
	}
	respond := func(v any) {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			f.t.Error(err)
		}
	}
	fail := func() { http.Error(w, `{"status":401,"message":"fixture failed","error":"fixture failed"}`, 401) }
	switch r.Host {
	case "api.machines.dev":
		check("path", r.URL.Path, "/v1/apps/fixture-app/certificates/custom")
		check("authorization", r.Header.Get("Authorization"), "Bearer fixture-token")
		if mode == "auth-failure" || mode == "update-failure" {
			fail()
			return
		}
		check("hostname", body["hostname"], "*.example.com")
		check("fullchain", body["fullchain"], f.certPEM)
		check("private key", body["private_key"], f.keyPEM)
		f.updates++
		respond(map[string]any{"hostname": "*.example.com", "configured": true})
	case "cloud.mhjz1.cn":
		if r.URL.Path == "/v1/login_api" {
			check("username", body["account"], "fixture-user")
			check("password", body["password"], "fixture-password")
			if mode == "auth-failure" {
				fail()
				return
			}
			respond(map[string]any{"status": 200, "jwt": "fixture-jwt"})
			return
		}
		check("path", r.URL.Path, "/provision/custom/42/domains")
		check("jwt", r.Header.Get("JWT"), "Bearer fixture-jwt")
		if mode == "update-failure" {
			fail()
			return
		}
		check("function", body["func"], "SetSSL")
		check("domain id", body["id"], json.Number("73"))
		check("cert", body["sslCert"], url.QueryEscape(f.certPEM))
		check("key", body["sslKey"], url.QueryEscape(f.keyPEM))
		check("force", body["ssl_force"], "")
		f.updates++
		respond(map[string]any{"status": 200})
	case "account.dcloud.net.cn", "unicloud.dcloud.net.cn":
		if mode == "auth-failure" {
			fail()
			return
		}
		check("path", r.URL.Path, "/client")
		secret := "ba461799-fde8-429f-8cc4-4b6d306e2339"
		if r.Host == "unicloud.dcloud.net.cn" {
			secret = "4c1f7fbf-c732-42b0-ab10-4634a8bbe834"
		}
		keys := make([]string, 0, len(body))
		for key := range body {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, key+"="+fmt.Sprint(body[key]))
		}
		signature := hmac.New(md5.New, []byte(secret))
		signature.Write([]byte(strings.Join(parts, "&")))
		check("signature", r.Header.Get("X-Serverless-Sign"), hex.EncodeToString(signature.Sum(nil)))
		params := map[string]any{}
		raw, _ := body["params"].(string)
		if err := json.Unmarshal([]byte(raw), &params); err != nil {
			f.t.Error(err)
			fail()
			return
		}
		args, _ := params["functionArgs"].(map[string]any)
		switch args["method"] {
		case "createPasswordChallenge":
			pub, e := x509.MarshalPKIXPublicKey(&f.rsaKey.PublicKey)
			if e != nil {
				f.t.Error(e)
			}
			respond(map[string]any{"success": true, "data": map[string]any{"version": 1, "algorithm": "RSA-OAEP-256+A256GCM", "keyId": "fixture-key", "challengeId": "fixture-challenge", "publicKey": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}))}})
		case "login":
			f.checkEnvelope(args)
			respond(map[string]any{"success": true, "data": map[string]any{"newToken": map[string]any{"token": "fixture-identity", "tokenExpired": time.Now().Add(time.Hour).UnixMilli()}}})
		default:
			check("console action", args["action"], "user/getUserToken")
			check("identity token", args["uniIdToken"], "fixture-identity")
			respond(map[string]any{"success": true, "data": map[string]any{"code": 0, "data": map[string]any{"ret": 0, "data": map[string]any{"token": "fixture-console"}}}})
		}
	case "unicloud-api.dcloud.net.cn":
		check("path", r.URL.Path, "/unicloud/api/host/create-domain-with-cert")
		check("console token", r.Header.Get("Token"), "fixture-console")
		if mode == "update-failure" {
			fail()
			return
		}
		check("provider", body["provider"], "tencent")
		check("space", body["spaceId"], "fixture-space")
		check("domain", body["domain"], "app.example.com")
		check("cert", body["cert"], url.QueryEscape(f.certPEM))
		check("key", body["key"], url.QueryEscape(f.keyPEM))
		f.updates++
		respond(map[string]any{"ret": 0})
	default:
		f.t.Errorf("unexpected host %s", r.Host)
		fail()
	}
}

func (f *b1AppsFixture) checkEnvelope(args map[string]any) {
	params, ok := args["params"].([]any)
	if !ok || len(params) != 1 {
		f.t.Error("missing login params")
		return
	}
	param, _ := params[0].(map[string]any)
	envelope, _ := param["passwordEnvelope"].(map[string]any)
	decode := func(key string) []byte {
		value, _ := envelope[key].(string)
		data, err := base64.RawStdEncoding.DecodeString(value)
		if err != nil {
			f.t.Errorf("invalid %s", key)
		}
		return data
	}
	key, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, f.rsaKey, decode("encryptedKey"), nil)
	if err != nil {
		f.t.Errorf("decrypt login key: %v", err)
		return
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		f.t.Error(err)
		return
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		f.t.Error(err)
		return
	}
	aad := []byte(`{"version":1,"algorithm":"RSA-OAEP-256+A256GCM","keyId":"fixture-key","challengeId":"fixture-challenge"}`)
	plain, err := gcm.Open(nil, decode("iv"), append(decode("ciphertext"), decode("tag")...), aad)
	if err != nil {
		f.t.Errorf("decrypt login envelope: %v", err)
		return
	}
	values := map[string]any{}
	if err := json.Unmarshal(plain, &values); err != nil {
		f.t.Error(err)
		return
	}
	if values["username"] != "fixture-user" || values["password"] != "fixture-password" || values["challengeId"] != "fixture-challenge" {
		f.t.Error("decrypted login credentials mismatch")
	}
}

func newB1AppsFixture(t *testing.T) *b1AppsFixture {
	t.Helper()
	certPEM, keyPEM, _ := makeCertificate(t, []string{"cdn.example.com"}, false)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &b1AppsFixture{t: t, certPEM: certPEM, keyPEM: keyPEM, rsaKey: rsaKey}
	hosts := []string{"api.machines.dev", "cloud.mhjz1.cn", "account.dcloud.net.cn", "unicloud.dcloud.net.cn", "unicloud-api.dcloud.net.cn"}
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
		if r.Method != http.MethodConnect || !strings.HasSuffix(r.Host, ":443") {
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
	return f
}

func buildB1AppsBatch(t *testing.T, dir, arch string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range b1AppsProviders {
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

func runB1AppsBatchInLinux(t *testing.T) {
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
	buildB1AppsBatch(t, dir, arch)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "b1-apps-batch.test"), "./integration")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Linux tests: %v\n%s", err, output)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	args := []string{"run", "--rm", "--network=none", "--mount", "type=bind,src=" + dir + ",dst=/fixtures,readonly", "-e", "CERTIMATE_B1_APPS_E2E_BINARIES=/fixtures"}
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
	args = append(args, image, "/fixtures/b1-apps-batch.test", "-test.run=^TestB1AppsDeploymentBatch$", "-test.v", "-test.timeout=5m")
	cmd = exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	t.Logf("Linux plugin test output:\n%s", output)
	if err != nil {
		t.Fatalf("Linux end-to-end tests: %v", err)
	}
}
