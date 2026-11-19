package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/certimate-go/plugins/internal/registry"
)

func publishMarket(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet(arguments[0], flag.ContinueOnError)
	options := registry.PublishOptions{}
	flags.StringVar(&options.Checkout, "checkout", ".", "read-only registry source checkout with market Git objects")
	flags.StringVar(&options.MarketCommit, "market-sha", "", "current market Git commit, omitted for bootstrap only")
	keysPath := flags.String("public-keys", "", "JSON map of trusted key IDs to base64 public keys")
	var epoch *uint
	var keyPath *string
	if arguments[0] == "stage-publication" {
		flags.StringVar(&options.SourceCommit, "source-sha", "", "verified merged registry commit")
		flags.StringVar(&options.KeyID, "key-id", "", "active signing key ID")
		flags.DurationVar(&options.Lifetime, "lifetime", 0, "measured validity window for published indexes")
		flags.DurationVar(&options.RenewBefore, "renew-before", 0, "renewal lead time")
		epoch = flags.Uint("epoch", 0, "active trust epoch")
		keyPath = flags.String("key-file", "", "file containing base64 Ed25519 signing seed")
	}
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *keysPath == "" {
		return fmt.Errorf("-public-keys is required")
	}
	keys, err := readPublicKeys(*keysPath)
	if err != nil {
		return err
	}
	options.PublicKeys = keys
	options.Now = time.Now()
	var publication registry.Publication
	if arguments[0] == "observe-publication" {
		publication, err = registry.ObservePublishedIndex(ctx, options.Checkout, options.MarketCommit, options.PublicKeys, options.Now)
	} else {
		if *epoch > math.MaxUint32 || *keyPath == "" {
			return fmt.Errorf("-key-file is required and epoch must fit uint32")
		}
		options.Epoch = uint32(*epoch)
		options.Seed, err = readSeed(*keyPath)
		if err != nil {
			return err
		}
		defer clear(options.Seed)
		publication, err = registry.StagePublication(ctx, options)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(publication)
}

func readPublicKeys(path string) (map[string]ed25519.PublicKey, error) {
	keyData, err := readLimitedFile(path, 8192)
	if err != nil {
		return nil, err
	}
	var encoded map[string][]byte
	if err := json.Unmarshal(keyData, &encoded); err != nil || len(encoded) == 0 {
		return nil, fmt.Errorf("invalid market index public key set")
	}
	keys := make(map[string]ed25519.PublicKey, len(encoded))
	for id, key := range encoded {
		if len(key) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid Ed25519 public key for %q", id)
		}
		keys[id] = key
	}
	return keys, nil
}

func readSeed(path string) ([]byte, error) {
	encoded, err := readLimitedFile(path, 256)
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	data := bytes.TrimSpace(encoded)
	seed := make([]byte, base64.StdEncoding.DecodedLen(len(data)))
	length, err := base64.StdEncoding.Strict().Decode(seed, data)
	if err != nil || length != ed25519.SeedSize {
		clear(seed)
		return nil, fmt.Errorf("key file does not contain a valid base64 Ed25519 seed")
	}
	return seed[:length], nil
}

func readLimitedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		clear(body)
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return body, nil
}
