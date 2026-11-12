package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogKeepsPublishedDetailsAddressable(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(mkdir(t, root, "example"), "manifest.json")
	publish := func(version, releaseVersion string) string {
		t.Helper()
		writeFile(t, manifestPath, fmt.Sprintf(`{
			"provider_type":"example", "version":%q, "binary":"example",
			"release":{"repo":"certimate-go/plugins","tag":"example/v%s","assets":{"linux/amd64":"example.zip"}}
		}`, version, releaseVersion))
		manifests, err := buildIndex(root)
		if err != nil {
			t.Fatal(err)
		}
		body, err := renderIndex(root, manifests)
		if err != nil {
			t.Fatal(err)
		}
		var catalog struct {
			Plugins []struct {
				Version string           `json:"version"`
				Release json.RawMessage  `json:"release"`
				Detail  *detailReference `json:"detail"`
			} `json:"plugins"`
		}
		if err := json.Unmarshal(body, &catalog); err != nil {
			t.Fatal(err)
		}
		if len(catalog.Plugins) != 1 || catalog.Plugins[0].Version != version || catalog.Plugins[0].Release != nil {
			t.Fatalf("unexpected catalog: %s", body)
		}
		ref := catalog.Plugins[0].Detail
		if version != releaseVersion {
			if ref != nil {
				t.Fatal("unpublished version refers to an older release")
			}
			return ""
		}
		if ref == nil || len(ref.Platforms) != 1 || ref.Platforms[0] != "linux/amd64" {
			t.Fatalf("missing published platform: %+v", ref)
		}
		detail, err := os.ReadFile(filepath.Join(root, "market", "details", ref.SHA256+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(detail)) != ref.SHA256 {
			t.Fatal("catalog digest does not match published detail")
		}
		return ref.SHA256
	}

	first := publish("1.0.0", "1.0.0")
	publish("2.0.0", "1.0.0")
	second := publish("2.0.0", "2.0.0")
	if first == second {
		t.Fatal("new version reused previous detail address")
	}
	if _, err := os.Stat(filepath.Join(root, "market", "details", first+".json")); err != nil {
		t.Fatalf("cached catalogs lost access to the previous detail: %v", err)
	}
}
