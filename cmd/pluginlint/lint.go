package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("pluginlint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repository := flags.String("repository", "", "official GitHub owner/repository, for checkouts with a different directory name")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "pluginlint: usage: pluginlint [-repository owner/repository] <plugin-dir>")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	rep, err := lint(flags.Arg(0), *repository)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 2
	}
	for _, line := range rep.lines() {
		fmt.Fprintln(stdout, line)
	}
	if !rep.empty() {
		return 1
	}
	return 0
}

func lint(dir, repository string) (report, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return report{}, fmt.Errorf("pluginlint: %w", err)
	}
	if !info.IsDir() {
		return report{}, fmt.Errorf("pluginlint: %s is not a directory", dir)
	}
	fsys := os.DirFS(dir)
	b, err := read(fsys)
	if err != nil {
		return report{}, err
	}
	rep := assess(b)
	if repository == "" {
		absolute, err := filepath.Abs(dir)
		if err != nil {
			return report{}, err
		}
		if name := filepath.Base(absolute); strings.HasPrefix(name, "plugin-provider-") {
			repository = "certimate-go/" + name
		}
	}
	if repository != "" {
		f, err := checkRepository(fsys, repository, b.capability)
		if err != nil {
			return report{}, err
		}
		if f != nil {
			rep.items = append(rep.items, *f)
		}
	}
	return rep, nil
}

type code string

const (
	codeIconEmpty     code = "icon.empty"
	codeIconOutside   code = "icon.outside"
	codeIconExtension code = "icon.extension"
	codeIconMissing   code = "icon.missing"
	codeNameEmpty     code = "name.empty"
	codeSchemaMissing code = "schema.missing"
	codeSchemaParse   code = "schema.parse"
	codeLocaleMissing code = "locale.missing"
	codeLocaleParse   code = "locale.parse"
	codeI18nMissing   code = "i18n.missing"
	codeI18nEmpty     code = "i18n.empty"
)

type localeIndex int

const (
	localeZH localeIndex = 0
	localeEN localeIndex = 1
)

const localeCount = 2

const (
	relManifest = "manifest.json"
	relConfig   = "schema/config.json"
	relAccess   = "schema/access.json"
)

var localeRel = [localeCount]string{
	localeZH: "schema/i18n/zh.json",
	localeEN: "schema/i18n/en.json",
}

type finding struct {
	code code
	rel  string
	ref  string
}

func (f finding) line() string {
	ref := strings.ReplaceAll(f.ref, "\n", " ")
	ref = strings.ReplaceAll(ref, "\t", " ")
	if ref == "" {
		return string(f.code) + "\t" + f.rel
	}
	return string(f.code) + "\t" + f.rel + "\t" + ref
}

type report struct {
	items []finding
}

func (r report) empty() bool { return len(r.items) == 0 }

func (r report) lines() []string {
	out := make([]string, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, item.line())
	}
	return out
}

type fileBody struct {
	rel    string
	absent bool
	raw    []byte
}

type bundle struct {
	iconName    string
	iconExists  bool
	displayName string
	capability  string
	config      fileBody
	access      fileBody
	locales     [localeCount]fileBody
}

func read(fsys fs.FS) (bundle, error) {
	raw, err := fs.ReadFile(fsys, relManifest)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return bundle{}, fmt.Errorf("pluginlint: %s: file does not exist", relManifest)
		}
		return bundle{}, fmt.Errorf("pluginlint: %s: %w", relManifest, err)
	}
	icon, display, capability, err := manifestFields(raw)
	if err != nil {
		return bundle{}, err
	}
	var b bundle
	b.iconName = strings.TrimSpace(icon)
	b.displayName = display
	b.capability = capability
	if singleFileName(b.iconName) {
		info, statErr := fs.Stat(fsys, b.iconName)
		if statErr != nil {
			if !errors.Is(statErr, fs.ErrNotExist) {
				return bundle{}, fmt.Errorf("pluginlint: %s: %w", b.iconName, statErr)
			}
		} else {
			b.iconExists = info.Mode().IsRegular()
		}
	}
	b.config, err = loadBody(fsys, relConfig)
	if err != nil {
		return bundle{}, err
	}
	b.access, err = loadBody(fsys, relAccess)
	if err != nil {
		return bundle{}, err
	}
	for i, rel := range localeRel {
		b.locales[i], err = loadBody(fsys, rel)
		if err != nil {
			return bundle{}, err
		}
	}
	return b, nil
}

func manifestFields(raw []byte) (string, string, string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return "", "", "", fmt.Errorf("pluginlint: %s: not a JSON object", relManifest)
	}
	icon, err := optionalString(fields, "icon")
	if err != nil {
		return "", "", "", err
	}
	display, err := optionalString(fields, "display_name_key")
	if err != nil {
		return "", "", "", err
	}
	capability, err := optionalString(fields, "capability")
	return icon, display, capability, err
}

func optionalString(fields map[string]json.RawMessage, key string) (string, error) {
	raw, ok := fields[key]
	if !ok {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("pluginlint: %s: %s is not a JSON string", relManifest, key)
	}
	return s, nil
}

func loadBody(fsys fs.FS, rel string) (fileBody, error) {
	raw, err := fs.ReadFile(fsys, rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fileBody{rel: rel, absent: true}, nil
		}
		return fileBody{}, fmt.Errorf("pluginlint: %s: %w", rel, err)
	}
	return fileBody{rel: rel, raw: raw}, nil
}

func assess(b bundle) report {
	var items []finding
	if f := iconFinding(b.iconName, b.iconExists); f != nil {
		items = append(items, *f)
	}
	if b.displayName == "" {
		items = append(items, finding{code: codeNameEmpty, rel: relManifest})
	}
	configKeys, configFindings := schemaKeys(b.config, true)
	items = append(items, configFindings...)
	accessKeys, accessFindings := schemaKeys(b.access, false)
	items = append(items, accessFindings...)

	var locales [localeCount]map[string]string
	for i, body := range b.locales {
		m, f := localeMap(body)
		if f != nil {
			items = append(items, *f)
		}
		locales[i] = m
	}

	var display []string
	if b.displayName != "" {
		display = []string{b.displayName}
	}
	for _, key := range sortedSet(configKeys, accessKeys, display) {
		for i, m := range locales {
			if m == nil {
				continue
			}
			val, ok := m[key]
			if !ok {
				items = append(items, finding{code: codeI18nMissing, rel: localeRel[i], ref: key})
				continue
			}
			if strings.TrimSpace(val) == "" {
				items = append(items, finding{code: codeI18nEmpty, rel: localeRel[i], ref: key})
			}
		}
	}
	return report{items: items}
}

func schemaKeys(body fileBody, required bool) ([]string, []finding) {
	if body.absent {
		if required {
			return nil, []finding{{code: codeSchemaMissing, rel: body.rel}}
		}
		return nil, nil
	}
	keys, err := referencedKeys(body.raw)
	if err != nil {
		return nil, []finding{{code: codeSchemaParse, rel: body.rel}}
	}
	return keys, nil
}

func localeMap(body fileBody) (map[string]string, *finding) {
	if body.absent {
		return nil, &finding{code: codeLocaleMissing, rel: body.rel}
	}
	var m map[string]string
	if err := json.Unmarshal(body.raw, &m); err != nil || m == nil {
		return nil, &finding{code: codeLocaleParse, rel: body.rel}
	}
	return m, nil
}

func sortedSet(groups ...[]string) []string {
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, key := range group {
			if key == "" {
				continue
			}
			seen[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func referencedKeys(raw []byte) ([]string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	set := make(map[string]struct{})
	collectKeyRefs(v, set)
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func collectKeyRefs(v any, set map[string]struct{}) {
	switch node := v.(type) {
	case map[string]any:
		for name, child := range node {
			if strings.HasSuffix(name, "Key") {
				if s, ok := child.(string); ok && s != "" {
					set[s] = struct{}{}
				}
			}
			collectKeyRefs(child, set)
		}
	case []any:
		for _, child := range node {
			collectKeyRefs(child, set)
		}
	}
}

func iconFinding(name string, exists bool) *finding {
	if name == "" {
		return &finding{code: codeIconEmpty, rel: relManifest}
	}
	if !singleFileName(name) {
		return &finding{code: codeIconOutside, rel: relManifest, ref: name}
	}
	if !iconExtOK(filepath.Ext(name)) {
		return &finding{code: codeIconExtension, rel: relManifest, ref: name}
	}
	if !exists {
		return &finding{code: codeIconMissing, rel: relManifest, ref: name}
	}
	return nil
}

func singleFileName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	if filepath.IsAbs(name) {
		return false
	}
	return filepath.Base(name) == name
}

func iconExtOK(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return true
	default:
		return false
	}
}
