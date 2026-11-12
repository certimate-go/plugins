package integration

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

const (
	b7AADHost    = "aad.myhuaweicloud.com"
	b7APIGHost   = "apig.cn-north-4.myhuaweicloud.com"
	b7CDNHost    = "cdn.myhuaweicloud.com"
	b7LiveHost   = "live.cn-north-4.myhuaweicloud.com"
	b7VODHost    = "vod.cn-north-4.myhuaweicloud.com"
	b7ELBHost    = "elb.cn-north-4.myhuaweicloud.com"
	b7WAFHost    = "waf.cn-north-4.myhuaweicloud.com"
	b7IAMHost    = "iam.cn-north-4.myhuaweicloud.com"
	b7SCMHost    = "scm.cn-north-4.myhuaweicloud.com"
	b7OBSHost    = "fixture-bucket.obs.cn-north-4.myhuaweicloud.com"
	b7Domain     = "fixture.example"
	b7Project    = "fixture-project"
	b7Key        = "fixture-huawei-access"
	b7IAMProject = "fixture-huawei-project"
)

type b7Certificate struct {
	Name        string `json:"name"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
	Project     string `json:"enterprise_project_id"`
}

type b7SCMEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Domain  string `json:"domain"`
	Expires string `json:"expire_time"`
}

type b7OBSBinding struct {
	XMLName          xml.Name `xml:"CustomDomainConfiguration"`
	Name             string
	CertificateID    string `xml:"CertificateId"`
	Certificate      string
	CertificateChain string
	PrivateKey       string
}

type b7HuaweiTarget struct {
	mu          sync.Mutex
	barrier     workflowRequestBarrier
	certs       []b7Certificate
	exports     int
	binding     b7OBSBinding
	boundID     string
	replacement b7Certificate
	bindings    int
	rejectAuth  bool
	rejectBind  bool
}

func TestB7HuaweiWorkflow(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for workflow verification")
	}
	cases := []struct {
		provider  string
		config    string
		host      string
		usesStore bool
	}{
		{"huaweicloud-scm", "{}", b7SCMHost, true},
		{"huaweicloud-obs", `{"region":"cn-north-4","bucket":"fixture-bucket","domain":"fixture.example"}`, b7OBSHost, true},
		{"huaweicloud-aad", `{"region":"cn-north-4","instanceId":"fixture-instance","domainMatchPattern":"exact","domain":"fixture.example"}`, b7AADHost, false},
		{"huaweicloud-apig", `{"region":"cn-north-4","deployTarget":"certificate","certificateId":"fixture-cert"}`, b7APIGHost, false},
		{"huaweicloud-cdn", `{"region":"cn-north-1","domainMatchPattern":"exact","domain":"fixture.example"}`, b7CDNHost, true},
		{"huaweicloud-live", `{"region":"cn-north-4","domainMatchPattern":"exact","domain":"fixture.example"}`, b7LiveHost, true},
		{"huaweicloud-vod", `{"region":"cn-north-4","domainMatchPattern":"exact","domain":"fixture.example"}`, b7VODHost, true},
		{"huaweicloud-elb", `{"region":"cn-north-4","deployTarget":"certificate","certificateId":"fixture-cert"}`, b7ELBHost, false},
		{"huaweicloud-waf", `{"region":"cn-north-4","deployTarget":"cloudserver","domain":"fixture.example"}`, b7WAFHost, true},
	}
	providers := make([]string, 0, len(cases))
	hosts := []string{b7IAMHost, "iam.cn-north-1.myhuaweicloud.com"}
	for _, target := range cases {
		providers = append(providers, target.provider)
		hosts = append(hosts, target.host)
	}
	if os.Getenv("CERTIMATE_B7_DOCKER") != "1" {
		runB7HuaweiInLinux(t, providers, hosts)
		return
	}

	dir := "/plugins"
	for _, target := range cases {
		t.Run(target.provider, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			f := &b7HuaweiTarget{}
			binds := target.host != b7SCMHost
			startB7HuaweiTarget(t, hosts, http.HandlerFunc(f.serve))
			verify := func(req *plugin.DeployRequest) {
				t.Helper()
				f.mu.Lock()
				defer f.mu.Unlock()
				cert := f.replacement
				if target.usesStore {
					if len(f.certs) == 0 {
						t.Fatal("certificate store did not receive the certificate")
					}
					cert = f.certs[len(f.certs)-1]
					if binds && f.boundID != strconv.Itoa(len(f.certs)) {
						t.Fatal("target did not bind the uploaded certificate")
					}
				}
				if !b7SamePEM(cert.Certificate, req.CertificatePEM) || !b7SamePEM(cert.PrivateKey, req.PrivateKeyPEM) {
					t.Fatal("target did not store the supplied chain and private key")
				}
				if target.host == b7OBSHost && (f.binding.Name != cert.Name || !b7SamePEM(f.binding.Certificate, req.CertificatePEM) ||
					!b7SamePEM(f.binding.CertificateChain, req.CertificatePEM) ||
					!b7SamePEM(f.binding.PrivateKey, req.PrivateKeyPEM)) {
					t.Fatal("OBS did not bind the supplied chain and key")
				}
			}
			deploy := newB1WorkflowDeployer(t, dir, target.provider, workflowLifecycleTarget{barrier: &f.barrier, verify: verify})
			req := &plugin.DeployRequest{
				AccessConfigJSON:   `{"accessKeyId":"fixture-huawei-access","secretAccessKey":"fixture-huawei-secret","enterpriseProjectId":"fixture-project"}`,
				ExtendedConfigJSON: target.config,
			}
			issuer, _, _ := makeCertificate(t, []string{"fixture-issuer.example"}, true)
			type step struct {
				name     string
				fresh    bool
				authFail bool
				bindFail bool
				imports  int
				binds    int
			}
			steps := []step{
				{name: "initial", fresh: true, imports: 1, binds: 1},
				{name: "repeat", binds: 1},
				{name: "renew", fresh: true, imports: 1, binds: 1},
				{name: "auth-failure", authFail: true},
				{name: "auth-recovery", binds: 1},
			}
			if target.host == b7OBSHost {
				steps = append(steps,
					step{name: "binding-failure", fresh: true, bindFail: true, imports: 1},
					step{name: "binding-recovery", binds: 1})
			}
			for _, step := range steps {
				passed := t.Run(step.name, func(t *testing.T) {
					if step.fresh {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{b7Domain}, false)
						req.CertificatePEM += issuer
					}
					f.mu.Lock()
					f.rejectAuth, f.rejectBind = step.authFail, step.bindFail
					imports, exports, bindings := len(f.certs), f.exports, f.bindings
					f.mu.Unlock()
					_, err := deploy(ctx, req)
					switch {
					case step.authFail:
						if err == nil || !strings.Contains(err.Error(), "B7_DENIED") {
							t.Fatalf("expected target authentication failure, got %v", err)
						}
					case step.bindFail:
						if err == nil || !strings.Contains(err.Error(), "B7_BIND_REJECTED") {
							t.Fatalf("expected OBS binding failure, got %v", err)
						}
					default:
						if err != nil {
							t.Fatal(err)
						}
						verify(req)
					}
					f.mu.Lock()
					defer f.mu.Unlock()
					wantBinds := step.binds
					if !binds || (target.host == b7VODHost && !step.fresh) {
						wantBinds = 0
					}
					wantImports := step.imports
					if !target.usesStore {
						wantImports = 0
					}
					if len(f.certs)-imports != wantImports || f.bindings-bindings != wantBinds {
						t.Fatalf("certificate uploads=%d and target bindings=%d, want %d and %d", len(f.certs)-imports, f.bindings-bindings, wantImports, wantBinds)
					}
					if target.usesStore && step.name == "repeat" && f.exports <= exports {
						t.Fatal("repeat deployment did not verify the existing certificate")
					}
				})
				if !passed {
					return
				}
			}
		})
	}
}

func (f *b7HuaweiTarget) serve(w http.ResponseWriter, r *http.Request) {
	if err := f.barrier.waitHTTP(r); err != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Host {
	case b7IAMHost, "iam.cn-north-1.myhuaweicloud.com":
		serveB7IAM(w, r)
	case b7OBSHost:
		f.serveOBS(w, r)
	case b7SCMHost:
		f.serveSCM(w, r)
	case b7AADHost, b7APIGHost, b7CDNHost, b7LiveHost, b7VODHost, b7ELBHost, b7WAFHost:
		f.serveProduct(w, r)
	default:
		http.Error(w, "unexpected Huawei target", http.StatusBadRequest)
	}
}

func serveB7IAM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !strings.Contains(r.Header.Get("Authorization"), "Access="+b7Key+",") {
		http.Error(w, "unexpected IAM request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/v3/projects":
		region := r.URL.Query().Get("name")
		if region != "cn-north-4" && region != "cn-north-1" {
			http.Error(w, "unexpected IAM project region", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]any{
			{"id": b7IAMProject, "name": region, "domain_id": "fixture-domain", "parent_id": "fixture-domain", "enabled": true},
		}})
	case "/v3/auth/domains":
		_, _ = fmt.Fprint(w, `{"domains":[{"id":"fixture-domain","name":"fixture-domain","enabled":true}]}`)
	default:
		http.Error(w, "unexpected IAM path "+r.URL.Path, http.StatusBadRequest)
	}
}

func (f *b7HuaweiTarget) serveSCM(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	send := func(body any) { _ = json.NewEncoder(w).Encode(body) }
	if f.rejectAuth || !strings.Contains(r.Header.Get("Authorization"), "Access="+b7Key+",") {
		w.WriteHeader(http.StatusUnauthorized)
		send(map[string]string{"error_code": "B7_DENIED", "error_msg": "fixture credentials rejected"})
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v3/scm/certificates":
		if r.URL.Query().Get("enterprise_project_id") != b7Project {
			http.Error(w, "incorrect SCM enterprise project", http.StatusBadRequest)
			return
		}
		entries := make([]b7SCMEntry, 0, len(f.certs))
		for i, stored := range f.certs {
			cert, err := xcert.ParseCertificateFromPEM(stored.Certificate)
			if err != nil {
				http.Error(w, "invalid stored certificate", http.StatusInternalServerError)
				return
			}
			entries = append(entries, b7SCMEntry{strconv.Itoa(i + 1), stored.Name, cert.Subject.CommonName, cert.NotAfter.Local().Format(time.DateTime)})
		}
		send(map[string]any{"certificates": entries, "total_count": len(entries)})
	case r.Method == http.MethodPost && r.URL.Path == "/v3/scm/certificates/import":
		var cert b7Certificate
		if json.NewDecoder(r.Body).Decode(&cert) != nil || cert.Name == "" || cert.Certificate == "" || cert.PrivateKey == "" || cert.Project != b7Project {
			http.Error(w, "invalid SCM certificate import", http.StatusBadRequest)
			return
		}
		f.certs = append(f.certs, cert)
		send(map[string]string{"certificate_id": strconv.Itoa(len(f.certs))})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/export"):
		var id int
		if _, err := fmt.Sscanf(r.URL.Path, "/v3/scm/certificates/%d/export", &id); err != nil || id < 1 || id > len(f.certs) {
			http.NotFound(w, r)
			return
		}
		f.exports++
		send(map[string]string{"certificate": f.certs[id-1].Certificate})
	default:
		http.Error(w, "unexpected SCM request", http.StatusBadRequest)
	}
}

func (f *b7HuaweiTarget) serveOBS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut || r.URL.Path != "/" || r.URL.Query().Get("customdomain") != b7Domain ||
		!strings.HasPrefix(r.Header.Get("Authorization"), "OBS "+b7Key+":") {
		http.Error(w, "unexpected OBS binding request", http.StatusBadRequest)
		return
	}
	if f.rejectBind {
		http.Error(w, "B7_BIND_REJECTED", http.StatusUnprocessableEntity)
		return
	}
	var binding b7OBSBinding
	if err := xml.NewDecoder(r.Body).Decode(&binding); err != nil {
		http.Error(w, "invalid OBS binding XML", http.StatusBadRequest)
		return
	}
	f.binding = binding
	f.boundID = binding.CertificateID
	f.bindings++
}

func b7SamePEM(got, want string) bool {
	return strings.TrimSpace(got) == strings.TrimSpace(want)
}

func startB7HuaweiTarget(t *testing.T, hosts []string, handler http.Handler) {
	t.Helper()
	ca, key, _ := makeCertificate(t, hosts, true)
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

func runB7HuaweiInLinux(t *testing.T, providers, hosts []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.Architecture}}").CombinedOutput()
	if err != nil {
		t.Fatalf("Docker is required for isolated B7 API fixtures: %v\n%s", err, out)
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
	binaries := dir
	if baseline != "1" {
		if prebuilt := os.Getenv("CERTIMATE_B7_PLUGIN_BINARIES"); prebuilt != "" {
			binaries, err = filepath.Abs(prebuilt)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			buildB4CloudPackages(t, binaries, providers, "linux", arch)
		}
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "b7-huawei.test"), "./integration")
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
	for _, host := range hosts {
		args = append(args, "--add-host", host+":127.0.0.1")
	}
	args = append(args,
		"--mount", "type=bind,src="+dir+",dst=/fixtures,readonly",
		"--mount", "type=bind,src="+binaries+",dst=/plugins,readonly",
		"--mount", "type=bind,src="+binary+",dst=/core,readonly",
		"-e", "CERTIMATE_B7_DOCKER=1",
		"-e", "CERTIMATE_B1_CORE_BINARY=/core",
		"-e", "CERTIMATE_B1_BASELINE="+baseline,
		image, "/fixtures/b7-huawei.test", "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=10m",
	)
	out, err = exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	t.Logf("Linux workflow output:\n%s", out)
	if err != nil {
		t.Fatalf("Linux workflow verification: %v", err)
	}
}
