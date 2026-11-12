package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	yc "github.com/yandex-cloud/go-genproto/yandex/cloud/certificatemanager/v1"
	endpoint "github.com/yandex-cloud/go-genproto/yandex/cloud/endpoint"
	iam "github.com/yandex-cloud/go-genproto/yandex/cloud/iam/v1"
	operation "github.com/yandex-cloud/go-genproto/yandex/cloud/operation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	b3PlatformProviders = []string{"oraclecloud-certificatesmgmt", "qingcloud-lb", "vercel", "yandexcloud-certificatemanager"}
	b3PlatformHosts     = []string{"certificatesmanagement.us-ashburn-1.oci.oraclecloud.com", "api.qingcloud.com", "api.vercel.com", "api.cloud.yandex.net", "iam.api.cloud.yandex.net", "certificate-manager.api.cloud.yandex.net"}
)

func TestB3PlatformDeploymentBatch(t *testing.T) {
	if runtime.GOOS != "linux" {
		runB3PlatformLinux(t)
		return
	}
	dir := os.Getenv("CERTIMATE_B3_PLATFORM_BINARIES")
	if dir == "" {
		t.Fatal("run with packaged plugin binaries in CERTIMATE_B3_PLATFORM_BINARIES")
	}
	fixture := newB3PlatformFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	cfg := plugin.PluginConfig{PluginDir: dir, StartTimeout: 15 * time.Second, DeployTimeout: 20 * time.Second}
	discovered, failures := plugin.Discover(ctx, cfg)
	if len(failures) != 0 || len(discovered) != len(b3PlatformProviders) {
		t.Fatalf("discovery count=%d failures=%v", len(discovered), failures)
	}
	manager := plugin.NewManager(cfg, slog.New(slog.DiscardHandler))
	for _, dp := range discovered {
		t.Run(dp.Manifest.ProviderType, func(t *testing.T) {
			provider := dp.Manifest.ProviderType
			meta, schema, err := manager.Bootstrap(ctx, dp)
			if err != nil {
				t.Fatal(err)
			}
			if meta.ProviderType != provider || meta.AccessProviderType != dp.Manifest.AccessProviderType || len(schema.AccessSchemaJSON) != 0 {
				t.Fatal("shared built-in access metadata changed")
			}
			validateB3PlatformSchema(t, schema, provider, meta)
			access, extended := fixture.config(provider)
			accessJSON, _ := json.Marshal(access)
			extendedJSON, _ := json.Marshal(extended)
			req := &plugin.DeployRequest{AccessConfigJSON: string(accessJSON), ExtendedConfigJSON: string(extendedJSON), LogLevel: "DEBUG"}
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			deploy := func(ctx context.Context, req *plugin.DeployRequest) (*plugin.DeployResult, error) {
				return manager.Deploy(ctx, dp, req, logger)
			}
			if os.Getenv("CERTIMATE_B1_CORE_BINARY") != "" {
				deploy = newB1WorkflowDeployer(t, dir, provider, workflowLifecycleTarget{
					barrier: &fixture.barrier,
					verify: func(request *plugin.DeployRequest) {
						fixture.mu.Lock()
						defer fixture.mu.Unlock()
						if strings.TrimSpace(fixture.cert) != strings.TrimSpace(request.CertificatePEM) || strings.TrimSpace(fixture.key) != strings.TrimSpace(request.PrivateKeyPEM) {
							t.Fatal("lifecycle deployment did not write the fresh certificate and private key to the target")
						}
					},
				})
			}
			fixture.mu.Lock()
			fixture.provider = provider
			fixture.mode = ""
			fixture.cert = ""
			fixture.key = ""
			fixture.writes = 0
			fixture.bindings = map[string]string{}
			fixture.mu.Unlock()
			for _, step := range []string{"initial", "repeat", "renew", "auth-failure", "recovery"} {
				t.Run(step, func(t *testing.T) {
					if step == "initial" || step == "renew" {
						req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{"platform.example.com"}, false)
						issuer, _, _ := makeCertificate(t, []string{"issuer.example.com"}, true)
						req.CertificatePEM += issuer
					}
					fixture.mu.Lock()
					fixture.mode = step
					before := fixture.writes
					fixture.mu.Unlock()
					_, err := deploy(ctx, req)
					fixture.mu.Lock()
					defer fixture.mu.Unlock()
					if step == "auth-failure" {
						if err == nil {
							t.Fatal("authentication error reported success")
						}
						if fixture.writes != before {
							t.Fatal("failed authentication changed certificate")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if strings.TrimSpace(fixture.cert) != strings.TrimSpace(req.CertificatePEM) || strings.TrimSpace(fixture.key) != strings.TrimSpace(req.PrivateKeyPEM) {
						t.Fatal("target full certificate chain or key differs")
					}
					if step == "repeat" && provider != "vercel" && fixture.writes != before {
						t.Fatal("identical certificate was uploaded twice")
					}
					if step == "initial" || step == "renew" || provider == "vercel" {
						if fixture.writes <= before {
							t.Fatal("expected certificate upload missing")
						}
					}
					if provider == "qingcloud-lb" && fixture.bindings["listener-1"] != fixture.certID() {
						t.Fatal("listener certificate binding differs")
					}
					if strings.Contains(logs.String(), req.PrivateKeyPEM) || strings.Contains(logs.String(), "fixture-token") {
						t.Fatal("secret leaked into plugin log")
					}
				})
			}
			if provider == "qingcloud-lb" {
				t.Run("loadbalancer-and-binding-recovery", func(t *testing.T) {
					extended["deployTarget"] = "loadbalancer"
					extended["loadbalancerId"] = "lb-1"
					data, _ := json.Marshal(extended)
					req.ExtendedConfigJSON = string(data)
					fixture.mu.Lock()
					fixture.mode = "bind-failure"
					fixture.mu.Unlock()
					if _, err := deploy(ctx, req); err == nil {
						t.Fatal("binding failure reported success")
					}
					fixture.mu.Lock()
					fixture.mode = "recovery"
					fixture.mu.Unlock()
					if _, err := deploy(ctx, req); err != nil {
						t.Fatal(err)
					}
					fixture.mu.Lock()
					defer fixture.mu.Unlock()
					if fixture.bindings["listener-2"] != fixture.certID() || len(fixture.bindings) != 2 {
						t.Fatal("load balancer did not bind HTTPS listeners only")
					}
				})
			}
			if provider == "yandexcloud-certificatemanager" {
				t.Run("replace-preserves-metadata", func(t *testing.T) {
					req.ExtendedConfigJSON = `{"certificateId":"certificate-1"}`
					req.CertificatePEM, req.PrivateKeyPEM, _ = makeCertificate(t, []string{"renewed.example.com"}, false)
					if _, err := deploy(ctx, req); err != nil {
						t.Fatal(err)
					}
					fixture.mu.Lock()
					defer fixture.mu.Unlock()
					if strings.TrimSpace(fixture.cert) != strings.TrimSpace(req.CertificatePEM) || strings.TrimSpace(fixture.key) != strings.TrimSpace(req.PrivateKeyPEM) || !fixture.replaced {
						t.Fatal("replacement certificate or metadata differs")
					}
				})
			}
			if provider == "oraclecloud-certificatesmgmt" {
				t.Run("legacy-apikey-empty-region", func(t *testing.T) {
					apiAccess := map[string]any{"authMethod": "apikey", "privateKey": fixture.signingKey, "publicKeyFingerprint": "fingerprint", "tenancyOcid": "ocid1.tenancy.test", "userOcid": "ocid1.user.test"}
					data, _ := json.Marshal(apiAccess)
					req.AccessConfigJSON = string(data)
					if _, err := deploy(ctx, req); err == nil || !strings.Contains(err.Error(), "region") {
						t.Fatalf("expected pre-existing empty-region rejection, got %v", err)
					}
				})
			}
		})
	}
}

type b3PlatformFixture struct {
	barrier                               workflowRequestBarrier
	t                                     *testing.T
	mu                                    sync.Mutex
	provider, mode, cert, key, signingKey string
	writes                                int
	bindings                              map[string]string
	replaced                              bool
	yc.UnimplementedCertificateServiceServer
}

func (f *b3PlatformFixture) certID() string { return fmt.Sprintf("certificate-%d", f.writes) }
func (f *b3PlatformFixture) config(provider string) (map[string]any, map[string]any) {
	switch provider {
	case "oraclecloud-certificatesmgmt":
		return map[string]any{"authMethod": "resourceprincipal"}, map[string]any{"compartmentOcid": "ocid1.compartment.fixture"}
	case "qingcloud-lb":
		return map[string]any{"accessKeyId": "fixture-access", "secretAccessKey": "fixture-secret"}, map[string]any{"zoneId": "pek3a", "deployTarget": "listener", "listenerId": "listener-1"}
	case "vercel":
		return map[string]any{"apiAccessToken": "fixture-token", "teamId": "team-1"}, map[string]any{}
	default:
		key, _ := json.Marshal(map[string]any{"id": "key-1", "service_account_id": "service-account-1", "private_key": f.signingKey})
		return map[string]any{"folderId": "folder-1", "serviceAccountKey": string(key)}, map[string]any{}
	}
}

func (f *b3PlatformFixture) handle(w http.ResponseWriter, r *http.Request) {
	if err := f.barrier.waitHTTP(r); err != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if f.mode == "auth-failure" {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"code":"NotAuthenticated","message":"fixture authentication failure","ret_code":1401}`)
		return
	}
	response := any(map[string]any{})
	switch strings.TrimSuffix(r.Host, ":443") {
	case "api.vercel.com":
		if r.URL.Path != "/v8/certs" || r.URL.Query().Get("teamId") != "team-1" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			f.t.Error("Vercel target/team/auth mismatch")
		}
		var v struct {
			Cert, CA, Key  string
			SkipValidation bool
		}
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			f.t.Error(err)
		}
		if !v.SkipValidation {
			f.t.Error("Vercel skip validation changed")
		}
		f.cert = v.Cert + v.CA
		f.key = v.Key
		f.writes++
		response = map[string]any{"uid": f.certID()}
	case "api.qingcloud.com":
		r.ParseForm()
		if r.Form.Get("access_key_id") != "fixture-access" || r.Form.Get("signature") == "" || r.Form.Get("zone") != "pek3a" {
			f.t.Error("QingCloud signed credentials/zone missing")
		}
		action := r.Form.Get("action")
		response = map[string]any{"ret_code": 0}
		switch action {
		case "DescribeServerCertificates":
			items := []map[string]any{}
			if f.cert != "" {
				items = append(items, map[string]any{"server_certificate_id": f.certID(), "server_certificate_name": "fixture", "certificate_content": f.cert})
			}
			response = map[string]any{"ret_code": 0, "server_certificate_set": items}
		case "CreateServerCertificate":
			f.cert = r.Form.Get("certificate_content")
			f.key = r.Form.Get("private_key")
			f.writes++
			response = map[string]any{"ret_code": 0, "server_certificate_id": f.certID()}
		case "AssociateServerCertsToLBListener":
			if f.mode == "bind-failure" {
				response = map[string]any{"ret_code": 1401, "message": "binding rejected"}
			} else {
				f.bindings[r.Form.Get("loadbalancer_listener")] = r.Form.Get("server_certificates.1")
			}
		case "DescribeLoadBalancerListeners":
			if r.Form.Get("loadbalancer") != "lb-1" {
				f.t.Error("wrong load balancer")
			}
			response = map[string]any{"ret_code": 0, "loadbalancer_listener_set": []map[string]any{{"loadbalancer_listener_id": "listener-1", "listener_protocol": "https"}, {"loadbalancer_listener_id": "listener-2", "listener_protocol": "https"}, {"loadbalancer_listener_id": "listener-3", "listener_protocol": "http"}}}
		default:
			f.t.Errorf("unexpected QingCloud action %s", action)
		}
	default:
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Signature ") {
			f.t.Error("OCI signature missing")
		}
		if r.URL.Path != "/20210224/certificates" {
			f.t.Errorf("unexpected OCI path %s", r.URL.Path)
		}
		if r.Method == http.MethodGet {
			items := []map[string]any{}
			if f.cert != "" {
				cert, _ := xcert.ParseCertificateFromPEM(f.cert)
				items = append(items, map[string]any{"id": f.certID(), "name": "fixture", "subject": map[string]any{"commonName": cert.Subject.CommonName}, "keyAlgorithm": "ECDSA_P256", "currentVersionSummary": map[string]any{"serialNumber": cert.SerialNumber.String(), "validity": map[string]any{"timeOfValidityNotBefore": cert.NotBefore.Format(time.RFC3339), "timeOfValidityNotAfter": cert.NotAfter.Format(time.RFC3339)}}})
			}
			response = map[string]any{"items": items}
		} else {
			var v struct {
				CompartmentID     string `json:"compartmentId"`
				CertificateConfig struct {
					CertificatePEM string `json:"certificatePem"`
					CertChainPEM   string `json:"certChainPem"`
					PrivateKeyPEM  string `json:"privateKeyPem"`
				} `json:"certificateConfig"`
			}
			if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
				f.t.Error(err)
			}
			if v.CompartmentID != "ocid1.compartment.fixture" {
				f.t.Error("OCI compartment changed")
			}
			f.cert = v.CertificateConfig.CertificatePEM + v.CertificateConfig.CertChainPEM
			f.key = v.CertificateConfig.PrivateKeyPEM
			f.writes++
			response = map[string]any{"id": f.certID(), "name": "fixture"}
		}
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		f.t.Error(err)
	}
}

func (f *b3PlatformFixture) List(ctx context.Context, r *yc.ListCertificatesRequest) (*yc.ListCertificatesResponse, error) {
	if err := f.barrier.wait(ctx, nil); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mode == "auth-failure" {
		return nil, status.Error(codes.PermissionDenied, "fixture authentication failure")
	}
	if r.FolderId != "folder-1" {
		f.t.Error("Yandex folder changed")
	}
	result := &yc.ListCertificatesResponse{}
	if f.cert != "" {
		result.Certificates = []*yc.Certificate{f.ycCert()}
	}
	return result, nil
}

func (f *b3PlatformFixture) ycCert() *yc.Certificate {
	cert, _ := xcert.ParseCertificateFromPEM(f.cert)
	return &yc.Certificate{Id: f.certID(), Name: "fixture-name", Description: "fixture-description", Labels: map[string]string{"owner": "fixture"}, DeletionProtection: true, Domains: cert.DNSNames, Subject: cert.Subject.String(), Issuer: cert.Issuer.String(), Serial: cert.SerialNumber.Text(16), NotBefore: timestamppb.New(cert.NotBefore), NotAfter: timestamppb.New(cert.NotAfter)}
}

func (f *b3PlatformFixture) Create(ctx context.Context, r *yc.CreateCertificateRequest) (*operation.Operation, error) {
	if err := f.barrier.wait(ctx, nil); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mode == "auth-failure" {
		return nil, status.Error(codes.PermissionDenied, "fixture authentication failure")
	}
	if r.FolderId != "folder-1" {
		f.t.Error("Yandex create folder changed")
	}
	f.cert = r.Certificate + r.Chain
	f.key = r.PrivateKey
	f.writes++
	cert, _ := anypb.New(f.ycCert())
	meta, _ := anypb.New(&yc.CreateCertificateMetadata{CertificateId: f.certID()})
	return &operation.Operation{Id: "op-1", Done: true, Metadata: meta, Result: &operation.Operation_Response{Response: cert}}, nil
}

func (f *b3PlatformFixture) Get(ctx context.Context, r *yc.GetCertificateRequest) (*yc.Certificate, error) {
	if err := f.barrier.wait(ctx, nil); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cert := f.ycCert()
	cert.Id = r.CertificateId
	return cert, nil
}

func (f *b3PlatformFixture) Update(ctx context.Context, r *yc.UpdateCertificateRequest) (*operation.Operation, error) {
	if err := f.barrier.wait(ctx, nil); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.CertificateId != "certificate-1" || r.Name != "fixture-name" || r.Description != "fixture-description" || r.Labels["owner"] != "fixture" || !r.DeletionProtection {
		f.t.Error("Yandex replacement discarded metadata")
	}
	f.cert = r.Certificate + r.Chain
	f.key = r.PrivateKey
	f.writes++
	f.replaced = true
	cert, _ := anypb.New(f.ycCert())
	meta, _ := anypb.New(&yc.UpdateCertificateMetadata{CertificateId: r.CertificateId})
	return &operation.Operation{Id: "op-2", Done: true, Metadata: meta, Result: &operation.Operation_Response{Response: cert}}, nil
}

type b3PlatformEndpoints struct {
	endpoint.UnimplementedApiEndpointServiceServer
}

func (*b3PlatformEndpoints) List(context.Context, *endpoint.ListApiEndpointsRequest) (*endpoint.ListApiEndpointsResponse, error) {
	return &endpoint.ListApiEndpointsResponse{Endpoints: []*endpoint.ApiEndpoint{{Id: "iam", Address: "iam.api.cloud.yandex.net:443"}, {Id: "certificate-manager", Address: "certificate-manager.api.cloud.yandex.net:443"}}}, nil
}

type b3PlatformIAM struct {
	iam.UnimplementedIamTokenServiceServer
}

func (*b3PlatformIAM) Create(context.Context, *iam.CreateIamTokenRequest) (*iam.CreateIamTokenResponse, error) {
	return &iam.CreateIamTokenResponse{IamToken: "fixture-iam", ExpiresAt: timestamppb.New(time.Now().Add(time.Hour))}, nil
}

func newB3PlatformFixture(t *testing.T) *b3PlatformFixture {
	t.Helper()
	signer, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(signer)}))
	f := &b3PlatformFixture{t: t, signingKey: key, bindings: map[string]string{}}
	ca, keyPEM, _ := makeCertificate(t, b3PlatformHosts, true)
	pair, err := tls.X509KeyPair([]byte(ca), []byte(keyPEM))
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	yc.RegisterCertificateServiceServer(grpcServer, f)
	endpoint.RegisterApiEndpointServiceServer(grpcServer, &b3PlatformEndpoints{})
	iam.RegisterIamTokenServiceServer(grpcServer, &b3PlatformIAM{})
	cloud := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			f.handle(w, r)
		}
	}))
	cloud.Listener.Close()
	cloud.Listener, err = net.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		t.Fatal(err)
	}
	cloud.EnableHTTP2 = true
	cloud.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	cloud.StartTLS()
	t.Cleanup(cloud.Close)
	t.Cleanup(grpcServer.Stop)
	path := filepath.Join(t.TempDir(), "cloud-ca.pem")
	if err := os.WriteFile(path, []byte(ca), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", path)
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("OCI_RESOURCE_PRINCIPAL_VERSION", "2.2")
	t.Setenv("OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM", key)
	t.Setenv("OCI_RESOURCE_PRINCIPAL_REGION", "us-ashburn-1")
	claims, _ := json.Marshal(map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "res_tenant": "ocid1.tenancy.fixture", "res_compartment": "ocid1.compartment.fixture"})
	t.Setenv("OCI_RESOURCE_PRINCIPAL_RPST", base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))+"."+base64.RawURLEncoding.EncodeToString(claims)+".fixture")
	return f
}

func runB3PlatformLinux(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	dir := os.Getenv("CERTIMATE_B3_PLATFORM_BINARIES")
	if dir == "" {
		dir = t.TempDir()
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	archData, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.Architecture}}").CombinedOutput()
	if err != nil {
		t.Fatalf("Docker required: %v %s", err, archData)
	}
	arch := strings.TrimSpace(string(archData))
	if arch == "aarch64" {
		arch = "arm64"
	}
	if arch == "x86_64" {
		arch = "amd64"
	}
	for _, provider := range b3PlatformProviders {
		out := filepath.Join(dir, provider)
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(out, provider), "./"+provider)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
		if data, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v %s", provider, err, data)
		}
		raw, err := os.ReadFile(filepath.Join(root, provider, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := plugin.ParseManifest(raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"manifest.json", manifest.Icon} {
			data, err := os.ReadFile(filepath.Join(root, provider, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, "platform.test"), "./integration")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile tests: %v %s", err, data)
	}
	args := []string{"run", "--rm", "--network=none", "--mount", "type=bind,src=" + dir + ",dst=/fixtures,readonly", "-e", "CERTIMATE_B3_PLATFORM_BINARIES=/fixtures"}
	for _, host := range b3PlatformHosts {
		args = append(args, "--add-host", host+":127.0.0.1")
	}
	if binary := os.Getenv("CERTIMATE_B1_CORE_BINARY"); binary != "" {
		data, err := os.ReadFile(binary)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "core-app"), data, 0o755); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-e", "CERTIMATE_B1_CORE_BINARY=/fixtures/core-app")
	}
	for _, env := range []string{"CERTIMATE_B1_BASELINE", "CERTIMATE_B1_LIFECYCLE", "CERTIMATE_B1_RESTART"} {
		if value := os.Getenv(env); value != "" {
			args = append(args, "-e", env+"="+value)
		}
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	args = append(args, image, "/fixtures/platform.test", "-test.run=^TestB3PlatformDeploymentBatch$", "-test.v", "-test.timeout=10m")
	cmd = exec.CommandContext(ctx, "docker", args...)
	data, err := cmd.CombinedOutput()
	t.Logf("Linux verification:\n%s", data)
	if err != nil {
		t.Fatalf("Linux verification failed: %v", err)
	}
}

func validateB3PlatformSchema(t *testing.T, schema *plugin.ConfigSchema, provider string, meta *plugin.Metadata) {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(schema.DeploySchemaJSON, &env); err != nil {
		t.Fatal(err)
	}
	if env["provider"] != provider || env["category"] != "deploy" || env["schemaVersion"] != "form/v1" {
		t.Fatal("invalid deploy schema")
	}
	var check func(any)
	check = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if strings.HasSuffix(key, "Key") {
					if str, ok := child.(string); ok {
						for _, locale := range []string{"en", "zh"} {
							if schema.I18n[locale][str] == "" {
								t.Errorf("missing %s translation %s", locale, str)
							}
						}
					}
				}
				check(child)
			}
		case []any:
			for _, child := range value {
				check(child)
			}
		}
	}
	check(env)
	for _, locale := range []string{"en", "zh"} {
		if schema.I18n[locale][meta.DeployDisplayNameKey] == "" {
			t.Errorf("missing %s display name", locale)
		}
	}
}
