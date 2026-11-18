package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCLIExportsLocalizedSummaries(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "genindex")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build genindex: %v\n%s", err, output)
	}
	root := t.TempDir()
	outputDir := t.TempDir()
	pluginDir := mkdir(t, root, "sample")
	manifest := `{"provider_type":"sample","display_name_key":"plugin.sample.name","version":"1.0.0","description":"Legacy description","release":{"repo":"certimate-go/plugins","tag":"sample/v1.0.0","assets":{"linux/amd64":"sample.zip"}}}`
	writeFile(t, filepath.Join(pluginDir, "manifest.json"), manifest)
	i18nDir := mkdir(t, pluginDir, "schema", "i18n")
	writeFile(t, filepath.Join(i18nDir, "zh.json"), `{"plugin.sample.name":"示例插件","plugin.sample.description":"发送通知。","plugin.sample.form.token.label":"接口令牌"}`)
	writeFile(t, filepath.Join(i18nDir, "en.json"), `{"plugin.sample.name":"Sample plugin","plugin.sample.description":"Send notifications.","plugin.sample.form.token.label":"API token"}`)
	legacyDir := mkdir(t, root, "legacy")
	writeFile(t, filepath.Join(legacyDir, "manifest.json"), `{"provider_type":"legacy","version":"1.0.0"}`)
	outputPath := filepath.Join(outputDir, "index.json")
	if output, err := exec.Command(binary, "-root", root, "-o", outputPath).CombinedOutput(); err != nil {
		t.Fatalf("generate catalog: %v\n%s", err, output)
	}
	body, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Plugins []struct {
			Provider string                       `json:"provider_type"`
			I18n     map[string]map[string]string `json:"i18n"`
			Detail   *detailReference             `json:"detail"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Plugins) != 2 || catalog.Plugins[0].Provider != "legacy" || catalog.Plugins[0].I18n != nil {
		t.Fatalf("legacy entry changed: %s", body)
	}
	sample := catalog.Plugins[1]
	if sample.I18n["zh"]["name"] != "示例插件" || sample.I18n["en"]["name"] != "Sample plugin" || sample.I18n["zh"]["description"] != "发送通知。" || sample.I18n["en"]["description"] != "Send notifications." {
		t.Fatalf("localized summary missing: %s", body)
	}
	if bytes.Contains(body, []byte("token")) || len(sample.I18n["zh"]) != 2 || len(sample.I18n["en"]) != 2 {
		t.Fatalf("form translations leaked into catalog: %s", body)
	}
	var original map[string]any
	if err := json.Unmarshal([]byte(manifest), &original); err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	canonical = append(canonical, '\n')
	if sample.Detail == nil || sample.Detail.SHA256 != fmt.Sprintf("%x", sha256.Sum256(canonical)) {
		t.Fatalf("localized summary changed published detail: %+v", sample.Detail)
	}
	if _, err := os.Stat(filepath.Join(root, "market")); !os.IsNotExist(err) {
		t.Fatalf("generated files written into input directory: %v", err)
	}
	writeFile(t, filepath.Join(i18nDir, "zh.json"), `{broken`)
	failedOutput := filepath.Join(outputDir, "failed.json")
	if output, err := exec.Command(binary, "-root", root, "-o", failedOutput).CombinedOutput(); err == nil || !bytes.Contains(output, []byte("zh.json")) {
		t.Fatalf("malformed translations must fail with filename: %v\n%s", err, output)
	}
	if _, err := os.Stat(failedOutput); !os.IsNotExist(err) {
		t.Fatalf("malformed translations wrote a catalog: %v", err)
	}
}
