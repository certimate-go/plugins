package integration

import (
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const (
	b8BytePlusLiveHost = "open.byteplusapi.com"
	b8VolcLiveHost     = "live.volcengineapi.com"
	b8VODSpace         = "vod-space-fixture"
)

func TestB8AVWorkflow(t *testing.T) {
	testB8CloudWorkflow(t, []b8CloudCase{
		{
			provider: "byteplus-medialive", certHost: b8BytePlusLiveHost, bindHost: b8BytePlusLiveHost,
			config:        `{"domainMatchPattern":"exact","domain":"` + b8FixtureDomain + `"}`,
			handleBinding: (*b8CloudFixture).serveB8Live,
			newFixture:    newB8BytePlusLiveFixture,
		},
		{
			provider: "volcengine-live", certHost: b8VolcLiveHost, bindHost: b8VolcLiveHost,
			config:        `{"domainMatchPattern":"certsan"}`,
			handleBinding: (*b8CloudFixture).serveB8Live,
		},
		{
			provider: "volcengine-vod", certHost: b8VolcCertHost, bindHost: b8VolcCertHost,
			config:        `{"region":"cn-north-1","spaceName":"` + b8VODSpace + `","domainType":"play","domainMatchPattern":"certsan"}`,
			reusesBinding: true,
			handleBinding: (*b8CloudFixture).serveB8VOD,
		},
	}, runB8BytePlusLoopbackInLinux)
}

func (f *b8CloudFixture) serveB8Live(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodPost || r.URL.Query().Get("Version") != "2023-01-01" {
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedLiveRoute")
		return
	}
	if f.mode == "auth-failure" {
		b8CloudError(w, http.StatusForbidden, "FixtureAccessDenied")
		return
	}
	switch action {
	case "ListCertV2":
		var input struct {
			ProjectName string
			PageNum     int
			PageSize    int
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.ProjectName != b8FixtureProject ||
			input.PageNum < 1 || input.PageSize != 10 {
			b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedLiveCertificateList")
			return
		}
		ids := make([]string, 0, len(f.certs))
		for id := range f.certs {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		start := min((input.PageNum-1)*input.PageSize, len(ids))
		end := min(start+input.PageSize, len(ids))
		certs := make([]map[string]string, 0, end-start)
		for _, id := range ids[start:end] {
			certs = append(certs, map[string]string{"ChainID": id, "CertName": id})
		}
		b8CloudReply(w, map[string]any{"CertList": certs, "Total": len(ids)})
	case "DescribeCertDetailSecretV2":
		var input struct{ ChainID string }
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedLiveCertificateLookup")
			return
		}
		cert, ok := f.certs[input.ChainID]
		if !ok {
			b8CloudError(w, http.StatusNotFound, "FixtureUnknownCertificate")
			return
		}
		b8CloudReply(w, map[string]any{"SSL": map[string]any{"Chain": []string{cert.CertificateChain}, "PrivateKey": cert.PrivateKey}})
	case "CreateCert":
		var input struct {
			ProjectName string
			CertName    string
			UseWay      string
			Rsa         struct {
				Prikey string
				Pubkey string
			}
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.ProjectName != b8FixtureProject ||
			input.CertName == "" || input.UseWay != "https" || input.Rsa.Pubkey == "" || input.Rsa.Prikey == "" {
			b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedLiveCertificate")
			return
		}
		id := f.storeCertificate(b8CloudCertificate{CertificateChain: input.Rsa.Pubkey, PrivateKey: input.Rsa.Prikey})
		b8CloudReply(w, map[string]any{"ChainID": id})
	case "ListDomainDetail":
		var input struct {
			DomainStatusList []int
			PageNum          int
			PageSize         int
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.PageNum != 1 || input.PageSize != 1000 ||
			!slices.Equal(input.DomainStatusList, []int{0}) {
			b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedLiveDomainList")
			return
		}
		b8CloudReply(w, map[string]any{"DomainList": b8AVDomains(), "Total": 2})
	case "BindCert":
		var input struct {
			ChainID string
			Domain  string
			HTTPS   bool
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Domain != b8FixtureDomain || !input.HTTPS {
			b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedLiveBinding")
			return
		}
		f.bindCertificate(w, input.ChainID)
	default:
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedLiveAction")
	}
}

func b8AVDomains() []map[string]string {
	return []map[string]string{{"Domain": b8FixtureDomain}, {"Domain": "unrelated.fixture.example"}}
}

func (f *b8CloudFixture) serveB8VOD(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodPost || r.URL.Query().Get("Version") != "2026-01-01" {
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedVODRoute")
		return
	}
	var input struct {
		SpaceName           string
		DomainType          string
		ListCdnDomainsParam struct {
			PageNum  int
			PageSize int
		}
		DescribeCdnDomainParam struct{ Domain string }
		UpdateCdnConfigParam   struct {
			Domain string
			HTTPS  struct {
				Switch   bool
				CertInfo struct{ CertID string }
			}
		}
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.SpaceName != b8VODSpace || input.DomainType != "vod_play" {
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedVODSpace")
		return
	}
	switch action {
	case "ListVodDomains":
		if input.ListCdnDomainsParam.PageNum != 1 || input.ListCdnDomainsParam.PageSize != 100 {
			b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedVODDomainList")
			return
		}
		b8CloudReply(w, map[string]any{"VodInfo": map[string]any{"Domains": b8AVDomains()}})
	case "DescribeVodDomainConfig":
		if input.DescribeCdnDomainParam.Domain != b8FixtureDomain {
			b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedVODDomain")
			return
		}
		b8CloudReply(w, map[string]any{"DomainInfo": map[string]any{"DomainConfig": map[string]any{"HTTPS": map[string]any{
			"Switch": f.bound != "", "CertInfo": map[string]string{"CertId": f.bound},
		}}}})
	case "UpdateVodDomainConfig":
		if input.UpdateCdnConfigParam.Domain != b8FixtureDomain || !input.UpdateCdnConfigParam.HTTPS.Switch {
			b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedVODBinding")
			return
		}
		f.bindCertificate(w, input.UpdateCdnConfigParam.HTTPS.CertInfo.CertID)
	default:
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedVODAction")
	}
}

func newB8BytePlusLiveFixture(t *testing.T, handler http.Handler) {
	t.Helper()
	addrs, err := net.LookupIP(b8BytePlusLiveHost)
	if err != nil || len(addrs) != 1 || !addrs[0].Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("fixture host %s is not isolated on loopback: %v, %v", b8BytePlusLiveHost, addrs, err)
	}
	ca, key, _ := makeCertificate(t, []string{b8BytePlusLiveHost}, true)
	pair, err := tls.X509KeyPair([]byte(ca), []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(caPath, []byte(ca), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", caPath)
	listener, err := net.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		t.Fatal(err)
	}
	server := &httptest.Server{
		Listener: listener, Config: &http.Server{Handler: handler},
		TLS: &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12},
	}
	server.StartTLS()
	t.Cleanup(server.Close)
}
