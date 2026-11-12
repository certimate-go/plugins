package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/baidubce/bce-sdk-go/services/cert"
	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

const (
	b4BaiduAccessKey = "fixture-baidu-access"
	b4BaiduSecretKey = "fixture-baidu-secret"
	b4BaiduDomain    = "b4.example.com"
	b4BaiduLBID      = "lb-fixture"
)

var b4BaiduProviders = []string{"baiducloud-appblb", "baiducloud-blb", "baiducloud-cdn", "baiducloud-cert"}

func TestB4BaiduWorkflow(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_B4_BAIDU_BINARIES") == "" {
		runB4BaiduInDocker(t)
		return
	}
	dir := os.Getenv("CERTIMATE_B4_BAIDU_BINARIES")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for _, provider := range b4BaiduProviders {
		t.Run(provider, func(t *testing.T) {
			fixture := startB4BaiduFixture(t, provider)
			deploy := newB1WorkflowDeployer(t, dir, provider, workflowLifecycleTarget{
				barrier: &fixture.barrier,
				verify: func(req *plugin.DeployRequest) {
					fixture.verify(t, req)
				},
			})
			access := map[string]string{"accessKeyId": b4BaiduAccessKey, "secretAccessKey": b4BaiduSecretKey}
			config := map[string]any{}
			switch provider {
			case "baiducloud-appblb":
				config = map[string]any{"region": "bj", "deployTarget": "listener", "loadbalancerId": b4BaiduLBID, "listenerPort": "443", "domain": b4BaiduDomain}
			case "baiducloud-blb":
				config = map[string]any{"region": "bj", "deployTarget": "loadbalancer", "loadbalancerId": b4BaiduLBID, "listenerPort": 443}
			case "baiducloud-cdn":
				config = map[string]any{"domainMatchPattern": "certsan"}
			}
			rawAccess, err := json.Marshal(access)
			if err != nil {
				t.Fatal(err)
			}
			rawConfig, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			req := &plugin.DeployRequest{AccessConfigJSON: string(rawAccess), ExtendedConfigJSON: string(rawConfig)}
			issuer, _, _ := makeCertificate(t, []string{"issuer.example.com"}, true)
			for _, step := range []string{"initial", "repeat", "renew", "auth-failure", "auth-recovery", "write-failure", "write-recovery"} {
				t.Run(step, func(t *testing.T) {
					if step == "initial" || step == "renew" || step == "auth-failure" || step == "write-failure" {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{b4BaiduDomain}, false)
						req.CertificatePEM += issuer
					}
					fixture.mu.Lock()
					fixture.mode = step
					beforeUploads := len(fixture.uploads)
					beforeTarget := fixture.targetCertificate()
					fixture.mu.Unlock()
					_, err := deploy(ctx, req)
					fixture.mu.Lock()
					fixture.mode = ""
					fixture.mu.Unlock()
					if step == "auth-failure" || step == "write-failure" {
						if err == nil || !strings.Contains(err.Error(), "fixture "+step) {
							t.Fatalf("expected injected %s, got %v", step, err)
						}
						fixture.mu.Lock()
						defer fixture.mu.Unlock()
						if fixture.targetCertificate() != beforeTarget {
							t.Fatal("failed request changed the binding")
						}
						if step == "auth-failure" && len(fixture.uploads) != beforeUploads {
							t.Fatal("authentication failure wrote a certificate")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					fixture.verify(t, req)
					if step == "repeat" && provider != "baiducloud-cdn" {
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

type b4BaiduCertificate struct {
	Name string `json:"certName"`
	PEM  string `json:"certServerData"`
	Key  string `json:"certPrivateData"`
}

type b4BaiduFixture struct {
	barrier  workflowRequestBarrier
	mu       sync.Mutex
	provider string
	mode     string
	uploads  map[string]b4BaiduCertificate
	bound    string
	cdn      b4BaiduCertificate
	lastID   string
	writes   int
	bindings int
}

func startB4BaiduFixture(t *testing.T, provider string) *b4BaiduFixture {
	t.Helper()
	f := &b4BaiduFixture{provider: provider, uploads: map[string]b4BaiduCertificate{}}
	listener, err := net.Listen("tcp4", "127.0.0.1:80")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(f.serve), ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })
	return f
}

func (f *b4BaiduFixture) verify(t *testing.T, req *plugin.DeployRequest) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	got := f.targetCertificate()
	if strings.TrimSpace(got.PEM) != strings.TrimSpace(req.CertificatePEM) || strings.TrimSpace(got.Key) != strings.TrimSpace(req.PrivateKeyPEM) {
		t.Fatal("target certificate chain, private key, or binding differs")
	}
}

func (f *b4BaiduFixture) targetCertificate() b4BaiduCertificate {
	switch f.provider {
	case "baiducloud-cert":
		return f.uploads[f.lastID]
	case "baiducloud-cdn":
		return f.cdn
	default:
		return f.uploads[f.bound]
	}
}

func (f *b4BaiduFixture) serve(w http.ResponseWriter, r *http.Request) {
	if err := f.barrier.wait(r.Context(), nil); err != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(result any) { _ = json.NewEncoder(w).Encode(result) }
	fail := func(status int, code, message string) {
		w.WriteHeader(status)
		write(map[string]string{"code": code, "message": message, "requestId": "fixture"})
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "bce-auth-v1/"+b4BaiduAccessKey+"/") {
		fail(http.StatusUnauthorized, "InvalidAccessKeyId", "unexpected access key")
		return
	}
	if f.mode == "auth-failure" {
		fail(http.StatusForbidden, "AccessDenied", "fixture auth-failure")
		return
	}
	path := r.URL.Path
	switch r.Host {
	case "certificate.baidubce.com":
		switch {
		case r.Method == http.MethodGet && path == "/v1/certificate/detail":
			result := cert.ListCertDetailResult{Certs: []cert.CertificateDetailMeta{}}
			for id, upload := range f.uploads {
				leaf, err := xcert.ParseCertificateFromPEM(upload.PEM)
				if err != nil {
					fail(http.StatusBadRequest, "InvalidCertificate", "invalid uploaded certificate")
					return
				}
				result.Certs = append(result.Certs, cert.CertificateDetailMeta{
					CertId: id, CertName: upload.Name, CertCommonName: leaf.Subject.CommonName,
					CertDNSNames: strings.Join(leaf.DNSNames, ","), CertStartTime: leaf.NotBefore.Format(time.RFC3339), CertStopTime: leaf.NotAfter.Format(time.RFC3339),
				})
			}
			write(result)
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/certificate/") && strings.HasSuffix(path, "/rawData"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/certificate/"), "/rawData")
			upload, ok := f.uploads[id]
			if !ok {
				fail(http.StatusNotFound, "ResourceNotFoundException", "unknown certificate")
				return
			}
			write(map[string]string{"certServerData": upload.PEM})
			return
		case r.Method == http.MethodPost && path == "/v1/certificate":
			if f.mode == "write-failure" && f.provider == "baiducloud-cert" {
				fail(http.StatusConflict, "OperationDenied", "fixture write-failure")
				return
			}
			var upload b4BaiduCertificate
			if json.NewDecoder(r.Body).Decode(&upload) != nil || upload.PEM == "" || upload.Key == "" {
				fail(http.StatusBadRequest, "InvalidCertificate", "missing certificate data")
				return
			}
			id := fmt.Sprintf("cert-%d", len(f.uploads)+1)
			f.uploads[id] = upload
			f.writes++
			f.lastID = id
			write(cert.CreateCertResult{CertId: id, CertName: upload.Name})
			return
		}
	case "blb.bj.baidubce.com":
		lbPath := "/v1/" + strings.TrimPrefix(f.provider, "baiducloud-") + "/" + b4BaiduLBID
		switch {
		case r.Method == http.MethodGet && path == lbPath && f.provider == "baiducloud-blb":
			write(map[string]any{"blbId": b4BaiduLBID, "listener": []map[string]any{{"type": "HTTPS", "port": "443"}}})
			return
		case r.Method == http.MethodGet && path == lbPath+"/listener" && r.URL.Query().Get("listenerPort") == "443":
			write(map[string]any{"listenerList": []map[string]any{{"listenerType": "HTTPS", "listenerPort": 443}}})
			return
		case r.Method == http.MethodGet && path == lbPath+"/HTTPSlistener" && r.URL.Query().Get("listenerPort") == "443":
			defaultID, sniID := f.bound, "initial-sni"
			if f.provider == "baiducloud-appblb" {
				defaultID, sniID = "default-cert", f.bound
			}
			write(map[string]any{"listenerList": []map[string]any{{
				"listenerPort": 443, "scheduler": "RoundRobin",
				"certIds": []string{defaultID}, "additionalCertDomains": []map[string]string{{"host": b4BaiduDomain, "certId": sniID}, {"host": "other.example.com", "certId": "other-cert"}},
			}}})
			return
		case r.Method == http.MethodPut && path == lbPath+"/HTTPSlistener" && r.URL.Query().Get("listenerPort") == "443":
			if f.mode == "write-failure" {
				fail(http.StatusConflict, "OperationDenied", "fixture write-failure")
				return
			}
			var binding struct {
				CertIDs []string `json:"certIds"`
				Domains []struct {
					Host string `json:"host"`
					ID   string `json:"certId"`
				} `json:"additionalCertDomains"`
			}
			if json.NewDecoder(r.Body).Decode(&binding) != nil || len(binding.CertIDs) != 1 {
				fail(http.StatusBadRequest, "InvalidBinding", "unexpected certificate binding")
				return
			}
			id := binding.CertIDs[0]
			if f.provider == "baiducloud-appblb" {
				if id != "default-cert" || len(binding.Domains) != 2 || binding.Domains[0].Host != b4BaiduDomain || binding.Domains[1].Host != "other.example.com" || binding.Domains[1].ID != "other-cert" {
					fail(http.StatusBadRequest, "InvalidBinding", "unexpected SNI binding")
					return
				}
				id = binding.Domains[0].ID
			}
			if _, ok := f.uploads[id]; !ok {
				fail(http.StatusBadRequest, "InvalidBinding", "binding does not reference an uploaded certificate")
				return
			}
			f.bound = id
			f.bindings++
			write(struct{}{})
			return
		}
	case "cdn.baidubce.com":
		switch {
		case r.Method == http.MethodGet && path == "/v2/domain":
			write(map[string]any{"domains": []map[string]string{{"name": b4BaiduDomain}, {"name": "unrelated.example.com"}}})
			return
		case r.Method == http.MethodPut && path == "/v2/"+b4BaiduDomain+"/certificates":
			if f.mode == "write-failure" {
				fail(http.StatusConflict, "OperationDenied", "fixture write-failure")
				return
			}
			var binding struct {
				Certificate b4BaiduCertificate `json:"certificate"`
				HTTPSEnable string             `json:"httpsEnable"`
			}
			if json.NewDecoder(r.Body).Decode(&binding) != nil || binding.HTTPSEnable != "ON" {
				fail(http.StatusBadRequest, "InvalidBinding", "unexpected CDN certificate binding")
				return
			}
			f.cdn = binding.Certificate
			f.writes++
			f.bindings++
			f.bound = b4BaiduDomain
			write(map[string]string{"certId": "cdn-fixture"})
			return
		}
	}
	fail(http.StatusNotFound, "UnexpectedRequest", fmt.Sprintf("unexpected fixture request %s %s%s", r.Method, r.Host, path))
}

func runB4BaiduInDocker(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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
		buildB4CloudPackages(t, dir, b4BaiduProviders, "linux", arch)
	}
	run(env, "go", "-C", root, "test", "-c", "-o", filepath.Join(dir, "b4-baidu.test"), "./integration")
	binary, err := filepath.Abs(os.Getenv("CERTIMATE_B1_CORE_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	output := run(nil, "docker", "run", "--rm", "--network=none",
		"--add-host=certificate.baidubce.com:127.0.0.1",
		"--add-host=blb.bj.baidubce.com:127.0.0.1",
		"--add-host=cdn.baidubce.com:127.0.0.1",
		"--mount", "type=bind,src="+dir+",dst=/fixtures,readonly",
		"--mount", "type=bind,src="+binary+",dst=/core,readonly",
		"-e", "CERTIMATE_B1_CORE_BINARY=/core",
		"-e", "CERTIMATE_B1_BASELINE="+os.Getenv("CERTIMATE_B1_BASELINE"),
		"-e", "CERTIMATE_B4_BAIDU_BINARIES=/fixtures",
		image, "/fixtures/b4-baidu.test", "-test.run=^TestB4BaiduWorkflow$", "-test.v", "-test.timeout=8m")
	t.Logf("Linux workflow output:\n%s", output)
}
