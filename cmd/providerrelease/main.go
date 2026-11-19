package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/certimate-go/plugins/internal/providerrelease"
)

func main() {
	options := providerrelease.Options{}
	flag.StringVar(&options.SourceDir, "source-dir", ".", "clean provider checkout")
	flag.StringVar(&options.Repository, "repository", "", "provider GitHub owner/repository")
	flag.StringVar(&options.Trigger, "trigger-tag", "", "provider vX.Y.Z-detach tag")
	flag.StringVar(&options.SourceSHA, "source-sha", "", "verified source commit of the trigger tag")
	flag.StringVar(&options.BuildRunID, "run-id", "", "trusted build workflow run ID")
	flag.StringVar(&options.BuildRepository, "build-repository", "", "protected registry repository for centrally built releases")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	record, err := providerrelease.Build(ctx, options)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Built %s %s (%d platform ZIPs)\n", record.ProviderType, record.Tag, len(record.Artifacts))
}
