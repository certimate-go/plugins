package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/certimate-go/plugins/internal/providerrelease"
	"github.com/certimate-go/plugins/internal/registry"
)

func installPublishedFixture(t *testing.T, destination, provider, goos, arch string) {
	t.Helper()
	versions := map[string]string{
		"gotify":                       "v0.2.2",
		"ntfy":                         "v0.2.2",
		"volcengine-cdn":               "v0.2.1",
		"volcengine-certcenter-purger": "v0.1.1",
	}
	tag, ok := versions[provider]
	if !ok {
		t.Fatalf("no source or published fixture for %s", provider)
	}
	release, err := registry.ReadVersion(os.DirFS("../registry"), path.Join(provider, tag+".json"))
	if err != nil {
		t.Fatal(err)
	}
	target := providerrelease.Target{OS: goos, Arch: arch}
	index := slices.IndexFunc(release.Artifacts, func(artifact providerrelease.Artifact) bool {
		return artifact.Target == target
	})
	if index == -1 {
		t.Fatalf("%s %s has no published package for %s/%s", provider, tag, goos, arch)
	}
	artifact := release.Artifacts[index]
	address := "https://cdn.jsdelivr.net/gh/" + release.Repository + "@" + release.ReleaseCommit + "/" + artifact.Path
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("download %s: %s", address, response.Status)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, artifact.Size+1))
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(payload)) != artifact.Size || fmt.Sprintf("%x", sha256.Sum256(payload)) != artifact.SHA256 {
		t.Fatalf("published package does not match its registry record: %s", address)
	}
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(destination, archive); err != nil {
		t.Fatal(err)
	}
}

func validateSchema(t *testing.T, schema *plugin.ConfigSchema, provider, access string, meta *plugin.Metadata) {
	t.Helper()
	for _, item := range []struct {
		data               []byte
		provider, category string
	}{{schema.AccessSchemaJSON, access, "access"}, {schema.ProviderSchemaJSON, provider, "deploy"}} {
		if item.category == "access" && len(item.data) == 0 {
			continue
		}
		var envelope map[string]any
		if err := json.Unmarshal(item.data, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope["schemaVersion"] != "form/v1" || envelope["provider"] != item.provider || envelope["category"] != item.category {
			t.Fatalf("invalid schema envelope: %s", item.data)
		}
		var checkKeys func(any)
		checkKeys = func(value any) {
			switch value := value.(type) {
			case map[string]any:
				for key, child := range value {
					if strings.HasSuffix(key, "Key") {
						if keyString, ok := child.(string); ok && keyString != "" {
							for _, locale := range []string{"en", "zh"} {
								if schema.I18n[locale][keyString] == "" {
									t.Errorf("missing %s translation %s", locale, keyString)
								}
							}
						}
					}
					checkKeys(child)
				}
			case []any:
				for _, child := range value {
					checkKeys(child)
				}
			}
		}
		checkKeys(envelope)
	}
	for _, locale := range []string{"en", "zh"} {
		for _, key := range []string{meta.AccessDisplayNameKey, meta.DisplayNameKey} {
			if key == "" || schema.I18n[locale][key] == "" {
				t.Errorf("missing %s display name %q", locale, key)
			}
		}
	}
}

func makeCertificate(t *testing.T, names []string, isCA bool) (string, string, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: names[0]}, DNSNames: names, NotBefore: time.Now().Add(-time.Hour).Truncate(time.Second), NotAfter: time.Now().Add(24 * time.Hour).Truncate(time.Second), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: isCA, BasicConstraintsValid: true}
	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})), cert
}

func deployViaSession(ctx context.Context, manager *plugin.Manager, dp *plugin.DiscoveredPlugin, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error) {
	session, err := manager.StartSession(ctx, dp)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	res, err := session.Process(ctx, req, logger)
	if req.ExecutionID != "" {
		if ferr := session.PostProcess(ctx, req.ExecutionID, logger); ferr != nil {
			if err != nil {
				return nil, fmt.Errorf("%w; finalization also failed: %v", err, ferr)
			}
			err = ferr
		}
	}
	return res, err
}
