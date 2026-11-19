package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/certimate-go/plugins/internal/registry"
)

func main() {
	records := flag.String("registry", "registry", "directory with registered version records and policy")
	ready := flag.String("ready", "", "trusted CDN observations JSON file")
	destination := flag.String("out-dir", "", "staging directory for catalog.json and its addressed assets")
	flag.Parse()
	if err := generateRegisteredCatalog(*records, *ready, *destination); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generateRegisteredCatalog(records, ready, destination string) error {
	if ready == "" || destination == "" {
		return fmt.Errorf("-ready and -out-dir are required")
	}
	observations, err := registry.ReadObservations(os.DirFS(filepath.Dir(ready)), filepath.Base(ready))
	if err != nil {
		return err
	}
	source, err := os.OpenRoot(records)
	if err != nil {
		return err
	}
	defer source.Close()
	snapshot, err := registry.Generate(source.FS(), observations)
	if err != nil {
		return err
	}
	catalog, err := json.Marshal(snapshot.Catalog)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(snapshot.Files))
	for name := range snapshot.Files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		target := filepath.Join(destination, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, snapshot.Files[name], 0o644); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destination, "catalog.json"), append(catalog, '\n'), 0o644)
}
