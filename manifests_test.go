package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestAllPluginManifestsSatisfyCoreContract(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read repo root: %v", err)
	}

	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == "_template" || strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(name, "manifest.json")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Errorf("%s: read manifest: %v", name, err)
			continue
		}
		m, err := plugin.ParseManifest(data)
		if err != nil {
			t.Errorf("%s: parse manifest: %v", name, err)
			continue
		}
		if err := m.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if m.ProtocolVersion != plugin.ProtocolVersion {
			t.Errorf("%s: protocol_version %d does not match core protocol %d", name, m.ProtocolVersion, plugin.ProtocolVersion)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no plugin manifests found")
	}
	t.Logf("validated %d plugin manifests against core contract", checked)
}
