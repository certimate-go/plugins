package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestGetMetadata(t *testing.T) {
	meta, err := (&wangsuCertificateDeployer{}).GetMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProviderType != providerType {
		t.Fatalf("provider type = %q, want %q", meta.ProviderType, providerType)
	}
	if meta.AccessProviderType != accessType {
		t.Fatalf("access type = %q, want %q", meta.AccessProviderType, accessType)
	}
	if meta.ProtocolVersion != plugin.ProtocolVersion {
		t.Fatalf("protocol version = %d", meta.ProtocolVersion)
	}
	if meta.DeployCategory != "ssl" {
		t.Fatalf("deploy category = %q, want ssl", meta.DeployCategory)
	}
	if meta.DeployDisplayNameKey != displayNameKey {
		t.Fatalf("display name key = %q", meta.DeployDisplayNameKey)
	}
}

func TestGetConfigSchema(t *testing.T) {
	schema, err := (&wangsuCertificateDeployer{}).GetConfigSchema(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	type envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		Provider      string `json:"provider"`
		Category      string `json:"category"`
		Schema        struct {
			Columns []struct {
				Name           string `json:"name"`
				ValueType      string `json:"valueType"`
				LabelKey       string `json:"labelKey,omitempty"`
				PlaceholderKey string `json:"placeholderKey,omitempty"`
				Options        []struct {
					Value    string `json:"value"`
					LabelKey string `json:"labelKey,omitempty"`
				} `json:"options,omitempty"`
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

	credentialFields := map[string]bool{"accessKeySecret": true, "apiKey": true}
	for _, c := range accessEnv.Schema.Columns {
		if credentialFields[c.Name] && c.ValueType != "secret" {
			t.Fatalf("access column %q must carry the secret valueType, got %q", c.Name, c.ValueType)
		}
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
				if c.PlaceholderKey != "" && bundle[c.PlaceholderKey] == "" {
					t.Fatalf("locale %s missing i18n key %q", locale, c.PlaceholderKey)
				}
				for _, opt := range c.Options {
					if opt.LabelKey != "" && bundle[opt.LabelKey] == "" {
						t.Fatalf("locale %s missing i18n key %q", locale, opt.LabelKey)
					}
				}
			}
		}
	}
}

func TestDeploy_InvalidAccessConfig(t *testing.T) {
	_, err := (&wangsuCertificateDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{not-json`,
		ExtendedConfigJSON: `{}`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid access config") {
		t.Fatalf("expected invalid access config error, got %v", err)
	}
}

func TestDeploy_InvalidDeployConfig(t *testing.T) {
	_, err := (&wangsuCertificateDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{"accessKeyId":"ak","accessKeySecret":"sk","apiKey":"api-key"}`,
		ExtendedConfigJSON: `{not-json`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid deploy config") {
		t.Fatalf("expected invalid deploy config error, got %v", err)
	}
}

func TestDeploy_BranchValidation(t *testing.T) {
	_, err := (&wangsuCertificateDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{"accessKeyId":"","accessKeySecret":""}`,
		ExtendedConfigJSON: `{"certificateId":"123456"}`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "could not create certmgr") {
		t.Fatalf("expected certmgr creation error, got %v", err)
	}
}

func TestConfigParseParity(t *testing.T) {
	var access accessConfig
	if err := json.Unmarshal([]byte(`{"accessKeyId":"ak","accessKeySecret":"sk","apiKey":"api-key"}`), &access); err != nil {
		t.Fatal(err)
	}
	if access.AccessKeyId != "ak" || access.AccessKeySecret != "sk" || access.ApiKey != "api-key" {
		t.Fatalf("access config round-trip mismatch: %+v", access)
	}

	var uploadExtended extendedConfig
	if err := json.Unmarshal([]byte(`{}`), &uploadExtended); err != nil {
		t.Fatal(err)
	}
	if uploadExtended.CertificateId != "" {
		t.Fatalf("certificateId = %q, want empty for the upload branch", uploadExtended.CertificateId)
	}

	var replaceExtended extendedConfig
	if err := json.Unmarshal([]byte(`{"certificateId":"123456"}`), &replaceExtended); err != nil {
		t.Fatal(err)
	}
	if replaceExtended.CertificateId != "123456" {
		t.Fatalf("certificateId = %q, want 123456", replaceExtended.CertificateId)
	}
}
