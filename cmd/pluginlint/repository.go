package main

import (
	"io/fs"
	"regexp"
	"strconv"
	"strings"
)

var serviceSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var repositoryRoles = map[string]string{
	"deploy": "deployer", "purge": "purger", "notify": "notifier", "dns": "acmedns01challenger",
}

func checkRepository(fsys fs.FS, repository, capability string) (*finding, error) {
	role := repositoryRoles[capability]
	if role == "" {
		return &finding{code: "repository.capability", rel: relManifest, ref: capability}, nil
	}
	name, prefix := strings.CutPrefix(repository, "certimate-go/plugin-provider-")
	service, suffix := strings.CutSuffix(name, "-"+role)
	if !prefix || !suffix || !serviceSlug.MatchString(service) {
		return &finding{code: "repository.name", rel: relManifest, ref: "expected certimate-go/plugin-provider-<service>-" + role}, nil
	}
	const relModule = "go.mod"
	module, err := loadBody(fsys, relModule)
	if err != nil {
		return nil, err
	}
	want := "github.com/" + repository
	if modulePath(string(module.raw)) != want {
		return &finding{code: "repository.module", rel: relModule, ref: "expected module " + want}, nil
	}
	return nil, nil
}

func modulePath(source string) string {
	for line := range strings.SplitSeq(source, "\n") {
		entry, _, _ := strings.Cut(line, "//")
		fields := strings.Fields(entry)
		if len(fields) == 2 && fields[0] == "module" {
			if path, err := strconv.Unquote(fields[1]); err == nil {
				return path
			}
			return fields[1]
		}
	}
	return ""
}
