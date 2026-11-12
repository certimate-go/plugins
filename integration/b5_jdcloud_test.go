package integration

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

const (
	b5JDCloudAccessKey = "fixture-jdcloud-access"
	b5JDCloudSecretKey = "fixture-jdcloud-secret"
	b5JDCloudDomain    = "b5.example.com"
)

var b5JDCloudProviders = []string{"jdcloud-alb", "jdcloud-cdn", "jdcloud-live", "jdcloud-ssl", "jdcloud-vod", "jdcloud-waf"}

func TestB5JDCloudWorkflow(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to verify application workflows")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_B5_JDCLOUD_BINARIES") == "" {
		runB5JDCloudInDocker(t)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	for _, provider := range b5JDCloudProviders {
		t.Run(provider, func(t *testing.T) {
			fixture := startB5JDCloudFixture(t, provider)
			deploy := newB1WorkflowDeployer(t, os.Getenv("CERTIMATE_B5_JDCLOUD_BINARIES"), provider, workflowLifecycleTarget{barrier: &fixture.barrier, verify: func(req *plugin.DeployRequest) { fixture.verify(t, req) }})
			config := map[string]any{}
			switch provider {
			case "jdcloud-alb":
				config = map[string]any{"regionId": "cn-north-1", "deployTarget": "listener", "listenerId": "listener-fixture", "domain": b5JDCloudDomain}
			case "jdcloud-cdn", "jdcloud-live", "jdcloud-vod":
				config = map[string]any{"domainMatchPattern": "certsan"}
			case "jdcloud-waf":
				config = map[string]any{"regionId": "cn-north-1", "instanceId": "waf-fixture", "domain": b5JDCloudDomain}
			}
			rawConfig, _ := json.Marshal(config)
			rawAccess, _ := json.Marshal(map[string]string{"accessKeyId": b5JDCloudAccessKey, "accessKeySecret": b5JDCloudSecretKey})
			req := &plugin.DeployRequest{AccessConfigJSON: string(rawAccess), ExtendedConfigJSON: string(rawConfig)}
			issuer, _, _ := makeCertificate(t, []string{"issuer.example.com"}, true)
			for _, step := range []string{"initial", "repeat", "renew", "auth-failure", "auth-recovery", "write-failure", "write-recovery"} {
				t.Run(step, func(t *testing.T) {
					if step == "initial" || step == "renew" || step == "auth-failure" || step == "write-failure" {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{b5JDCloudDomain}, false)
						req.CertificatePEM += issuer
					}
					fixture.mu.Lock()
					fixture.mode = step
					before := fixture.targetCertificate()
					beforeUploads := len(fixture.uploads)
					fixture.mu.Unlock()
					_, err := deploy(ctx, req)
					fixture.mu.Lock()
					fixture.mode = ""
					fixture.mu.Unlock()
					if step == "auth-failure" || step == "write-failure" {
						if os.Getenv("CERTIMATE_B1_BASELINE") == "1" && (err == nil || !strings.Contains(err.Error(), "fixture "+step)) {
							fixture.mu.Lock()
							unchanged := fixture.targetCertificate() == before
							fixture.mu.Unlock()
							if !unchanged {
								t.Fatal("baseline error response unexpectedly changed target")
							}
							t.Logf("BASELINE DEFECT %s API %s returned %v; target unchanged", provider, step, err)
							return
						}
						if err == nil || !strings.Contains(err.Error(), "fixture "+step) {
							t.Fatalf("expected injected %s, got %v", step, err)
						}
						fixture.mu.Lock()
						defer fixture.mu.Unlock()
						if fixture.targetCertificate() != before {
							t.Fatal("failed request changed certificate binding")
						}
						if step == "auth-failure" && len(fixture.uploads) != beforeUploads {
							t.Fatal("authentication failure wrote certificate")
						}
						if step == "write-failure" && (provider == "jdcloud-alb" || provider == "jdcloud-cdn" || provider == "jdcloud-waf") && len(fixture.uploads) != beforeUploads+1 {
							t.Fatal("binding failure did not occur after successful certificate upload")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					fixture.verify(t, req)
					if step == "repeat" && provider != "jdcloud-live" && provider != "jdcloud-vod" {
						fixture.mu.Lock()
						defer fixture.mu.Unlock()
						if len(fixture.uploads) != beforeUploads {
							t.Fatal("identical certificate uploaded again")
						}
					}
				})
			}
		})
	}
}

type (
	b5JDCloudCertificate struct {
		Name string
		PEM  string
		Key  string
	}
	b5JDCloudFixture struct {
		barrier  workflowRequestBarrier
		mu       sync.Mutex
		provider string
		mode     string
		uploads  map[string]b5JDCloudCertificate
		bound    string
		direct   b5JDCloudCertificate
		lastID   string
	}
)

func (f *b5JDCloudFixture) targetCertificate() b5JDCloudCertificate {
	switch f.provider {
	case "jdcloud-ssl":
		return f.uploads[f.lastID]
	case "jdcloud-live", "jdcloud-vod":
		return f.direct
	default:
		return f.uploads[f.bound]
	}
}

func (f *b5JDCloudFixture) verify(t *testing.T, req *plugin.DeployRequest) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	got := f.targetCertificate()
	normalize := func(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")) }
	if normalize(got.PEM) != normalize(req.CertificatePEM) || normalize(got.Key) != normalize(req.PrivateKeyPEM) {
		t.Fatal("target certificate chain, key or binding differs")
	}
}

func (f *b5JDCloudFixture) serve(w http.ResponseWriter, r *http.Request) {
	if err := f.barrier.wait(r.Context(), nil); err != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(result any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"requestId": "fixture", "result": result})
	}
	fail := func(status int, message string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"requestId": "fixture", "error": map[string]any{"code": status, "status": "PERMISSION_DENIED", "message": message}})
	}
	if !strings.Contains(r.Header.Get("Authorization"), "Credential="+b5JDCloudAccessKey+"/") {
		fail(403, "unexpected access key")
		return
	}
	if f.mode == "auth-failure" {
		fail(403, "fixture auth-failure")
		return
	}
	if f.mode == "write-failure" && r.Method != "GET" && (r.Host != "ssl.jdcloud-api.com" || f.provider == "jdcloud-ssl") {
		fail(409, "fixture write-failure")
		return
	}
	body := map[string]any{}
	if r.Body != nil && r.Method != "GET" {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fail(400, "invalid request body")
			return
		}
	}
	path := r.URL.Path
	switch r.Host {
	case "ssl.jdcloud-api.com":
		switch {
		case r.Method == "GET" && strings.HasSuffix(path, "/sslCert"):
			certs := []map[string]any{}
			for id, c := range f.uploads {
				leaf, err := xcert.ParseCertificateFromPEM(c.PEM)
				if err != nil {
					fail(400, "invalid stored certificate")
					return
				}
				digest := sha256.Sum256([]byte(c.Key))
				certs = append(certs, map[string]any{"certId": id, "certName": c.Name, "commonName": leaf.Subject.CommonName, "dnsNames": leaf.DNSNames, "startTime": leaf.NotBefore.Format(time.RFC3339), "endTime": leaf.NotAfter.Format(time.RFC3339), "digest": hex.EncodeToString(digest[:])})
			}
			sort.Slice(certs, func(i, j int) bool { return b5JDCloudString(certs[i], "certId") < b5JDCloudString(certs[j], "certId") })
			total := len(certs)
			page, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
			size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
			if page < 1 || size < 1 {
				fail(400, "missing certificate pagination")
				return
			}
			start := min((page-1)*size, total)
			end := min(start+size, total)
			write(map[string]any{"certListDetails": certs[start:end], "totalCount": total})
			return
		case r.Method == "POST" && strings.HasSuffix(path, "/sslCert:upload"):
			c := b5JDCloudCertificate{Name: b5JDCloudString(body, "certName"), PEM: b5JDCloudString(body, "certFile"), Key: b5JDCloudString(body, "keyFile")}
			if c.PEM == "" || c.Key == "" {
				fail(400, "missing certificate payload")
				return
			}
			id := fmt.Sprintf("cert-%d", len(f.uploads)+1)
			f.uploads[id] = c
			f.lastID = id
			write(map[string]string{"certId": id})
			return
		}
	case "lb.jdcloud-api.com":
		if r.Method == "PATCH" && path == "/v1/regions/cn-north-1/listeners/listener-fixture" {
			specs, ok := body["certificateSpecs"].([]any)
			if !ok || len(specs) != 1 {
				fail(400, "missing certificate binding")
				return
			}
			spec, _ := specs[0].(map[string]any)
			id := b5JDCloudString(spec, "certificateId")
			if _, ok := f.uploads[id]; !ok {
				fail(400, "unknown certificate")
				return
			}
			f.bound = id
			write(map[string]any{})
			return
		}
	case "cdn.jdcloud-api.com":
		switch {
		case r.Method == "GET" && strings.HasSuffix(path, "/domain"):
			write(map[string]any{"domains": []map[string]string{{"domain": b5JDCloudDomain, "status": "online"}, {"domain": "unrelated.example.com", "status": "online"}}})
			return
		case r.Method == "GET" && strings.HasSuffix(path, "/domain/"+b5JDCloudDomain+"/config"):
			write(map[string]string{"httpsJumpType": "redirect"})
			return
		case r.Method == "POST" && strings.HasSuffix(path, "/domain/"+b5JDCloudDomain+"/httpType"):
			id := b5JDCloudString(body, "sslCertId")
			if _, ok := f.uploads[id]; !ok || b5JDCloudString(body, "jumpType") != "redirect" || b5JDCloudString(body, "httpType") != "https" || b5JDCloudString(body, "certFrom") != "ssl" {
				fail(400, "invalid CDN binding")
				return
			}
			f.bound = id
			write(map[string]string{"taskId": "task-fixture"})
			return
		}
	case "live.jdcloud-api.com":
		switch {
		case r.Method == "GET" && strings.HasSuffix(path, "/domains"):
			write(map[string]any{"domainDetails": []map[string]any{{"playDomains": []map[string]string{{"playDomain": b5JDCloudDomain, "domainStatus": "online"}, {"playDomain": "unrelated.example.com", "domainStatus": "online"}}}}})
			return
		case r.Method == "POST" && strings.HasSuffix(path, "/liveDomainCertificate"):
			if b5JDCloudString(body, "playDomain") != b5JDCloudDomain || b5JDCloudString(body, "certStatus") != "on" {
				fail(400, "unexpected live domain")
				return
			}
			f.direct = b5JDCloudCertificate{PEM: b5JDCloudString(body, "cert"), Key: b5JDCloudString(body, "key")}
			write(map[string]any{})
			return
		}
	case "vod.jdcloud-api.com":
		switch {
		case r.Method == "GET" && strings.HasSuffix(path, "/domains"):
			write(map[string]any{"content": []map[string]string{{"id": "41", "name": b5JDCloudDomain, "status": "running"}, {"id": "42", "name": "unrelated.example.com", "status": "running"}}})
			return
		case r.Method == "GET" && strings.HasSuffix(path, "/domains/41:getHttpSsl"):
			write(map[string]string{"jumpType": "redirect"})
			return
		case r.Method == "POST" && strings.HasSuffix(path, "/domains/41:setHttpSsl"):
			if b5JDCloudString(body, "jumpType") != "redirect" || body["enabled"] != true {
				fail(400, "invalid VOD binding")
				return
			}
			f.direct = b5JDCloudCertificate{PEM: b5JDCloudString(body, "sslCert"), Key: b5JDCloudString(body, "sslKey")}
			write(map[string]any{})
			return
		}
	case "waf.jdcloud-api.com":
		if r.Method == "POST" && strings.HasSuffix(path, "/regions/cn-north-1/wafInstanceIds/waf-fixture/cert:bindCert") {
			binding, _ := body["req"].(map[string]any)
			id := b5JDCloudString(binding, "certId")
			if _, ok := f.uploads[id]; !ok || b5JDCloudString(binding, "domain") != b5JDCloudDomain || b5JDCloudString(binding, "wafInstanceId") != "waf-fixture" {
				fail(400, "invalid WAF binding")
				return
			}
			f.bound = id
			write(map[string]any{})
			return
		}
	}
	fail(404, fmt.Sprintf("unexpected fixture request %s %s%s", r.Method, r.Host, path))
}

func b5JDCloudString(m map[string]any, key string) string { value, _ := m[key].(string); return value }

func startB5JDCloudFixture(t *testing.T, provider string) *b5JDCloudFixture {
	t.Helper()
	f := &b5JDCloudFixture{provider: provider, uploads: map[string]b5JDCloudCertificate{}}
	hosts := []string{"ssl.jdcloud-api.com", "lb.jdcloud-api.com", "cdn.jdcloud-api.com", "live.jdcloud-api.com", "vod.jdcloud-api.com", "waf.jdcloud-api.com"}
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

func runB5JDCloudInDocker(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	run := func(env []string, program string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, program, args...)
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", program, args, err, out)
		}
		return string(out)
	}
	arch := strings.TrimSpace(run(nil, "docker", "info", "--format", "{{.Architecture}}"))
	switch arch {
	case "aarch64":
		arch = "arm64"
	case "x86_64":
		arch = "amd64"
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	env := []string{"GOOS=linux", "GOARCH=" + arch, "CGO_ENABLED=0"}
	if os.Getenv("CERTIMATE_B1_BASELINE") != "1" {
		buildB4CloudPackages(t, dir, b5JDCloudProviders, "linux", arch)
	}
	run(env, "go", "-C", root, "test", "-c", "-o", filepath.Join(dir, "b5-jdcloud.test"), "integration/helpers_test.go", "integration/b1_workflow_test.go", "integration/b3_lifecycle_test.go", "integration/b4_rainyun_test.go", "integration/b5_jdcloud_test.go")
	binary, err := filepath.Abs(os.Getenv("CERTIMATE_B1_CORE_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	output := run(nil, "docker", "run", "--rm", "--network=none",
		"--mount", "type=bind,src="+dir+",dst=/fixtures,readonly",
		"--mount", "type=bind,src="+binary+",dst=/core,readonly",
		"-e", "CERTIMATE_B1_CORE_BINARY=/core",
		"-e", "CERTIMATE_B1_BASELINE="+os.Getenv("CERTIMATE_B1_BASELINE"),
		"-e", "CERTIMATE_B5_JDCLOUD_BINARIES=/fixtures",
		image, "/fixtures/b5-jdcloud.test", "-test.run=^TestB5JDCloudWorkflow$", "-test.v", "-test.timeout=10m")
	t.Logf("Linux workflow output:\n%s", output)
}
