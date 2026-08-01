package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type target struct{ goos, goarch string }

var targets = []target{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
}

var (
	providerTypePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)
	versionPattern      = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$`)
)

func assetName(plugin, goos, goarch string) string {
	return fmt.Sprintf("%s_%s_%s.zip", plugin, goos, goarch)
}

func assetKey(goos, goarch string) string { return goos + "/" + goarch }

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func buildRelease(plugin, repo, version, distDir string) (map[string]any, error) {
	assets := make(map[string]string, len(targets))
	checksums := make(map[string]string, len(targets))
	for _, t := range targets {
		name := assetName(plugin, t.goos, t.goarch)
		sum, err := sha256File(filepath.Join(distDir, name))
		if err != nil {
			return nil, fmt.Errorf("checksum %s: %w", name, err)
		}
		assets[assetKey(t.goos, t.goarch)] = name
		checksums[assetKey(t.goos, t.goarch)] = sum
	}
	return map[string]any{
		"repo":      repo,
		"tag":       fmt.Sprintf("%s/v%s", plugin, version),
		"assets":    assets,
		"checksums": checksums,
	}, nil
}

func applyRelease(manifestPath, plugin, repo, distDir string) ([]byte, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %q: %w", manifestPath, err)
	}
	pt, _ := m["provider_type"].(string)
	if !providerTypePattern.MatchString(pt) {
		return nil, fmt.Errorf("manifest %q has invalid provider_type %q", manifestPath, pt)
	}
	if pt != plugin {
		return nil, fmt.Errorf("manifest provider_type %q does not match --plugin %q", pt, plugin)
	}
	version, _ := m["version"].(string)
	if !versionPattern.MatchString(version) {
		return nil, fmt.Errorf("manifest %q has invalid version %q (want semver major.minor.patch)", manifestPath, version)
	}
	release, err := buildRelease(plugin, repo, version, distDir)
	if err != nil {
		return nil, err
	}
	m["release"] = release
	buf, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(buf, '\n'), nil
}

func main() {
	plugin := flag.String("plugin", "", "plugin provider_type (also its directory name)")
	repo := flag.String("repo", "", "release repo, e.g. usual2970/certimate-plugins")
	distDir := flag.String("dist", "dist", "directory holding the built release binaries")
	root := flag.String("root", ".", "plugins repo root directory")
	flag.Parse()

	if *plugin == "" || *repo == "" {
		fmt.Fprintln(os.Stderr, "usage: releaseinfo -plugin <provider_type> -repo <repo> [-dist dist] [-root .]")
		os.Exit(2)
	}
	manifestPath := filepath.Join(*root, *plugin, "manifest.json")
	out, err := applyRelease(manifestPath, *plugin, *repo, *distDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(manifestPath, out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
