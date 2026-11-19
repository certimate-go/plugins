package registry

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/certimate-go/plugins/internal/providerrelease"
)

const (
	maxRecordBytes = 512 << 10
	maxIconBytes   = 256 << 10
	maxPolicyBytes = 1 << 20
)

var (
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	localePattern  = regexp.MustCompile(`^[a-z]{2,3}(-[a-zA-Z0-9]{2,8})*$`)
)

type Version struct {
	providerrelease.Record
	ReleaseCommit string                 `json:"release_commit"`
	Manifest      plugin.Manifest        `json:"manifest"`
	Translations  map[string]Translation `json:"i18n,omitempty"`
	Icon          []byte                 `json:"icon,omitempty"`
}

type Translation struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type Identity struct {
	ProviderType string `json:"provider_type"`
	Version      string `json:"version"`
}

type ReadyRelease struct {
	Identity
	ReleaseCommit string `json:"release_commit"`
}

type Withdrawal struct {
	Identity
	Reason string `json:"reason"`
}

type ArtifactReference struct {
	SHA256 string `json:"sha256"`
}

type IconReference struct {
	ArtifactReference
	Format string `json:"format"`
}

type Summary struct {
	Identity
	Capability      string                 `json:"capability"`
	Category        string                 `json:"category,omitempty"`
	DisplayNameKey  string                 `json:"display_name_key,omitempty"`
	Description     string                 `json:"description,omitempty"`
	ProtocolVersion uint32                 `json:"protocol_version"`
	MinCoreVersion  string                 `json:"min_core_version,omitempty"`
	MaxCoreVersion  string                 `json:"max_core_version,omitempty"`
	Repository      string                 `json:"repository"`
	ReleaseCommit   string                 `json:"release_commit"`
	Platforms       []string               `json:"platforms"`
	Translations    map[string]Translation `json:"i18n,omitempty"`
	Detail          ArtifactReference      `json:"detail"`
	Icon            *IconReference         `json:"icon,omitempty"`
}

type Detail struct {
	providerrelease.Record
	ReleaseCommit string          `json:"release_commit"`
	Manifest      plugin.Manifest `json:"manifest"`
	Icon          *IconReference  `json:"icon,omitempty"`
}

type Catalog struct {
	Plugins []Summary `json:"plugins"`
}

type Snapshot struct {
	Catalog Catalog
	Files   map[string][]byte
}

func Generate(records fs.FS, ready []ReadyRelease) (Snapshot, error) {
	catalog := Snapshot{Catalog: Catalog{Plugins: []Summary{}}, Files: make(map[string][]byte)}
	dirs, err := fs.ReadDir(records, ".")
	if err != nil {
		return Snapshot{}, err
	}
	if !slices.ContainsFunc(dirs, func(entry fs.DirEntry) bool { return entry.Name() == "_policy" && entry.IsDir() }) {
		return Snapshot{}, errors.New("registry policy must be a directory")
	}
	admitted, withdrawn, err := loadPolicy(records)
	if err != nil {
		return Snapshot{}, err
	}
	readyByID := make(map[Identity]string, len(ready))
	for _, release := range ready {
		if err := checkIdentity(release.Identity); err != nil {
			return Snapshot{}, err
		}
		if !commitPattern.MatchString(release.ReleaseCommit) {
			return Snapshot{}, fmt.Errorf("invalid observed commit for %+v", release.Identity)
		}
		if _, duplicate := readyByID[release.Identity]; duplicate {
			return Snapshot{}, fmt.Errorf("duplicate observed release %+v", release.Identity)
		}
		readyByID[release.Identity] = release.ReleaseCommit
	}

	for _, dir := range dirs {
		if (dir.Name() == "_policy" || dir.Name() == "market") && dir.IsDir() ||
			dir.Name() == "index.json" && dir.Type().IsRegular() {
			continue
		}
		if !dir.IsDir() || admitted[dir.Name()] == "" {
			return Snapshot{}, fmt.Errorf("unexpected or unadmitted registry path %q", dir.Name())
		}
		versions, err := loadProvider(records, dir.Name(), admitted[dir.Name()])
		if err != nil {
			return Snapshot{}, err
		}
		var current *Version
		for _, version := range versions {
			id := Identity{ProviderType: version.ProviderType, Version: version.Version}
			readyCommit, observed := readyByID[id]
			if observed && readyCommit != version.ReleaseCommit {
				return Snapshot{}, fmt.Errorf("observed commit differs from the registered commit for %+v", id)
			}
			delete(readyByID, id)
			if withdrawn[id] || !observed {
				continue
			}
			if current == nil || keepsCompatibility(current.Manifest, version.Manifest) {
				current = version
			}
		}
		if current != nil {
			if err := catalog.add(current); err != nil {
				return Snapshot{}, err
			}
		}
	}
	if len(readyByID) != 0 {
		return Snapshot{}, errors.New("CDN observations include an unregistered release")
	}
	return catalog, nil
}

func loadPolicy(records fs.FS) (map[string]string, map[Identity]bool, error) {
	policyEntries, err := fs.ReadDir(records, "_policy")
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range policyEntries {
		if entry.Name() == admissionBatchName && entry.IsDir() {
			continue
		}
		if !entry.Type().IsRegular() || (entry.Name() != "admissions.json" && entry.Name() != "withdrawals.json") {
			return nil, nil, fmt.Errorf("unexpected policy path %q", entry.Name())
		}
	}
	var admissions map[string]string
	if err := readJSON(records, "_policy/admissions.json", maxPolicyBytes, &admissions); err != nil {
		return nil, nil, err
	}
	if err := validateAdmissions(admissions); err != nil {
		return nil, nil, err
	}
	var withdrawals []Withdrawal
	if err := readJSON(records, "_policy/withdrawals.json", maxPolicyBytes, &withdrawals); err != nil {
		return nil, nil, err
	}
	withdrawn, err := withdrawalSet(withdrawals)
	if err != nil {
		return nil, nil, err
	}
	return admissions, withdrawn, nil
}

func withdrawalSet(withdrawals []Withdrawal) (map[Identity]bool, error) {
	if withdrawals == nil {
		return nil, errors.New("withdrawals must be an array")
	}
	withdrawn := make(map[Identity]bool, len(withdrawals))
	for _, entry := range withdrawals {
		if err := checkIdentity(entry.Identity); err != nil {
			return nil, err
		}
		if strings.TrimSpace(entry.Reason) == "" || len(entry.Reason) > 1024 || withdrawn[entry.Identity] {
			return nil, fmt.Errorf("invalid or duplicate withdrawal %+v", entry.Identity)
		}
		withdrawn[entry.Identity] = true
	}
	return withdrawn, nil
}

func loadProvider(records fs.FS, provider, repository string) ([]*Version, error) {
	paths, err := fs.ReadDir(records, provider)
	if err != nil {
		return nil, err
	}
	versions := make([]*Version, 0, len(paths))
	for _, entry := range paths {
		tag, matched := strings.CutSuffix(entry.Name(), ".json")
		if !entry.Type().IsRegular() || !matched || !strings.HasPrefix(tag, "v") || !versionPattern.MatchString(tag[1:]) {
			return nil, fmt.Errorf("unexpected registry file %q", path.Join(provider, entry.Name()))
		}
		var version Version
		filename := path.Join(provider, entry.Name())
		if err := readJSON(records, filename, maxRecordBytes, &version); err != nil {
			return nil, err
		}
		if version.ProviderType != provider || version.Repository != repository || version.Tag != tag {
			return nil, fmt.Errorf("version record %q does not match path or admission policy", filename)
		}
		if err := version.Validate(); err != nil {
			return nil, fmt.Errorf("version record %q: %w", filename, err)
		}
		versions = append(versions, &version)
	}
	slices.SortFunc(versions, func(left, right *Version) int { return compareVersions(left.Version, right.Version) })
	return versions, nil
}

func (version *Version) Validate() error {
	if err := checkIdentity(Identity{version.ProviderType, version.Version}); err != nil {
		return err
	}
	if err := providerrelease.ValidateRepository(version.Repository); err != nil {
		return err
	}
	if version.Tag != "v"+version.Version || !commitPattern.MatchString(version.SourceCommit) ||
		!commitPattern.MatchString(version.ReleaseCommit) || version.SourceCommit == version.ReleaseCommit {
		return errors.New("invalid tag, source commit or artifact commit")
	}
	if runID, err := strconv.ParseUint(version.BuildRunID, 10, 64); err != nil || runID == 0 || strconv.FormatUint(runID, 10) != version.BuildRunID {
		return errors.New("invalid trusted build run ID")
	}
	manifest := &version.Manifest
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := plugin.ValidateBinaryName(manifest.Binary); err != nil {
		return err
	}
	if manifest.ProviderType != version.ProviderType || manifest.Version != version.Version ||
		manifest.OS != "" || manifest.Arch != "" || manifest.SHA256 != "" {
		return errors.New("manifest identity or platform-specific fields are invalid")
	}
	if err := plugin.ValidateProviderType(manifest.AccessProviderType); err != nil {
		return fmt.Errorf("invalid access provider: %w", err)
	}
	if !validCoreBound(manifest.MinCoreVersion) || !validCoreBound(manifest.MaxCoreVersion) ||
		(manifest.MinCoreVersion != "" && manifest.MaxCoreVersion != "" &&
			compareVersions(coreVersion(manifest.MinCoreVersion), coreVersion(manifest.MaxCoreVersion)) > 0) {
		return errors.New("invalid Core compatibility range")
	}
	if len(version.Translations) > 32 {
		return errors.New("too many translations")
	}
	for locale, translation := range version.Translations {
		if !localePattern.MatchString(locale) || len(translation.Name) > 256 || len(translation.Description) > 2048 {
			return fmt.Errorf("invalid translation for %q", locale)
		}
	}
	if _, err := iconFormat(version.Icon); err != nil {
		return err
	}
	targets := providerrelease.Targets()
	if len(version.Artifacts) != len(targets) {
		return errors.New("incomplete platform matrix")
	}
	seen := make(map[providerrelease.Target]bool, len(targets))
	var total int64
	for _, artifact := range version.Artifacts {
		if !slices.Contains(targets, artifact.Target) || seen[artifact.Target] || artifact.Path != "build/"+artifact.ArchiveName() ||
			artifact.Size <= 0 || artifact.Size > providerrelease.MaxArtifactBytes || !digestPattern.MatchString(artifact.SHA256) {
			return fmt.Errorf("invalid or duplicate artifact for %s/%s", artifact.OS, artifact.Arch)
		}
		seen[artifact.Target] = true
		total += artifact.Size
	}
	if total > providerrelease.MaxSnapshotBytes {
		return errors.New("artifact total exceeds CDN package limit")
	}
	return nil
}

func (snapshot *Snapshot) add(version *Version) error {
	assets := slices.Clone(version.Artifacts)
	slices.SortFunc(assets, func(left, right providerrelease.Artifact) int {
		return strings.Compare(left.OS+"/"+left.Arch, right.OS+"/"+right.Arch)
	})
	platforms := make([]string, 0, len(assets))
	for _, artifact := range assets {
		platforms = append(platforms, artifact.OS+"/"+artifact.Arch)
	}
	format, err := iconFormat(version.Icon)
	if err != nil {
		return err
	}
	var icon *IconReference
	if format != "" {
		digest := fmt.Sprintf("%x", sha256.Sum256(version.Icon))
		icon = &IconReference{ArtifactReference{digest}, format}
		snapshot.Files["market/icons/"+digest+"."+format] = version.Icon
	}
	record := version.Record
	record.Artifacts = assets
	manifest := version.Manifest
	manifest.MinCoreVersion = coreVersion(manifest.MinCoreVersion)
	manifest.MaxCoreVersion = coreVersion(manifest.MaxCoreVersion)
	detail, err := json.Marshal(Detail{record, version.ReleaseCommit, manifest, icon})
	if err != nil {
		return err
	}
	detail = append(detail, '\n')
	digest := fmt.Sprintf("%x", sha256.Sum256(detail))
	snapshot.Files["market/details/"+digest+".json"] = detail
	snapshot.Catalog.Plugins = append(snapshot.Catalog.Plugins, Summary{
		Identity:   Identity{version.ProviderType, version.Version},
		Capability: manifest.Capability, Category: manifest.Category, DisplayNameKey: manifest.DisplayNameKey, Description: manifest.Description,
		ProtocolVersion: manifest.ProtocolVersion, MinCoreVersion: manifest.MinCoreVersion, MaxCoreVersion: manifest.MaxCoreVersion,
		Repository: version.Repository, ReleaseCommit: version.ReleaseCommit, Platforms: platforms,
		Translations: version.Translations, Detail: ArtifactReference{digest}, Icon: icon,
	})
	return nil
}

func keepsCompatibility(previous, candidate plugin.Manifest) bool {
	oldMin, newMin := coreVersion(previous.MinCoreVersion), coreVersion(candidate.MinCoreVersion)
	oldMax, newMax := coreVersion(previous.MaxCoreVersion), coreVersion(candidate.MaxCoreVersion)
	return previous.ProtocolVersion == candidate.ProtocolVersion && previous.Capability == candidate.Capability &&
		previous.AccessProviderType == candidate.AccessProviderType &&
		(newMin == "" || (oldMin != "" && compareVersions(newMin, oldMin) <= 0)) &&
		(newMax == "" || (oldMax != "" && compareVersions(newMax, oldMax) >= 0))
}

func checkIdentity(id Identity) error {
	if err := plugin.ValidateProviderType(id.ProviderType); err != nil {
		return err
	}
	if !versionPattern.MatchString(id.Version) || len(id.Version) > 64 {
		return fmt.Errorf("invalid stable version %q", id.Version)
	}
	return nil
}

func validateAdmissions(admissions map[string]string) error {
	if admissions == nil {
		return errors.New("admissions must be an object")
	}
	seen := make(map[string]bool, len(admissions))
	for provider, repository := range admissions {
		if err := plugin.ValidateProviderType(provider); err != nil {
			return err
		}
		if err := providerrelease.ValidateRepository(repository); err != nil {
			return err
		}
		if seen[repository] {
			return fmt.Errorf("repository %q is admitted for multiple providers", repository)
		}
		seen[repository] = true
	}
	return nil
}

func coreVersion(version string) string { return strings.TrimPrefix(version, "v") }

func validCoreBound(version string) bool {
	return version == "" || (len(version) <= 64 && versionPattern.MatchString(coreVersion(version)))
}

func compareVersions(left, right string) int {
	leftParts, rightParts := strings.Split(left, "."), strings.Split(right, ".")
	for index, part := range leftParts {
		other := rightParts[index]
		if len(part) != len(other) {
			return len(part) - len(other)
		}
		if comparison := strings.Compare(part, other); comparison != 0 {
			return comparison
		}
	}
	return 0
}

func iconFormat(icon []byte) (string, error) {
	if len(icon) == 0 {
		return "", nil
	}
	if len(icon) > maxIconBytes {
		return "", errors.New("market icon exceeds size limit")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(icon))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 1024 || config.Height > 1024 || (format != "png" && format != "jpeg") {
		return "", errors.New("market icon must be a PNG or JPEG no larger than 1024×1024")
	}
	if _, _, err := image.Decode(bytes.NewReader(icon)); err != nil {
		return "", errors.New("invalid encoded market icon")
	}
	return format, nil
}

func readJSON(source fs.FS, filename string, limit int64, destination any) error {
	file, err := source.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("%q exceeds %d bytes", filename, limit)
	}
	if err := decodeJSON(body, destination); err != nil {
		return fmt.Errorf("%q: %w", filename, err)
	}
	return nil
}

func decodeJSON(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("unexpected content following the JSON value")
	}
	return nil
}

func ReadObservations(source fs.FS, filename string) ([]ReadyRelease, error) {
	var observations []ReadyRelease
	if err := readJSON(source, filename, maxPolicyBytes, &observations); err != nil {
		return nil, err
	}
	if observations == nil {
		return nil, errors.New("CDN observations must be an array")
	}
	return observations, nil
}

func ReadVersion(source fs.FS, filename string) (Version, error) {
	var version Version
	if err := readJSON(source, filename, maxRecordBytes, &version); err != nil {
		return Version{}, err
	}
	return version, version.Validate()
}
