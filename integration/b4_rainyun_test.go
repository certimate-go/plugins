package integration

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

const b4RainyunKey = "b4-rainyun-fixture-key"

type b4RainyunCertificate struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type b4RainyunTarget struct {
	mu      sync.Mutex
	barrier workflowRequestBarrier
	mode    string
	certs   map[int]b4RainyunCertificate
	nextID  int
	boundID int
	created int
	updated int
	bound   int
}

func TestB4RainyunWorkflow(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for workflow verification")
	}
	providers := []string{"rainyun-rcdn", "rainyun-sslcenter"}
	if runtime.GOOS != "linux" {
		runB4CloudInLinux(t, providers)
		return
	}

	dir := os.Getenv("CERTIMATE_B4_PLUGIN_BINARIES")
	if dir == "" {
		dir = t.TempDir()
		buildB4CloudPackages(t, dir, providers, runtime.GOOS, runtime.GOARCH)
	}
	for _, target := range []struct {
		name     string
		provider string
		config   string
	}{
		{"rcdn", "rainyun-rcdn", `{"instanceId":"11","domainMatchPattern":"exact","domain":"*.fixture.example"}`},
		{"ssl-upload", "rainyun-sslcenter", "{}"},
		{"ssl-replace", "rainyun-sslcenter", `{"certificateId":41}`},
	} {
		t.Run(target.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			f := &b4RainyunTarget{certs: map[int]b4RainyunCertificate{}, nextID: 41}
			if target.name == "ssl-replace" {
				oldCert, oldKey, _ := makeCertificate(t, []string{"fixture.example"}, false)
				f.certs[41] = b4RainyunCertificate{oldCert, oldKey}
			}
			newB4CloudProxy(t, []string{"api.v2.rainyun.com"}, http.HandlerFunc(f.serve))
			verify := func(req *plugin.DeployRequest) {
				t.Helper()
				f.mu.Lock()
				defer f.mu.Unlock()
				id := f.nextID - 1
				switch target.name {
				case "rcdn":
					id = f.boundID
				case "ssl-replace":
					id = 41
				}
				got := f.certs[id]
				if strings.TrimSpace(got.Cert) != strings.TrimSpace(req.CertificatePEM) || strings.TrimSpace(got.Key) != strings.TrimSpace(req.PrivateKeyPEM) {
					t.Fatalf("target %s did not store the certificate chain and key at certificate ID %d", target.name, id)
				}
			}
			deploy := newB1WorkflowDeployer(t, dir, target.provider, workflowLifecycleTarget{barrier: &f.barrier, verify: verify})
			req := &plugin.DeployRequest{AccessConfigJSON: `{"apiKey":"` + b4RainyunKey + `"}`, ExtendedConfigJSON: target.config, LogLevel: "DEBUG"}
			steps := []string{"initial", "repeat", "renew", "auth-failure", "auth-recovery"}
			if target.name == "rcdn" {
				steps = append(steps, "binding-failure", "binding-recovery")
			}
			for _, step := range steps {
				t.Run(step, func(t *testing.T) {
					fresh := step == "initial" || step == "renew" || step == "binding-failure"
					if fresh {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{"fixture.example", "*.fixture.example"}, false)
						issuer, _, _ := makeCertificate(t, []string{"fixture-issuer.example"}, true)
						req.CertificatePEM += issuer
					}
					f.mu.Lock()
					f.mode = step
					c, u, b := f.created, f.updated, f.bound
					f.mu.Unlock()
					_, err := deploy(ctx, req)
					wantCreate, wantUpdate, wantBind := 0, 0, 0
					switch step {
					case "auth-failure", "binding-failure":
						want := "401"
						if step == "binding-failure" {
							want = "503"
							wantCreate = 1
						}
						if err == nil || !strings.Contains(err.Error(), want) {
							t.Fatalf("expected target status %s, got %v", want, err)
						}
					default:
						if err != nil {
							t.Fatal(err)
						}
						verify(req)
						if target.name == "ssl-replace" {
							wantUpdate = 1
						} else if fresh {
							wantCreate = 1
						}
						if target.name == "rcdn" {
							wantBind = 1
						}
					}
					f.mu.Lock()
					defer f.mu.Unlock()
					if f.created-c != wantCreate || f.updated-u != wantUpdate || f.bound-b != wantBind {
						t.Fatalf("target changes (create/update/bind) = %d/%d/%d, want %d/%d/%d", f.created-c, f.updated-u, f.bound-b, wantCreate, wantUpdate, wantBind)
					}
				})
			}
		})
	}
}

func (f *b4RainyunTarget) serve(w http.ResponseWriter, r *http.Request) {
	if err := f.barrier.waitHTTP(r); err != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	reply := func(status int, body any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
	if r.Header.Get("X-API-Key") != b4RainyunKey || f.mode == "auth-failure" {
		reply(http.StatusUnauthorized, map[string]any{"code": 401, "message": "denied " + r.Header.Get("X-API-Key")})
		return
	}
	path := r.URL.Path
	response := map[string]any{"code": 200}
	switch {
	case r.Method == http.MethodGet && path == "/product/sslcenter":
		var options struct {
			Page    int `json:"page"`
			PerPage int `json:"perPage"`
			Filters struct {
				Domain string `json:"Domain"`
			} `json:"columnFilters"`
		}
		if err := json.Unmarshal([]byte(r.URL.Query().Get("options")), &options); err != nil || options.Page != 1 || options.PerPage != 100 || options.Filters.Domain != "fixture.example" {
			reply(http.StatusBadRequest, map[string]any{"code": 400, "message": "unexpected certificate list query"})
			return
		}
		records := make([]map[string]any, 0, len(f.certs))
		for id, item := range f.certs {
			cert, err := xcert.ParseCertificateFromPEM(item.Cert)
			if err != nil {
				reply(http.StatusBadRequest, map[string]int{"code": 400})
				return
			}
			records = append(records, map[string]any{
				"ID": id, "Domain": strings.Join(cert.DNSNames, ", "), "StartDate": cert.NotBefore.Unix(), "ExpDate": cert.NotAfter.Unix(),
			})
		}
		response["data"] = map[string]any{"TotalRecords": len(records), "Records": records}
	case r.Method == http.MethodPost && path == "/product/sslcenter/":
		var cert b4RainyunCertificate
		if err := json.NewDecoder(r.Body).Decode(&cert); err != nil || cert.Cert == "" || cert.Key == "" {
			reply(http.StatusBadRequest, map[string]int{"code": 400})
			return
		}
		f.certs[f.nextID] = cert
		f.nextID++
		f.created++
	case strings.HasPrefix(path, "/product/sslcenter/") && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		id, err := strconv.Atoi(strings.TrimPrefix(path, "/product/sslcenter/"))
		cert, ok := f.certs[id]
		if err != nil || !ok {
			reply(http.StatusNotFound, map[string]int{"code": 404})
			return
		}
		if r.Method == http.MethodPut {
			if err := json.NewDecoder(r.Body).Decode(&cert); err != nil || cert.Cert == "" || cert.Key == "" {
				reply(http.StatusBadRequest, map[string]int{"code": 400})
				return
			}
			f.certs[id] = cert
			f.updated++
		} else {
			response["data"] = map[string]string{"Cert": cert.Cert, "Key": cert.Key}
		}
	case r.Method == http.MethodPost && path == "/product/rcdn/instance/11/ssl_bind":
		var binding struct {
			CertID  int      `json:"cert_id"`
			Domains []string `json:"domains"`
		}
		if err := json.NewDecoder(r.Body).Decode(&binding); err != nil || f.certs[binding.CertID].Cert == "" || len(binding.Domains) != 1 || binding.Domains[0] != "*.fixture.example" {
			reply(http.StatusBadRequest, map[string]int{"code": 400})
			return
		}
		if f.mode == "binding-failure" {
			reply(http.StatusServiceUnavailable, map[string]int{"code": 503})
			return
		}
		f.boundID = binding.CertID
		f.bound++
	default:
		reply(http.StatusNotFound, map[string]int{"code": 404})
		return
	}
	reply(http.StatusOK, response)
}

func newB4CloudProxy(t *testing.T, hosts []string, handler http.Handler) {
	t.Helper()
	ca, key, _ := makeCertificate(t, hosts, true)
	pair, err := tls.X509KeyPair([]byte(ca), []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	target := httptest.NewUnstartedServer(handler)
	target.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	target.StartTLS()
	t.Cleanup(target.Close)
	allowed := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		allowed[host+":443"] = true
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || !allowed[r.Host] {
			http.Error(w, "unexpected proxy destination", http.StatusBadGateway)
			return
		}
		upstream, err := net.DialTimeout("tcp", target.Listener.Addr().String(), 5*time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer upstream.Close()
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		if _, err := buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if err := buffer.Flush(); err != nil {
			return
		}
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(upstream, buffer)
			_ = upstream.Close()
			close(done)
		}()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		<-done
	}))
	t.Cleanup(proxy.Close)
	path := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(path, []byte(ca), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("SSL_CERT_FILE", path)
}

func buildB4CloudPackages(t *testing.T, dir string, providers []string, goos, arch string) {
	t.Helper()
	for _, provider := range providers {
		manifest, err := os.ReadFile(filepath.Join("..", provider, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		m, err := plugin.ParseManifest(manifest)
		if err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, provider)
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("go", "build", "-o", filepath.Join(out, m.Binary), "./"+provider)
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+arch, "CGO_ENABLED=0")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", provider, err, output)
		}
		for _, name := range []string{"manifest.json", m.Icon} {
			raw, err := os.ReadFile(filepath.Join("..", provider, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, name), raw, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func runB4CloudInLinux(t *testing.T, providers []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.Architecture}}").CombinedOutput()
	if err != nil {
		t.Fatalf("Docker is required for TLS fixtures: %v\n%s", err, output)
	}
	arch := strings.TrimSpace(string(output))
	switch arch {
	case "aarch64":
		arch = "arm64"
	case "x86_64":
		arch = "amd64"
	}
	dir := t.TempDir()
	if os.Getenv("CERTIMATE_B1_BASELINE") != "1" {
		buildB4CloudPackages(t, dir, providers, "linux", arch)
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "b4-cloud.test"), "./integration")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Linux integration tests: %v\n%s", err, output)
	}
	binary := os.Getenv("CERTIMATE_B1_CORE_BINARY")
	if binary == "" {
		t.Fatal("CERTIMATE_B1_CORE_BINARY must point to the Linux Certimate binary")
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	args := []string{
		"run", "--rm", "--network=none",
		"--mount", "type=bind,src=" + dir + ",dst=/fixtures,readonly",
		"--mount", "type=bind,src=" + binary + ",dst=/core,readonly",
		"-e", "CERTIMATE_B4_PLUGIN_BINARIES=/fixtures",
		"-e", "CERTIMATE_B1_CORE_BINARY=/core",
		"-e", "CERTIMATE_B1_BASELINE=" + os.Getenv("CERTIMATE_B1_BASELINE"),
		image, "/fixtures/b4-cloud.test", "-test.run=^" + t.Name() + "$", "-test.v", "-test.timeout=10m",
	}
	output, err = exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	t.Logf("Linux workflow output:\n%s", output)
	if err != nil {
		t.Fatal(fmt.Errorf("Linux workflow verification: %w", err))
	}
}
