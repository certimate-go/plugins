package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var providerTypePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

type catalogManifest struct {
	directory string
	fields    map[string]any
}

func buildIndex(rootDir string) ([]catalogManifest, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("read repo root %q: %w", rootDir, err)
	}
	plugins := make([]catalogManifest, 0)
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "_template" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		manifestPath := filepath.Join(rootDir, e.Name(), "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read manifest %q: %w", manifestPath, err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse manifest %q: %w", manifestPath, err)
		}
		pt, _ := m["provider_type"].(string)
		if !providerTypePattern.MatchString(pt) {
			continue
		}
		plugins = append(plugins, catalogManifest{directory: e.Name(), fields: m})
	}
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].fields["provider_type"].(string) < plugins[j].fields["provider_type"].(string)
	})
	return plugins, nil
}

func main() {
	rootDir := flag.String("root", ".", "plugins repo root directory")
	out := flag.String("o", "", "output file (defaults to stdout)")
	flag.Parse()

	plugins, err := buildIndex(*rootDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	destination := *rootDir
	if *out != "" {
		destination = filepath.Dir(*out)
	}
	buf, err := renderIndex(destination, plugins)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *out == "" {
		_, _ = os.Stdout.Write(buf)
		return
	}
	if err := os.WriteFile(*out, buf, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
