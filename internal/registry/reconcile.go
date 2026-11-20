package registry

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/certimate-go/plugins/internal/providerrelease"
)

type ProviderReconciliation struct {
	Register []RegisterTag `json:"register"`
}

type RegisterTag struct {
	Tag           string `json:"tag"`
	ReleaseCommit string `json:"release_commit"`
}

func PlanProviderReconciliation(registry fs.FS, repository string, remoteRefs io.Reader) (ProviderReconciliation, error) {
	actions := ProviderReconciliation{Register: []RegisterTag{}}
	if err := providerrelease.ValidateRepository(repository); err != nil {
		return ProviderReconciliation{}, err
	}
	providers, withdrawals, err := loadRegistry(registry)
	if err != nil {
		return ProviderReconciliation{}, err
	}
	var provider string
	var published []*Version
	for name, versions := range providers {
		if versions[0].Repository == repository {
			provider, published = name, versions
			break
		}
	}
	formal, err := readProviderTags(remoteRefs)
	if err != nil {
		return ProviderReconciliation{}, err
	}
	for _, record := range published {
		if formal[record.Tag] != record.ReleaseCommit {
			return ProviderReconciliation{}, fmt.Errorf("immutable formal tag %q no longer matches its registration", record.Tag)
		}
		delete(formal, record.Tag)
	}
	for tag, commit := range formal {
		if !withdrawals[Identity{provider, tag[1:]}] {
			actions.Register = append(actions.Register, RegisterTag{tag, commit})
		}
	}
	slices.SortFunc(actions.Register, func(left, right RegisterTag) int { return compareVersions(left.Tag[1:], right.Tag[1:]) })
	return actions, nil
}

func readProviderTags(input io.Reader) (map[string]string, error) {
	formal := make(map[string]string)
	peeled := make(map[string]string)
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		commit, ref, validLine := strings.Cut(scanner.Text(), "\t")
		tag, tagRef := strings.CutPrefix(ref, "refs/tags/")
		if !validLine || !tagRef || !commitPattern.MatchString(commit) {
			return nil, errors.New("invalid Git tag listing")
		}
		tag, isPeeled := strings.CutSuffix(tag, "^{}")
		if len(tag) < 2 || len(tag) > 65 || tag[0] != 'v' || !versionPattern.MatchString(tag[1:]) {
			continue
		}
		objects := formal
		if isPeeled {
			objects = peeled
		}
		if current := objects[tag]; current != "" && current != commit {
			return nil, fmt.Errorf("conflicting Git objects for formal tag %s", tag)
		}
		objects[tag] = commit
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for tag, commit := range peeled {
		if formal[tag] == "" {
			return nil, fmt.Errorf("missing object for annotated formal tag %s", tag)
		}
		formal[tag] = commit
	}
	return formal, nil
}
