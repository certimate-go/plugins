package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/certimate-go/plugins/internal/registry"
)

func auditMirror(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet(arguments[0], flag.ContinueOnError)
	options := registry.MirrorAuditOptions{}
	flags.StringVar(&options.Checkout, "checkout", ".", "trusted registry checkout with source and market Git commits")
	flags.StringVar(&options.MarketCommit, "market-sha", "", "expected immutable market commit")
	flags.StringVar(&options.BaseURL, "base-url", "", "HTTPS CDN base URL with optional path prefix")
	flags.StringVar(&options.Network, "network", "", "label identifying the deployment network")
	keys := flags.String("public-keys", "", "JSON map of trusted key IDs to base64 public keys")
	flags.Func("provider", "provider whose entire platform matrix is checked, repeatable", func(value string) error {
		options.Providers = append(options.Providers, value)
		return nil
	})
	flags.Func("withdrawn", "provider@vX.Y.Z that must be withdrawn in the expected index, repeatable", func(value string) error {
		provider, version, ok := strings.Cut(value, "@v")
		if !ok || provider == "" || version == "" {
			return fmt.Errorf("expected provider@vX.Y.Z")
		}
		options.Withdrawn = append(options.Withdrawn, registry.Identity{ProviderType: provider, Version: version})
		return nil
	})
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *keys == "" {
		return fmt.Errorf("-public-keys is required")
	}
	var err error
	options.PublicKeys, err = readPublicKeys(*keys)
	if err != nil {
		return err
	}
	options.Now = time.Now()
	report, err := registry.AuditMirror(ctx, options)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(output).Encode(report); err != nil {
		return err
	}
	if !report.Ready {
		return fmt.Errorf("mirror has not served all expected files for generation %d", report.Generation)
	}
	return nil
}
