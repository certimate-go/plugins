package integration

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
)

const (
	b8EdgeImageXHost = "imagex.volcengineapi.com"
	b8EdgeServiceID  = "fixture-image-service"
	b8EdgeWAFVPC     = "fixture-vpc"
)

type b8EdgeHTTPS struct {
	CertID        string   `json:"cert_id,omitempty"`
	Enabled       bool     `json:"enable_https"`
	HTTP2         bool     `json:"enable_http2"`
	OCSP          bool     `json:"enable_ocsp"`
	ForceRedirect bool     `json:"enable_force_redirect"`
	RedirectCode  string   `json:"force_redirect_code"`
	RedirectType  string   `json:"force_redirect_type"`
	TLSVersions   []string `json:"tls_versions"`
}

type b8EdgeWAFBackend struct {
	IP       string
	Port     int
	Protocol string
	Weight   int
}

type b8EdgeWAFGroup struct {
	Name       string
	AccessPort []int
	Backends   []b8EdgeWAFBackend
}

func TestB8EdgeWorkflow(t *testing.T) {
	testB8CloudWorkflow(t, []b8CloudCase{
		{
			provider:      "volcengine-dcdn",
			config:        `{"domainMatchPattern":"certsan"}`,
			certHost:      b8VolcCertHost,
			bindHost:      b8VolcCertHost,
			handleBinding: (*b8CloudFixture).serveDCDN,
		},
		{
			provider:      "volcengine-imagex",
			config:        `{"region":"cn-north-1","serviceId":"` + b8EdgeServiceID + `","domain":"` + b8FixtureDomain + `"}`,
			certHost:      b8VolcCertHost,
			bindHost:      b8EdgeImageXHost,
			handleBinding: (*b8CloudFixture).serveImageX,
		},
		{
			provider:      "volcengine-waf",
			config:        `{"region":"cn-beijing","accessMode":"cname","domain":"` + b8FixtureDomain + `"}`,
			certHost:      b8VolcCertHost,
			bindHost:      b8VolcCertHost,
			handleBinding: (*b8CloudFixture).serveWAF,
		},
	}, runB4CloudInLinux)
}

func (f *b8CloudFixture) serveDCDN(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method == http.MethodPost {
		switch action {
		case "ListDomainConfig":
			var body struct {
				ProjectName []string
				PageNumber  int
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.PageNumber != 1 || !slices.Equal(body.ProjectName, []string{b8FixtureProject}) {
				break
			}
			b8CloudReply(w, map[string]any{"DomainList": []map[string]string{
				{"Domain": b8FixtureDomain, "Status": "Online"},
				{"Domain": b8FixtureDomain, "Status": "Stop"},
				{"Domain": "unrelated.fixture.example", "Status": "Online"},
			}})
			return
		case "CreateCertBind":
			var body struct {
				CertID      string
				CertSource  string
				DomainNames []string
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.CertSource != "volc" || !slices.Equal(body.DomainNames, []string{b8FixtureDomain}) {
				break
			}
			f.bindCertificate(w, body.CertID)
			return
		}
	}
	b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedDCDN"+action)
}

func (f *b8CloudFixture) serveImageX(w http.ResponseWriter, r *http.Request, action string) {
	query := r.URL.Query()
	https := b8EdgeHTTPS{
		Enabled: true, HTTP2: true, OCSP: true, ForceRedirect: true,
		RedirectCode: "301", RedirectType: "http_to_https", TLSVersions: []string{"tlsv1.2", "tlsv1.3"},
	}
	switch {
	case action == "GetDomainConfig" && r.Method == http.MethodGet && query.Get("ServiceId") == b8EdgeServiceID && query.Get("DomainName") == b8FixtureDomain:
		b8CloudReply(w, map[string]any{"https_config": https})
		return
	case action == "UpdateHttps" && r.Method == http.MethodPost && query.Get("ServiceId") == b8EdgeServiceID:
		var body struct {
			Domain string
			HTTPS  b8EdgeHTTPS
		}
		if json.NewDecoder(r.Body).Decode(&body) == nil && body.Domain == b8FixtureDomain {
			actual := body.HTTPS
			if actual.Enabled == https.Enabled && actual.HTTP2 == https.HTTP2 && actual.OCSP == https.OCSP &&
				actual.ForceRedirect == https.ForceRedirect && actual.RedirectCode == https.RedirectCode &&
				actual.RedirectType == https.RedirectType && slices.Equal(actual.TLSVersions, https.TLSVersions) {
				f.bindCertificateResult(w, actual.CertID, "success")
				return
			}
		}
	}
	b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedImageX"+action)
}

func (f *b8CloudFixture) serveWAF(w http.ResponseWriter, r *http.Request, action string) {
	group := b8EdgeWAFGroup{
		Name: "fixture-backend", AccessPort: []int{8080},
		Backends: []b8EdgeWAFBackend{{IP: "192.0.2.45", Port: 8080, Protocol: "HTTP", Weight: 7}},
	}
	if r.Method == http.MethodPost {
		switch action {
		case "ListDomain":
			var body struct {
				Domain        string
				ProjectName   string
				Region        string
				AccurateQuery int
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Domain != b8FixtureDomain ||
				body.ProjectName != b8FixtureProject || body.Region != "cn-beijing" || body.AccurateQuery != 1 {
				break
			}
			b8CloudReply(w, map[string]any{"Data": []map[string]any{{
				"Domain": b8FixtureDomain, "LBAlgorithm": "round_robin", "VpcID": b8EdgeWAFVPC, "BackendGroups": []b8EdgeWAFGroup{group},
				"Protocols": "HTTP", "ProtocolPorts": map[string][]int{"HTTP": {8080}},
			}}})
			return
		case "UpdateDomain":
			var body struct {
				Domain              string
				ProjectName         string
				Region              string
				AccessMode          int
				LBAlgorithm         string
				VpcID               string
				BackendGroups       []b8EdgeWAFGroup
				Protocols           []string
				ProtocolPorts       struct{ HTTP, HTTPS []int }
				VolcCertificateID   string
				CertificatePlatform string
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Domain != b8FixtureDomain || body.ProjectName != b8FixtureProject ||
				body.Region != "cn-beijing" || body.AccessMode != 10 || body.LBAlgorithm != "round_robin" ||
				body.VpcID != b8EdgeWAFVPC || body.CertificatePlatform != "certificate-service" ||
				!slices.Equal(body.Protocols, []string{"HTTP", "HTTPS"}) ||
				!slices.Equal(body.ProtocolPorts.HTTP, []int{8080}) || !slices.Equal(body.ProtocolPorts.HTTPS, []int{443}) ||
				len(body.BackendGroups) != 1 || body.BackendGroups[0].Name != group.Name ||
				!slices.Equal(body.BackendGroups[0].AccessPort, group.AccessPort) ||
				!slices.Equal(body.BackendGroups[0].Backends, group.Backends) {
				break
			}
			f.bindCertificate(w, body.VolcCertificateID)
			return
		}
	}
	b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedWAF"+action)
}
