package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestGetMetadata(t *testing.T) {
	meta, err := (&wangsuCdnproDeployer{}).GetMetadata(context.Background())
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
	schema, err := (&wangsuCdnproDeployer{}).GetConfigSchema(context.Background())
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

	domainRequired := false
	for _, c := range deployEnv.Schema.Columns {
		if c.Name == "domain" {
			domainRequired = c.Required
		}
	}
	if !domainRequired {
		t.Fatalf("deploy schema must mark the domain column as required")
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
	_, err := (&wangsuCdnproDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{not-json`,
		ExtendedConfigJSON: `{"environment":"production","domainMatchPattern":"exact","domain":"example.com"}`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid access config") {
		t.Fatalf("expected invalid access config error, got %v", err)
	}
}

func TestDeploy_InvalidDeployConfig(t *testing.T) {
	_, err := (&wangsuCdnproDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
		AccessConfigJSON:   `{"accessKeyId":"ak","accessKeySecret":"sk","apiKey":"api-key"}`,
		ExtendedConfigJSON: `{not-json`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid deploy config") {
		t.Fatalf("expected invalid deploy config error, got %v", err)
	}
}

func TestDeploy_BranchValidation(t *testing.T) {
	accessConfigJSON := `{"accessKeyId":"test-access-key-id","accessKeySecret":"test-secret-access-key","apiKey":"test-api-key"}`

	tests := []struct {
		name             string
		accessConfig     string
		extendedConfig   string
		wantErrSubstring string
	}{
		{
			name:             "missing domain",
			accessConfig:     accessConfigJSON,
			extendedConfig:   `{"environment":"production","domainMatchPattern":"exact","domain":""}`,
			wantErrSubstring: "config `domain` is required",
		},
		{
			name:             "unset credentials",
			accessConfig:     `{"accessKeyId":"","accessKeySecret":"","apiKey":""}`,
			extendedConfig:   `{"environment":"production","domainMatchPattern":"exact","domain":"example.com"}`,
			wantErrSubstring: "could not create client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&wangsuCdnproDeployer{}).Deploy(context.Background(), &plugin.DeployRequest{
				AccessConfigJSON:   tt.accessConfig,
				ExtendedConfigJSON: tt.extendedConfig,
			}, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstring) {
				t.Fatalf("Deploy() error = %v, want error containing %q", err, tt.wantErrSubstring)
			}
		})
	}
}

func TestParseExtendedConfig(t *testing.T) {
	extended, err := parseExtendedConfig(`{"domainMatchPattern":"exact","domain":"example.com"}`)
	if err != nil {
		t.Fatal(err)
	}
	if extended.Environment != environmentProduction {
		t.Fatalf("absent environment must default to %q, got %q", environmentProduction, extended.Environment)
	}

	extended, err = parseExtendedConfig(`{"environment":"staging","domain":"example.com"}`)
	if err != nil {
		t.Fatal(err)
	}
	if extended.Environment != "staging" {
		t.Fatalf("explicit environment must be preserved, got %q", extended.Environment)
	}

	_, err = parseExtendedConfig(`{not-json`)
	if err == nil || !strings.Contains(err.Error(), "invalid deploy config") {
		t.Fatalf("expected invalid deploy config error, got %v", err)
	}
}

func TestEncryptPrivateKey(t *testing.T) {
	privkeyPEM := "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQQexample0123456789\n-----END PRIVATE KEY-----\n"
	apiKey := "test-api-key"
	timestamp := int64(1700000000)

	encrypted, err := encryptPrivateKey(privkeyPEM, apiKey, timestamp)
	if err != nil {
		t.Fatal(err)
	}

	dateStr := time.Unix(timestamp, 0).UTC().Format(http.TimeFormat)
	h := hmac.New(sha256.New, []byte(apiKey))
	h.Write([]byte(dateStr))
	aesivkeyHex := hex.EncodeToString(h.Sum(nil))
	iv, err := hex.DecodeString(aesivkeyHex[:32])
	if err != nil {
		t.Fatal(err)
	}
	key, err := hex.DecodeString(aesivkeyHex[32:64])
	if err != nil {
		t.Fatal(err)
	}

	encBytes, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if len(encBytes)%aes.BlockSize != 0 {
		t.Fatalf("ciphertext length %d is not a multiple of the AES block size", len(encBytes))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	decBytes := make([]byte, len(encBytes))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(decBytes, encBytes)

	padlen := int(decBytes[len(decBytes)-1])
	if padlen <= 0 || padlen > aes.BlockSize {
		t.Fatalf("invalid padding length %d", padlen)
	}
	for _, b := range decBytes[len(decBytes)-padlen:] {
		if int(b) != padlen {
			t.Fatal("padding bytes are not uniform")
		}
	}
	if string(decBytes[:len(decBytes)-padlen]) != privkeyPEM {
		t.Fatal("decrypted plaintext does not match the original private key PEM")
	}
}
