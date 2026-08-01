package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeBinary(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// setUpPlugin creates a temp repo root with one plugin manifest and fake dist
// binaries for every target, returning the root, dist dir, and a map of
// assetKey -> expected sha256.
func setUpPlugin(t *testing.T, manifestBody string, binaryContent []byte) (root, distDir string, expected map[string]string) {
	t.Helper()
	root = t.TempDir()
	pluginDir := filepath.Join(root, "alpha")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, filepath.Join(pluginDir, "manifest.json"), manifestBody)
	distDir = filepath.Join(root, "dist")
	expected = map[string]string{}
	sum := sha256.Sum256(binaryContent)
	hexSum := hex.EncodeToString(sum[:])
	for _, tg := range targets {
		writeBinary(t, filepath.Join(distDir, assetName("alpha", tg.goos, tg.goarch)), binaryContent)
		expected[assetKey(tg.goos, tg.goarch)] = hexSum
	}
	return root, distDir, expected
}

func TestApplyRelease_HappyPath(t *testing.T) {
	root, distDir, expected := setUpPlugin(t,
		`{"provider_type":"alpha","version":"1.2.0","icon":"alpha.png","priority":50,"usages":["hosting"]}`,
		[]byte("fake binary bytes"),
	)
	manifest := filepath.Join(root, "alpha", "manifest.json")

	out, err := applyRelease(manifest, "alpha", "usual2970/certimate-plugins", distDir)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	rel, ok := m["release"].(map[string]any)
	if !ok {
		t.Fatalf("missing release block: %s", out)
	}
	if rel["repo"] != "usual2970/certimate-plugins" {
		t.Fatalf("repo: want usual2970/certimate-plugins, got %v", rel["repo"])
	}
	if rel["tag"] != "alpha/v1.2.0" {
		t.Fatalf("tag: want alpha/v1.2.0, got %v", rel["tag"])
	}
	assets, _ := rel["assets"].(map[string]any)
	if assets["linux/amd64"] != "alpha_linux_amd64" {
		t.Fatalf("assets[linux/amd64]: want alpha_linux_amd64, got %v", assets["linux/amd64"])
	}
	if assets["windows/amd64"] != "alpha_windows_amd64.exe" {
		t.Fatalf("assets[windows/amd64]: want alpha_windows_amd64.exe, got %v", assets["windows/amd64"])
	}
	if len(assets) != len(targets) {
		t.Fatalf("assets: want %d entries, got %d", len(targets), len(assets))
	}
	checksums, _ := rel["checksums"].(map[string]any)
	if len(checksums) != len(targets) {
		t.Fatalf("checksums: want %d entries, got %d", len(targets), len(checksums))
	}
	for key, want := range expected {
		if got := checksums[key]; got != want {
			t.Fatalf("checksums[%s]: want %s, got %v", key, want, got)
		}
	}
	// Non-release fields are preserved.
	if m["icon"] != "alpha.png" || m["priority"] != float64(50) {
		t.Fatalf("non-release fields not preserved: %s", out)
	}
}

func TestApplyRelease_Idempotent(t *testing.T) {
	root, distDir, _ := setUpPlugin(t,
		`{"provider_type":"alpha","version":"1.2.0"}`,
		[]byte("stable bytes"),
	)
	manifest := filepath.Join(root, "alpha", "manifest.json")
	first, err := applyRelease(manifest, "alpha", "o/r", distDir)
	if err != nil {
		t.Fatal(err)
	}
	// Write the first result back, then apply again on the updated manifest.
	if err := os.WriteFile(manifest, first, 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := applyRelease(manifest, "alpha", "o/r", distDir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("output not idempotent\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestApplyRelease_MissingBinaryErrors(t *testing.T) {
	root, distDir, _ := setUpPlugin(t,
		`{"provider_type":"alpha","version":"1.0.0"}`,
		[]byte("x"),
	)
	// Remove one binary to simulate an incomplete build.
	if err := os.Remove(filepath.Join(distDir, "alpha_linux_arm64")); err != nil {
		t.Fatal(err)
	}
	_, err := applyRelease(filepath.Join(root, "alpha", "manifest.json"), "alpha", "o/r", distDir)
	if err == nil {
		t.Fatal("want error for missing binary, got nil")
	}
}

func TestApplyRelease_MalformedManifestErrors(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "alpha")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, filepath.Join(pluginDir, "manifest.json"), `{not valid json`)
	_, err := applyRelease(filepath.Join(pluginDir, "manifest.json"), "alpha", "o/r", filepath.Join(root, "dist"))
	if err == nil {
		t.Fatal("want error for malformed manifest, got nil")
	}
}

func TestApplyRelease_BadVersionErrors(t *testing.T) {
	root, distDir, _ := setUpPlugin(t,
		`{"provider_type":"alpha","version":"1.0"}`, // not full semver
		[]byte("x"),
	)
	_, err := applyRelease(filepath.Join(root, "alpha", "manifest.json"), "alpha", "o/r", distDir)
	if err == nil {
		t.Fatal("want error for bad version, got nil")
	}
}

func TestApplyRelease_ProviderTypeMismatchErrors(t *testing.T) {
	root, distDir, _ := setUpPlugin(t,
		`{"provider_type":"alpha","version":"1.0.0"}`,
		[]byte("x"),
	)
	_, err := applyRelease(filepath.Join(root, "alpha", "manifest.json"), "beta", "o/r", distDir)
	if err == nil {
		t.Fatal("want error for provider_type mismatch, got nil")
	}
}
