package registry

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/certimate-go/plugins/internal/providerrelease"
)

func TestCentralReleaseProvenance(t *testing.T) {
	initRepository := func(kind string) (releaseGit, string) {
		t.Helper()
		directory := t.TempDir()
		command := func(args ...string) string {
			t.Helper()
			cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", directory, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "commit.gpgSign=false"}, args...)...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, output)
			}
			return strings.TrimSpace(string(output))
		}
		command("init", "-q", "-b", "main")
		command("commit", "-qm", kind, "--allow-empty")
		revision := command("rev-parse", "HEAD")
		command("update-ref", "refs/remotes/origin/main", revision)
		return releaseGit{t.Context(), directory}, revision
	}
	registryGit, registryRevision := initRepository("registry")
	providerGit, source := initRepository("provider")
	version := newPublicationVersion("example", "1.2.3", "a")
	version.SourceCommit = source
	version.BuildRepository = providerrelease.RegistryRepository
	runProof := map[string]any{
		"id": 1, "display_title": "Release " + version.Repository + " v1.2.3-detach",
		"event": "workflow_dispatch", "status": "completed", "conclusion": "success", "head_branch": "main",
		"head_sha": registryRevision, "path": ".github/workflows/provider-release.yml",
		"repository":      map[string]string{"full_name": providerrelease.RegistryRepository},
		"head_repository": map[string]string{"full_name": providerrelease.RegistryRepository},
	}
	jobs := map[string]any{"jobs": []any{map[string]any{
		"name": "publish", "run_id": 1, "head_sha": registryRevision, "status": "completed",
		"steps": []any{map[string]string{"name": publishTagStep, "status": "completed", "conclusion": "success"}},
	}}}
	proof := fstest.MapFS{}
	write := func(name string, value any) {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		proof[filepath.ToSlash(filepath.Join(version.ProviderType, name))] = &fstest.MapFile{Data: body}
	}
	write("main.json", map[string]any{"name": "main", "protected": true, "commit": map[string]string{"sha": source}})
	write("v1.2.3.json", runProof)
	write("v1.2.3-jobs.json", jobs)

	if err := verifyBuildProvenance(registryGit, providerGit, registryRevision, proof, version); err != nil {
		t.Fatalf("valid central release: %v", err)
	}
	for name, changes := range map[string]map[string]any{
		"different provider": {"display_title": "Release certimate-go/plugin-provider-other v1.2.3-detach"},
		"other workflow":     {"path": ".github/workflows/other.yml"},
		"untrusted revision": {"head_sha": source},
		"unsupported event":  {"event": "pull_request"},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := make(map[string]any, len(runProof))
			for field, value := range runProof {
				candidate[field] = value
			}
			for field, value := range changes {
				candidate[field] = value
			}
			write("v1.2.3.json", candidate)
			if err := verifyBuildProvenance(registryGit, providerGit, registryRevision, proof, version); err == nil {
				t.Fatal("unexpected central release evidence accepted")
			}
		})
	}
}
