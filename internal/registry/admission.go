package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"

	"github.com/certimate-go/certimate/pkg/plugin"
)

const (
	admissionPolicyPath = "registry/_policy/admissions.json"
	admissionBatchName  = "admission-batches"
	admissionBatchDir   = "registry/_policy/" + admissionBatchName + "/"
	admissionBatchLimit = 24
)

var admissionReceiptPath = regexp.MustCompile(`^registry/_policy/admission-batches/([1-9][0-9]{0,19}-[1-9][0-9]{0,8})\.json$`)

type MigrationEntry struct {
	Provider     string `json:"provider_type"`
	Repository   string `json:"repository"`
	SourceCommit string `json:"source_commit"`
	SourcePath   string `json:"source_path"`
	TargetCommit string `json:"target_commit"`
	TargetTree   string `json:"target_tree"`
	Version      string `json:"version"`
}

type AdmissionPlan struct {
	Admissions map[string]string `json:"admissions"`
	Entries    []MigrationEntry  `json:"entries"`
}

func PlanAdmissions(ctx context.Context, checkout, base string, body []byte) (AdmissionPlan, error) {
	if !commitPattern.MatchString(base) {
		return AdmissionPlan{}, errors.New("expected the protected registry base commit")
	}
	git := releaseGit{ctx, checkout}
	admissions, err := git.admissions(base)
	if err != nil {
		return AdmissionPlan{}, err
	}
	entries, err := git.migrationEntries(base, body)
	if err != nil {
		return AdmissionPlan{}, err
	}
	pending := make([]MigrationEntry, 0, len(entries))
	for _, entry := range entries {
		if current, exists := admissions[entry.Provider]; exists {
			if current != entry.Repository {
				return AdmissionPlan{}, fmt.Errorf("provider %q is already admitted to %q", entry.Provider, current)
			}
			continue
		}
		admissions[entry.Provider] = entry.Repository
		pending = append(pending, entry)
	}
	if err := validateAdmissions(admissions); err != nil {
		return AdmissionPlan{}, err
	}
	return AdmissionPlan{Admissions: admissions, Entries: pending}, nil
}

func (git releaseGit) admissions(commit string) (map[string]string, error) {
	body, err := git.blob(commit, admissionPolicyPath, maxPolicyBytes)
	if err != nil {
		return nil, err
	}
	var admissions map[string]string
	if err := decodeJSON(body, &admissions); err != nil {
		return nil, fmt.Errorf("decode admission policy at %s: %w", commit, err)
	}
	return admissions, validateAdmissions(admissions)
}

func (git releaseGit) migrationEntries(base string, body []byte) ([]MigrationEntry, error) {
	var entries []MigrationEntry
	if len(body) > maxPolicyBytes || decodeJSON(body, &entries) != nil || len(entries) == 0 || len(entries) > admissionBatchLimit {
		return nil, fmt.Errorf("migration batch must contain 1 to %d valid entries", admissionBatchLimit)
	}
	admissions := make(map[string]string, len(entries))
	for _, entry := range entries {
		if _, ok := admissions[entry.Provider]; ok {
			return nil, fmt.Errorf("migration batch repeats provider %q", entry.Provider)
		}
		admissions[entry.Provider] = entry.Repository
		if err := plugin.ValidateProviderType(entry.SourcePath); err != nil {
			return nil, fmt.Errorf("invalid legacy source path: %w", err)
		}
		if !commitPattern.MatchString(entry.SourceCommit) || !git.ancestor(entry.SourceCommit, base) ||
			!commitPattern.MatchString(entry.TargetCommit) || !commitPattern.MatchString(entry.TargetTree) {
			return nil, fmt.Errorf("untrusted source or invalid target Git objects for %q", entry.Provider)
		}
		if err := validateReleaseTag("v" + entry.Version); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Provider, err)
		}
		source, err := git.blob(entry.SourceCommit, entry.SourcePath+"/manifest.json", maxManifestBytes)
		if err != nil {
			return nil, err
		}
		var identity struct {
			Provider string `json:"provider_type"`
			Version  string `json:"version"`
		}
		if err := json.Unmarshal(source, &identity); err != nil || identity.Provider != entry.Provider ||
			validateReleaseTag("v"+identity.Version) != nil || compareVersions(entry.Version, identity.Version) < 0 {
			return nil, fmt.Errorf("legacy provider or version does not match migration entry for %q", entry.Provider)
		}
	}
	if err := validateAdmissions(admissions); err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(left, right MigrationEntry) int {
		return bytes.Compare([]byte(left.Provider), []byte(right.Provider))
	})
	return entries, nil
}

func (git releaseGit) migrationSources(base string) (map[Identity]string, error) {
	paths, err := git.output("ls-tree", "-r", "-z", "--name-only", base, "--", admissionBatchDir)
	if err != nil {
		return nil, err
	}
	sources := make(map[Identity]string)
	for _, name := range bytes.Split(paths, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		filename := string(name)
		if !admissionReceiptPath.MatchString(filename) {
			return nil, fmt.Errorf("invalid protected admission receipt path %q", filename)
		}
		data, err := git.blob(base, filename, maxPolicyBytes)
		if err != nil {
			return nil, err
		}
		var entries []MigrationEntry
		if err := decodeJSON(data, &entries); err != nil {
			return nil, fmt.Errorf("read protected batch %q: %w", filename, err)
		}
		for _, entry := range entries {
			sources[Identity{entry.Provider, entry.Version}] = entry.TargetCommit
		}
	}
	return sources, nil
}

func (git releaseGit) verifyAdmissionChanges(options RegistrationOptions, admissions map[string]string, changes map[string]string, evidence fs.FS) error {
	if len(changes) == 0 {
		return nil
	}
	if changes[admissionPolicyPath] != "M" || len(changes) < 2 || len(changes) > maxQueueChanges+1 {
		return errors.New("admission changes require a modified policy and new batch receipts")
	}
	if !options.Queue && len(changes) != 2 {
		return errors.New("an admission pull request must correspond to one batch")
	}
	expected := maps.Clone(admissions)
	for filename, status := range changes {
		if filename == admissionPolicyPath {
			continue
		}
		match := admissionReceiptPath.FindStringSubmatch(filename)
		if match == nil || status != "A" {
			return fmt.Errorf("unexpected admission change %q", filename)
		}
		body, err := git.blob(options.Candidate, filename, maxPolicyBytes)
		if err != nil {
			return err
		}
		if evidence == nil {
			return errors.New("missing trusted migration workflow evidence")
		}
		trusted, err := fs.ReadFile(evidence, "admission-batches/"+match[1]+".json")
		if err != nil || !bytes.Equal(body, trusted) {
			return fmt.Errorf("batch %s differs from the trusted migration workflow artifact", match[1])
		}
		entries, err := git.migrationEntries(options.Base, body)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if _, exists := expected[entry.Provider]; exists {
				return fmt.Errorf("admission batch would replace or repeat provider %q", entry.Provider)
			}
			expected[entry.Provider] = entry.Repository
		}
	}
	actual, err := git.admissions(options.Candidate)
	if err != nil {
		return err
	}
	if !maps.Equal(expected, actual) {
		return errors.New("admission policy differs from the authorized additions")
	}
	return nil
}
