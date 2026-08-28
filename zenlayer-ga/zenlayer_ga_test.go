package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestGetMetadata(t *testing.T) {
	meta, err := (&zenlayerGaDeployer{}).GetMetadata(context.Background())
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
	if meta.DeployCategory != "accelerator" {
		t.Fatalf("deploy category = %q, want accelerator", meta.DeployCategory)
	}
	if meta.DeployDisplayNameKey != displayNameKey {
		t.Fatalf("display name key = %q", meta.DeployDisplayNameKey)
	}
}

func TestGetConfigSchema(t *testing.T) {
	schema, err := (&zenlayerGaDeployer{}).GetConfigSchema(context.Background())
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
	_, err := (&zenlayerGaDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{not-json`,
		ExtendedConfigJSON: `{"deployTarget":"accelerator","acceleratorId":"ga-123"}`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid access config") {
		t.Fatalf("expected invalid access config error, got %v", err)
	}
}

func TestDeploy_InvalidDeployConfig(t *testing.T) {
	_, err := (&zenlayerGaDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{"accessKeyId":"ak","accessKeyPassword":"sk","resourceGroupId":"rg"}`,
		ExtendedConfigJSON: `{not-json`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid deploy config") {
		t.Fatalf("expected invalid deploy config error, got %v", err)
	}
}

func TestDeploy_BranchValidation(t *testing.T) {
	accessConfigJSON := `{"accessKeyId":"test-access-key-id","accessKeyPassword":"test-access-key-password"}`

	tests := []struct {
		name             string
		extendedConfig   string
		wantErrSubstring string
	}{
		{
			name:             "unsupported deploy target",
			extendedConfig:   `{"deployTarget":"bogus"}`,
			wantErrSubstring: "unsupported deploy target 'bogus'",
		},
		{
			name:             "deploy to accelerator without acceleratorId",
			extendedConfig:   `{"deployTarget":"accelerator"}`,
			wantErrSubstring: "config `acceleratorId` is required",
		},
		{
			name:             "deploy to certificate without certificateId",
			extendedConfig:   `{"deployTarget":"certificate"}`,
			wantErrSubstring: "config `certificateId` is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&zenlayerGaDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
				AccessConfigJSON:   accessConfigJSON,
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
	if err := json.Unmarshal([]byte(`{"accessKeyId":"ak","accessKeyPassword":"sk","resourceGroupId":"rg"}`), &access); err != nil {
		t.Fatal(err)
	}
	if access.AccessKeyId != "ak" || access.AccessKeyPassword != "sk" || access.ResourceGroupId != "rg" {
		t.Fatalf("access config round-trip mismatch: %+v", access)
	}

	var acceleratorExtended extendedConfig
	if err := json.Unmarshal([]byte(`{"deployTarget":"accelerator","acceleratorId":"ga-123"}`), &acceleratorExtended); err != nil {
		t.Fatal(err)
	}
	if acceleratorExtended.DeployTarget != deployTargetAccelerator {
		t.Fatalf("deployTarget = %q, want %q", acceleratorExtended.DeployTarget, deployTargetAccelerator)
	}
	if acceleratorExtended.AcceleratorId != "ga-123" {
		t.Fatalf("acceleratorId = %q, want ga-123", acceleratorExtended.AcceleratorId)
	}

	var certExtended extendedConfig
	if err := json.Unmarshal([]byte(`{"deployTarget":"certificate","certificateId":"cert-123"}`), &certExtended); err != nil {
		t.Fatal(err)
	}
	if certExtended.DeployTarget != deployTargetCertificate {
		t.Fatalf("deployTarget = %q, want %q", certExtended.DeployTarget, deployTargetCertificate)
	}
	if certExtended.CertificateId != "cert-123" {
		t.Fatalf("certificateId = %q, want cert-123", certExtended.CertificateId)
	}
}
