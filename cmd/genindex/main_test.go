package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	d := filepath.Join(parts...)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func generate(t *testing.T, root string) []byte {
	t.Helper()
	plugins, err := buildIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	buf, err := renderIndex(plugins)
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

func TestBuildIndex_HappyPathSorted(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "gamma")
	writeFile(t, filepath.Join(root, "gamma", "manifest.json"), `{"provider_type":"gamma","version":"3.0.0"}`)
	mkdir(t, root, "alpha")
	writeFile(t, filepath.Join(root, "alpha", "manifest.json"), `{"provider_type":"alpha","version":"1.0.0"}`)
	mkdir(t, root, "beta")
	writeFile(t, filepath.Join(root, "beta", "manifest.json"), `{"provider_type":"beta","version":"2.0.0"}`)

	got, err := buildIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d", len(got))
	}
	want := []string{"alpha", "beta", "gamma"}
	for i, w := range want {
		if got[i]["provider_type"] != w {
			t.Fatalf("entry %d: want %q, got %v", i, w, got[i]["provider_type"])
		}
	}
}

func TestBuildIndex_SkipsNonPluginDirs(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "real")
	writeFile(t, filepath.Join(root, "real", "manifest.json"), `{"provider_type":"real","version":"1.0.0"}`)
	mkdir(t, root, "_template")
	writeFile(t, filepath.Join(root, "_template", "manifest.json"), `{"provider_type":"__PLUGIN_NAME__","version":"0.1.0"}`)
	mkdir(t, root, ".hidden")
	writeFile(t, filepath.Join(root, ".hidden", "manifest.json"), `{"provider_type":"sneaky","version":"1.0.0"}`)
	mkdir(t, root, "nodoc")

	got, err := buildIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entry (real only), got %d: %v", len(got), got)
	}
	if got[0]["provider_type"] != "real" {
		t.Fatalf("want real, got %v", got[0]["provider_type"])
	}
}

func TestBuildIndex_MalformedFails(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "broken")
	writeFile(t, filepath.Join(root, "broken", "manifest.json"), `{not valid json`)

	if _, err := buildIndex(root); err == nil {
		t.Fatal("want error for malformed manifest, got nil")
	}
}

func TestGenerate_IdempotentAndFaithful(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "beta")
	writeFile(t, filepath.Join(root, "beta", "manifest.json"), `{"provider_type":"beta","version":"2.0.0","binary":"beta","release":{"repo":"certimate-go/plugins","tag":"v2"}}`)
	mkdir(t, root, "alpha")
	writeFile(t, filepath.Join(root, "alpha", "manifest.json"), `{"provider_type":"alpha","version":"1.0.0","binary":"alpha"}`)

	first := generate(t, root)
	second := generate(t, root)
	if !bytes.Equal(first, second) {
		t.Fatalf("output not stable across runs\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if !bytes.Contains(first, []byte(`"alpha"`)) || !bytes.Contains(first, []byte(`"beta"`)) {
		t.Fatalf("output missing entries: %s", first)
	}
	if !bytes.Contains(first, []byte(`"release"`)) || !bytes.Contains(first, []byte(`"certimate-go/plugins"`)) {
		t.Fatalf("release block not preserved faithfully: %s", first)
	}
	if bytes.Index(first, []byte(`"alpha"`)) > bytes.Index(first, []byte(`"beta"`)) {
		t.Fatalf("entries not sorted by provider_type: %s", first)
	}
}
