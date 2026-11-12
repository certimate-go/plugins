package integration

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const b8BytePlusCDNHost = "open.byteplusapi.com"

func TestB8CDNWorkflow(t *testing.T) {
	testB8CloudWorkflow(t, []b8CloudCase{
		{provider: "byteplus-cdn", config: `{"domainMatchPattern":"wildcard","domain":"*.fixture.example"}`, certHost: b8BytePlusCDNHost, bindHost: b8BytePlusCDNHost, newFixture: newB8BytePlusCDNFixture},
		{provider: "volcengine-cdn", config: `{"domainMatchPattern":"certsan"}`, certHost: b8VolcCertHost, bindHost: b8VolcCertHost},
	}, runB8BytePlusLoopbackInLinux)
}

func (f *b8CloudFixture) serveCDN(w http.ResponseWriter, r *http.Request, action string) {
	var input struct {
		Source      string
		Domain      string
		Project     string
		Status      string
		PageNum     int
		PageSize    int
		CertID      string
		Certificate string
		PrivateKey  string
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		b8CloudError(w, http.StatusBadRequest, "FixtureInvalidCDNRequest")
		return
	}
	source := "volc_cert_center"
	if strings.HasPrefix(f.provider, "byteplus-") {
		source = "cert_center"
	}
	switch {
	case action == "ListCertInfo" && input.Source == source && input.PageNum == 1 && input.PageSize == 100:
		if f.mode == "auth-failure" {
			b8CloudError(w, http.StatusForbidden, "FixtureAccessDenied")
			return
		}
		certificates := make([]map[string]any, 0, len(f.certs))
		for id, cert := range f.certs {
			block, _ := pem.Decode([]byte(cert.CertificateChain))
			if block == nil || block.Type != "CERTIFICATE" {
				b8CloudError(w, http.StatusInternalServerError, "FixtureStoredInvalidCertificate")
				return
			}
			sha1Digest, sha256Digest := sha1.Sum(block.Bytes), sha256.Sum256(block.Bytes)
			certificates = append(certificates, map[string]any{
				"CertId": id,
				"CertFingerprint": map[string]string{
					"Sha1": hex.EncodeToString(sha1Digest[:]), "Sha256": hex.EncodeToString(sha256Digest[:]),
				},
			})
		}
		b8CloudReply(w, map[string]any{"CertInfo": certificates})
	case action == "AddCertificate" && f.mode == "redirect":
		w.Header().Set("Location", "http://"+b8BytePlusCDNHost+r.URL.RequestURI())
		w.WriteHeader(http.StatusTemporaryRedirect)
	case action == "AddCertificate" && input.Source == source && input.Certificate != "" && input.PrivateKey != "":
		certificate := b8CloudCertificate{CertificateChain: input.Certificate, PrivateKey: input.PrivateKey}
		for _, existing := range f.certs {
			if existing == certificate {
				b8CloudError(w, http.StatusConflict, "FixtureDuplicateCertificate")
				return
			}
		}
		b8CloudReply(w, map[string]any{"CertId": f.storeCertificate(certificate)})
	case action == "ListCdnDomains" && input.Domain == "fixture.example" && input.Project == b8FixtureProject &&
		input.Status == "online" && input.PageNum == 1 && input.PageSize == 100:
		b8CloudReply(w, map[string]any{"Data": []map[string]string{
			{"Domain": b8FixtureDomain},
			{"Domain": "other.example"},
		}})
	case action == "DescribeCertConfig" && f.certs[input.CertID].CertificateChain != "":
		b8CloudReply(w, map[string]any{"CertNotConfig": []map[string]string{{"Domain": b8FixtureDomain}}})
	case action == "BatchDeployCert" && input.Domain == b8FixtureDomain:
		f.bindCertificate(w, input.CertID)
	default:
		b8CloudError(w, http.StatusBadRequest, "FixtureUnexpectedCDNRequest")
	}
}

func newB8BytePlusCDNFixture(t *testing.T, handler http.Handler) {
	t.Helper()
	raw, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatal(err)
	}
	local := false
	for _, line := range strings.Split(string(raw), "\n") {
		data, _, _ := strings.Cut(line, "#")
		fields := strings.Fields(data)
		if len(fields) >= 2 && fields[0] == "127.0.0.1" && slices.Contains(fields[1:], b8BytePlusCDNHost) {
			local = true
			break
		}
	}
	if !local {
		t.Fatalf("BytePlus CDN fixture requires --add-host %s:127.0.0.1 in a network-disabled Docker run", b8BytePlusCDNHost)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:80")
	if err != nil {
		t.Fatal(err)
	}
	plainHandler := handler
	if os.Getenv("CERTIMATE_B1_BASELINE") != "1" {
		plainHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("BytePlus CDN sent %s over plaintext HTTP", r.URL.Query().Get("Action"))
			w.WriteHeader(http.StatusBadRequest)
		})
		ca, key, _ := makeCertificate(t, []string{b8BytePlusCDNHost}, true)
		pair, err := tls.X509KeyPair([]byte(ca), []byte(key))
		if err != nil {
			t.Fatal(err)
		}
		caPath := filepath.Join(t.TempDir(), "fixture-ca.pem")
		if err := os.WriteFile(caPath, []byte(ca), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SSL_CERT_FILE", caPath)
		tlsListener, err := net.Listen("tcp", "127.0.0.1:443")
		if err != nil {
			t.Fatal(err)
		}
		server := &httptest.Server{
			Listener: tlsListener,
			Config:   &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second},
			TLS:      &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12},
		}
		server.StartTLS()
		t.Cleanup(server.Close)
	}
	server := &http.Server{Handler: plainHandler, ReadHeaderTimeout: 5 * time.Second}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(listener) }()
}

func runB8BytePlusLoopbackInLinux(t *testing.T, providers []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	arch := runtime.GOARCH
	dir := t.TempDir()
	baseline := os.Getenv("CERTIMATE_B1_BASELINE")
	packageDir := os.Getenv("CERTIMATE_B4_PLUGIN_BINARIES")
	if packageDir == "" {
		packageDir = dir
		if baseline != "1" {
			buildB4CloudPackages(t, packageDir, providers, "linux", arch)
		}
	} else {
		absolute, err := filepath.Abs(packageDir)
		if err != nil {
			t.Fatal(err)
		}
		packageDir = absolute
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "b8-loopback.test"), "./integration")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Linux BytePlus fixtures: %v\n%s", err, output)
	}
	binary, err := filepath.Abs(os.Getenv("CERTIMATE_B1_CORE_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	cmd = exec.CommandContext(ctx, "docker", "run", "--rm", "--network=none",
		"--platform", "linux/"+arch, "--add-host", b8BytePlusCDNHost+":127.0.0.1",
		"--mount", "type=bind,src="+dir+",dst=/fixtures,readonly",
		"--mount", "type=bind,src="+packageDir+",dst=/plugins,readonly",
		"--mount", "type=bind,src="+binary+",dst=/core,readonly",
		"-e", "CERTIMATE_B4_PLUGIN_BINARIES=/plugins", "-e", "CERTIMATE_B1_CORE_BINARY=/core",
		"-e", "CERTIMATE_B1_BASELINE="+baseline,
		image, "/fixtures/b8-loopback.test", "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=8m")
	output, err := cmd.CombinedOutput()
	t.Logf("Linux BytePlus loopback workflow output:\n%s", output)
	if err != nil {
		t.Fatalf("Linux BytePlus loopback workflow verification: %v", err)
	}
}
