package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestGetMetadata(t *testing.T) {
	meta, err := (&ksyunCdnDeployer{}).GetMetadata(context.Background())
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
	if meta.DeployCategory != "cdn" {
		t.Fatalf("deploy category = %q, want cdn", meta.DeployCategory)
	}
	if meta.DeployDisplayNameKey != displayNameKey {
		t.Fatalf("display name key = %q", meta.DeployDisplayNameKey)
	}
}

func TestGetConfigSchema(t *testing.T) {
	schema, err := (&ksyunCdnDeployer{}).GetConfigSchema(context.Background())
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
				Required       bool   `json:"required,omitempty"`
				RequiredWhen   []struct {
					Field  string   `json:"field"`
					Op     string   `json:"op"`
					Values []string `json:"values"`
				} `json:"requiredWhen,omitempty"`
				ValidateWhen []struct {
					Field  string   `json:"field"`
					Op     string   `json:"op"`
					Values []string `json:"values"`
				} `json:"validateWhen,omitempty"`
				Options []struct {
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

	credentialFields := map[string]bool{"secretAccessKey": true}
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

	domainSeen := false
	for i := range deployEnv.Schema.Columns {
		col := &deployEnv.Schema.Columns[i]
		if col.Name != "domain" {
			continue
		}
		domainSeen = true
		if col.Required {
			t.Fatalf("domain column must stay conditionally required, not unconditionally required")
		}
		if len(col.RequiredWhen) == 0 {
			t.Fatalf("domain column must gate requiredness on domainMatchPattern")
		}
		for _, cond := range col.RequiredWhen {
			if cond.Field != "domainMatchPattern" || cond.Op != "in" {
				t.Fatalf("domain requiredWhen must match domainMatchPattern in [exact wildcard], got %+v", cond)
			}
			for _, v := range cond.Values {
				if v == "certsan" {
					t.Fatalf("domain must not be required when domainMatchPattern is certsan")
				}
			}
		}
		for _, rule := range col.ValidateWhen {
			if rule.Field != "domainMatchPattern" || rule.Op != "in" {
				t.Fatalf("domain validateWhen must match domainMatchPattern in [exact wildcard], got %+v", rule)
			}
			for _, v := range rule.Values {
				if v == "certsan" {
					t.Fatalf("domain format must not be enforced when domainMatchPattern is certsan")
				}
			}
		}
	}
	if !domainSeen {
		t.Fatalf("deploy schema must include the domain column")
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
	_, err := (&ksyunCdnDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{not-json`,
		ExtendedConfigJSON: `{"deployTarget":"domain","domainMatchPattern":"exact","domain":"example.com"}`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid access config") {
		t.Fatalf("expected invalid access config error, got %v", err)
	}
}

func TestDeploy_InvalidDeployConfig(t *testing.T) {
	_, err := (&ksyunCdnDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{"accessKeyId":"ak","secretAccessKey":"sk"}`,
		ExtendedConfigJSON: `{not-json`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid deploy config") {
		t.Fatalf("expected invalid deploy config error, got %v", err)
	}
}

func TestDeploy_BranchValidation(t *testing.T) {
	accessConfigJSON := `{"accessKeyId":"test-access-key-id","secretAccessKey":"test-secret-access-key"}`

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
			name:             "deploy to certificate without certificateId",
			extendedConfig:   `{"deployTarget":"certificate"}`,
			wantErrSubstring: "config `certificateId` is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&ksyunCdnDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
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
	if err := json.Unmarshal([]byte(`{"accessKeyId":"ak","secretAccessKey":"sk"}`), &access); err != nil {
		t.Fatal(err)
	}
	if access.AccessKeyId != "ak" || access.SecretAccessKey != "sk" {
		t.Fatalf("access config round-trip mismatch: %+v", access)
	}

	var domainExtended extendedConfig
	if err := json.Unmarshal([]byte(`{"deployTarget":"domain","domainMatchPattern":"exact","domain":"example.com"}`), &domainExtended); err != nil {
		t.Fatal(err)
	}
	if domainExtended.DeployTarget != deployTargetDomain {
		t.Fatalf("deployTarget = %q, want %q", domainExtended.DeployTarget, deployTargetDomain)
	}
	if domainExtended.DomainMatchPattern != domainMatchPatternExact {
		t.Fatalf("domainMatchPattern = %q, want %q", domainExtended.DomainMatchPattern, domainMatchPatternExact)
	}
	if domainExtended.Domain != "example.com" {
		t.Fatalf("domain = %q, want example.com", domainExtended.Domain)
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
