package registry

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/certimate-go/plugins/internal/providerrelease"
)

const marketIndexPath = "registry/index.json"

type PublishOptions struct {
	Checkout     string
	SourceCommit string
	MarketCommit string
	Epoch        uint32
	KeyID        string
	PublicKeys   map[string]ed25519.PublicKey
	Seed         []byte
	Now          time.Time
	Lifetime     time.Duration
	RenewBefore  time.Duration
}

type Publication struct {
	Commit       string    `json:"commit"`
	SourceCommit string    `json:"source_commit"`
	KeyID        string    `json:"key_id"`
	Epoch        uint32    `json:"epoch"`
	Generation   uint64    `json:"generation"`
	ExpiresAt    time.Time `json:"expires_at"`
	Changed      bool      `json:"changed"`
	Pending      []string  `json:"pending"`
}

type releaseObserver func(context.Context, Version) (ReadyRelease, error)

func StagePublication(ctx context.Context, options PublishOptions) (Publication, error) {
	return stagePublication(ctx, options, ObserveDefaultCDN)
}

func stagePublication(ctx context.Context, options PublishOptions, observe releaseObserver) (Publication, error) {
	if !commitPattern.MatchString(options.SourceCommit) ||
		(options.MarketCommit != "" && !commitPattern.MatchString(options.MarketCommit)) {
		return Publication{}, errors.New("publication requires verified full Git commit SHAs")
	}
	if options.Epoch == 0 || len(options.Seed) != ed25519.SeedSize ||
		!keyIDPattern.MatchString(options.KeyID) || options.Lifetime <= 0 ||
		options.Lifetime > MaxIndexLifetime || options.RenewBefore <= 0 || options.RenewBefore >= options.Lifetime {
		return Publication{}, errors.New("invalid index key, epoch, lifetime, or renewal lead time")
	}
	privateKey := ed25519.NewKeyFromSeed(options.Seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	clear(privateKey)
	if !bytes.Equal(publicKey, options.PublicKeys[options.KeyID]) {
		return Publication{}, errors.New("index signing seed does not match the configured public key")
	}
	git := releaseGit{ctx, options.Checkout}
	head, err := git.output("rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return Publication{}, err
	}
	dirty, err := git.output("status", "--porcelain", "--untracked-files=all", "--", "registry")
	if err != nil {
		return Publication{}, err
	}
	if strings.TrimSpace(string(head)) != options.SourceCommit || len(dirty) != 0 {
		return Publication{}, errors.New("publication requires a clean registry at the exact protected source commit")
	}
	root, err := os.OpenRoot(filepath.Join(options.Checkout, "registry"))
	if err != nil {
		return Publication{}, err
	}
	defer root.Close()

	var previous IndexPayload
	var previousKey string
	if options.MarketCommit != "" {
		body, err := git.blob(options.MarketCommit, marketIndexPath, MaxIndexEnvelope)
		if err != nil {
			return Publication{}, err
		}
		previous, previousKey, err = VerifyIndex(body, options.PublicKeys)
		if err != nil {
			return Publication{}, fmt.Errorf("verify current market branch index: %w", err)
		}
		if previous.Epoch > options.Epoch || !git.ancestor(previous.SourceCommit, options.SourceCommit) ||
			previous.IssuedAt.After(options.Now.Add(5*time.Minute)) {
			return Publication{}, errors.New("refusing to publish from an older source, older epoch, or invalid previous issuance time")
		}
	}
	options.Now = options.Now.UTC().Truncate(time.Second)
	renewalDue := previous.Epoch != options.Epoch || previousKey != options.KeyID || !options.Now.Add(options.RenewBefore).Before(previous.ExpiresAt)
	ready, pending, err := observeCatalog(ctx, root.FS(), previous.Catalog, renewalDue, observe)
	if err != nil {
		return Publication{}, err
	}
	snapshot, err := Generate(root.FS(), ready)
	if err != nil {
		return Publication{}, err
	}
	if previous.Epoch != 0 && !renewalDue && sameCatalog(previous.Catalog, snapshot.Catalog) {
		return Publication{
			Commit: options.MarketCommit, SourceCommit: previous.SourceCommit, KeyID: previousKey,
			Epoch: previous.Epoch, Generation: previous.Generation, ExpiresAt: previous.ExpiresAt, Pending: pending,
		}, nil
	}
	generation := uint64(1)
	if previous.Epoch == options.Epoch {
		if previous.Generation == math.MaxUint64 {
			return Publication{}, errors.New("index generation is exhausted")
		}
		generation = previous.Generation + 1
	}
	metadata := IndexMetadata{
		Epoch: options.Epoch, Generation: generation, SourceCommit: options.SourceCommit,
		IssuedAt: options.Now, ExpiresAt: options.Now.Add(options.Lifetime),
	}
	envelope, err := SignIndex(snapshot, metadata, options.KeyID, options.Seed)
	if err != nil {
		return Publication{}, err
	}
	commit, err := git.marketCommit(options.MarketCommit, snapshot, envelope, metadata)
	if err != nil {
		return Publication{}, err
	}
	return Publication{
		Commit: commit, SourceCommit: options.SourceCommit, KeyID: envelope.KeyID,
		Epoch: options.Epoch, Generation: generation, ExpiresAt: metadata.ExpiresAt, Changed: true, Pending: pending,
	}, nil
}

func sameCatalog(before, after Catalog) bool {
	oldBytes, _ := json.Marshal(before)
	newBytes, _ := json.Marshal(after)
	return bytes.Equal(oldBytes, newBytes)
}

type publicationProvider struct {
	versions []*Version
	previous *Version
}

type observationResult struct {
	release *ReadyRelease
	pending []string
}

func observeCatalog(ctx context.Context, records fs.FS, previous Catalog, renew bool, observe releaseObserver) ([]ReadyRelease, []string, error) {
	admissions, withdrawals, err := loadPolicy(records)
	if err != nil {
		return nil, nil, err
	}
	previousByProvider := make(map[string]Summary, len(previous.Plugins))
	for _, summary := range previous.Plugins {
		previousByProvider[summary.ProviderType] = summary
	}
	names := make([]string, 0, len(admissions))
	for name := range admissions {
		names = append(names, name)
	}
	slices.Sort(names)
	providers := make([]publicationProvider, 0, len(names))
	for _, name := range names {
		versions, err := loadProvider(records, name, admissions[name])
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
		var eligible publicationProvider
		var legacyFound bool
		legacy, hadLegacy := previousByProvider[name]
		for _, version := range versions {
			id := Identity{version.ProviderType, version.Version}
			if hadLegacy && legacy.Identity == id {
				reconstructed := Snapshot{Files: make(map[string][]byte)}
				if err := reconstructed.add(version); err != nil {
					return nil, nil, err
				}
				if !sameCatalog(Catalog{Plugins: []Summary{legacy}}, reconstructed.Catalog) {
					return nil, nil, fmt.Errorf("registered version %s %s changed after publication", version.ProviderType, version.Version)
				}
				legacyFound = true
				if !withdrawals[id] {
					eligible.previous = version
				}
			}
			if !withdrawals[id] {
				eligible.versions = append(eligible.versions, version)
			}
		}
		if hadLegacy && !legacyFound {
			return nil, nil, fmt.Errorf("published provider %q is missing its immutable registration", name)
		}
		providers = append(providers, eligible)
	}
	results := make([]observationResult, len(providers))
	const workers = 6
	pool := make(chan int)
	var group sync.WaitGroup
	for range min(workers, len(providers)) {
		group.Go(func() {
			for index := range pool {
				results[index] = observeProvider(ctx, providers[index], renew, observe)
			}
		})
	}
	for index := range providers {
		pool <- index
	}
	close(pool)
	group.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	ready := make([]ReadyRelease, 0, len(results))
	pending := make([]string, 0)
	for _, result := range results {
		if result.release != nil {
			ready = append(ready, *result.release)
		}
		pending = append(pending, result.pending...)
	}
	return ready, pending, nil
}

func observeProvider(ctx context.Context, provider publicationProvider, renew bool, observe releaseObserver) observationResult {
	ctx, stop := context.WithTimeout(ctx, 2*time.Minute)
	defer stop()
	result := observationResult{}
	candidates := []*Version{}
	var baseline *Version
	for _, version := range provider.versions {
		if previous := provider.previous; previous != nil {
			if compareVersions(version.Version, previous.Version) <= 0 || keepsCompatibility(previous.Manifest, version.Manifest) {
				candidates = append(candidates, version)
			}
			continue
		}
		if baseline == nil || keepsCompatibility(baseline.Manifest, version.Manifest) {
			candidates = append(candidates, version)
			baseline = version
		}
	}
	for index := len(candidates) - 1; index >= 0; index-- {
		version := candidates[index]
		if version == provider.previous && !renew {
			result.release = &ReadyRelease{Identity{version.ProviderType, version.Version}, version.ReleaseCommit}
			break
		}
		if ctx.Err() != nil {
			if provider.previous != nil && !renew {
				previous := provider.previous
				result.release = &ReadyRelease{Identity{previous.ProviderType, previous.Version}, previous.ReleaseCommit}
			}
			break
		}
		timeout, cancel := context.WithTimeout(ctx, 70*time.Second)
		receipt, err := observe(timeout, *version)
		cancel()
		if err == nil {
			result.release = &receipt
			break
		}
		result.pending = append(result.pending, fmt.Sprintf("%s %s: %v", version.ProviderType, version.Tag, err))
	}
	return result
}

func (git releaseGit) marketCommit(parent string, snapshot Snapshot, envelope IndexEnvelope, metadata IndexMetadata) (string, error) {
	temporary, err := os.MkdirTemp("", "registry-index-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	index := filepath.Join(temporary, "index")
	root := parent
	if root == "" {
		root = "--empty"
	}
	if _, err := git.withIndex(index, nil, "read-tree", root); err != nil {
		return "", err
	}
	names := make([]string, 0, len(snapshot.Files))
	for name := range snapshot.Files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := git.addBlob(index, path.Join("registry", name), snapshot.Files[name]); err != nil {
			return "", err
		}
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	if err := git.addBlob(index, marketIndexPath, append(body, '\n')); err != nil {
		return "", err
	}
	tree, err := git.withIndex(index, nil, "write-tree")
	if err != nil {
		return "", err
	}
	args := []string{
		"-c", "user.name=Certimate Registry Publisher", "-c", "user.email=release@certimate.me", "-c", "commit.gpgSign=false",
		"commit-tree", strings.TrimSpace(string(tree)),
	}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	args = append(args, "-m", fmt.Sprintf("Publish market index %d/%d from %s", metadata.Epoch, metadata.Generation, metadata.SourceCommit))
	commit, err := git.output(args...)
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(commit))
	if _, err := providerrelease.TrackedSnapshotSize(git.ctx, git.checkout, sha); err != nil {
		return "", fmt.Errorf("market snapshot exceeds CDN package constraints: %w", err)
	}
	return sha, nil
}
