package registry

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBatchAdmissionsRequireMatchingTrustedEvidence(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "commit.gpgSign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name string, value any) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", name)
		return data
	}
	git("init", "-q", "-b", "main")
	write("alpha/manifest.json", map[string]any{"provider_type": "alpha", "version": "1.0.0"})
	write("beta/manifest.json", map[string]any{"provider_type": "beta", "version": "1.0.0"})
	write("registry/_policy/admissions.json", map[string]string{})
	write("registry/_policy/withdrawals.json", []Withdrawal{})
	git("commit", "-qm", "Legacy plugins")
	base := git("rev-parse", "HEAD")
	type entry struct {
		Provider     string `json:"provider_type"`
		Repository   string `json:"repository"`
		SourceCommit string `json:"source_commit"`
		SourcePath   string `json:"source_path"`
		TargetCommit string `json:"target_commit"`
		TargetTree   string `json:"target_tree"`
		Version      string `json:"version"`
	}
	entries := []entry{
		{Provider: "alpha", Repository: "certimate-go/plugin-provider-alpha-notifier", SourceCommit: base, SourcePath: "alpha", TargetCommit: strings.Repeat("a", 40), TargetTree: strings.Repeat("b", 40), Version: "1.0.0"},
		{Provider: "beta", Repository: "certimate-go/plugin-provider-beta-notifier", SourceCommit: base, SourcePath: "beta", TargetCommit: strings.Repeat("c", 40), TargetTree: strings.Repeat("d", 40), Version: "1.0.0"},
	}
	policy := map[string]string{"alpha": entries[0].Repository, "beta": entries[1].Repository}
	write("registry/_policy/admissions.json", policy)
	receipt := write("registry/_policy/admission-batches/123-1.json", entries)
	git("commit", "-qm", "Admit two plugins")
	options := RegistrationOptions{Checkout: dir, Base: base, Candidate: git("rev-parse", "HEAD")}
	evidence := fstest.MapFS{"admission-batches/123-1.json": {Data: receipt}}
	for _, queue := range []bool{false, true} {
		options.Queue = queue
		if _, err := VerifyRegistrations(t.Context(), options, dir, evidence); err != nil {
			t.Fatalf("valid batch, queue=%t: %v", queue, err)
		}
		if _, err := VerifyRegistrations(t.Context(), options, dir, fstest.MapFS{}); err == nil {
			t.Fatalf("missing trusted batch evidence accepted, queue=%t", queue)
		}
		forged := fstest.MapFS{"admission-batches/123-1.json": {Data: []byte("[]")}}
		if _, err := VerifyRegistrations(t.Context(), options, dir, forged); err == nil {
			t.Fatalf("mismatched batch evidence accepted, queue=%t", queue)
		}
	}
	if _, err := Generate(os.DirFS(filepath.Join(dir, "registry")), nil); err != nil {
		t.Fatalf("catalog generation after admission: %v", err)
	}
	admitted := options.Candidate
	policy["gamma"] = "certimate-go/plugin-provider-gamma-notifier"
	write("registry/_policy/admissions.json", policy)
	git("commit", "-qm", "Add unauthorized mapping")
	options.Candidate = git("rev-parse", "HEAD")
	if _, err := VerifyRegistrations(t.Context(), options, dir, evidence); err == nil {
		t.Fatal("unauthorized admission accepted")
	}

	git("switch", "--detach", admitted)
	version := newPublicationVersion("alpha", "1.0.0", "2")
	version.Repository = entries[0].Repository
	write("registry/alpha/v1.0.0.json", version)
	git("commit", "-qm", "Register a different migration source")
	options.Base = admitted
	options.Candidate = git("rev-parse", "HEAD")
	if _, err := RegistrationChanges(t.Context(), options); err == nil || !strings.Contains(err.Error(), "migration source commit") {
		t.Fatalf("unexpected result for a registration from a different source: %v", err)
	}
}
