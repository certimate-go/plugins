package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestGetMetadata(t *testing.T) {
	meta, err := (&upyunFileDeployer{}).GetMetadata(context.Background())
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
	if meta.DeployCategory != "storage" {
		t.Fatalf("deploy category = %q, want storage", meta.DeployCategory)
	}
	if meta.DeployDisplayNameKey != displayNameKey {
		t.Fatalf("display name key = %q", meta.DeployDisplayNameKey)
	}
}

func TestGetConfigSchema(t *testing.T) {
	schema, err := (&upyunFileDeployer{}).GetConfigSchema(context.Background())
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
	_, err := (&upyunFileDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{not-json`,
		ExtendedConfigJSON: `{"bucket":"b","domain":"example.com"}`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid access config") {
		t.Fatalf("expected invalid access config error, got %v", err)
	}
}

func TestDeploy_InvalidDeployConfig(t *testing.T) {
	_, err := (&upyunFileDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{"username":"u","password":"p"}`,
		ExtendedConfigJSON: `{not-json`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid deploy config") {
		t.Fatalf("expected invalid deploy config error, got %v", err)
	}
}

func TestDeploy_BranchValidation(t *testing.T) {
	tests := []struct {
		name             string
		accessConfig     string
		extendedConfig   string
		wantErrSubstring string
	}{
		{
			name:             "empty credentials rejected before any request",
			accessConfig:     `{}`,
			extendedConfig:   `{"bucket":"test-bucket","domain":"example.com"}`,
			wantErrSubstring: "could not create certmgr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&upyunFileDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
				AccessConfigJSON:   tt.accessConfig,
				ExtendedConfigJSON: tt.extendedConfig,
			}, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstring) {
				t.Fatalf("Deploy() error = %v, want error containing %q", err, tt.wantErrSubstring)
			}
		})
	}
}

func TestConfigParseParity(t *testing.T) {
	var access accessConfig
	if err := json.Unmarshal([]byte(`{"username":"u","password":"p"}`), &access); err != nil {
		t.Fatal(err)
	}
	if access.Username != "u" || access.Password != "p" {
		t.Fatalf("access config round-trip mismatch: %+v", access)
	}

	var bucketExtended extendedConfig
	if err := json.Unmarshal([]byte(`{"bucket":"my-bucket","domain":"cdn.example.com"}`), &bucketExtended); err != nil {
		t.Fatal(err)
	}
	if bucketExtended.Bucket != "my-bucket" {
		t.Fatalf("bucket = %q, want my-bucket", bucketExtended.Bucket)
	}
	if bucketExtended.Domain != "cdn.example.com" {
		t.Fatalf("domain = %q, want cdn.example.com", bucketExtended.Domain)
	}

	var domainOnlyExtended extendedConfig
	if err := json.Unmarshal([]byte(`{"domain":"cdn.example.com"}`), &domainOnlyExtended); err != nil {
		t.Fatal(err)
	}
	if domainOnlyExtended.Bucket != "" {
		t.Fatalf("bucket = %q, want empty when absent", domainOnlyExtended.Bucket)
	}
	if domainOnlyExtended.Domain != "cdn.example.com" {
		t.Fatalf("domain = %q, want cdn.example.com", domainOnlyExtended.Domain)
	}
}
