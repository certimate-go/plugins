package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

type detailReference struct {
	SHA256    string   `json:"sha256"`
	Repo      string   `json:"repo"`
	Platforms []string `json:"platforms"`
}

func renderIndex(root string, plugins []catalogManifest) ([]byte, error) {
	index := struct {
		Plugins []map[string]any `json:"plugins"`
	}{Plugins: make([]map[string]any, 0, len(plugins))}
	dir := filepath.Join(root, "market", "details")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(plugins))
	for _, entry := range plugins {
		manifest := entry.fields
		provider := manifest["provider_type"].(string)
		if seen[provider] {
			return nil, fmt.Errorf("duplicate provider_type %q", provider)
		}
		seen[provider] = true
		summary := make(map[string]any)
		for _, key := range []string{
			"provider_type", "version", "display_name_key", "deploy_category", "capability", "description",
			"protocol_version", "min_core_version", "max_core_version",
		} {
			if value, ok := manifest[key]; ok && value != "" {
				summary[key] = value
			}
		}

		if icon, ok := manifest["icon"].(string); ok && icon != "" {
			if !fs.ValidPath(icon) || icon == "." {
				return nil, fmt.Errorf("invalid icon path for %q", provider)
			}
			summary["icon_path"] = path.Join(entry.directory, icon)
		}

		release, _ := manifest["release"].(map[string]any)
		version, _ := manifest["version"].(string)
		tag, _ := release["tag"].(string)
		if version != "" && path.Base(tag) == "v"+version {
			repo, _ := release["repo"].(string)
			assets, _ := release["assets"].(map[string]any)
			platforms := make([]string, 0, len(assets))
			for platform, asset := range assets {
				if name, ok := asset.(string); ok && name != "" {
					platforms = append(platforms, platform)
				}
			}
			if len(platforms) > 0 {
				if repo == "" {
					return nil, fmt.Errorf("missing release repository for %q", provider)
				}
				sort.Strings(platforms)
				body, err := json.Marshal(manifest)
				if err != nil {
					return nil, err
				}
				body = append(body, '\n')
				sum := sha256.Sum256(body)
				digest := hex.EncodeToString(sum[:])
				if err := os.WriteFile(filepath.Join(dir, digest+".json"), body, 0o644); err != nil {
					return nil, err
				}
				summary["detail"] = detailReference{SHA256: digest, Repo: repo, Platforms: platforms}
			}
		}
		index.Plugins = append(index.Plugins, summary)
	}

	body, err := json.Marshal(index)
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
