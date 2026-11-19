package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/certimate-go/plugins/internal/registry"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, output io.Writer) error {
	if len(arguments) != 0 {
		switch arguments[0] {
		case "inspect-release":
			return inspectRelease(ctx, arguments, output)
		case "stage-release":
			return stageRelease(ctx, arguments, output)
		case "observe-release":
			return observeRelease(ctx, arguments, output)
		case "audit-mirror":
			return auditMirror(ctx, arguments, output)
		case "sign-index":
			return signIndex(arguments, output)
		case "stage-publication", "observe-publication":
			return publishMarket(ctx, arguments, output)
		case "plan-provider":
			return planProvider(arguments, output)
		}
	}
	return fmt.Errorf("usage: registryctl <inspect-release|stage-release|observe-release|audit-mirror|sign-index|stage-publication|observe-publication|plan-provider> [flags]")
}

func planProvider(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet(arguments[0], flag.ContinueOnError)
	directory := flags.String("registry", "registry", "protected registry checkout")
	repository := flags.String("repository", "", "official provider repository")
	refsPath := flags.String("refs", "", "complete git ls-remote --tags output for the provider")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *repository == "" || *refsPath == "" {
		return fmt.Errorf("-repository and -refs are required")
	}
	root, err := os.OpenRoot(*directory)
	if err != nil {
		return err
	}
	defer root.Close()
	refs, err := os.Open(*refsPath)
	if err != nil {
		return err
	}
	defer refs.Close()
	plan, err := registry.PlanProviderReconciliation(root.FS(), *repository, refs)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(plan)
}

func stageRelease(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet(arguments[0], flag.ContinueOnError)
	options := registry.StageReleaseOptions{}
	flags.StringVar(&options.Checkout, "checkout", ".", "Git repository with source and existing formal tag, if any")
	flags.StringVar(&options.ArtifactsDir, "artifacts", "", "downloaded build output directory")
	flags.StringVar(&options.Repository, "repository", "", "verified provider repository")
	flags.StringVar(&options.Tag, "tag", "", "formal release tag")
	flags.StringVar(&options.SourceCommit, "source-sha", "", "source commit already checked for protected-branch ancestry")
	flags.StringVar(&options.BuildRunID, "run-id", "", "current trusted build workflow run")
	flags.StringVar(&options.BuildRepository, "build-repository", "", "protected registry repository for centrally built releases")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	staged, err := registry.StageRelease(ctx, options)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(staged)
}

func inspectRelease(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet(arguments[0], flag.ContinueOnError)
	checkout := flags.String("checkout", ".", "checkout with the formal tag and its source history")
	repository := flags.String("repository", "", "official provider repository")
	tag := flags.String("tag", "", "immutable formal release tag")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	version, err := registry.InspectRelease(ctx, *checkout, *repository, *tag)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(version)
}

func observeRelease(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet(arguments[0], flag.ContinueOnError)
	filename := flags.String("record", "", "registered version metadata")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *filename == "" {
		return fmt.Errorf("-record is required")
	}
	version, err := registry.ReadVersion(os.DirFS(filepath.Dir(*filename)), filepath.Base(*filename))
	if err != nil {
		return err
	}
	observation, err := registry.ObserveDefaultCDN(ctx, version)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(observation)
}

func signIndex(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet(arguments[0], flag.ContinueOnError)
	directory := flags.String("registry", "registry", "protected registry checkout")
	ready := flags.String("ready", "", "trusted CDN observations JSON file")
	keyFile := flags.String("key-file", "", "file containing a base64 Ed25519 seed")
	keyID := flags.String("key-id", "", "trusted key ID")
	epoch := flags.Uint64("epoch", 0, "Core trust epoch")
	generation := flags.Uint64("generation", 0, "generation allocated by the publishing job")
	source := flags.String("source-commit", "", "immutable parent registry commit")
	issued := flags.String("issued-at", "", "index issuance time in RFC3339")
	expires := flags.String("expires-at", "", "index expiration time in RFC3339")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *ready == "" || *keyFile == "" || *epoch > uint64(^uint32(0)) {
		return fmt.Errorf("invalid signing parameters: -ready and -key-file are required, and epoch must fit uint32")
	}
	issuedAt, err := time.Parse(time.RFC3339, *issued)
	if err != nil {
		return fmt.Errorf("parse issued-at: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, *expires)
	if err != nil {
		return fmt.Errorf("parse expires-at: %w", err)
	}
	observations, err := registry.ReadObservations(os.DirFS(filepath.Dir(*ready)), filepath.Base(*ready))
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(*directory)
	if err != nil {
		return err
	}
	defer root.Close()
	snapshot, err := registry.Generate(root.FS(), observations)
	if err != nil {
		return err
	}
	seed, err := readSeed(*keyFile)
	if err != nil {
		return err
	}
	defer clear(seed)
	envelope, err := registry.SignIndex(snapshot, registry.IndexMetadata{
		Epoch: uint32(*epoch), Generation: *generation, IssuedAt: issuedAt, ExpiresAt: expiresAt, SourceCommit: *source,
	}, *keyID, seed)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(envelope)
}
