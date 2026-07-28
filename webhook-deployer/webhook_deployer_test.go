package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestGetMetadata(t *testing.T) {
	meta, err := (&webhookDeployer{}).GetMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProviderType != providerType {
		t.Fatalf("provider type = %q, want %q", meta.ProviderType, providerType)
	}
	if meta.AccessProviderType != accessType {
		t.Fatalf("access type = %q, wants builtin %q", meta.AccessProviderType, accessType)
	}
	if meta.ProtocolVersion != plugin.ProtocolVersion {
		t.Fatalf("protocol version = %d", meta.ProtocolVersion)
	}
	if meta.DeployDisplayNameKey == "" {
		t.Fatal("display name key empty")
	}
}

func TestGetConfigSchema_NoAccessSchema_ReusesBuiltin(t *testing.T) {
	schema, err := (&webhookDeployer{}).GetConfigSchema(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.AccessSchemaJSON) != 0 {
		t.Fatalf("plugin must NOT emit an access schema (reuses builtin webhook access), got %q", schema.AccessSchemaJSON)
	}

	var deployEnv struct {
		SchemaVersion string `json:"schemaVersion"`
		Provider      string `json:"provider"`
		Category      string `json:"category"`
		Schema        struct {
			Columns []struct {
				Name           string `json:"name"`
				ValueType      string `json:"valueType"`
				LabelKey       string `json:"labelKey,omitempty"`
				PlaceholderKey string `json:"placeholderKey,omitempty"`
				TooltipKey     string `json:"tooltipKey,omitempty"`
				UnitKey        string `json:"unitKey,omitempty"`
				TipsKey        string `json:"tipsKey,omitempty"`
			} `json:"columns"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(schema.DeploySchemaJSON, &deployEnv); err != nil {
		t.Fatalf("deploy envelope invalid: %v", err)
	}
	if deployEnv.SchemaVersion != "form/v1" || deployEnv.Provider != "webhook-deployer" || deployEnv.Category != "deploy" {
		t.Fatalf("deploy envelope mismatch: %+v", deployEnv)
	}

	bundles, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		t.Fatalf("LoadI18n: %v", err)
	}
	for _, locale := range []string{"zh", "en"} {
		bundle, ok := bundles[locale]
		if !ok {
			t.Fatalf("missing i18n bundle for %s", locale)
		}
		for _, c := range deployEnv.Schema.Columns {
			for _, key := range []string{c.LabelKey, c.PlaceholderKey, c.UnitKey, c.TooltipKey} {
				if key == "" {
					continue
				}
				if _, ok := bundle[key]; !ok {
					t.Fatalf("locale %s missing i18n key %q", locale, key)
				}
			}
		}
		if bundle["plugin.webhook-deployer.name"] == "" {
			t.Fatalf("locale %s missing display name", locale)
		}
	}
}

func TestDeploy_TemplatedBodyAndAccessConfig(t *testing.T) {
	certPEM, keyPEM := selfSignedCert(t, "example.com")
	cap := &logCapture{}

	var seen struct {
		method      string
		auth        string
		contentType string
		body        string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.method = r.Method
		seen.auth = r.Header.Get("Authorization")
		seen.contentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		seen.body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	access, _ := json.Marshal(map[string]any{
		"url":     srv.URL,
		"method":  "POST",
		"headers": "Authorization: Bearer tok-abc",
	})
	extended, _ := json.Marshal(map[string]any{
		"webhookData": `{"commonName":"${CERTIMATE_DEPLOYER_COMMONNAME}","cert":"${CERTIMATE_DEPLOYER_CERTIFICATE}"}`,
		"timeout":     5,
	})

	res, err := (&webhookDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   string(access),
		ExtendedConfigJSON: string(extended),
		CertificatePEM:     certPEM,
		PrivateKeyPEM:      keyPEM,
	}, slog.New(cap))
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}

	if seen.method != http.MethodPost {
		t.Fatalf("method = %q, want POST (sourced from access config)", seen.method)
	}
	if seen.contentType != "application/json" {
		t.Fatalf("content-type = %q, want application/json", seen.contentType)
	}
	if seen.auth != "Bearer tok-abc" {
		t.Fatalf("Authorization not forwarded from access config: %q", seen.auth)
	}
	if strings.Contains(seen.body, "${CERTIMATE_DEPLOYER_") {
		t.Fatalf("variable placeholder left unsubstituted: %s", seen.body)
	}

	var body map[string]string
	if err := json.Unmarshal([]byte(seen.body), &body); err != nil {
		t.Fatalf("body not json: %v (%s)", err, seen.body)
	}
	if body["commonName"] != "example.com" {
		t.Fatalf("commonName = %q, want example.com", body["commonName"])
	}
	if body["cert"] != certPEM {
		t.Fatalf("${CERTIMATE_DEPLOYER_CERTIFICATE} not substituted with actual PEM")
	}

	var extendedData map[string]any
	if err := json.Unmarshal([]byte(res.ExtendedDataJSON), &extendedData); err != nil {
		t.Fatalf("extended data not json: %v", err)
	}

	var respondedStatus string
	for _, r := range cap.records {
		if r.Message == "webhook responded" {
			respondedStatus = r.Attrs["status"]
		}
	}
	if respondedStatus != "200" {
		t.Fatalf("expected 'webhook responded' log with status 200, got %+v", cap.records)
	}
	if leaked := cap.dump(); strings.Contains(leaked, "tok-abc") || strings.Contains(leaked, keyPEM) {
		t.Fatalf("secret leaked into plugin logs:\n%s", leaked)
	}
}

func TestDeploy_DefaultBodyWhenWebhookDataEmpty(t *testing.T) {
	certPEM, keyPEM := selfSignedCert(t, "example.com")

	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	access, _ := json.Marshal(map[string]any{"url": srv.URL})
	if _, err := (&webhookDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON: string(access),
		CertificatePEM:   certPEM,
		PrivateKeyPEM:    keyPEM,
	}, nil); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body not json: %v (%s)", err, body)
	}
	if got["name"] != "example.com" {
		t.Fatalf("default body name = %q, want example.com (SANs)", got["name"])
	}
	if got["cert"] != certPEM || got["privkey"] != keyPEM {
		t.Fatalf("default body did not embed cert/privatekey PEM")
	}
}

func TestDeploy_MissingURL(t *testing.T) {
	_, err := (&webhookDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON: `{"headers":"x: y"}`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "url") {
		t.Fatalf("expected missing url error, got %v", err)
	}
}

func selfSignedCert(t *testing.T, commonName string) (certPEM, keyPEM string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     []string{commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM
}

type logCapture struct {
	mu      sync.Mutex
	records []logRecord
}

type logRecord struct {
	Level   slog.Level
	Message string
	Attrs   map[string]string
}

func (c *logCapture) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	attrs := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	c.records = append(c.records, logRecord{Level: r.Level, Message: r.Message, Attrs: attrs})
	return nil
}

func (c *logCapture) WithAttrs(_ []slog.Attr) slog.Handler { return c }
func (c *logCapture) WithGroup(_ string) slog.Handler      { return c }

func (c *logCapture) dump() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var sb strings.Builder
	for _, r := range c.records {
		sb.WriteString(r.Message)
		for k, v := range r.Attrs {
			sb.WriteString(" ")
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(v)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
