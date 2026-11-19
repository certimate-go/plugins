package registry

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	controllerWorkflow = "certimate-go/plugins/.github/workflows/provider-release.yml"
	publishTagStep     = "Create the immutable formal tag"
	maxQueueChanges    = 32
	maxEvidenceBytes   = 1 << 20
)

type RegistrationOptions struct {
	Checkout   string
	Base       string
	Candidate  string
	Queue      bool
	PublicKeys map[string]ed25519.PublicKey
}

type Registration struct {
	Provider   string `json:"provider_type"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	BuildRunID string `json:"build_run_id"`
	path       string
	content    []byte
}

func RegistrationChanges(ctx context.Context, options RegistrationOptions) ([]Registration, error) {
	if !commitPattern.MatchString(options.Base) || !commitPattern.MatchString(options.Candidate) {
		return nil, errors.New("registry base and candidate must be full Git commit SHAs")
	}
	git := releaseGit{ctx, options.Checkout}
	if !git.ancestor(options.Base, options.Candidate) {
		return nil, errors.New("registration candidate does not contain the protected base")
	}
	policy, err := git.blob(options.Base, "registry/_policy/admissions.json", maxPolicyBytes)
	if err != nil {
		return nil, err
	}
	var admissions map[string]string
	if err := decodeJSON(policy, &admissions); err != nil || admissions == nil {
		return nil, errors.New("invalid admission policy at protected base")
	}
	if err := validateAdmissions(admissions); err != nil {
		return nil, err
	}
	diff, err := git.output("diff-tree", "--no-commit-id", "-r", "-z", "--no-renames", "--name-status", options.Base, options.Candidate)
	if err != nil {
		return nil, err
	}
	fields := bytes.Split(bytes.TrimSuffix(diff, []byte{0}), []byte{0})
	limit := 1
	if options.Queue {
		limit = maxQueueChanges
	}
	if len(fields)%2 != 0 {
		return nil, errors.New("automatic registration must only add immutable version records")
	}
	changes := make([]Registration, 0, limit)
	marketChanges := make(map[string]string)
	for index := 0; index < len(fields); index += 2 {
		filename := string(fields[index+1])
		status := string(fields[index])
		if filename == marketIndexPath || strings.HasPrefix(filename, "registry/market/") {
			marketChanges[filename] = status
			continue
		}
		if len(changes) == limit {
			return nil, errors.New("too many version records in the registration candidate")
		}
		parts := strings.Split(filename, "/")
		if status != "A" || len(parts) != 3 || parts[0] != "registry" {
			return nil, fmt.Errorf("unexpected registration change %q", filename)
		}
		provider := parts[1]
		tag, hasSuffix := strings.CutSuffix(parts[2], ".json")
		repository := admissions[provider]
		if !hasSuffix || repository == "" {
			return nil, fmt.Errorf("registration is not an admitted version path: %q", filename)
		}
		if err := validateReleaseTag(tag); err != nil {
			return nil, err
		}
		content, err := git.blob(options.Candidate, filename, maxRecordBytes)
		if err != nil {
			return nil, err
		}
		var version Version
		if err := decodeJSON(content, &version); err != nil {
			return nil, fmt.Errorf("%s: %w", filename, err)
		}
		if version.ProviderType != provider || version.Repository != repository || version.Tag != tag {
			return nil, fmt.Errorf("registration %q differs from the protected admission policy", filename)
		}
		if err := version.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", filename, err)
		}
		changes = append(changes, Registration{provider, repository, tag, version.BuildRunID, filename, content})
	}
	if len(marketChanges) != 0 {
		if !options.Queue && len(changes) != 0 {
			return nil, errors.New("publication and version registration require separate pull requests")
		}
		if err := verifyMarketChanges(git, options, marketChanges); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

func verifyMarketChanges(git releaseGit, options RegistrationOptions, changes map[string]string) error {
	indexChange := changes[marketIndexPath]
	if indexChange != "A" && indexChange != "M" {
		return errors.New("market changes require a new signed index")
	}
	_, payload, _, err := readPublishedIndex(git, options.Candidate, options.PublicKeys, time.Now())
	if err != nil {
		return fmt.Errorf("verify candidate market index: %w", err)
	}
	if payload.SourceCommit != options.Base {
		return errors.New("market index must be signed from the current protected main")
	}
	if indexChange == "M" {
		previousBytes, err := git.blob(options.Base, marketIndexPath, MaxIndexEnvelope)
		if err != nil {
			return err
		}
		previous, _, err := VerifyIndex(previousBytes, options.PublicKeys)
		if err != nil {
			return fmt.Errorf("verify previous market index: %w", err)
		}
		if payload.Epoch < previous.Epoch || payload.Epoch == previous.Epoch && payload.Generation <= previous.Generation {
			return errors.New("market index must advance the epoch or generation")
		}
	}
	assets, err := signedAssetDigests(payload)
	if err != nil {
		return err
	}
	for filename, status := range changes {
		if filename != marketIndexPath && (status != "A" || assets[filename] == "") {
			return fmt.Errorf("unexpected change to market asset %q", filename)
		}
	}
	for filename, digest := range assets {
		body, err := git.blob(options.Candidate, filename, maxRecordBytes)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(body)) != digest {
			return fmt.Errorf("market asset %q does not match its signed digest", filename)
		}
	}
	return nil
}

func VerifyRegistrations(ctx context.Context, options RegistrationOptions, sources string, evidence fs.FS) ([]Registration, error) {
	registrations, err := RegistrationChanges(ctx, options)
	if err != nil {
		return nil, err
	}
	registryGit := releaseGit{ctx, options.Checkout}
	for _, registration := range registrations {
		checkout := filepath.Join(sources, registration.Provider)
		version, err := InspectRelease(ctx, checkout, registration.Repository, registration.Tag)
		if err != nil {
			return nil, err
		}
		canonical, err := json.MarshalIndent(version, "", "  ")
		if err != nil {
			return nil, err
		}
		canonical = append(canonical, '\n')
		if !bytes.Equal(registration.content, canonical) {
			return nil, fmt.Errorf("%s does not match the canonical verified release", registration.path)
		}
		if err := verifyBuildProvenance(registryGit, releaseGit{ctx, checkout}, options.Base, evidence, version); err != nil {
			return nil, fmt.Errorf("%s: %w", registration.path, err)
		}
	}
	return registrations, nil
}

type runEvidence struct {
	ID         uint64 `json:"id"`
	Title      string `json:"display_title"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Branch     string `json:"head_branch"`
	Commit     string `json:"head_sha"`
	Path       string `json:"path"`
	Repository struct {
		Name string `json:"full_name"`
	} `json:"repository"`
	HeadRepository struct {
		Name string `json:"full_name"`
	} `json:"head_repository"`
	Referenced []struct {
		Path   string `json:"path"`
		Ref    string `json:"ref"`
		Commit string `json:"sha"`
	} `json:"referenced_workflows"`
}

func verifyBuildProvenance(registryGit, providerGit releaseGit, base string, evidence fs.FS, version Version) error {
	var branch struct {
		Name      string `json:"name"`
		Protected bool   `json:"protected"`
		Commit    struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := readEvidence(evidence, version.ProviderType+"/main.json", &branch); err != nil {
		return err
	}
	fetched, err := providerGit.output("rev-parse", "--verify", "refs/remotes/origin/main^{commit}")
	if err != nil {
		return err
	}
	if branch.Name != "main" || !branch.Protected || !commitPattern.MatchString(branch.Commit.SHA) ||
		branch.Commit.SHA != strings.TrimSpace(string(fetched)) {
		return errors.New("provider branch protection or branch evidence does not match the fetched main")
	}
	var run runEvidence
	if err := readEvidence(evidence, version.ProviderType+"/"+version.Tag+".json", &run); err != nil {
		return err
	}
	runPath, runRef, _ := strings.Cut(run.Path, "@")
	runPath = strings.TrimPrefix(runPath, version.Repository+"/")
	if strconv.FormatUint(run.ID, 10) != version.BuildRunID || run.Title != "Release "+version.Tag+"-detach" ||
		run.Event != "workflow_dispatch" || run.Status != "completed" ||
		run.Conclusion != "success" || run.Branch != "main" || run.Repository.Name != version.Repository || run.HeadRepository.Name != version.Repository ||
		runPath != ".github/workflows/release.yml" || (runRef != "" && !isMainRef(runRef)) ||
		!commitPattern.MatchString(run.Commit) || !providerGit.ancestor(run.Commit, branch.Commit.SHA) ||
		!providerGit.ancestor(version.SourceCommit, run.Commit) {
		return errors.New("formal tag does not have a successful protected provider workflow for its source")
	}
	for _, workflow := range run.Referenced {
		workflowPath, workflowRef, _ := strings.Cut(workflow.Path, "@")
		if workflow.Ref != "" && !isMainRef(workflow.Ref) || workflowRef != "" && !isMainRef(workflowRef) ||
			workflow.Ref == "" && workflowRef == "" {
			continue
		}
		if workflowPath == controllerWorkflow && commitPattern.MatchString(workflow.Commit) &&
			registryGit.ancestor(workflow.Commit, base) {
			return verifyTagPublication(evidence, version.ProviderType+"/"+version.Tag+"-jobs.json", run)
		}
	}
	return errors.New("provider run did not use a trusted registry controller revision")
}

func verifyTagPublication(evidence fs.FS, path string, run runEvidence) error {
	var jobs struct {
		Jobs []struct {
			Name   string `json:"name"`
			RunID  uint64 `json:"run_id"`
			Commit string `json:"head_sha"`
			Status string `json:"status"`
			Steps  []struct {
				Name       string `json:"name"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			} `json:"steps"`
		} `json:"jobs"`
	}
	if err := readEvidence(evidence, path, &jobs); err != nil {
		return err
	}
	for _, job := range jobs.Jobs {
		if job.RunID != run.ID || job.Commit != run.Commit || job.Status != "completed" ||
			(job.Name != "publish" && !strings.HasSuffix(job.Name, " / publish")) {
			continue
		}
		for _, step := range job.Steps {
			if step.Name == publishTagStep && step.Status == "completed" && step.Conclusion == "success" {
				return nil
			}
		}
	}
	return errors.New("formal tag does not have a successful publish step in its recorded workflow run")
}

func isMainRef(ref string) bool { return ref == "main" || ref == "refs/heads/main" }

func (git releaseGit) ancestor(ancestor, descendant string) bool {
	_, err := git.output("merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}

func readEvidence(evidence fs.FS, path string, value any) error {
	file, err := evidence.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxEvidenceBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxEvidenceBytes {
		return fmt.Errorf("GitHub evidence %q exceeds size limit", path)
	}
	if err := json.Unmarshal(body, value); err != nil {
		return fmt.Errorf("invalid GitHub evidence %q: %w", path, err)
	}
	return nil
}
