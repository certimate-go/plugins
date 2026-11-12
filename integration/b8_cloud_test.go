package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

const (
	b8FixtureKeyID     = "b8-fixture-access-key"
	b8FixtureSecret    = "b8-fixture-access-secret"
	b8FixtureDomain    = "b8.fixture.example"
	b8FixtureDomainID  = "apig-domain-fixture"
	b8FixtureProject   = "b8-fixture-project"
	b8FixtureBucket    = "bucket-fixture"
	b8VolcCertHost     = "open.volcengineapi.com"
	b8BytePlusCertHost = "certificate-service.byteplusapi.com"
)

type b8CloudCase struct {
	provider      string
	config        string
	certHost      string
	bindHost      string
	reusesBinding bool
	handleBinding func(*b8CloudFixture, http.ResponseWriter, *http.Request, string)
	newFixture    func(*testing.T, http.Handler)
}

type b8CloudCertificate struct {
	CertificateChain string
	PrivateKey       string
}

type b8CloudFixture struct {
	b8CloudCase
	mu       sync.Mutex
	barrier  workflowRequestBarrier
	mode     string
	certs    map[string]b8CloudCertificate
	bound    string
	uploaded string
	writes   int
}

func TestB8CloudWorkflow(t *testing.T) {
	cases := []b8CloudCase{
		{provider: "byteplus-apig", config: `{"region":"cn-beijing","domainMatchPattern":"exact","domain":"` + b8FixtureDomain + `"}`, certHost: b8VolcCertHost, bindHost: "open.volcengineapi.com"},
		{provider: "volcengine-apig", config: `{"region":"cn-beijing","domainMatchPattern":"certsan"}`, certHost: b8VolcCertHost, bindHost: "open.volcengineapi.com"},
		{provider: "byteplus-certcenter", config: `{"region":"ap-singapore-1"}`, certHost: b8BytePlusCertHost, bindHost: ""},
		{provider: "volcengine-certcenter", config: `{"region":"cn-beijing"}`, certHost: b8VolcCertHost, bindHost: ""},
		{provider: "byteplus-tos", config: `{"region":"ap-singapore-1","bucket":"` + b8FixtureBucket + `","domain":"` + b8FixtureDomain + `"}`, certHost: b8BytePlusCertHost, bindHost: b8FixtureBucket + ".tos-ap-singapore-1.bytepluses.com"},
		{provider: "volcengine-tos", config: `{"region":"cn-beijing","bucket":"` + b8FixtureBucket + `","domain":"` + b8FixtureDomain + `"}`, certHost: b8VolcCertHost, bindHost: b8FixtureBucket + ".tos-cn-beijing.volces.com"},
	}
	testB8CloudWorkflow(t, cases, runB4CloudInLinux)
}

func testB8CloudWorkflow(t *testing.T, cases []b8CloudCase, runInLinux func(*testing.T, []string)) {
	t.Helper()
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for workflow verification")
	}
	providers := make([]string, 0, len(cases))
	for _, target := range cases {
		providers = append(providers, target.provider)
	}
	if runtime.GOOS != "linux" {
		runInLinux(t, providers)
		return
	}
	dir := os.Getenv("CERTIMATE_B4_PLUGIN_BINARIES")
	if dir == "" && os.Getenv("CERTIMATE_B1_BASELINE") != "1" {
		dir = t.TempDir()
		buildB4CloudPackages(t, dir, providers, runtime.GOOS, runtime.GOARCH)
	}

	for _, target := range cases {
		t.Run(target.provider, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			fixture := &b8CloudFixture{b8CloudCase: target, certs: make(map[string]b8CloudCertificate)}
			if target.newFixture != nil {
				target.newFixture(t, http.HandlerFunc(fixture.serve))
			} else {
				hosts := []string{target.certHost}
				if target.bindHost != "" && target.bindHost != target.certHost {
					hosts = append(hosts, target.bindHost)
				}
				newB4CloudProxy(t, hosts, http.HandlerFunc(fixture.serve))
				t.Setenv("VOLC_HTTP_PROXY", os.Getenv("HTTP_PROXY"))
				t.Setenv("VOLC_HTTPS_PROXY", os.Getenv("HTTPS_PROXY"))
				t.Setenv("VOLC_NO_PROXY", os.Getenv("NO_PROXY"))
			}
			verify := func(request *plugin.DeployRequest) {
				t.Helper()
				fixture.mu.Lock()
				defer fixture.mu.Unlock()
				id := fixture.bound
				if target.bindHost == "" {
					id = fixture.uploaded
				}
				stored := fixture.certs[id]
				if stored.CertificateChain != request.CertificatePEM || stored.PrivateKey != request.PrivateKeyPEM {
					t.Fatalf("%s did not store the expected certificate chain and key on its target", target.provider)
				}
			}
			deploy := newB1WorkflowDeployer(t, dir, target.provider, workflowLifecycleTarget{barrier: &fixture.barrier, verify: verify})
			request := &plugin.DeployRequest{
				AccessConfigJSON:   `{"accessKeyId":"` + b8FixtureKeyID + `","secretAccessKey":"` + b8FixtureSecret + `","projectName":"` + b8FixtureProject + `"}`,
				ExtendedConfigJSON: target.config,
				LogLevel:           "DEBUG",
			}
			issuer, _, _ := makeCertificate(t, []string{"b8-issuer.fixture.example"}, true)
			type step struct {
				name      string
				fresh     bool
				wantError string
				newCerts  int
				bindings  int
			}
			steps := []step{
				{name: "initial", fresh: true, newCerts: 1, bindings: 1},
				{name: "repeat", bindings: 1},
				{name: "renew", fresh: true, newCerts: 1, bindings: 1},
				{name: "auth-failure", wantError: "FixtureAccessDenied"},
				{name: "auth-recovery", bindings: 1},
				{name: "binding-failure", fresh: true, wantError: "FixtureBindingDenied", newCerts: 1},
				{name: "binding-recovery", bindings: 1},
			}
			if target.reusesBinding {
				for i := range steps {
					if steps[i].name == "repeat" || steps[i].name == "auth-recovery" {
						steps[i].bindings = 0
					}
				}
			}
			if target.bindHost == "" {
				steps = steps[:5]
				for i := range steps {
					steps[i].bindings = 0
				}
			}
			if target.provider == "byteplus-cdn" && os.Getenv("CERTIMATE_B1_BASELINE") != "1" {
				steps = append(steps,
					step{name: "redirect", fresh: true, wantError: "BytePlus CDN API redirects are not allowed"},
					step{name: "redirect-recovery", newCerts: 1, bindings: 1},
				)
			}
			for _, step := range steps {
				t.Run(step.name, func(t *testing.T) {
					if step.fresh {
						request.CertificatePEM, request.PrivateKeyPEM, _ = makeCertificate(t, []string{b8FixtureDomain}, false)
						request.CertificatePEM += issuer
					}
					fixture.mu.Lock()
					fixture.mode = step.name
					initialCerts, initialWrites, initialBound := len(fixture.certs), fixture.writes, fixture.bound
					fixture.mu.Unlock()

					_, err := deploy(ctx, request)
					if step.wantError == "" {
						if err != nil {
							t.Fatal(err)
						}
						verify(request)
					} else if err == nil || !strings.Contains(err.Error(), step.wantError) {
						t.Fatalf("expected error %s, got %v", step.wantError, err)
					}

					fixture.mu.Lock()
					defer fixture.mu.Unlock()
					if certs, writes := len(fixture.certs)-initialCerts, fixture.writes-initialWrites; certs != step.newCerts || writes != step.bindings {
						t.Fatalf("target recorded %d new certificates and %d bindings, want %d and %d", certs, writes, step.newCerts, step.bindings)
					}
					if step.wantError != "" && fixture.bound != initialBound {
						t.Fatal("failed workflow changed the bound certificate")
					}
				})
			}
			fixture.mu.Lock()
			fixture.mode = ""
			fixture.mu.Unlock()
		})
	}
}

func (f *b8CloudFixture) serve(w http.ResponseWriter, r *http.Request) {
	if err := f.barrier.waitHTTP(r); err != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != "/" || !strings.Contains(r.Header.Get("Authorization"), "Credential="+b8FixtureKeyID+"/") {
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedRequest")
		return
	}
	action := r.URL.Query().Get("Action")
	switch {
	case r.Host == f.certHost && r.Method == http.MethodPost && action == "ImportCertificate":
		f.importCertificate(w, r)
	case r.Host == f.bindHost && f.handleBinding != nil:
		f.handleBinding(f, w, r, action)
	case r.Host == f.bindHost && r.Method == http.MethodGet && (strings.HasSuffix(f.provider, "-alb") || strings.HasSuffix(f.provider, "-clb")):
		f.serveLoadBalancer(w, r, action)
	case r.Host == f.bindHost && r.Method == http.MethodPost && strings.HasSuffix(f.provider, "-cdn"):
		f.serveCDN(w, r, action)
	case r.Host == f.bindHost && r.Method == http.MethodPost && strings.HasSuffix(f.provider, "-apig"):
		f.serveAPIG(w, r, action)
	case r.Host == f.bindHost && r.Method == http.MethodPut && r.URL.Query().Has("customdomain") && strings.HasSuffix(f.provider, "-tos"):
		var input struct {
			CustomDomainRule struct {
				Domain string
				CertID string
			}
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil ||
			input.CustomDomainRule.Domain != b8FixtureDomain ||
			!strings.HasPrefix(r.Header.Get("Authorization"), "TOS4-HMAC-SHA256 ") ||
			!strings.Contains(r.Header.Get("Authorization"), "/tos/request") {
			b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedBinding")
			return
		}
		f.bindCertificate(w, input.CustomDomainRule.CertID)
	default:
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedRoute")
	}
}

func (f *b8CloudFixture) importCertificate(w http.ResponseWriter, r *http.Request) {
	if f.mode == "auth-failure" {
		b8CloudError(w, http.StatusForbidden, "FixtureAccessDenied")
		return
	}
	var input struct {
		ProjectName     string
		Repeatable      *bool
		CertificateInfo b8CloudCertificate
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil ||
		input.ProjectName != b8FixtureProject || input.Repeatable == nil || *input.Repeatable ||
		input.CertificateInfo.CertificateChain == "" || input.CertificateInfo.PrivateKey == "" {
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedCertificate")
		return
	}
	for id, existing := range f.certs {
		if existing == input.CertificateInfo {
			f.uploaded = id
			b8CloudReply(w, map[string]any{"RepeatId": id})
			return
		}
	}
	id := f.storeCertificate(input.CertificateInfo)
	b8CloudReply(w, map[string]any{"InstanceId": id})
}

func (f *b8CloudFixture) storeCertificate(certificate b8CloudCertificate) string {
	id := fmt.Sprintf("cert-%d", len(f.certs)+1)
	f.certs[id] = certificate
	f.uploaded = id
	return id
}

func (f *b8CloudFixture) serveAPIG(w http.ResponseWriter, r *http.Request, action string) {
	var input struct {
		ID            string
		CertificateID string
		Protocol      []string
		PageNumber    int
		PageSize      int
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		b8CloudError(w, http.StatusBadRequest, "FixtureInvalidAPIGRequest")
		return
	}
	switch {
	case action == "ListCustomDomains" && input.PageNumber == 1 && input.PageSize == 100:
		b8CloudReply(w, map[string]any{"Items": []map[string]string{
			{"Id": b8FixtureDomainID, "Domain": b8FixtureDomain, "Status": "Available"},
			{"Id": "not-a-target", "Domain": "other.fixture.example", "Status": "Available"},
		}})
	case action == "GetCustomDomain" && input.ID == b8FixtureDomainID:
		b8CloudReply(w, map[string]any{"CustomDomain": map[string]any{"Protocol": []string{"HTTP"}}})
	case action == "UpdateCustomDomain" && input.ID == b8FixtureDomainID && slices.Equal(input.Protocol, []string{"HTTP", "HTTPS"}):
		f.bindCertificate(w, input.CertificateID)
	default:
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedAPIGRequest")
	}
}

func (f *b8CloudFixture) bindCertificate(w http.ResponseWriter, id string) {
	f.bindCertificateResult(w, id, map[string]any{})
}

func (f *b8CloudFixture) bindCertificateResult(w http.ResponseWriter, id string, result any) {
	if f.mode == "binding-failure" {
		b8CloudError(w, http.StatusForbidden, "FixtureBindingDenied")
		return
	}
	if _, ok := f.certs[id]; !ok {
		b8CloudError(w, http.StatusBadRequest, "FixtureUnknownCertificate")
		return
	}
	f.bound = id
	f.writes++
	b8CloudReply(w, result)
}

func b8CloudReply(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ResponseMetadata": map[string]string{"RequestId": "b8-fixture"},
		"Result":           result,
	})
}

func b8CloudError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ResponseMetadata": map[string]any{
			"RequestId": "b8-fixture",
			"Error":     map[string]string{"Code": code, "Message": code},
		},
	})
}
