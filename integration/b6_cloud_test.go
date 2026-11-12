package integration

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
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
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

const (
	b6AccessKey = "b6-fixture-access"
	b6SecretKey = "b6-fixture-secret"
	b6CMSHost   = "ccms-global.ctapi.ctyun.cn"
	b6CDNHost   = "ecloud.10086.cn"
	b6AOHost    = "accessone-global.ctapi.ctyun.cn"
	b6CTCDNHost = "ctcdn-global.ctapi.ctyun.cn"
	b6ELBHost   = "ctelb-global.ctapi.ctyun.cn"
	b6FaaSHost  = "cf-global.ctapi.ctyun.cn"
	b6ICDNHost  = "icdn-global.ctapi.ctyun.cn"
	b6LVDNHost  = "cdnapi-global.ctapi.ctyun.cn"
	b6Region    = "fixture-region"
	b6Listener  = "fixture-listener"
	b6LB        = "fixture-loadbalancer"
	b6Domain    = "fixture.example"
)

type b6Certificate struct {
	Certificate      string `json:"certificate"`
	CertificateChain string `json:"certificateChain"`
	PrivateKey       string `json:"privateKey"`
}

var b6TargetHosts = []string{b6CMSHost, b6CDNHost, b6AOHost, b6CTCDNHost, b6ELBHost, b6FaaSHost, b6ICDNHost, b6LVDNHost}

type b6CloudTarget struct {
	provider string
	uploads  []b6UploadedCertificate
	bound    int
	mu       sync.Mutex
	barrier  workflowRequestBarrier
	mode     string
	current  b6Certificate
	certs    []map[string]any
	writes   int
}

func TestB6CloudWorkflow(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to a Linux Certimate executable")
	}
	ctccAccess := `{"accessKeyId":"` + b6AccessKey + `","secretAccessKey":"` + b6SecretKey + `"}`
	cmccAccess := `{"accessKeyId":"` + b6AccessKey + `","accessKeySecret":"` + b6SecretKey + `"}`
	exactDomain := `{"domainMatchPattern":"exact","domain":"` + b6Domain + `"}`
	cases := []struct {
		provider    string
		access      string
		config      string
		repeatBinds bool
	}{
		{"ctcccloud-ao", ctccAccess, exactDomain, false},
		{"ctcccloud-cdn", ctccAccess, exactDomain, false},
		{"ctcccloud-cms", ctccAccess, `{}`, false},
		{"ctcccloud-elb", ctccAccess, `{"regionId":"` + b6Region + `","deployTarget":"listener","listenerId":"` + b6Listener + `"}`, true},
		{"ctcccloud-faas", ctccAccess, `{"regionId":"` + b6Region + `","domain":"` + b6Domain + `"}`, false},
		{"ctcccloud-icdn", ctccAccess, exactDomain, false},
		{"ctcccloud-lvdn", ctccAccess, exactDomain, false},
		{"cmcccloud-cdn", cmccAccess, exactDomain, false},
		{"cmcccloud-vlb", cmccAccess, `{"poolId":"CIDC-CORE-00","deployTarget":"listener","loadbalancerId":"` + b6LB + `","listenerId":"` + b6Listener + `"}`, false},
	}
	if os.Getenv("CERTIMATE_B6_DOCKER") != "1" {
		providers := make([]string, 0, len(cases))
		for _, target := range cases {
			providers = append(providers, target.provider)
		}
		runB6CloudInLinux(t, providers)
		return
	}
	for _, host := range b6TargetHosts {
		addrs, err := net.LookupIP(host)
		if err != nil || len(addrs) != 1 || !addrs[0].Equal(net.IPv4(127, 0, 0, 1)) {
			t.Fatalf("fixture host %s is not isolated on loopback: %v, %v", host, addrs, err)
		}
	}
	for _, target := range cases {
		t.Run(target.provider, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			fixture := &b6CloudTarget{provider: target.provider, bound: -1}
			startB6CloudTarget(t, http.HandlerFunc(fixture.serve))
			verify := func(req *plugin.DeployRequest) {
				t.Helper()
				fixture.mu.Lock()
				defer fixture.mu.Unlock()
				got := fixture.current
				trim := strings.TrimSpace
				if trim(got.Certificate+got.CertificateChain) != trim(req.CertificatePEM) || trim(got.PrivateKey) != trim(req.PrivateKeyPEM) {
					t.Fatal("target did not store the expected certificate chain and private key")
				}
			}
			deploy := newB1WorkflowDeployer(t, "/packages", target.provider, workflowLifecycleTarget{barrier: &fixture.barrier, verify: verify})
			req := &plugin.DeployRequest{AccessConfigJSON: target.access, ExtendedConfigJSON: target.config, LogLevel: "DEBUG"}
			issuer, _, _ := makeCertificate(t, []string{"issuer.fixture.example"}, true)
			for _, step := range []struct {
				name       string
				newCert    bool
				wantError  string
				wantWrites int
			}{
				{"initial", true, "", 1},
				{"repeat", false, "", 0},
				{"renew", true, "", 1},
				{"auth-failure", false, "401", 0},
				{"auth-recovery", true, "", 1},
				{"write-failure", true, "503", 0},
				{"write-recovery", false, "", 1},
			} {
				t.Run(step.name, func(t *testing.T) {
					if step.newCert {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{b6Domain}, false)
						req.CertificatePEM += issuer
					}
					fixture.mu.Lock()
					fixture.mode = step.name
					before := fixture.writes
					beforeUploads := len(fixture.uploads)
					fixture.mu.Unlock()
					_, err := deploy(ctx, req)
					if step.wantError != "" {
						if err == nil || !strings.Contains(err.Error(), step.wantError) {
							t.Fatalf("expected target status %s, got %v", step.wantError, err)
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						verify(req)
					}
					fixture.mu.Lock()
					defer fixture.mu.Unlock()
					wantWrites := step.wantWrites
					if step.name == "repeat" && target.repeatBinds {
						wantWrites = 1
					}
					if writes := fixture.writes - before; writes != wantWrites {
						t.Fatalf("target wrote %d certificate bindings or uploads, want %d", writes, wantWrites)
					}
					if (step.name == "repeat" || step.name == "write-recovery") && len(fixture.uploads) != beforeUploads {
						t.Fatal("target should reuse the existing uploaded certificate")
					}
				})
			}
			if target.provider == "cmcccloud-vlb" && os.Getenv("CERTIMATE_B1_BASELINE") != "1" {
				t.Run("missing-loadbalancer-id", func(t *testing.T) {
					invalid := *req
					invalid.ExtendedConfigJSON = `{"poolId":"CIDC-CORE-00","deployTarget":"listener","listenerId":"` + b6Listener + `"}`
					fixture.mu.Lock()
					fixture.mode = "auth-failure"
					fixture.mu.Unlock()
					defer func() {
						fixture.mu.Lock()
						fixture.mode = ""
						fixture.mu.Unlock()
					}()
					if _, err := deploy(ctx, &invalid); err == nil || !strings.Contains(err.Error(), "config `loadbalancerId` is required") {
						t.Fatalf("expected missing load balancer ID to fail before contacting the target, got %v", err)
					}
				})
			}
		})
	}
}

func (f *b6CloudTarget) serve(w http.ResponseWriter, r *http.Request) {
	if err := f.barrier.waitHTTP(r); err != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	reply := func(status int, payload any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}
	if f.mode == "auth-failure" {
		reply(http.StatusUnauthorized, map[string]any{"message": "fixture denied"})
		return
	}
	if strings.HasSuffix(r.Host, ".ctapi.ctyun.cn") {
		if !strings.HasPrefix(r.Header.Get("Eop-Authorization"), b6AccessKey+" Headers=ctyun-eop-request-id;eop-date Signature=") || len(r.Header.Get("Ctyun-Eop-Request-Id")) != 32 || r.Header.Get("Eop-Date") == "" {
			reply(http.StatusUnauthorized, map[string]any{"message": "missing CTCC signature"})
			return
		}
	}
	switch r.Host {
	case b6CMSHost:
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/certificate/list":
			var query struct {
				PageNum  int    `json:"pageNum"`
				PageSize int    `json:"pageSize"`
				Keyword  string `json:"keyword"`
				Origin   string `json:"origin"`
			}
			if err := json.NewDecoder(r.Body).Decode(&query); err != nil || query.PageNum < 1 || query.PageSize != 10 || query.Keyword != b6Domain || query.Origin != "UPLOAD" {
				reply(http.StatusBadRequest, map[string]any{"message": "unexpected CMS list query"})
				return
			}
			start := min((query.PageNum-1)*query.PageSize, len(f.certs))
			end := min(start+query.PageSize, len(f.certs))
			reply(http.StatusOK, map[string]any{"statusCode": 200, "returnObj": map[string]any{"list": f.certs[start:end], "totalSize": len(f.certs)}})
		case "POST /v1/certificate/upload":
			var input struct {
				b6Certificate
				Name               string `json:"name"`
				EncryptionStandard string `json:"encryptionStandard"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.EncryptionStandard != "INTERNATIONAL" || input.Name == "" || input.PrivateKey == "" || input.CertificateChain == "" {
				reply(http.StatusBadRequest, map[string]any{"message": "unexpected CMS certificate body"})
				return
			}
			if f.mode == "write-failure" {
				reply(http.StatusServiceUnavailable, map[string]any{"message": "fixture blocked upload"})
				return
			}
			cert, err := xcert.ParseCertificateFromPEM(input.Certificate)
			if err != nil {
				reply(http.StatusBadRequest, map[string]any{"message": "invalid CMS certificate"})
				return
			}
			f.current = input.b6Certificate
			f.writes++
			fingerprint := sha1.Sum(cert.Raw)
			f.certs = append(f.certs, map[string]any{
				"id": fmt.Sprintf("cms-%d", f.writes), "name": input.Name, "domainName": strings.Join(cert.DNSNames, ","),
				"issueTime": cert.NotBefore.Format(time.RFC3339), "expireTime": cert.NotAfter.Format(time.RFC3339),
				"fingerprint": hex.EncodeToString(fingerprint[:]),
			})
			reply(http.StatusOK, map[string]any{"statusCode": 200})
		default:
			reply(http.StatusNotFound, map[string]any{"message": "unexpected CMS request"})
		}
	case b6CDNHost:
		query := r.URL.Query()
		if query.Get("AccessKey") != b6AccessKey || query.Get("Signature") == "" || query.Get("SignatureMethod") != "HmacSHA256" {
			reply(http.StatusUnauthorized, map[string]any{"message": "missing CDN signature"})
			return
		}
		if f.provider == "cmcccloud-vlb" {
			f.serveCMCCVLB(r, reply)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/openapi-ecdn")
		switch r.Method + " " + path {
		case "GET /domainManager/openapi/domain/describeUserDomains":
			if query.Get("page") != "1" || query.Get("pageSize") != "10" {
				reply(http.StatusBadRequest, map[string]any{"message": "unexpected CDN list query"})
				return
			}
			reply(http.StatusOK, map[string]any{"body": map[string]any{"list": []map[string]any{
				{"domainName": b6Domain, "domainId": 41, "domainStatus": "RUNNING"},
				{"domainName": b6Domain, "domainId": 42, "domainStatus": "OFFLINE"},
				{"domainName": "other.fixture.example", "domainId": 43, "domainStatus": "RUNNING"},
			}}})
		case "GET /domainManager/openapi/domain/describeCdnDomainDetail/41":
			body := map[string]any{}
			if f.writes > 0 {
				body["crtUniqueId"] = "fixture-cert"
			}
			reply(http.StatusOK, map[string]any{"body": body})
		case "GET /domainManager/openapi/certificate/describeCdnCertificateDetail/fixture-cert":
			reply(http.StatusOK, map[string]any{"body": map[string]any{"certificate": f.current.Certificate}})
		case "POST /domainManager/openapi/certificate/addDomainServerCertificate":
			var input struct {
				b6Certificate
				DomainID int    `json:"domainId"`
				Name     string `json:"crtName"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.DomainID != 41 || input.Name == "" || input.Certificate == "" || input.PrivateKey == "" {
				reply(http.StatusBadRequest, map[string]any{"message": "unexpected CDN certificate body"})
				return
			}
			if f.mode == "write-failure" {
				reply(http.StatusServiceUnavailable, map[string]any{"message": "fixture blocked CDN binding"})
				return
			}
			f.current = input.b6Certificate
			f.writes++
			reply(http.StatusOK, map[string]any{"body": "fixture-cert"})
		default:
			reply(http.StatusNotFound, map[string]any{"message": "unexpected CDN request: " + r.Method + " " + r.URL.Path})
		}
	case b6AOHost, b6CTCDNHost, b6ICDNHost, b6LVDNHost:
		f.serveCTCCEdge(r, reply)
	case b6ELBHost:
		f.serveCTCCELB(r, reply)
	case b6FaaSHost:
		f.serveCTCCFaaS(r, reply)
	default:
		reply(http.StatusBadRequest, map[string]any{"message": "unexpected target host"})
	}
}

func startB6CloudTarget(t *testing.T, handler http.Handler) {
	t.Helper()
	ca, key, _ := makeCertificate(t, b6TargetHosts, true)
	pair, err := tls.X509KeyPair([]byte(ca), []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(path, []byte(ca), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", path)
	listener, err := net.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		t.Fatal(err)
	}
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: handler}, TLS: &tls.Config{
		Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12,
	}}
	server.StartTLS()
	t.Cleanup(server.Close)
}

func runB6CloudInLinux(t *testing.T, providers []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.Architecture}}").CombinedOutput()
	if err != nil {
		t.Fatalf("Docker is required for isolated B6 API fixtures: %v\n%s", err, out)
	}
	arch := strings.TrimSpace(string(out))
	switch arch {
	case "aarch64":
		arch = "arm64"
	case "x86_64":
		arch = "amd64"
	}
	dir := t.TempDir()
	baseline := os.Getenv("CERTIMATE_B1_BASELINE")
	pluginDir := os.Getenv("CERTIMATE_B6_PLUGIN_BINARIES")
	if baseline != "1" && pluginDir == "" {
		pluginDir = dir
		buildB4CloudPackages(t, pluginDir, providers, "linux", arch)
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "b6-cloud.test"), "./integration")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Linux integration test: %v\n%s", err, out)
	}
	binary, err := filepath.Abs(os.Getenv("CERTIMATE_B1_CORE_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	args := []string{"run", "--rm", "--network=none"}
	for _, host := range b6TargetHosts {
		args = append(args, "--add-host", host+":127.0.0.1")
	}
	if baseline != "1" {
		args = append(args, "--mount", "type=bind,src="+pluginDir+",dst=/packages,readonly")
	}
	args = append(args,
		"--mount", "type=bind,src="+dir+",dst=/fixtures,readonly",
		"--mount", "type=bind,src="+binary+",dst=/core,readonly",
		"-e", "CERTIMATE_B6_DOCKER=1",
		"-e", "CERTIMATE_B1_CORE_BINARY=/core",
		"-e", "CERTIMATE_B1_BASELINE="+baseline,
		image, "/fixtures/b6-cloud.test", "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=10m",
	)
	out, err = exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	t.Logf("Linux workflow output:\n%s", out)
	if err != nil {
		t.Fatalf("Linux workflow verification: %v", err)
	}
}
