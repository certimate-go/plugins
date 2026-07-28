package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestGetMetadata(t *testing.T) {
	meta, err := (&dogeCloudCdnDeployer{}).GetMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProviderType != providerType {
		t.Fatalf("provider type = %q, want %q", meta.ProviderType, providerType)
	}
	if meta.AccessProviderType != accessType {
		t.Fatalf("access type = %q, want builtin %q", meta.AccessProviderType, accessType)
	}
	if meta.ProtocolVersion != plugin.ProtocolVersion {
		t.Fatalf("protocol version = %d", meta.ProtocolVersion)
	}
	if meta.DeployCategory != "cdn" {
		t.Fatalf("deploy category = %q, want cdn", meta.DeployCategory)
	}
	if meta.DeployDisplayNameKey != displayNameKey {
		t.Fatalf("display name key = %q", meta.DeployDisplayNameKey)
	}
}

func TestGetConfigSchema(t *testing.T) {
	schema, err := (&dogeCloudCdnDeployer{}).GetConfigSchema(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	type envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		Provider      string `json:"provider"`
		Category      string `json:"category"`
		Schema        struct {
			Columns []struct {
				Name      string `json:"name"`
				ValueType string `json:"valueType"`
				LabelKey  string `json:"labelKey,omitempty"`
			} `json:"columns"`
		} `json:"schema"`
	}

	if len(schema.AccessSchemaJSON) == 0 {
		t.Fatalf("plugin must emit an access schema for the shared %q access type", accessType)
	}
	var accessEnv envelope
	if err := json.Unmarshal(schema.AccessSchemaJSON, &accessEnv); err != nil {
		t.Fatalf("access envelope invalid: %v", err)
	}
	if accessEnv.SchemaVersion != "form/v1" || accessEnv.Provider != accessType || accessEnv.Category != "access" {
		t.Fatalf("access envelope mismatch: %+v", accessEnv)
	}

	var deployEnv envelope
	if err := json.Unmarshal(schema.DeploySchemaJSON, &deployEnv); err != nil {
		t.Fatalf("deploy envelope invalid: %v", err)
	}
	if deployEnv.SchemaVersion != "form/v1" || deployEnv.Provider != providerType || deployEnv.Category != "deploy" {
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
		if bundle[displayNameKey] == "" {
			t.Fatalf("locale %s missing display name", locale)
		}
		for _, env := range []envelope{accessEnv, deployEnv} {
			for _, c := range env.Schema.Columns {
				if c.LabelKey != "" && bundle[c.LabelKey] == "" {
					t.Fatalf("locale %s missing i18n key %q", locale, c.LabelKey)
				}
			}
		}
	}
}

func TestDeploy_InvalidAccessConfig(t *testing.T) {
	_, err := (&dogeCloudCdnDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{not-json`,
		ExtendedConfigJSON: `{"domainMatchPattern":"exact","domain":"example.com"}`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid access config") {
		t.Fatalf("expected invalid access config error, got %v", err)
	}
}
