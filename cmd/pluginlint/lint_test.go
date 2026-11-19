package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRun(t *testing.T) {
	cases := []struct {
		name   string
		edit   func(map[string]string)
		code   int
		stdout string
	}{
		{
			name:   "valid",
			code:   0,
			stdout: "",
		},
		{
			name: "empty icon",
			edit: func(files map[string]string) {
				files["manifest.json"] = `{"icon":"","display_name_key":"plugin.example.name"}`
			},
			code:   1,
			stdout: "icon.empty\tmanifest.json\n",
		},
		{
			name: "icon outside",
			edit: func(files map[string]string) {
				files["manifest.json"] = `{"icon":"subdir/logo.svg","display_name_key":"plugin.example.name"}`
			},
			code:   1,
			stdout: "icon.outside\tmanifest.json\tsubdir/logo.svg\n",
		},
		{
			name: "icon extension",
			edit: func(files map[string]string) {
				files["manifest.json"] = `{"icon":"logo.txt","display_name_key":"plugin.example.name"}`
				files["logo.txt"] = "not an image"
			},
			code:   1,
			stdout: "icon.extension\tmanifest.json\tlogo.txt\n",
		},
		{
			name: "icon missing",
			edit: func(files map[string]string) {
				delete(files, "logo.svg")
			},
			code:   1,
			stdout: "icon.missing\tmanifest.json\tlogo.svg\n",
		},
		{
			name: "key missing from zh",
			edit: func(files map[string]string) {
				files["schema/i18n/zh.json"] = `{
  "plugin.example.name": "示例",
  "plugin.example.mode": "模式",
  "plugin.example.token": "令牌"
}
`
			},
			code:   1,
			stdout: "i18n.missing\tschema/i18n/zh.json\tplugin.example.option\n",
		},
		{
			name: "whitespace translation",
			edit: func(files map[string]string) {
				files["schema/i18n/zh.json"] = `{
  "plugin.example.name": "示例",
  "plugin.example.mode": "模式",
  "plugin.example.option": "   ",
  "plugin.example.token": "令牌"
}
`
			},
			code:   1,
			stdout: "i18n.empty\tschema/i18n/zh.json\tplugin.example.option\n",
		},
		{
			name: "missing zh.json",
			edit: func(files map[string]string) {
				delete(files, "schema/i18n/zh.json")
			},
			code:   1,
			stdout: "locale.missing\tschema/i18n/zh.json\n",
		},
		{
			name: "missing config",
			edit: func(files map[string]string) {
				delete(files, "schema/config.json")
			},
			code:   1,
			stdout: "schema.missing\tschema/config.json\n",
		},
		{
			name: "absent access",
			edit: func(files map[string]string) {
				delete(files, "schema/access.json")
			},
			code:   0,
			stdout: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := validPlugin()
			if tc.edit != nil {
				tc.edit(files)
			}
			code, stdout, stderr := execLint(t, writePlugin(t, files))
			if code != tc.code || stdout != tc.stdout || stderr != "" {
				t.Fatalf("code %d stdout %q stderr %q", code, stdout, stderr)
			}
		})
	}
}

func TestRunPathIsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := execLint(t, path)
	if code != 2 || stdout != "" || stderr == "" {
		t.Fatalf("code %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func TestRepositoryNaming(t *testing.T) {
	for _, test := range []struct {
		name       string
		capability string
		repository string
		module     string
		finding    string
	}{
		{"deploy", "deploy", "plugin-provider-axisnow-cdn-deployer", "", ""},
		{"purge", "purge", "plugin-provider-volcengine-certcenter-purger", "", ""},
		{"DNS-01", "dns", "plugin-provider-volcengine-dns-acmedns01challenger", "", ""},
		{"notify", "notify", "plugin-provider-slackbot-notifier", "", ""},
		{"verb suffix", "deploy", "plugin-provider-axisnow-cdn-deploy", "", "repository.name"},
		{"missing service", "deploy", "plugin-provider--deployer", "", "repository.name"},
		{"incorrect role", "dns", "plugin-provider-volcengine-dns-deployer", "", "repository.name"},
		{"wrong module", "deploy", "plugin-provider-axisnow-cdn-deployer", "github.com/certimate-go/plugin-provider-axisnow", "repository.module"},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := validPlugin()
			files["manifest.json"] = `{"icon":"logo.svg","display_name_key":"plugin.example.name","capability":"` + test.capability + `"}`
			repository := "certimate-go/" + test.repository
			module := test.module
			if module == "" {
				module = "github.com/" + repository
			}
			files["go.mod"] = "module " + module + "\n"
			dir := writePlugin(t, files)
			status, stdout, stderr := execLint(t, "-repository", repository, dir)
			if test.finding == "" {
				if status != 0 || stdout != "" || stderr != "" {
					t.Fatalf("valid repository rejected: %d %q %q", status, stdout, stderr)
				}
			} else if status != 1 || !bytes.HasPrefix([]byte(stdout), []byte(test.finding+"\t")) || stderr != "" {
				t.Fatalf("expected %s: %d %q %q", test.finding, status, stdout, stderr)
			}
		})
	}
}

func execLint(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writePlugin(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func validPlugin() map[string]string {
	return map[string]string{
		"manifest.json": `{
  "icon": "logo.svg",
  "display_name_key": "plugin.example.name"
}
`,
		"logo.svg": "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>\n",
		"schema/config.json": `{
  "schema": {
    "columns": [
      {
        "labelKey": "plugin.example.mode",
        "options": [
          {"labelKey": "plugin.example.option"}
        ]
      }
    ]
  }
}
`,
		"schema/access.json": `{
  "columns": [
    {"labelKey": "plugin.example.token"}
  ]
}
`,
		"schema/i18n/zh.json": `{
  "plugin.example.name": "示例",
  "plugin.example.mode": "模式",
  "plugin.example.option": "选项",
  "plugin.example.token": "令牌"
}
`,
		"schema/i18n/en.json": `{
  "plugin.example.name": "Example",
  "plugin.example.mode": "Mode",
  "plugin.example.option": "Option",
  "plugin.example.token": "Token"
}
`,
	}
}
