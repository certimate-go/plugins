package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestPublishedPluginManifestsSatisfyCoreContract(t *testing.T) {
	paths, err := filepath.Glob("registry/market/details/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no published plugin details found")
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var detail struct {
			ProviderType string          `json:"provider_type"`
			Manifest     json.RawMessage `json:"manifest"`
		}
		if err := json.Unmarshal(data, &detail); err != nil {
			t.Fatalf("%s: parse published detail: %v", path, err)
		}
		manifest, err := plugin.ParseManifest(detail.Manifest)
		if err != nil {
			t.Fatalf("%s: parse published manifest: %v", path, err)
		}
		if err := manifest.Validate(); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if manifest.ProviderType != detail.ProviderType {
			t.Errorf("%s: detail provider %q differs from manifest provider %q", path, detail.ProviderType, manifest.ProviderType)
		}
		if manifest.ProtocolVersion != plugin.ProtocolVersion {
			t.Errorf("%s: protocol_version %d does not match core protocol %d", path, manifest.ProtocolVersion, plugin.ProtocolVersion)
		}
	}
}
