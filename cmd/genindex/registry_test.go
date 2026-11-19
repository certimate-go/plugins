package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/certimate-go/plugins/internal/providerrelease"
	"github.com/certimate-go/plugins/internal/registry"
)

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	dir := filepath.Join(parts...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func registryFixture(t *testing.T) (records, observations string) {
	t.Helper()
	root := t.TempDir()
	records = mkdir(t, root, "registry")
	policy := mkdir(t, records, "_policy")
	writeFile(t, filepath.Join(policy, "admissions.json"), `{"alpha":"certimate-go/plugin-provider-alpha","beta":"certimate-go/plugin-provider-beta"}`)
	writeFile(t, filepath.Join(policy, "withdrawals.json"), "[]")
	observations = filepath.Join(root, "observations.json")
	writeFile(t, observations, "[]")
	return records, observations
}

func registryVersion(provider, version, hash string) registry.Version {
	targets := providerrelease.Targets()
	artifacts := make([]providerrelease.Artifact, 0, len(targets))
	for _, target := range targets {
		artifacts = append(artifacts, providerrelease.Artifact{
			Target: target, Path: "build/" + target.ArchiveName(), Size: 123, SHA256: strings.Repeat("2", 64),
		})
	}
	return registry.Version{
		Record: providerrelease.Record{
			ProviderType: provider, Version: version, Repository: "certimate-go/plugin-provider-" + provider,
			Tag: "v" + version, SourceCommit: strings.Repeat("1", 40), BuildRunID: "123456", Artifacts: artifacts,
		},
		ReleaseCommit: strings.Repeat(hash, 40),
		Manifest: plugin.Manifest{
			ProviderType: provider, Version: version, Capability: plugin.CapabilityDns, ProtocolVersion: plugin.ProtocolVersion,
			AccessProviderType: "volcengine", Binary: provider + ".exe", MinCoreVersion: "v0.5.0",
		},
		Translations: map[string]registry.Translation{"zh": {Name: "火山引擎"}},
	}
}

func putVersion(t *testing.T, records string, version registry.Version) registry.ReadyRelease {
	t.Helper()
	providerDir := mkdir(t, records, version.ProviderType)
	writeJSON(t, filepath.Join(providerDir, version.Tag+".json"), version)
	return registry.ReadyRelease{
		Identity:      registry.Identity{ProviderType: version.ProviderType, Version: version.Version},
		ReleaseCommit: version.ReleaseCommit,
	}
}

func writeJSON(t *testing.T, destination string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, destination, string(data))
}

func readCatalog(t *testing.T, directory string) ([]byte, registry.Catalog) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(directory, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog registry.Catalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatal(err)
	}
	return body, catalog
}

func TestRegistryCatalogSelectsPromotedStableVersions(t *testing.T) {
	records, observations := registryFixture(t)
	output := t.TempDir()
	old := registryVersion("alpha", "1.9.0", "a")
	current := registryVersion("alpha", "1.10.0", "b")
	pending := registryVersion("alpha", "1.11.0", "c")
	narrowing := registryVersion("alpha", "1.12.0", "d")
	narrowing.Manifest.MinCoreVersion = "0.6.0"
	beta := registryVersion("beta", "1.1.0", "e")
	imageFile := image.NewRGBA(image.Rect(0, 0, 1, 1))
	imageFile.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, imageFile); err != nil {
		t.Fatal(err)
	}
	current.Icon = encoded.Bytes()

	oldObservation := putVersion(t, records, old)
	newObservation := putVersion(t, records, current)
	putVersion(t, records, pending)
	narrowObservation := putVersion(t, records, narrowing)
	writeJSON(t, observations, []registry.ReadyRelease{narrowObservation, newObservation, oldObservation})
	if err := generateRegisteredCatalog(records, observations, output); err != nil {
		t.Fatal(err)
	}
	before, catalog := readCatalog(t, output)
	if len(catalog.Plugins) != 1 || catalog.Plugins[0].Version != current.Version {
		t.Fatalf("expected only alpha %s, got %+v", current.Version, catalog.Plugins)
	}
	summary := catalog.Plugins[0]
	if len(summary.Platforms) != 8 || !slices.IsSorted(summary.Platforms) || summary.Translations["zh"].Name != "火山引擎" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.Icon == nil || summary.Icon.Format != "png" {
		t.Fatalf("missing PNG icon reference: %+v", summary.Icon)
	}
	for relative, expected := range map[string]string{
		"market/details/" + summary.Detail.SHA256 + ".json": summary.Detail.SHA256,
		"market/icons/" + summary.Icon.SHA256 + ".png":      summary.Icon.SHA256,
	} {
		body, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(relative)))
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(body)) != expected {
			t.Fatalf("asset %q digest mismatch: %v", relative, err)
		}
	}

	slices.Reverse(current.Artifacts)
	putVersion(t, records, current)
	writeJSON(t, observations, []registry.ReadyRelease{oldObservation, narrowObservation, newObservation})
	secondOutput := t.TempDir()
	if err := generateRegisteredCatalog(records, observations, secondOutput); err != nil {
		t.Fatal(err)
	}
	after, _ := readCatalog(t, secondOutput)
	if !bytes.Equal(before, after) {
		t.Fatal("catalog bytes changed after only reordering observations and platform records")
	}

	writeJSON(t, filepath.Join(records, "_policy", "withdrawals.json"), []registry.Withdrawal{
		{Identity: newObservation.Identity, Reason: "Withdraw broken release"},
	})
	writeJSON(t, observations, []registry.ReadyRelease{oldObservation, narrowObservation, newObservation, putVersion(t, records, beta)})
	if err := generateRegisteredCatalog(records, observations, output); err != nil {
		t.Fatal(err)
	}
	_, recovered := readCatalog(t, output)
	if len(recovered.Plugins) != 2 || recovered.Plugins[0].Version != old.Version || recovered.Plugins[1].Version != beta.Version {
		t.Fatalf("withdrawal did not restore the last usable alpha version while allowing beta: %+v", recovered.Plugins)
	}
}

func TestCLIExportsLocalizedRegistryCatalog(t *testing.T) {
	records, observations := registryFixture(t)
	version := registryVersion("alpha", "1.2.3", "d")
	version.Translations["en"] = registry.Translation{Name: "Example plugin", Description: "Example description"}
	writeJSON(t, observations, []registry.ReadyRelease{putVersion(t, records, version)})
	sourceOnly := mkdir(t, filepath.Dir(records), "source-only")
	writeFile(t, filepath.Join(sourceOnly, "manifest.json"), `{"provider_type":"source-only","version":"9.0.0"}`)
	binary := filepath.Join(t.TempDir(), "genindex")
	if output, err := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build genindex: %v\n%s", err, output)
	}
	destination := t.TempDir()
	if output, err := exec.CommandContext(t.Context(), binary,
		"-registry", records, "-ready", observations, "-out-dir", destination).CombinedOutput(); err != nil {
		t.Fatalf("generate stage: %v\n%s", err, output)
	}
	_, catalog := readCatalog(t, destination)
	if len(catalog.Plugins) != 1 || catalog.Plugins[0].ProviderType != "alpha" ||
		catalog.Plugins[0].Translations["en"] != version.Translations["en"] || catalog.Plugins[0].Translations["zh"] != version.Translations["zh"] {
		t.Fatalf("unexpected registry translation output: %+v", catalog)
	}
	if _, err := os.Stat(filepath.Join(destination, "index.json")); !os.IsNotExist(err) {
		t.Fatalf("unsigned generator must not produce a public index: %v", err)
	}
}

func TestRegistryCatalogRefusesInvalidRecords(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*registry.Version, *registry.ReadyRelease)
		want   string
	}{
		{"bad-path", func(record *registry.Version, ready *registry.ReadyRelease) { record.Tag = "v2.0.0" }, "invalid tag"},
		{"missing-digest", func(record *registry.Version, ready *registry.ReadyRelease) { record.Artifacts[0].SHA256 = "" }, "invalid or duplicate artifact"},
		{"duplicate-platform", func(record *registry.Version, ready *registry.ReadyRelease) {
			record.Artifacts[7] = record.Artifacts[0]
		}, "invalid or duplicate artifact"},
		{"incomplete-platforms", func(record *registry.Version, ready *registry.ReadyRelease) { record.Artifacts = record.Artifacts[1:] }, "incomplete platform"},
		{"different-artifact-commit", func(record *registry.Version, ready *registry.ReadyRelease) {
			ready.ReleaseCommit = strings.Repeat("b", 40)
		}, "observed commit differs"},
		{"wrong-repository", func(record *registry.Version, ready *registry.ReadyRelease) {
			record.Repository = "attacker/plugin-provider-alpha"
		}, "does not match path or admission"},
		{"invalid-download-path", func(record *registry.Version, ready *registry.ReadyRelease) {
			record.Artifacts[0].Path = "../wrong.zip"
		}, "invalid or duplicate artifact"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			records, observations := registryFixture(t)
			version := registryVersion("alpha", "1.0.0", "a")
			ready := registry.ReadyRelease{Identity: registry.Identity{ProviderType: "alpha", Version: "1.0.0"}, ReleaseCommit: version.ReleaseCommit}
			scenario.change(&version, &ready)
			putVersion(t, records, version)
			writeJSON(t, observations, []registry.ReadyRelease{ready})
			output := t.TempDir()
			err := generateRegisteredCatalog(records, observations, output)
			if err == nil || !strings.Contains(err.Error(), scenario.want) {
				t.Fatalf("expected %q, got %v", scenario.want, err)
			}
			if _, err := os.Stat(filepath.Join(output, "catalog.json")); !os.IsNotExist(err) {
				t.Fatalf("invalid records emitted a catalog: %v", err)
			}
		})
	}
}
