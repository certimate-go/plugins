package registry

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
)

type ProviderReconciliation struct {
	Dispatch []string      `json:"dispatch"`
	Register []RegisterTag `json:"register"`
}

type RegisterTag struct {
	Tag           string `json:"tag"`
	ReleaseCommit string `json:"release_commit"`
}

func PlanProviderReconciliation(registry fs.FS, repository string, remoteRefs io.Reader) (ProviderReconciliation, error) {
	actions := ProviderReconciliation{Dispatch: []string{}, Register: []RegisterTag{}}
	admissions, withdrawals, err := loadPolicy(registry)
	if err != nil {
		return ProviderReconciliation{}, err
	}
	var provider string
	for candidate, admittedRepository := range admissions {
		if admittedRepository == repository {
			provider = candidate
			break
		}
	}
	if provider == "" {
		return ProviderReconciliation{}, fmt.Errorf("repository %q is not admitted", repository)
	}
	published, err := loadProvider(registry, provider, repository)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return ProviderReconciliation{}, err
	}
	formal, requests, err := readProviderTags(remoteRefs)
	if err != nil {
		return ProviderReconciliation{}, err
	}
	for _, record := range published {
		if formal[record.Tag] != record.ReleaseCommit {
			return ProviderReconciliation{}, fmt.Errorf("immutable formal tag %q no longer matches its registration", record.Tag)
		}
		delete(formal, record.Tag)
		delete(requests, record.Tag)
	}
	for tag, commit := range formal {
		if !withdrawals[Identity{provider, tag[1:]}] {
			actions.Register = append(actions.Register, RegisterTag{tag, commit})
		}
		delete(requests, tag)
	}
	for tag := range requests {
		if !withdrawals[Identity{provider, tag[1:]}] {
			actions.Dispatch = append(actions.Dispatch, tag+"-detach")
		}
	}
	slices.SortFunc(actions.Dispatch, func(left, right string) int {
		return compareVersions(strings.TrimSuffix(left[1:], "-detach"), strings.TrimSuffix(right[1:], "-detach"))
	})
	slices.SortFunc(actions.Register, func(left, right RegisterTag) int { return compareVersions(left.Tag[1:], right.Tag[1:]) })
	return actions, nil
}

func readProviderTags(input io.Reader) (formal map[string]string, requests map[string]bool, err error) {
	formal, requests = make(map[string]string), make(map[string]bool)
	peeled := make(map[string]string)
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		commit, ref, validLine := strings.Cut(scanner.Text(), "\t")
		tag, tagRef := strings.CutPrefix(ref, "refs/tags/")
		if !validLine || !tagRef || !commitPattern.MatchString(commit) {
			return nil, nil, errors.New("invalid Git tag listing")
		}
		tag, isPeeled := strings.CutSuffix(tag, "^{}")
		stableTag, isTrigger := strings.CutSuffix(tag, "-detach")
		if len(stableTag) < 2 || len(stableTag) > 65 || stableTag[0] != 'v' || !versionPattern.MatchString(stableTag[1:]) {
			continue
		}
		switch {
		case isTrigger:
			requests[stableTag] = true
		case isPeeled:
			if current := peeled[stableTag]; current != "" && current != commit {
				return nil, nil, fmt.Errorf("conflicting peeled Git objects for formal tag %s", stableTag)
			}
			peeled[stableTag] = commit
		default:
			if current := formal[stableTag]; current != "" && current != commit {
				return nil, nil, fmt.Errorf("conflicting Git objects for formal tag %s", stableTag)
			}
			formal[stableTag] = commit
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	for tag, commit := range peeled {
		if formal[tag] == "" {
			return nil, nil, fmt.Errorf("missing object for annotated formal tag %s", tag)
		}
		formal[tag] = commit
	}
	return formal, requests, nil
}
