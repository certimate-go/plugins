package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/certimate-go/plugins/internal/providerrelease"
)

type StageReleaseOptions struct {
	Checkout     string
	ArtifactsDir string
	Repository   string
	Tag          string
	SourceCommit string
	BuildRunID   string
}

type StagedRelease struct {
	ReleaseCommit string `json:"release_commit"`
	AlreadyTagged bool   `json:"already_tagged"`
}

func StageRelease(ctx context.Context, options StageReleaseOptions) (StagedRelease, error) {
	if err := providerrelease.ValidateRepository(options.Repository); err != nil {
		return StagedRelease{}, err
	}
	if err := validateReleaseTag(options.Tag); err != nil {
		return StagedRelease{}, err
	}
	if !commitPattern.MatchString(options.SourceCommit) {
		return StagedRelease{}, errors.New("invalid verified source commit")
	}
	git := releaseGit{ctx, options.Checkout}
	formalRef := "refs/tags/" + options.Tag
	present, err := git.output("for-each-ref", "--format=%(refname)", formalRef)
	if err != nil {
		return StagedRelease{}, err
	}
	if strings.TrimSpace(string(present)) != "" {
		existing, err := InspectRelease(ctx, options.Checkout, options.Repository, options.Tag)
		if err != nil {
			return StagedRelease{}, err
		}
		if existing.SourceCommit != options.SourceCommit {
			return StagedRelease{}, fmt.Errorf("formal tag %s already exists for a different source commit", options.Tag)
		}
		return StagedRelease{ReleaseCommit: existing.ReleaseCommit, AlreadyTagged: true}, nil
	}
	root, err := os.OpenRoot(options.ArtifactsDir)
	if err != nil {
		return StagedRelease{}, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return StagedRelease{}, err
	}
	recordBytes, err := readBuildFile(root.FS(), providerrelease.RecordName, maxRecordBytes)
	if err != nil {
		return StagedRelease{}, err
	}
	var record providerrelease.Record
	if err := decodeJSON(recordBytes, &record); err != nil {
		return StagedRelease{}, err
	}
	if err := checkIdentity(Identity{record.ProviderType, record.Version}); err != nil {
		return StagedRelease{}, err
	}
	targets := providerrelease.Targets()
	if record.Tag != options.Tag || record.Version != strings.TrimPrefix(options.Tag, "v") ||
		record.SourceCommit != options.SourceCommit || record.Repository != options.Repository || options.BuildRunID == "" ||
		record.BuildRunID != options.BuildRunID || len(record.Artifacts) != len(targets) || len(entries) != len(targets)+1 {
		return StagedRelease{}, errors.New("build record does not match the trusted run, source, or platform matrix")
	}
	expectedFiles := map[string]bool{providerrelease.RecordName: true}
	for _, target := range targets {
		expectedFiles[target.ArchiveName()] = true
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !expectedFiles[entry.Name()] {
			return StagedRelease{}, fmt.Errorf("unexpected build output %q", entry.Name())
		}
	}
	indexDir, err := os.MkdirTemp("", "provider-release-index-")
	if err != nil {
		return StagedRelease{}, err
	}
	defer os.RemoveAll(indexDir)
	index := filepath.Join(indexDir, "index")
	if _, err := git.withIndex(index, nil, "read-tree", options.SourceCommit); err != nil {
		return StagedRelease{}, err
	}
	if err := git.addBuildBlob(index, providerrelease.RecordName, recordBytes); err != nil {
		return StagedRelease{}, err
	}
	seen := make(map[string]bool, len(targets))
	for _, artifact := range record.Artifacts {
		name := artifact.ArchiveName()
		if name == providerrelease.RecordName || seen[name] || !expectedFiles[name] || artifact.Path != "build/"+name {
			return StagedRelease{}, fmt.Errorf("invalid or repeated build archive path %q", artifact.Path)
		}
		seen[name] = true
		archive, err := readBuildFile(root.FS(), name, providerrelease.MaxArtifactBytes)
		if err != nil {
			return StagedRelease{}, err
		}
		if int64(len(archive)) != artifact.Size || fmt.Sprintf("%x", sha256.Sum256(archive)) != artifact.SHA256 {
			return StagedRelease{}, fmt.Errorf("%s does not match recorded ZIP size and digest", name)
		}
		if err := git.addBuildBlob(index, name, archive); err != nil {
			return StagedRelease{}, err
		}
	}
	tree, err := git.withIndex(index, nil, "write-tree")
	if err != nil {
		return StagedRelease{}, err
	}
	commit, err := git.output("-c", "user.name=Certimate Plugin Publisher", "-c", "user.email=release@certimate.me", "-c", "commit.gpgSign=false",
		"commit-tree", strings.TrimSpace(string(tree)), "-p", options.SourceCommit, "-m", "Publish "+record.ProviderType+" "+options.Tag)
	if err != nil {
		return StagedRelease{}, err
	}
	commitSHA := strings.TrimSpace(string(commit))
	if _, err := git.inspectRelease(options.Repository, options.Tag, commitSHA); err != nil {
		return StagedRelease{}, err
	}
	return StagedRelease{ReleaseCommit: commitSHA}, nil
}

func readBuildFile(root fs.FS, filename string, limit int64) ([]byte, error) {
	file, err := root.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("invalid build file type or size for %q", filename)
	}
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) != info.Size() {
		return nil, fmt.Errorf("build file %q changed while reading", filename)
	}
	return body, nil
}

func (git releaseGit) withIndex(index string, input io.Reader, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(git.ctx, "git", append([]string{"--no-replace-objects"}, arguments...)...)
	command.Dir = git.checkout
	command.Env = append(command.Environ(), "GIT_INDEX_FILE="+index)
	command.Stdin = input
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", arguments[0], err)
	}
	return output, nil
}

func (git releaseGit) addBuildBlob(index, filename string, body []byte) error {
	return git.addBlob(index, "build/"+filename, body)
}

func (git releaseGit) addBlob(index, filename string, body []byte) error {
	object, err := git.withIndex(index, bytes.NewReader(body), "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	_, err = git.withIndex(index, nil, "update-index", "--add", "--cacheinfo", "100644,"+strings.TrimSpace(string(object))+","+filename)
	return err
}
