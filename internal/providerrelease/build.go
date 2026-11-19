package providerrelease

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/certimate-go/certimate/pkg/plugin"
)

const (
	MaxArtifactBytes int64 = 20_000_000
	MaxSnapshotBytes int64 = 150_000_000
	RecordName             = "release.json"
)

var (
	stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	commitSHA     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	assetBasename = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
)

type Target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

func Targets() []Target {
	return []Target{
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"linux", "amd64"},
		{"linux", "arm"},
		{"linux", "arm64"},
		{"windows", "386"},
		{"windows", "amd64"},
		{"windows", "arm64"},
	}
}

func (target Target) ArchiveName() string {
	return target.OS + "_" + target.Arch + ".zip"
}

type Artifact struct {
	Target
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Record struct {
	ProviderType string     `json:"provider_type"`
	Repository   string     `json:"repository"`
	Version      string     `json:"version"`
	Tag          string     `json:"tag"`
	SourceCommit string     `json:"source_commit"`
	BuildRunID   string     `json:"build_run_id"`
	Artifacts    []Artifact `json:"artifacts"`
}

type Options struct {
	SourceDir  string
	Repository string
	Trigger    string
	SourceSHA  string
	BuildRunID string
}

func Build(ctx context.Context, options Options) (*Record, error) {
	if !commitSHA.MatchString(options.SourceSHA) {
		return nil, errors.New("source commit must be a full lowercase Git SHA")
	}
	if runID, err := strconv.ParseUint(options.BuildRunID, 10, 64); err != nil || runID == 0 || strconv.FormatUint(runID, 10) != options.BuildRunID {
		return nil, errors.New("build run ID must be a positive decimal GitHub Actions run ID")
	}
	if err := ValidateRepository(options.Repository); err != nil {
		return nil, err
	}
	commit, err := gitOutput(ctx, options.SourceDir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(commit)) != options.SourceSHA {
		return nil, errors.New("source commit does not match checkout HEAD")
	}
	status, err := gitOutput(ctx, options.SourceDir, "status", "--porcelain", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return nil, err
	}
	if len(status) != 0 {
		return nil, errors.New("source checkout must be clean, including submodules")
	}
	submodules, err := gitOutput(ctx, options.SourceDir, "submodule", "status", "--recursive")
	if err != nil {
		return nil, err
	}
	for status := range bytes.SplitSeq(submodules, []byte{'\n'}) {
		if len(status) != 0 && status[0] != ' ' {
			return nil, errors.New("submodules must be checked out at their recorded commits")
		}
	}
	manifest, raw, err := readManifest(options.SourceDir)
	if err != nil {
		return nil, err
	}
	if !stableVersion.MatchString(manifest.Version) || options.Trigger != "v"+manifest.Version+"-detach" {
		return nil, errors.New("trigger tag must be vX.Y.Z-detach and match the stable manifest version")
	}
	used, err := TrackedSnapshotSize(ctx, options.SourceDir, options.SourceSHA)
	if err != nil {
		return nil, err
	}
	buildDir := filepath.Join(options.SourceDir, "build")
	if _, err := os.Lstat(buildDir); !errors.Is(err, fs.ErrNotExist) {
		return nil, errors.New("build directory already exists or cannot be inspected")
	}
	tempDir, err := os.MkdirTemp(options.SourceDir, ".release-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	outputDir := filepath.Join(tempDir, "build")
	if err := os.Mkdir(outputDir, 0o755); err != nil {
		return nil, err
	}
	record := &Record{
		ProviderType: manifest.ProviderType, Repository: options.Repository, Version: manifest.Version,
		Tag: "v" + manifest.Version, SourceCommit: options.SourceSHA, BuildRunID: options.BuildRunID,
	}
	for _, target := range Targets() {
		binaryPath := filepath.Join(tempDir, manifest.Binary)
		if err := compileTarget(ctx, options.SourceDir, binaryPath, target); err != nil {
			return nil, err
		}
		artifact, err := packageTarget(options.SourceDir, outputDir, binaryPath, manifest, raw, target)
		if err != nil {
			return nil, fmt.Errorf("package %s: %w", target.ArchiveName(), err)
		}
		used += artifact.Size
		if used > MaxSnapshotBytes {
			return nil, fmt.Errorf("release tree exceeds %d bytes", MaxSnapshotBytes)
		}
		record.Artifacts = append(record.Artifacts, artifact)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if used+int64(len(data)) > MaxSnapshotBytes {
		return nil, fmt.Errorf("release tree exceeds %d bytes", MaxSnapshotBytes)
	}
	if err := os.WriteFile(filepath.Join(outputDir, RecordName), data, 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(outputDir, buildDir); err != nil {
		return nil, err
	}
	return record, nil
}

func ValidateRepository(repository string) error {
	name, ok := strings.CutPrefix(repository, "certimate-go/plugin-provider-")
	if !ok || plugin.ValidateProviderType(name) != nil {
		return fmt.Errorf("invalid official provider repository %q", repository)
	}
	return nil
}

func readManifest(dir string) (*plugin.Manifest, map[string]json.RawMessage, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, nil, err
	}
	manifest, err := plugin.ParseManifest(data)
	if err != nil {
		return nil, nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, nil, err
	}
	if err := plugin.ValidateProviderType(manifest.ProviderType); err != nil {
		return nil, nil, err
	}
	if err := plugin.ValidateBinaryName(manifest.Binary); err != nil {
		return nil, nil, err
	}
	if !strings.HasSuffix(manifest.Binary, ".exe") || !assetBasename.MatchString(manifest.Binary) {
		return nil, nil, errors.New("manifest binary must be a basename ending in .exe")
	}
	if manifest.Icon != "" && (!assetBasename.MatchString(manifest.Icon) || manifest.Icon == manifest.Binary || manifest.Icon == "manifest.json") {
		return nil, nil, errors.New("manifest icon must be a distinct basename")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	if _, legacy := raw["release"]; legacy {
		return nil, nil, errors.New("manifest must not contain legacy release metadata")
	}
	return manifest, raw, nil
}

func gitOutput(ctx context.Context, dir string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = dir
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", arguments[0], err)
	}
	return output, nil
}

func TrackedSnapshotSize(ctx context.Context, dir, sha string) (int64, error) {
	if !commitSHA.MatchString(sha) {
		return 0, errors.New("invalid commit SHA for Git tree size check")
	}
	output, err := gitOutput(ctx, dir, "--no-replace-objects", "ls-tree", "-lrz", sha)
	if err != nil {
		return 0, err
	}
	var total int64
	for entry := range bytes.SplitSeq(output, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		metadata, _, _ := bytes.Cut(entry, []byte{'\t'})
		fields := bytes.Fields(metadata)
		if len(fields) != 4 {
			return 0, errors.New("invalid Git tree entry")
		}
		if string(fields[1]) != "blob" {
			continue
		}
		size, err := strconv.ParseInt(string(fields[3]), 10, 64)
		if err != nil || size < 0 || size > MaxSnapshotBytes-total {
			return 0, fmt.Errorf("source tree exceeds %d bytes or has an invalid blob size", MaxSnapshotBytes)
		}
		total += size
	}
	return total, nil
}

func compileTarget(ctx context.Context, dir, binaryPath string, target Target) error {
	absolutePath, err := filepath.Abs(binaryPath)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildmode=exe", "-buildvcs=false", "-trimpath", "-ldflags=-s -w", "-o", absolutePath, ".")
	command.Dir = dir
	command.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0", "GOOS="+target.OS, "GOARCH="+target.Arch)
	if target.Arch == "arm" {
		command.Env = append(command.Env, "GOARM=7")
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("build %s/%s: %w\n%s", target.OS, target.Arch, err, output)
	}
	return nil
}

func packageTarget(sourceDir, outputDir, binaryPath string, manifest *plugin.Manifest, raw map[string]json.RawMessage, target Target) (Artifact, error) {
	binary, err := os.Open(binaryPath)
	if err != nil {
		return Artifact{}, err
	}
	defer binary.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, binary); err != nil {
		return Artifact{}, err
	}
	if _, err := binary.Seek(0, io.SeekStart); err != nil {
		return Artifact{}, err
	}
	for field, value := range map[string]string{"os": target.OS, "arch": target.Arch, "sha256": fmt.Sprintf("%x", hash.Sum(nil))} {
		raw[field], _ = json.Marshal(value)
	}
	manifestData, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return Artifact{}, err
	}
	filename := target.ArchiveName()
	archive, err := os.OpenFile(filepath.Join(outputDir, filename), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Artifact{}, err
	}
	hash.Reset()
	writer := zip.NewWriter(io.MultiWriter(archive, hash))
	writeErr := writeZipEntry(writer, manifest.Binary, 0o755, binary)
	if writeErr == nil {
		writeErr = writeZipEntry(writer, "manifest.json", 0o644, bytes.NewReader(manifestData))
	}
	if writeErr == nil && manifest.Icon != "" {
		iconPath := filepath.Join(sourceDir, manifest.Icon)
		if info, err := os.Lstat(iconPath); err != nil || !info.Mode().IsRegular() {
			writeErr = errors.New("manifest icon is not a regular file")
		} else if icon, err := os.Open(iconPath); err != nil {
			writeErr = err
		} else {
			writeErr = errors.Join(writeZipEntry(writer, manifest.Icon, 0o644, icon), icon.Close())
		}
	}
	if err := errors.Join(writeErr, writer.Close()); err != nil {
		return Artifact{}, errors.Join(err, archive.Close())
	}
	info, statErr := archive.Stat()
	if err := errors.Join(statErr, archive.Close()); err != nil {
		return Artifact{}, err
	}
	if info.Size() > MaxArtifactBytes {
		return Artifact{}, fmt.Errorf("ZIP exceeds %d bytes", MaxArtifactBytes)
	}
	return Artifact{Target: target, Path: "build/" + filename, Size: info.Size(), SHA256: fmt.Sprintf("%x", hash.Sum(nil))}, nil
}

func writeZipEntry(writer *zip.Writer, name string, mode fs.FileMode, contents io.Reader) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(mode)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, contents)
	return err
}
