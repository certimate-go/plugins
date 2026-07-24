package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
			for _, key := range []string{c.LabelKey, c.PlaceholderKey} {
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

func TestDeploy_UsesBuiltinWebhookAccess(t *testing.T) {
	var seen struct {
		method string
		path   string
		auth   string
		body   string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.method = r.Method
		seen.path = r.URL.Path
		seen.auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		seen.body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	access, _ := json.Marshal(map[string]any{
		"url":     srv.URL,
		"headers": "Authorization: Bearer tok-abc\nX-Source: plugin",
	})
	extended, _ := json.Marshal(map[string]string{"method": "PUT", "path": "cert"})

	res, err := (&webhookDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   string(access),
		ExtendedConfigJSON: string(extended),
		CertificatePEM:     "CERT",
		PrivateKeyPEM:      "KEY",
	})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if seen.method != http.MethodPut || seen.path != "/cert" {
		t.Fatalf("saw %s %s", seen.method, seen.path)
	}
	if seen.auth != "Bearer tok-abc" {
		t.Fatalf("auth header not forwarded from builtin webhook access: %q", seen.auth)
	}
	if !strings.Contains(seen.body, "CERT") {
		t.Fatalf("cert not forwarded: %s", seen.body)
	}
	var extended2 map[string]any
	if err := json.Unmarshal([]byte(res.ExtendedDataJSON), &extended2); err != nil {
		t.Fatalf("extended data not json: %v", err)
	}
}

func TestDeploy_MissingURL(t *testing.T) {
	_, err := (&webhookDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON: `{"headers":"x: y"}`,
	})
	if err == nil || !strings.Contains(err.Error(), "url") {
		t.Fatalf("expected missing url error, got %v", err)
	}
}
