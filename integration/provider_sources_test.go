package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func stageStandaloneProviders(t *testing.T, directory string, providers []string, targetOS, arch string) {
	t.Helper()
	workspace := standaloneTestWorkspace()

	for _, provider := range providers {
		source, manifest, raw := standaloneTestProvider(t, workspace, provider)
		destination := filepath.Join(directory, provider)
		if err := os.MkdirAll(destination, 0o755); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		command := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-o", filepath.Join(destination, manifest.Binary), ".")
		command.Dir = source
		command.Env = append(os.Environ(), "GOWORK=off", "GOOS="+targetOS, "GOARCH="+arch, "CGO_ENABLED=0")
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("build standalone %s: %v\n%s", provider, err, output)
		}
		if err := os.WriteFile(filepath.Join(destination, "manifest.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		if manifest.Icon != "" {
			icon, err := os.ReadFile(filepath.Join(source, manifest.Icon))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(destination, manifest.Icon), icon, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func standaloneTestProvider(t *testing.T, workspace, provider string) (string, *plugin.Manifest, []byte) {
	t.Helper()
	exact := filepath.Join(workspace, "plugin-provider-"+provider)
	paths, err := filepath.Glob(exact + "-*")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range append([]string{exact}, paths...) {
		raw, err := os.ReadFile(filepath.Join(source, "manifest.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := plugin.ParseManifest(raw)
		if err != nil {
			t.Fatal(err)
		}
		if manifest.ProviderType == provider {
			return source, manifest, raw
		}
	}
	t.Fatalf("missing standalone %s source in %s; initialize the workspace submodules or set CERTIMATE_PLUGIN_WORKSPACE", provider, workspace)
	return "", nil, nil
}

func standaloneTestWorkspace() string {
	if workspace := os.Getenv("CERTIMATE_PLUGIN_WORKSPACE"); workspace != "" {
		return workspace
	}
	return filepath.Join("..", "..")
}
