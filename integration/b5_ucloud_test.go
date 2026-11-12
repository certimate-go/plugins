package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"
)

const (
	b5UCloudPublicKey  = "fixture-ucloud-public"
	b5UCloudPrivateKey = "fixture-ucloud-private"
	b5UCloudDomain     = "b5.example.com"
)

var b5UCloudProviders = []string{"ucloud-ualb", "ucloud-uclb", "ucloud-ucdn", "ucloud-uewaf", "ucloud-pathx", "ucloud-us3"}

func TestB5UCloudWorkflow(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY for real workflow verification")
	}
	dir := os.Getenv("CERTIMATE_B5_UCLOUD_BINARIES")
	if dir == "" {
		dir = t.TempDir()
		if os.Getenv("CERTIMATE_B1_BASELINE") != "1" {
			buildB4CloudPackages(t, dir, b5UCloudProviders, runtime.GOOS, runtime.GOARCH)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	for _, provider := range b5UCloudProviders {
		t.Run(provider, func(t *testing.T) {
			target := &b5UCloudTarget{provider: provider, uploads: map[string]b5UCloudCertificate{}}
			server := httptest.NewServer(http.HandlerFunc(target.serve))
			target.url = server.URL
			t.Cleanup(server.Close)
			deploy := newB1WorkflowDeployer(t, dir, provider, workflowLifecycleTarget{barrier: &target.barrier, verify: func(req *plugin.DeployRequest) { target.verify(t, req) }})
			config := map[string]any{"endpoint": server.URL}
			switch provider {
			case "ucloud-ualb":
				config["region"] = "cn-bj2"
				config["deployTarget"] = "listener"
				config["loadbalancerId"] = "lb-fixture"
				config["listenerId"] = "listener-fixture"
				config["domain"] = b5UCloudDomain
				config["autoPrune"] = true
			case "ucloud-uclb":
				config["region"] = "cn-bj2"
				config["deployTarget"] = "loadbalancer"
				config["loadbalancerId"] = "lb-fixture"
			case "ucloud-ucdn":
				config["domainId"] = "cdn-fixture"
			case "ucloud-uewaf":
				config["domain"] = b5UCloudDomain
			case "ucloud-pathx":
				config["acceleratorId"] = "uga-fixture"
				config["listenerPort"] = "443"
			case "ucloud-us3":
				config["region"] = "cn-bj2"
				config["bucket"] = "bucket-fixture"
				config["domain"] = b5UCloudDomain
			}
			rawConfig, _ := json.Marshal(config)
			req := &plugin.DeployRequest{AccessConfigJSON: `{"publicKey":"` + b5UCloudPublicKey + `","privateKey":"` + b5UCloudPrivateKey + `","projectId":"project-fixture"}`, ExtendedConfigJSON: string(rawConfig)}
			issuer, _, _ := makeCertificate(t, []string{"issuer.example.com"}, true)
			for _, step := range []string{"initial", "repeat", "renew", "auth-failure", "auth-recovery", "write-failure", "write-recovery", "upload-failure", "upload-recovery"} {
				t.Run(step, func(t *testing.T) {
					if step == "initial" || step == "renew" || strings.HasSuffix(step, "failure") {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{b5UCloudDomain}, false)
						req.CertificatePEM += issuer
					}
					target.mu.Lock()
					target.mode = step
					before := target.bound
					beforeCount := len(target.uploads)
					beforeBindings := target.bindings
					target.mu.Unlock()
					_, err := deploy(ctx, req)
					target.mu.Lock()
					target.mode = ""
					target.mu.Unlock()
					if strings.HasSuffix(step, "failure") {
						if err == nil {
							t.Fatalf("expected %s", step)
						}
						if os.Getenv("CERTIMATE_B1_BASELINE") != "1" && !strings.Contains(err.Error(), "fixture "+step) {
							t.Fatalf("expected original %s, got %v", step, err)
						}
						target.mu.Lock()
						defer target.mu.Unlock()
						if target.bound != before || target.bindings != beforeBindings {
							t.Fatal("failed deployment changed target binding")
						}
						if (step == "auth-failure" || step == "upload-failure") && len(target.uploads) != beforeCount {
							t.Fatal("failed upload wrote certificate")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					target.verify(t, req)
					if step == "repeat" && provider != "ucloud-uewaf" {
						target.mu.Lock()
						defer target.mu.Unlock()
						if len(target.uploads) != beforeCount {
							t.Fatal("duplicate deployment added certificate")
						}
					}
				})
			}
			if provider == "ucloud-uclb" {
				t.Run("bind-failure-after-unbind", func(t *testing.T) {
					req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{b5UCloudDomain}, false)
					req.CertificatePEM += issuer
					target.mu.Lock()
					target.mode = "bind-failure"
					target.mu.Unlock()
					_, err := deploy(ctx, req)
					target.mu.Lock()
					target.mode = ""
					bound := target.bound
					target.mu.Unlock()
					if err == nil || !strings.Contains(err.Error(), "fixture bind-failure") {
						t.Fatalf("expected failed binding, got %v", err)
					}
					if bound != "" {
						t.Fatal("UCLB legacy unbind-first behavior did not remove the binding")
					}
				})
				t.Run("bind-recovery", func(t *testing.T) {
					if _, err := deploy(ctx, req); err != nil {
						t.Fatal(err)
					}
					target.verify(t, req)
				})
			}
			if provider == "ucloud-pathx" {
				t.Run("default-project", func(t *testing.T) {
					req.AccessConfigJSON = `{"publicKey":"` + b5UCloudPublicKey + `","privateKey":"` + b5UCloudPrivateKey + `"}`
					if _, err := deploy(ctx, req); err != nil {
						t.Fatal(err)
					}
					target.verify(t, req)
				})
			}
		})
	}
}

type b5UCloudCertificate struct{ PEM, Key, Name, Content string }

type b5UCloudTarget struct {
	barrier                    workflowRequestBarrier
	mu                         sync.Mutex
	provider, mode, url, bound string
	uploads                    map[string]b5UCloudCertificate
	bindings                   int
}

func (f *b5UCloudTarget) verify(t *testing.T, req *plugin.DeployRequest) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	got, ok := f.uploads[f.bound]
	if !ok || strings.TrimSpace(got.PEM) != strings.TrimSpace(req.CertificatePEM) || strings.TrimSpace(got.Key) != strings.TrimSpace(req.PrivateKeyPEM) {
		t.Fatal("target binding does not contain complete expected certificate chain and key")
	}
}

func (f *b5UCloudTarget) serve(w http.ResponseWriter, r *http.Request) {
	if err := f.barrier.wait(r.Context(), nil); err != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/download/") {
		cert, ok := f.uploads[strings.TrimPrefix(r.URL.Path, "/download/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		var out bytes.Buffer
		z := zip.NewWriter(&out)
		file, _ := z.Create("Nginx/public.pem")
		_, _ = file.Write([]byte(cert.PEM))
		_ = z.Close()
		_, _ = w.Write(out.Bytes())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	payload := map[string]any{}
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			http.Error(w, "invalid json", 400)
			return
		}
	} else {
		if r.ParseForm() != nil {
			http.Error(w, "invalid form", 400)
			return
		}
		for k, v := range r.PostForm {
			if len(v) > 0 {
				payload[k] = v[0]
			}
		}
	}
	value := func(key string) string {
		if v, ok := payload[key]; ok {
			return fmt.Sprint(v)
		}
		return ""
	}
	first := func(key string) string {
		if v, ok := payload[key].([]any); ok && len(v) > 0 {
			return fmt.Sprint(v[0])
		}
		return value(key + ".0")
	}
	action := value("Action")
	write := func(result map[string]any) {
		result["Action"] = action + "Response"
		if _, ok := result["RetCode"]; !ok {
			result["RetCode"] = 0
		}
		_ = json.NewEncoder(w).Encode(result)
	}
	fail := func(message string) { write(map[string]any{"RetCode": 161, "Message": message}) }
	signature := value("Signature")
	delete(payload, "Signature")
	if value("PublicKey") != b5UCloudPublicKey || auth.CalculateSignature(payload, b5UCloudPrivateKey).Sign != signature {
		fail("invalid fixture authentication")
		return
	}
	if f.mode == "auth-failure" {
		fail("fixture auth-failure")
		return
	}
	if action == "GetProjectList" {
		write(map[string]any{"ProjectSet": []any{map[string]any{"ProjectId": "project-fixture", "IsDefault": true}}})
		return
	}
	if value("ProjectId") != "project-fixture" {
		fail("unexpected project")
		return
	}
	uploadAction := action == "CreateSSL" || action == "CreatePathXSSL" || action == "UploadNormalCertificate" || action == "AddWafDomainCertificateInfo"
	bindingAction := action == "UpdateListenerAttribute" || action == "AddSSLBinding" || action == "DeleteSSLBinding" || action == "UnbindSSL" || action == "BindSSL" || action == "UpdateUcdnDomainHttpsConfigV2" || action == "BindPathXSSL" || action == "AddUFileSSLCert" || action == "AddWafDomainCertificateInfo"
	if f.mode == "upload-failure" && uploadAction {
		fail("fixture upload-failure")
		return
	}
	if f.mode == "write-failure" && bindingAction {
		fail("fixture write-failure")
		return
	}
	if f.mode == "bind-failure" && action == "BindSSL" {
		fail("fixture bind-failure")
		return
	}
	switch action {
	case "DescribeSSL", "DescribeSSLV2", "DescribePathXSSL":
		list := []any{}
		for id, c := range f.uploads {
			if value("SSLId") != "" && value("SSLId") != id {
				continue
			}
			leaf, err := xcert.ParseCertificateFromPEM(c.PEM)
			if err != nil {
				fail("invalid stored certificate")
				return
			}
			list = append(list, map[string]any{"SSLId": id, "SSLName": c.Name, "SSLContent": c.Content, "NotBefore": leaf.NotBefore.Unix(), "NotAfter": leaf.NotAfter.Unix(), "ExpireTime": leaf.NotAfter.Unix(), "Domains": b5UCloudDomain})
		}
		write(map[string]any{"DataSet": list})
		return
	case "CreateSSL", "CreatePathXSSL", "UploadNormalCertificate", "AddWafDomainCertificateInfo":
		c := b5UCloudCertificate{Name: value("SSLName"), Key: value("PrivateKey")}
		if action == "UploadNormalCertificate" || action == "AddWafDomainCertificateInfo" {
			p, err1 := base64.StdEncoding.DecodeString(value("SslPublicKey"))
			k, err2 := base64.StdEncoding.DecodeString(value("SslPrivateKey"))
			if err1 != nil || err2 != nil {
				fail("invalid certificate encoding")
				return
			}
			c.PEM = string(p)
			c.Key = string(k)
			c.Name = value("CertificateName")
			checksum := value("SslMD5")
			if action == "AddWafDomainCertificateInfo" {
				checksum = value("SslMD")
			}
			if checksum != fmt.Sprintf("%x", md5.Sum([]byte(value("SslPublicKey")+value("SslPrivateKey")))) {
				fail("certificate checksum differs")
				return
			}
		} else {
			ca := value("CaCert")
			if ca == "" {
				ca = value("CACert")
			}
			c.PEM = value("UserCert") + ca
			if action == "CreateSSL" {
				c.Content = value("UserCert") + c.Key + ca
			} else {
				c.Content = c.Key + c.PEM
			}
		}
		if _, err := xcert.ParseCertificateFromPEM(c.PEM); err != nil || c.Key == "" {
			fail("invalid uploaded certificate")
			return
		}
		if action == "UploadNormalCertificate" {
			for _, old := range f.uploads {
				if old.PEM == c.PEM && old.Key == c.Key {
					write(map[string]any{"RetCode": 80035, "Message": "certificate exists"})
					return
				}
			}
		}
		if action == "AddWafDomainCertificateInfo" && value("Domain") != b5UCloudDomain {
			fail("wrong WAF domain")
			return
		}
		id := strconv.Itoa(len(f.uploads) + 1)
		f.uploads[id] = c
		if action == "AddWafDomainCertificateInfo" {
			f.bound = id
			f.bindings++
		}
		write(map[string]any{"SSLId": id, "CertificateID": len(f.uploads), "LongResourceID": "resource-" + id, "Id": len(f.uploads)})
		return
	case "GetCertificateList":
		list := []any{}
		for id, c := range f.uploads {
			leaf, err := xcert.ParseCertificateFromPEM(c.PEM)
			if err != nil {
				fail("invalid stored certificate")
				return
			}
			n, _ := strconv.Atoi(id)
			list = append(list, map[string]any{"CertificateID": n, "CertificateSN": "resource-" + id, "Name": c.Name, "Domains": strings.Join(leaf.DNSNames, ","), "NotBefore": leaf.NotBefore.UnixMilli(), "NotAfter": leaf.NotAfter.UnixMilli()})
		}
		write(map[string]any{"CertificateList": list, "TotalCount": len(list)})
		return
	case "GetCertificateDetailInfo":
		certificate := f.uploads[value("CertificateID")]
		leaf, err := xcert.ParseCertificateFromPEM(certificate.PEM)
		if err != nil {
			fail("invalid stored certificate")
			return
		}
		write(map[string]any{"CertificateInfo": map[string]any{"Algorithm": leaf.SignatureAlgorithm.String()}})
		return
	case "DownloadCertificate":
		write(map[string]any{"CertificateUrl": f.url + "/download/" + value("CertificateID")})
		return
	case "DescribeListeners":
		if value("LoadBalancerId") != "lb-fixture" {
			fail("wrong ALB")
			return
		}
		certificates := []any{map[string]any{"SSLId": "default-fixture", "IsDefault": true}}
		if f.bound != "" {
			certificates = append(certificates, map[string]any{"SSLId": f.bound, "IsDefault": false})
		}
		write(map[string]any{"Listeners": []any{map[string]any{"ListenerId": "listener-fixture", "ListenerProtocol": "HTTPS", "Certificates": certificates}}})
		return
	case "DescribeVServer":
		if value("ULBId") != "lb-fixture" {
			fail("wrong CLB")
			return
		}
		certificates := []any{}
		if f.bound != "" {
			certificates = append(certificates, map[string]any{"SSLId": f.bound})
		}
		write(map[string]any{"DataSet": []any{map[string]any{"VServerId": "vserver-fixture", "Protocol": "HTTPS", "SSLSet": certificates}}})
		return
	case "GetUcdnDomainConfig":
		if first("DomainId") != "cdn-fixture" {
			fail("wrong CDN domain")
			return
		}
		write(map[string]any{"DomainList": []any{map[string]any{"DomainId": "cdn-fixture", "AreaCode": "all", "HttpsStatusCn": "enable", "HttpsStatusAbroad": "enable"}}})
		return
	case "DeleteSSLBinding":
		if first("SSLIds") == f.bound {
			fail("attempted to delete new SNI certificate")
			return
		}
		write(map[string]any{})
		return
	case "UnbindSSL":
		if value("ULBId") != "lb-fixture" || value("VServerId") != "vserver-fixture" || value("SSLId") != f.bound {
			fail("wrong CLB unbind")
			return
		}
		f.bound = ""
		write(map[string]any{})
		return
	case "AddSSLBinding", "UpdateListenerAttribute", "BindSSL", "UpdateUcdnDomainHttpsConfigV2", "BindPathXSSL", "AddUFileSSLCert":
		id := value("SSLId")
		switch action {
		case "AddSSLBinding", "UpdateListenerAttribute":
			if value("LoadBalancerId") != "lb-fixture" || value("ListenerId") != "listener-fixture" {
				fail("wrong ALB binding")
				return
			}
			id = first("SSLIds")
			if action == "UpdateListenerAttribute" {
				id = first("Certificates")
			}
		case "BindSSL":
			if value("ULBId") != "lb-fixture" || value("VServerId") != "vserver-fixture" {
				fail("wrong CLB binding")
				return
			}
		case "UpdateUcdnDomainHttpsConfigV2":
			if value("DomainId") != "cdn-fixture" || value("CertType") != "ussl" || value("HttpsStatusCn") != "enable" || value("HttpsStatusAbroad") != "enable" {
				fail("wrong CDN binding")
				return
			}
			id = value("CertId")
		case "BindPathXSSL":
			if value("UGAId") != "uga-fixture" || first("Port") != "443" {
				fail("wrong PathX binding")
				return
			}
		case "AddUFileSSLCert":
			if value("BucketName") != "bucket-fixture" || value("Domain") != b5UCloudDomain {
				fail("wrong bucket domain binding")
				return
			}
			id = value("USSLId")
		}
		if _, ok := f.uploads[id]; !ok {
			fail("unknown uploaded certificate")
			return
		}
		f.bound = id
		f.bindings++
		write(map[string]any{})
		return
	}
	fail("unexpected fixture action " + action)
}

func TestB5UCloudBrowserFixture(t *testing.T) {
	address := os.Getenv("CERTIMATE_B5_UCLOUD_BROWSER_ADDRESS")
	if address == "" {
		t.Skip("set CERTIMATE_B5_UCLOUD_BROWSER_ADDRESS for the controlled browser target")
	}
	provider := os.Getenv("CERTIMATE_B5_UCLOUD_BROWSER_PROVIDER")
	if provider == "" {
		provider = "ucloud-us3"
	}
	target := &b5UCloudTarget{provider: provider, uploads: map[string]b5UCloudCertificate{}, url: "http://" + address}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		target.mu.Lock()
		defer target.mu.Unlock()
		cert := target.uploads[target.bound]
		_ = json.NewEncoder(w).Encode(map[string]any{"provider": target.provider, "bound": target.bound, "uploads": len(target.uploads), "bindings": target.bindings, "certificateSHA256": fmt.Sprintf("%x", sha256.Sum256([]byte(cert.PEM))), "privateKeySHA256": fmt.Sprintf("%x", sha256.Sum256([]byte(cert.Key)))})
	})
	mux.HandleFunc("POST /mode", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode string `json:"mode"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		switch body.Mode {
		case "", "auth-failure", "write-failure", "upload-failure":
		default:
			http.Error(w, "unsupported mode", http.StatusBadRequest)
			return
		}
		target.mu.Lock()
		target.mode = body.Mode
		target.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", target.serve)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	t.Cleanup(func() { _ = server.Close() })
	go server.Serve(listener)
	t.Logf("UCloud browser target listening on %s for %s", address, provider)
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt)
	defer signal.Stop(stopped)
	<-stopped
}
