package integration

import (
	"net/http"
	"strings"
	"testing"
)

const (
	b8FixtureListenerID  = "b8-listener"
	b8FixtureExtensionID = "b8-target-extension"
)

func TestB8LoadBalancerWorkflow(t *testing.T) {
	const byteplusConfig = `{"region":"ap-singapore-1","deployTarget":"listener","listenerId":"` + b8FixtureListenerID + `"`
	const volcConfig = `{"region":"cn-beijing","deployTarget":"listener","listenerId":"` + b8FixtureListenerID + `"`
	const sniDomain = `,"domain":"` + b8FixtureDomain + `"}`
	testB8CloudWorkflow(t, []b8CloudCase{
		{provider: "byteplus-alb", config: byteplusConfig + sniDomain, certHost: b8BytePlusCertHost, bindHost: "alb.ap-singapore-1.byteplusapi.com"},
		{provider: "volcengine-alb", config: volcConfig + sniDomain, certHost: b8VolcCertHost, bindHost: b8VolcCertHost, reusesBinding: true},
		{provider: "byteplus-clb", config: byteplusConfig + "}", certHost: b8BytePlusCertHost, bindHost: "clb.ap-singapore-1.byteplusapi.com"},
		{provider: "volcengine-clb", config: volcConfig + sniDomain, certHost: b8VolcCertHost, bindHost: b8VolcCertHost, reusesBinding: true},
	}, runB4CloudInLinux)
}

func (f *b8CloudFixture) serveLoadBalancer(w http.ResponseWriter, r *http.Request, action string) {
	query := r.URL.Query()
	if query.Get("ListenerId") != b8FixtureListenerID {
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedListener")
		return
	}
	if action == "DescribeListenerAttributes" {
		b8CloudReply(w, map[string]any{
			"ListenerId": b8FixtureListenerID,
			"Protocol":   "HTTPS",
			"DomainExtensions": []map[string]string{
				{"Domain": b8FixtureDomain, "DomainExtensionId": b8FixtureExtensionID, "CertCenterCertificateId": f.bound},
				{"Domain": "other.example", "DomainExtensionId": "unrelated-extension", "CertCenterCertificateId": "unrelated-cert"},
			},
		})
		return
	}
	var certID, source string
	switch {
	case strings.HasSuffix(f.provider, "-alb") && action == "ModifyListenerAttributes":
		if query.Get("DomainExtensions.1.Domain") == b8FixtureDomain && query.Get("DomainExtensions.1.Action") == "modify" &&
			query.Get("DomainExtensions.1.DomainExtensionId") == b8FixtureExtensionID && !query.Has("DomainExtensions.2.DomainExtensionId") {
			certID = query.Get("DomainExtensions.1.CertCenterCertificateId")
			source = query.Get("DomainExtensions.1.CertificateSource")
		}
	case f.provider == "byteplus-clb" && action == "ModifyListenerAttributes":
		certID = query.Get("CertCenterCertificateId")
		source = query.Get("CertificateSource")
	case f.provider == "volcengine-clb" && action == "ModifyListenerDomainExtensions":
		if query.Get("ModifyDomainExtensions.1.DomainExtensionId") == b8FixtureExtensionID && !query.Has("ModifyDomainExtensions.2.DomainExtensionId") {
			certID = query.Get("ModifyDomainExtensions.1.CertCenterCertificateId")
			source = query.Get("ModifyDomainExtensions.1.CertificateSource")
		}
	}
	if source != "cert_center" || certID == "" {
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedListenerUpdate")
		return
	}
	f.bindCertificate(w, certID)
}
