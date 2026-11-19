package registry

import (
	"cmp"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

type MirrorAuditOptions struct {
	Checkout     string
	MarketCommit string
	BaseURL      string
	Network      string
	Providers    []string
	Withdrawn    []Identity
	PublicKeys   map[string]ed25519.PublicKey
	Now          time.Time
}

type MirrorAudit struct {
	BaseURL      string                `json:"base_url"`
	Network      string                `json:"network"`
	ObservedAt   time.Time             `json:"observed_at"`
	MarketCommit string                `json:"market_commit"`
	SourceCommit string                `json:"source_commit"`
	KeyID        string                `json:"key_id"`
	Epoch        uint32                `json:"epoch"`
	Generation   uint64                `json:"generation"`
	IssuedAt     time.Time             `json:"issued_at"`
	ExpiresAt    time.Time             `json:"expires_at"`
	Providers    []string              `json:"providers"`
	Withdrawals  []WithdrawalAudit     `json:"withdrawals,omitempty"`
	Probes       []DownloadObservation `json:"probes"`
	Ready        bool                  `json:"ready"`
}

type WithdrawalAudit struct {
	Identity
	ReplacementVersion string `json:"replacement_version,omitempty"`
	Confirmed          bool   `json:"confirmed"`
}

func AuditMirror(ctx context.Context, options MirrorAuditOptions) (MirrorAudit, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := cdnHTTPClient()
	client.Transport = transport
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 || request.URL.Scheme != "https" || request.URL.User != nil ||
			!strings.EqualFold(request.URL.Host, via[0].URL.Host) {
			return http.ErrUseLastResponse
		}
		return nil
	}
	return auditMirror(ctx, options, client)
}

type mirrorFile struct {
	path string
	size int64
	sha  string
}

func checksumFile(path string, bytes []byte) mirrorFile {
	return mirrorFile{path, int64(len(bytes)), fmt.Sprintf("%x", sha256.Sum256(bytes))}
}

func auditMirror(ctx context.Context, options MirrorAuditOptions, client *http.Client) (MirrorAudit, error) {
	base, err := mirrorAuditBase(options.BaseURL)
	if err != nil {
		return MirrorAudit{}, err
	}
	network := strings.TrimSpace(options.Network)
	if network == "" || (len(options.Providers) == 0 && len(options.Withdrawn) == 0) {
		return MirrorAudit{}, fmt.Errorf("network label and at least one provider or withdrawn version are required")
	}
	git := releaseGit{ctx, options.Checkout}
	index, payload, keyID, err := readPublishedIndex(git, options.MarketCommit, options.PublicKeys, options.Now)
	if err != nil {
		return MirrorAudit{}, err
	}
	listed := make(map[string]Summary, len(payload.Plugins))
	for _, summary := range payload.Plugins {
		listed[summary.ProviderType] = summary
	}
	withdrawals, err := auditWithdrawals(git, payload.SourceCommit, listed, options.Withdrawn)
	if err != nil {
		return MirrorAudit{}, err
	}
	report := MirrorAudit{
		BaseURL: base, Network: network, ObservedAt: options.Now.UTC(), MarketCommit: options.MarketCommit,
		SourceCommit: payload.SourceCommit, KeyID: keyID, Epoch: payload.Epoch, Generation: payload.Generation,
		IssuedAt: payload.IssuedAt, ExpiresAt: payload.ExpiresAt,
		Providers: append([]string{}, options.Providers...), Withdrawals: withdrawals,
	}
	for _, withdrawal := range withdrawals {
		if withdrawal.ReplacementVersion != "" {
			report.Providers = append(report.Providers, withdrawal.ProviderType)
		}
	}
	slices.Sort(report.Providers)
	report.Providers = slices.Compact(report.Providers)
	targets, err := auditFiles(git, payload.SourceCommit, listed, report.Providers)
	if err != nil {
		return MirrorAudit{}, err
	}
	first := checksumFile(marketCDNPath+marketIndexPath, index)
	probe, err := measureDownload(ctx, client, base+first.path, first.size, first.sha)
	report.Probes = append(report.Probes, probe)
	if err != nil {
		return report, nil
	}
	for index := range report.Withdrawals {
		report.Withdrawals[index].Confirmed = true
	}
	report.Ready = true
	for _, target := range targets {
		probe, err := measureDownload(ctx, client, base+target.path, target.size, target.sha)
		report.Probes = append(report.Probes, probe)
		report.Ready = report.Ready && err == nil
		if ctx.Err() != nil {
			return report, nil
		}
	}
	return report, nil
}

func auditWithdrawals(git releaseGit, source string, listed map[string]Summary, expected []Identity) ([]WithdrawalAudit, error) {
	if len(expected) == 0 {
		return nil, nil
	}
	body, err := git.blob(source, "registry/_policy/withdrawals.json", maxPolicyBytes)
	if err != nil {
		return nil, err
	}
	var policy []Withdrawal
	if err := decodeJSON(body, &policy); err != nil {
		return nil, fmt.Errorf("read signed source withdrawal policy: %w", err)
	}
	declared, err := withdrawalSet(policy)
	if err != nil {
		return nil, err
	}
	identities := slices.Clone(expected)
	slices.SortFunc(identities, func(left, right Identity) int {
		return cmp.Or(strings.Compare(left.ProviderType, right.ProviderType), strings.Compare(left.Version, right.Version))
	})
	result := make([]WithdrawalAudit, 0, len(identities))
	for _, id := range slices.Compact(identities) {
		if err := checkIdentity(id); err != nil {
			return nil, err
		}
		if !declared[id] {
			return nil, fmt.Errorf("%s@v%s is missing from the signed source withdrawal policy", id.ProviderType, id.Version)
		}
		current := listed[id.ProviderType]
		if declared[current.Identity] {
			return nil, fmt.Errorf("withdrawn version %s@v%s is still listed in the expected index", current.ProviderType, current.Version)
		}
		result = append(result, WithdrawalAudit{Identity: id, ReplacementVersion: current.Version})
	}
	return result, nil
}

func auditFiles(git releaseGit, source string, listed map[string]Summary, providers []string) ([]mirrorFile, error) {
	files := make(map[string]mirrorFile)
	for _, provider := range providers {
		summary, ok := listed[provider]
		if !ok {
			return nil, fmt.Errorf("provider %q is absent from the published index", provider)
		}
		body, err := git.blob(source, path.Join("registry", provider, "v"+summary.Version+".json"), maxRecordBytes)
		if err != nil {
			return nil, err
		}
		var version Version
		if err := decodeJSON(body, &version); err != nil {
			return nil, fmt.Errorf("read %s registry entry: %w", provider, err)
		}
		if err := version.Validate(); err != nil {
			return nil, fmt.Errorf("validate %s registry entry: %w", provider, err)
		}
		snapshot := Snapshot{Files: map[string][]byte{}}
		if err := snapshot.add(&version); err != nil {
			return nil, err
		}
		if !sameCatalog(Catalog{Plugins: []Summary{summary}}, snapshot.Catalog) {
			return nil, fmt.Errorf("registry entry for %s differs from the signed index", provider)
		}
		for file, body := range snapshot.Files {
			location := marketCDNPath + "registry/" + file
			files[location] = checksumFile(location, body)
		}
		for _, artifact := range version.Artifacts {
			location := "/gh/" + version.Repository + "@" + version.ReleaseCommit + "/" + artifact.Path
			files[location] = mirrorFile{location, artifact.Size, artifact.SHA256}
		}
	}
	return slices.SortedFunc(maps.Values(files), func(left, right mirrorFile) int {
		return strings.Compare(left.path, right.path)
	}), nil
}

var mirrorAuditPrefixPart = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

func mirrorAuditBase(input string) (string, error) {
	raw := strings.TrimSpace(input)
	base, err := url.Parse(raw)
	if err != nil || len(raw) > 1024 || base.Scheme != "https" || base.Hostname() == "" || base.Opaque != "" ||
		base.User != nil || strings.ContainsAny(raw, "?#%\\") || strings.HasSuffix(base.Host, ":") {
		return "", fmt.Errorf("mirror base must be HTTPS without credentials, escapes, query or fragment")
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	if base.Path != "" {
		for part := range strings.SplitSeq(strings.TrimPrefix(base.Path, "/"), "/") {
			if !mirrorAuditPrefixPart.MatchString(part) || part == "." || part == ".." {
				return "", fmt.Errorf("invalid mirror path prefix")
			}
		}
	}
	return base.String(), nil
}
