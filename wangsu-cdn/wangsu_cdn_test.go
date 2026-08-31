package main

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestGetMetadata(t *testing.T) {
	meta, err := (&wangsuCdnDeployer{}).GetMetadata(context.Background())
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
	schema, err := (&wangsuCdnDeployer{}).GetConfigSchema(context.Background())
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

func TestDeploySchema_DomainsColumnCarriesNoValidator(t *testing.T) {
	var deployEnv struct {
		Schema struct {
			Columns []map[string]any `json:"columns"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(mustLoadDeploySchemaJSON(t), &deployEnv); err != nil {
		t.Fatalf("deploy envelope invalid: %v", err)
	}

	for _, col := range deployEnv.Schema.Columns {
		if col["name"] != "domains" {
			continue
		}
		if _, ok := col["validateWhen"]; ok {
			t.Fatalf("domains column must not carry validateWhen: form/v1's domain validator applies to the whole ';'-joined string and would reject %q", "a.com;b.com")
		}
		return
	}

	t.Fatal("deploy schema has no domains column")
}

func mustLoadDeploySchemaJSON(t *testing.T) []byte {
	t.Helper()

	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		t.Fatal(err)
	}
	return deploySchema
}

func TestDeploy_InvalidAccessConfig(t *testing.T) {
	_, err := (&wangsuCdnDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{not-json`,
		ExtendedConfigJSON: `{"domainMatchPattern":"exact","domains":"example.com"}`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid access config") {
		t.Fatalf("expected invalid access config error, got %v", err)
	}
}

func TestDeploy_InvalidDeployConfig(t *testing.T) {
	_, err := (&wangsuCdnDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{"accessKeyId":"ak","accessKeySecret":"sk","apiKey":"api-key"}`,
		ExtendedConfigJSON: `{not-json`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid deploy config") {
		t.Fatalf("expected invalid deploy config error, got %v", err)
	}
}

func TestDeploy_BranchValidation(t *testing.T) {
	accessConfigJSON := `{"accessKeyId":"","accessKeySecret":""}`

	_, err := (&wangsuCdnDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   accessConfigJSON,
		ExtendedConfigJSON: `{"domainMatchPattern":"exact","domains":"example.com"}`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "could not create client") {
		t.Fatalf("expected client creation error, got %v", err)
	}
}

func TestResolveDeployDomains(t *testing.T) {
	tests := []struct {
		name             string
		pattern          string
		domainsRaw       string
		wantDomains      []string
		wantErrSubstring string
	}{
		{
			name:        "exact pattern with multiple semicolon-joined domains",
			pattern:     domainMatchPatternExact,
			domainsRaw:  "example.com;example.org",
			wantDomains: []string{"example.com", "example.org"},
		},
		{
			name:        "empty pattern defaults to exact",
			pattern:     "",
			domainsRaw:  "example.com",
			wantDomains: []string{"example.com"},
		},
		{
			name:        "wildcard prefix is normalized",
			pattern:     domainMatchPatternExact,
			domainsRaw:  "*.example.com;example.com",
			wantDomains: []string{".example.com", "example.com"},
		},
		{
			name:             "empty domains under exact pattern",
			pattern:          domainMatchPatternExact,
			domainsRaw:       "",
			wantErrSubstring: "config `domains` is required",
		},
		{
			name:             "unsupported pattern",
			pattern:          "wildcard",
			domainsRaw:       "example.com",
			wantErrSubstring: "unsupported domain match pattern: 'wildcard'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domains, err := resolveDeployDomains(tt.pattern, tt.domainsRaw)
			if tt.wantErrSubstring != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstring) {
					t.Fatalf("resolveDeployDomains() error = %v, want error containing %q", err, tt.wantErrSubstring)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(domains, tt.wantDomains) {
				t.Fatalf("resolveDeployDomains() = %v, want %v", domains, tt.wantDomains)
			}
		})
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

	var extended extendedConfig
	if err := json.Unmarshal([]byte(`{"domainMatchPattern":"exact","domains":"a.com;b.com"}`), &extended); err != nil {
		t.Fatal(err)
	}
	if extended.DomainMatchPattern != domainMatchPatternExact {
		t.Fatalf("domainMatchPattern = %q, want %q", extended.DomainMatchPattern, domainMatchPatternExact)
	}
	if extended.Domains != "a.com;b.com" {
		t.Fatalf("stored semicolon-joined domains must round-trip unchanged, got %q", extended.Domains)
	}
}
