package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/providerrelease"
)

func TestProviderReleaseCLI(t *testing.T) {
	providerDir := t.TempDir()
	for name, contents := range map[string]string{
		"go.mod":  "module example.test/dns\n\ngo 1.25.0\n",
		"main.go": "package main\n\nfunc main() {}\n",
		"manifest.json": `{
			"provider_type":"pilot-dns", "access_provider_type":"pilot", "capability":"dns",
			"version":"1.2.3", "binary":"pilot-dns.exe", "icon":"pilot.svg", "protocol_version":4
		}`,
		"pilot.svg": `<svg xmlns="http://www.w3.org/2000/svg"/>`,
	} {
		if err := os.WriteFile(filepath.Join(providerDir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(arguments ...string) string {
		t.Helper()
		return strings.TrimSpace(string(runCommand(t, providerDir, "git", arguments...)))
	}
	git("init", "-q")
	sharedDir := t.TempDir()
	runCommand(t, sharedDir, "git", "init", "-q")
	if err := os.WriteFile(filepath.Join(sharedDir, "state.go"), []byte("package shared\n\nconst Enabled = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCommand(t, sharedDir, "git", "add", "state.go")
	runCommand(t, sharedDir, "git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "shared source")
	git("-c", "protocol.file.allow=always", "submodule", "add", "-q", sharedDir, "internal/shared")
	git("add", ".")
	tree := git("write-tree")
	sha := git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit-tree", tree, "-m", "fixture")
	git("update-ref", "HEAD", sha)

	commandPath := filepath.Join(t.TempDir(), "providerrelease")
	runCommand(t, "", "go", "build", "-o", commandPath, ".")
	const repository = "certimate-go/plugin-provider-pilot-zone-dns"
	arguments := []string{"-source-dir", providerDir, "-repository", repository, "-source-sha", sha, "-run-id", "12345", "-trigger-tag"}
	bad := exec.CommandContext(t.Context(), commandPath, append(arguments, "v1.2.4-detach")...)
	if message, err := bad.CombinedOutput(); err == nil || !strings.Contains(string(message), "match the stable manifest version") {
		t.Fatalf("wrong version should fail before publishing: %v\n%s", err, message)
	}
	if _, err := os.Stat(filepath.Join(providerDir, "build")); !os.IsNotExist(err) {
		t.Fatalf("wrong version produced a build directory: %v", err)
	}
	git("submodule", "deinit", "-f", "internal/shared")
	missing := exec.CommandContext(t.Context(), commandPath, append(arguments, "v1.2.3-detach")...)
	if message, err := missing.CombinedOutput(); err == nil || !strings.Contains(string(message), "submodules must be checked out at their recorded commits") {
		t.Fatalf("release with a missing submodule should fail before building: %v\n%s", err, message)
	}
	git("-c", "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")
	runCommand(t, "", commandPath, append(arguments, "v1.2.3-detach")...)

	data, err := os.ReadFile(filepath.Join(providerDir, "build", providerrelease.RecordName))
	if err != nil {
		t.Fatal(err)
	}
	var record providerrelease.Record
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.ProviderType != "pilot-dns" || record.Repository != repository || record.SourceCommit != sha ||
		record.Tag != "v1.2.3" || record.BuildRunID != "12345" || len(record.Artifacts) != 8 {
		t.Fatalf("wrong release record: %+v", record)
	}
	for _, target := range providerrelease.Targets() {
		found := false
		for _, artifact := range record.Artifacts {
			if artifact.Target == target {
				checkArchive(t, providerDir, artifact)
				found = true
			}
		}
		if !found {
			t.Errorf("missing archive for %+v", target)
		}
	}
}

func checkArchive(t *testing.T, providerDir string, artifact providerrelease.Artifact) {
	t.Helper()
	if artifact.Path != "build/"+artifact.Target.ArchiveName() {
		t.Fatalf("unexpected artifact path: %q", artifact.Path)
	}
	archivePath := filepath.Join(providerDir, artifact.Path)
	contents, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Size != int64(len(contents)) || artifact.SHA256 != fmt.Sprintf("%x", sha256.Sum256(contents)) {
		t.Fatalf("archive metadata does not match %s", artifact.Path)
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 3 {
		t.Fatalf("expected manifest, icon and executable, got %d entries", len(reader.File))
	}
	var manifest *plugin.Manifest
	var binaryDigest string
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(opened)
		closeErr := opened.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("read entry %s: %v, %v", file.Name, err, closeErr)
		}
		switch file.Name {
		case "manifest.json":
			manifest, err = plugin.ParseManifest(data)
			if err != nil {
				t.Fatal(err)
			}
		case "pilot-dns.exe":
			if file.Mode().Perm() != 0o755 {
				t.Errorf("binary permissions: %v", file.Mode())
			}
			binaryDigest = fmt.Sprintf("%x", sha256.Sum256(data))
		case "pilot.svg":
			if !strings.HasPrefix(string(data), "<svg") {
				t.Error("icon contents missing")
			}
		default:
			t.Errorf("unexpected entry %q", file.Name)
		}
	}
	if manifest == nil || binaryDigest == "" || manifest.ProviderType != "pilot-dns" || manifest.OS != artifact.OS || manifest.Arch != artifact.Arch || manifest.SHA256 != binaryDigest {
		t.Fatalf("manifest does not identify the packaged binary: %+v", manifest)
	}
}

func runCommand(t *testing.T, dir, name string, arguments ...string) []byte {
	t.Helper()
	command := exec.CommandContext(t.Context(), name, arguments...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, arguments, err, output)
	}
	return output
}
