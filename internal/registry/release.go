package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"path"
	"reflect"
	"slices"
	"strings"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/certimate-go/plugins/internal/providerrelease"
)

const (
	maxManifestBytes = 128 << 10
	maxBundleBytes   = 256 << 10
	maxBinaryBytes   = 128 << 20
)

func InspectRelease(ctx context.Context, checkout, repository, tag string) (Version, error) {
	if err := providerrelease.ValidateRepository(repository); err != nil {
		return Version{}, err
	}
	if err := validateReleaseTag(tag); err != nil {
		return Version{}, err
	}
	git := releaseGit{ctx, checkout}
	artifactRef, err := git.output("rev-parse", "--verify", "refs/tags/"+tag+"^{commit}")
	if err != nil {
		return Version{}, err
	}
	return git.inspectRelease(repository, tag, strings.TrimSpace(string(artifactRef)))
}

func validateReleaseTag(tag string) error {
	versionNumber, matched := strings.CutPrefix(tag, "v")
	if !matched || len(versionNumber) > 64 || !versionPattern.MatchString(versionNumber) {
		return errors.New("expected a formal vX.Y.Z release tag")
	}
	return nil
}

func (git releaseGit) inspectRelease(repository, tag, releaseSHA string) (Version, error) {
	if !commitPattern.MatchString(releaseSHA) {
		return Version{}, errors.New("invalid formal tag commit")
	}
	if _, err := providerrelease.TrackedSnapshotSize(git.ctx, git.checkout, releaseSHA); err != nil {
		return Version{}, fmt.Errorf("check formal tag size: %w", err)
	}
	body, err := git.blob(releaseSHA, "build/"+providerrelease.RecordName, maxRecordBytes)
	if err != nil {
		return Version{}, err
	}
	var record providerrelease.Record
	if err := decodeJSON(body, &record); err != nil {
		return Version{}, fmt.Errorf("decode release record: %w", err)
	}
	if record.Repository != repository || record.Tag != tag || !commitPattern.MatchString(record.SourceCommit) {
		return Version{}, errors.New("release record does not match the formal tag or repository")
	}
	parentLine, err := git.output("rev-list", "--parents", "-n", "1", releaseSHA)
	if err != nil {
		return Version{}, err
	}
	parents := strings.Fields(string(parentLine))
	if len(parents) != 2 || parents[1] != record.SourceCommit {
		return Version{}, errors.New("artifact commit must have the recorded source commit as its only parent")
	}
	manifestBody, err := git.blob(record.SourceCommit, "manifest.json", maxManifestBytes)
	if err != nil {
		return Version{}, err
	}
	manifest, err := plugin.ParseManifest(manifestBody)
	if err != nil {
		return Version{}, err
	}
	var manifestFields map[string]json.RawMessage
	if err := json.Unmarshal(manifestBody, &manifestFields); err != nil {
		return Version{}, err
	}
	if _, legacy := manifestFields["release"]; legacy {
		return Version{}, errors.New("source manifest contains legacy release metadata")
	}
	if !strings.HasSuffix(manifest.Binary, ".exe") ||
		(manifest.Icon != "" && (manifest.Icon == manifest.Binary || manifest.Icon == "manifest.json" ||
			strings.ContainsAny(manifest.Icon, "/\\:") || strings.HasPrefix(manifest.Icon, "."))) {
		return Version{}, errors.New("invalid archive entry names in source manifest")
	}
	result := Version{Record: record, ReleaseCommit: releaseSHA, Manifest: *manifest}
	if err := result.Validate(); err != nil {
		return Version{}, err
	}
	if err := git.checkArtifactDiff(&result); err != nil {
		return Version{}, err
	}
	var icon []byte
	if manifest.Icon != "" {
		icon, err = git.blob(record.SourceCommit, manifest.Icon, maxIconBytes)
		if err != nil {
			return Version{}, err
		}
		iconPath := manifest.Icon
		if _, err := iconFormat(icon); err != nil {
			iconPath = "market-icon.png"
		}
		result.Icon, err = git.blob(record.SourceCommit, iconPath, maxIconBytes)
		if err != nil {
			return Version{}, fmt.Errorf("market requires a raster icon; retain the runtime icon and add market-icon.png: %w", err)
		}
	}
	for _, artifact := range record.Artifacts {
		archive, err := git.blob(releaseSHA, artifact.Path, providerrelease.MaxArtifactBytes)
		if err != nil {
			return Version{}, err
		}
		if int64(len(archive)) != artifact.Size || fmt.Sprintf("%x", sha256.Sum256(archive)) != artifact.SHA256 {
			return Version{}, fmt.Errorf("%s does not match recorded ZIP size and digest", artifact.Path)
		}
		if err := inspectArchive(archive, artifact, manifestFields, *manifest, icon); err != nil {
			return Version{}, fmt.Errorf("%s: %w", artifact.Path, err)
		}
	}
	result.Translations, err = git.translations(record.SourceCommit, manifest)
	if err != nil {
		return Version{}, err
	}
	if err := result.Validate(); err != nil {
		return Version{}, err
	}
	return result, nil
}

type releaseGit struct {
	ctx      context.Context
	checkout string
}

func (git releaseGit) ancestor(ancestor, descendant string) bool {
	_, err := git.output("merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}

func (git releaseGit) output(arguments ...string) ([]byte, error) {
	command := exec.CommandContext(git.ctx, "git", append([]string{"--no-replace-objects"}, arguments...)...)
	command.Dir = git.checkout
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(arguments, " "), err)
	}
	return output, nil
}

func (git releaseGit) blob(commit, filename string, limit int64) ([]byte, error) {
	tree, err := git.output("ls-tree", "-lz", commit, "--", ":(literal)"+filename)
	if err != nil {
		return nil, err
	}
	metadata, entry, _ := strings.Cut(string(tree), "\t")
	fields := strings.Fields(metadata)
	if entry != filename+"\x00" || len(fields) != 4 || (fields[0] != "100644" && fields[0] != "100755") ||
		fields[1] != "blob" || !commitPattern.MatchString(fields[2]) {
		return nil, fmt.Errorf("%s must be a regular tracked file", filename)
	}
	var size int64
	if _, err := fmt.Sscan(fields[3], &size); err != nil || size < 0 || size > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes or has an invalid size", filename, limit)
	}
	return git.output("cat-file", "blob", fields[2])
}

func (git releaseGit) checkArtifactDiff(version *Version) error {
	changes, err := git.output("diff-tree", "--no-commit-id", "-r", "-z", "--no-renames", "--name-status", version.SourceCommit, version.ReleaseCommit)
	if err != nil {
		return err
	}
	expected := map[string]bool{"build/" + providerrelease.RecordName: true}
	for _, artifact := range version.Artifacts {
		expected[artifact.Path] = true
	}
	fields := bytes.Split(bytes.TrimSuffix(changes, []byte{0}), []byte{0})
	if len(fields) != len(expected)*2 {
		return errors.New("formal tag must add only the expected build files")
	}
	for index := 0; index < len(fields); index += 2 {
		filename := string(fields[index+1])
		if string(fields[index]) != "A" || !expected[filename] {
			return fmt.Errorf("unexpected formal tag change to %q", filename)
		}
		delete(expected, filename)
	}
	return nil
}

func (git releaseGit) translations(commit string, manifest *plugin.Manifest) (map[string]Translation, error) {
	const directory = "schema/i18n"
	tree, err := git.output("ls-tree", "-r", "-z", "--name-only", commit, "--", directory+"/")
	if err != nil {
		return nil, err
	}
	result := make(map[string]Translation)
	for filename := range strings.SplitSeq(strings.TrimSuffix(string(tree), "\x00"), "\x00") {
		if filename == "" {
			continue
		}
		locale, matched := strings.CutSuffix(path.Base(filename), ".json")
		if !matched || path.Dir(filename) != directory || !localePattern.MatchString(locale) || len(result) >= 32 {
			return nil, fmt.Errorf("unexpected translation file %q", filename)
		}
		body, err := git.blob(commit, filename, maxBundleBytes)
		if err != nil {
			return nil, err
		}
		var bundle map[string]string
		if err := decodeJSON(body, &bundle); err != nil {
			return nil, fmt.Errorf("%s: %w", filename, err)
		}
		translation := Translation{bundle[manifest.DisplayNameKey], bundle["plugin."+manifest.ProviderType+".description"]}
		if translation.Name != "" || translation.Description != "" {
			result[locale] = translation
		}
	}
	return result, nil
}

func inspectArchive(data []byte, artifact providerrelease.Artifact, fields map[string]json.RawMessage, manifest plugin.Manifest, icon []byte) error {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	expected := []string{manifest.Binary, "manifest.json"}
	if manifest.Icon != "" {
		expected = append(expected, manifest.Icon)
	}
	if len(archive.File) != len(expected) {
		return errors.New("unexpected number of ZIP entries")
	}
	manifestCopy := maps.Clone(fields)
	var binaryDigest string
	var packedFields map[string]json.RawMessage
	for _, file := range archive.File {
		if !file.Mode().IsRegular() || !slices.Contains(expected, file.Name) {
			return fmt.Errorf("unexpected ZIP entry %q", file.Name)
		}
		expected = slices.DeleteFunc(expected, func(name string) bool { return name == file.Name })
		switch file.Name {
		case manifest.Binary:
			if file.Mode().Perm() != 0o755 {
				return errors.New("plugin binary must be executable")
			}
			hash := sha256.New()
			if err := copyZipFile(file, maxBinaryBytes, hash); err != nil {
				return err
			}
			binaryDigest = fmt.Sprintf("%x", hash.Sum(nil))
		case "manifest.json":
			var buffer bytes.Buffer
			if err := copyZipFile(file, maxManifestBytes, &buffer); err != nil {
				return err
			}
			if err := decodeJSON(buffer.Bytes(), &packedFields); err != nil {
				return err
			}
		case manifest.Icon:
			var buffer bytes.Buffer
			if err := copyZipFile(file, maxIconBytes, &buffer); err != nil {
				return err
			}
			if !bytes.Equal(buffer.Bytes(), icon) {
				return errors.New("packed icon differs from the source icon")
			}
		}
	}
	for key, value := range map[string]string{"os": artifact.OS, "arch": artifact.Arch, "sha256": binaryDigest} {
		manifestCopy[key], _ = json.Marshal(value)
	}
	var packed, source map[string]any
	reencodedArchive, _ := json.Marshal(packedFields)
	reencodedSource, _ := json.Marshal(manifestCopy)
	archiveDecoder := json.NewDecoder(bytes.NewReader(reencodedArchive))
	archiveDecoder.UseNumber()
	sourceDecoder := json.NewDecoder(bytes.NewReader(reencodedSource))
	sourceDecoder.UseNumber()
	if archiveDecoder.Decode(&packed) != nil || sourceDecoder.Decode(&source) != nil || !reflect.DeepEqual(packed, source) {
		return errors.New("packed manifest does not match source, platform and binary digest")
	}
	return nil
}

func copyZipFile(file *zip.File, limit int64, writer io.Writer) error {
	if file.UncompressedSize64 == 0 || file.UncompressedSize64 > uint64(limit) {
		return fmt.Errorf("invalid expanded size of ZIP entry %q", file.Name)
	}
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	size, err := io.Copy(writer, io.LimitReader(reader, limit+1))
	if err != nil {
		return err
	}
	if size > limit || uint64(size) != file.UncompressedSize64 {
		return fmt.Errorf("invalid expanded size of ZIP entry %q", file.Name)
	}
	return nil
}
