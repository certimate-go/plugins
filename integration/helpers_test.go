package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func validateSchema(t *testing.T, schema *plugin.ConfigSchema, provider, access string, meta *plugin.Metadata) {
	t.Helper()
	for _, item := range []struct {
		data               []byte
		provider, category string
	}{{schema.AccessSchemaJSON, access, "access"}, {schema.DeploySchemaJSON, provider, "deploy"}} {
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
		for _, key := range []string{meta.AccessDisplayNameKey, meta.DeployDisplayNameKey} {
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
