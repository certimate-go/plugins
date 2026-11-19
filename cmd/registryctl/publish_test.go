package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/certimate-go/plugins/internal/providerrelease"
	"github.com/certimate-go/plugins/internal/registry"
)

func TestSignIndexCommand(t *testing.T) {
	root := t.TempDir()
	records := filepath.Join(root, "registry")
	policy := filepath.Join(records, "_policy")
	if err := os.MkdirAll(policy, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := bytes.Repeat([]byte{0x42}, ed25519.SeedSize)
	observations := filepath.Join(root, "observations.json")
	key := filepath.Join(root, "seed")
	for file, content := range map[string]string{
		filepath.Join(policy, "withdrawals.json"): "[]",
		observations: "[]",
		key:          base64.StdEncoding.EncodeToString(seed),
	} {
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{
		"sign-index", "-registry", records, "-ready", observations, "-key-file", key, "-key-id", "local-fixture",
		"-epoch", "2", "-generation", "3", "-source-commit", strings.Repeat("a", 40),
		"-issued-at", "2026-11-18T08:30:00+08:00", "-expires-at", "2026-11-19T00:00:00Z",
	}
	var output bytes.Buffer
	if err := run(t.Context(), args, &output); err != nil {
		t.Fatal(err)
	}
	var envelope registry.IndexEnvelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	const domain = "certimate:plugin-market:index:v1\x00"
	public := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	if envelope.KeyID != "local-fixture" || !ed25519.Verify(public, append([]byte(domain), envelope.Payload...), envelope.Signature) ||
		ed25519.Verify(public, envelope.Payload, envelope.Signature) {
		t.Fatal("index signature is invalid or not domain-separated")
	}
	expected := `{"format_version":1,"epoch":2,"generation":3,"issued_at":"2026-11-18T00:30:00Z","expires_at":"2026-11-19T00:00:00Z","source_commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","plugins":[]}`
	if string(envelope.Payload) != expected {
		t.Fatalf("wire payload changed: %s", envelope.Payload)
	}
	tampered := bytes.Replace(envelope.Payload, []byte(`"generation":3`), []byte(`"generation":4`), 1)
	if ed25519.Verify(public, append([]byte(domain), tampered...), envelope.Signature) {
		t.Fatal("signature verified after changing the generation")
	}
	output.Reset()
	if err := run(t.Context(), append(args, "-expires-at", "2026-11-18T00:00:00Z"), &output); err == nil || output.Len() != 0 {
		t.Fatal("invalid expiration produced a signed index")
	}
}

func TestInspectReleaseCommand(t *testing.T) {
	root := t.TempDir()
	git := func(arguments ...string) string {
		t.Helper()
		command := exec.CommandContext(t.Context(), "git", append([]string{
			"-c", "user.name=Fixture", "-c", "user.email=fixture@example.test",
		}, arguments...)...)
		command.Dir = root
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	write := func(name, data string) {
		t.Helper()
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write("go.mod", "module example.test/dns\n\ngo 1.25\n")
	write("main.go", "package main\n\nfunc main() {}\n")
	write("manifest.json", `{"provider_type":"fixture-dns","access_provider_type":"volcengine","capability":"dns","version":"1.2.3","binary":"fixture-dns.exe","display_name_key":"plugin.fixture-dns.name","protocol_version":4}`)
	write("schema/i18n/zh.json", `{"plugin.fixture-dns.name":"样板","plugin.fixture-dns.description":"用于验证。","plugin.fixture-dns.secret.label":"密钥"}`)
	git("add", ".")
	source := git("commit-tree", git("write-tree"), "-m", "Source fixture")
	git("update-ref", "HEAD", source)
	const repository = "certimate-go/plugin-provider-fixture-zone-dns"
	const tag = "v1.2.3"
	record, err := providerrelease.Build(t.Context(), providerrelease.Options{
		SourceDir: root, Repository: repository, Trigger: tag + "-detach", SourceSHA: source, BuildRunID: "12",
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := filepath.Join(t.TempDir(), "publisher.git")
	git("clone", "--bare", root, publisher)
	stageArgs := []string{
		"stage-release", "-checkout", publisher, "-repository", repository, "-tag", tag,
		"-source-sha", source, "-run-id", "12", "-artifacts", filepath.Join(root, "build"),
	}
	var output bytes.Buffer
	if err := run(t.Context(), append(stageArgs, "-run-id", "13"), &output); err == nil || !strings.Contains(err.Error(), "trusted run") || output.Len() != 0 {
		t.Fatalf("foreign build run produced a release: %v %s", err, output.Bytes())
	}
	if err := run(t.Context(), stageArgs, &output); err != nil {
		t.Fatal(err)
	}
	var staged registry.StagedRelease
	if err := json.Unmarshal(output.Bytes(), &staged); err != nil || staged.AlreadyTagged || staged.ReleaseCommit == "" {
		t.Fatalf("stage-release: %+v, %v", staged, err)
	}
	if refs := git("-C", publisher, "tag", "--list"); refs != "" {
		t.Fatalf("staging created a tag: %s", refs)
	}
	if head := git("-C", publisher, "rev-parse", "HEAD"); head != source {
		t.Fatalf("staging moved the default branch: %s", head)
	}
	release := staged.ReleaseCommit
	git("-C", publisher, "push", root, release+":refs/tags/"+tag)
	git("-C", publisher, "fetch", root, "refs/tags/"+tag+":refs/tags/"+tag)
	output.Reset()
	if err := run(t.Context(), append(stageArgs, "-run-id", "13", "-artifacts", t.TempDir()), &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &staged); err != nil || !staged.AlreadyTagged || staged.ReleaseCommit != release {
		t.Fatalf("rerun changed the existing release: %+v, %v", staged, err)
	}
	conflictingSource := strings.Repeat("c", 40)
	output.Reset()
	if err := run(t.Context(), append(stageArgs, "-source-sha", conflictingSource), &output); err == nil ||
		!strings.Contains(err.Error(), "different source commit") || output.Len() != 0 {
		t.Fatalf("source conflict staged a release: %v %s", err, output.Bytes())
	}
	args := []string{"inspect-release", "-checkout", root, "-repository", repository, "-tag", tag}
	if err := run(t.Context(), args, &output); err != nil {
		t.Fatal(err)
	}
	var version registry.Version
	if err := json.Unmarshal(output.Bytes(), &version); err != nil {
		t.Fatal(err)
	}
	if version.SourceCommit != source || version.ReleaseCommit != release || len(version.Artifacts) != len(record.Artifacts) ||
		version.Translations["zh"] != (registry.Translation{Name: "样板", Description: "用于验证。"}) {
		t.Fatalf("unexpected version record: %+v", version)
	}
	t.Run("plan provider registration", func(t *testing.T) {
		registryDir := t.TempDir()
		writeJSONFile := func(directory, relative string, content []byte) {
			t.Helper()
			path := filepath.Join(directory, relative)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, content, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		writeJSONFile(registryDir, "registry/_policy/withdrawals.json", []byte(`[{"provider_type":"fixture-dns","version":"1.2.4","reason":"withdrawn on purpose"}]`))
		writeJSONFile(registryDir, "registry/fixture-dns/v1.2.3.json", bytes.Clone(output.Bytes()))
		tagLines := strings.Join([]string{
			source + "\trefs/tags/v1.2.3-detach",
			source + "\trefs/tags/v1.2.4-detach",
			source + "\trefs/tags/v1.10.0-detach",
			source + "\trefs/tags/v1.9.0-detach",
			source + "\trefs/tags/v2.0.0",
			strings.Repeat("2", 40) + "\trefs/tags/v2.0.0^{}",
			release + "\trefs/tags/v1.2.3",
			source + "\trefs/tags/v3.0.0-rc.1",
		}, "\n")
		refs := filepath.Join(registryDir, "refs.txt")
		writeJSONFile(registryDir, "refs.txt", []byte(tagLines))
		planArgs := []string{"plan-provider", "-registry", filepath.Join(registryDir, "registry"), "-repository", repository, "-refs", refs}
		var plan bytes.Buffer
		if err := run(t.Context(), planArgs, &plan); err != nil {
			t.Fatal(err)
		}
		want := `{"register":[{"tag":"v2.0.0","release_commit":"` + strings.Repeat("2", 40) + `"}]}`
		if strings.TrimSpace(plan.String()) != want {
			t.Fatalf("reconciliation output: %s, want %s", plan.String(), want)
		}
		writeJSONFile(registryDir, "refs.txt", []byte(strings.Replace(tagLines, release+"\trefs/tags/v1.2.3", source+"\trefs/tags/v1.2.3", 1)))
		plan.Reset()
		if err := run(t.Context(), planArgs, &plan); err == nil || !strings.Contains(err.Error(), "immutable formal tag") || plan.Len() != 0 {
			t.Fatalf("changed formal tag did not invalidate reconciliation: %v %s", err, plan.String())
		}
	})

	write(record.Artifacts[0].Path, "modified ZIP")
	git("add", "build")
	tampered := git("commit-tree", git("write-tree"), "-p", source, "-m", "Tampered archive fixture")
	git("update-ref", "refs/tags/"+tag, tampered)
	output.Reset()
	if err := run(t.Context(), args, &output); err == nil || !strings.Contains(err.Error(), "ZIP size and digest") || output.Len() != 0 {
		t.Fatalf("bad ZIP must produce an error and no version record: %v %s", err, output.Bytes())
	}

	write("main.go", "package main\n\nfunc main() { panic(\"changed\") }\n")
	git("add", "main.go")
	extraChange := git("commit-tree", git("write-tree"), "-p", source, "-m", "Modified source fixture")
	git("update-ref", "refs/tags/"+tag, extraChange)
	if err := run(t.Context(), args, &output); err == nil || !strings.Contains(err.Error(), "only the expected build files") || output.Len() != 0 {
		t.Fatalf("an artifact commit must not modify source: %v %s", err, output.Bytes())
	}
}
