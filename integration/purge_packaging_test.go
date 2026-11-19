package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	purgePackagingSourceRepo     = "fixture-purge/plugins"
	purgePackagingReleaseRepo    = "fixture-purge/releases"
	purgePackagingProtocolPlugin = "fixture-purge-protocol"
	purgePackagingUCloud         = "ucloud-ussl-purger"
)

var purgePackagingPurgers = []string{
	"aliyun-cas-purger",
	"aliyun-clb-purger",
	"aliyun-esa-purger",
	"aws-acm-purger",
	"aws-iam-purger",
	"azure-keyvault-purger",
	"baiducloud-cert-purger",
	"huaweicloud-elb-purger",
	"huaweicloud-scm-purger",
	"huaweicloud-waf-purger",
	"jdcloud-ssl-purger",
	"tencentcloud-gaap-purger",
	"tencentcloud-ssl-purger",
	"tencentcloud-tse-purger",
	"ucloud-ussl-purger",
	"volcengine-certcenter-purger",
}

var purgePackagingOldSamples = []string{"mockdns", "mocknotify", "baiducloud-cert"}

type purgePackagingMarket struct {
	t     *testing.T
	root  string
	dist  string
	tools string
}

type purgePackagingIndexEntry struct {
	ProviderType    string `json:"provider_type"`
	Capability      string `json:"capability"`
	Version         string `json:"version"`
	ProtocolVersion int    `json:"protocol_version"`
	Detail          *struct {
		SHA256    string   `json:"sha256"`
		Repo      string   `json:"repo"`
		Platforms []string `json:"platforms"`
	} `json:"detail"`
}

func newPurgePackagingMarket(t *testing.T, fixturesDir string) *purgePackagingMarket {
	t.Helper()
	m := &purgePackagingMarket{t: t, root: t.TempDir(), dist: t.TempDir(), tools: fixturesDir}
	for _, name := range append(append([]string{}, purgePackagingPurgers...), purgePackagingOldSamples...) {
		if err := os.CopyFS(filepath.Join(m.root, name), os.DirFS(filepath.Join(fixturesDir, name))); err != nil {
			t.Fatal(err)
		}
	}
	m.stageProtocolFixture(fixturesDir)
	for _, name := range m.staged() {
		for _, platform := range [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}} {
			target := filepath.Join(m.dist, fmt.Sprintf("%s_%s_%s.zip", name, platform[0], platform[1]))
			if platform[0] == "linux" && platform[1] == runtime.GOARCH {
				m.zipStagedDir(name, target)
				continue
			}
			placeholder := fmt.Sprintf("placeholder asset for %s/%s", platform[0], platform[1])
			if err := os.WriteFile(target, []byte(placeholder), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range m.staged() {
		m.runReleaseinfo(name)
	}
	m.runGenindex()
	return m
}

func (m *purgePackagingMarket) staged() []string {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		m.t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

func (m *purgePackagingMarket) stageProtocolFixture(fixturesDir string) {
	dir := filepath.Join(m.root, purgePackagingProtocolPlugin)
	if err := os.CopyFS(dir, os.DirFS(filepath.Join(fixturesDir, purgePackagingUCloud))); err != nil {
		m.t.Fatal(err)
	}
	m.patchManifest(purgePackagingProtocolPlugin, func(fields map[string]any) {
		fields["provider_type"] = purgePackagingProtocolPlugin
		fields["protocol_version"] = 3
	})
}

func (m *purgePackagingMarket) patchManifest(name string, mutate func(map[string]any)) {
	m.t.Helper()
	path := filepath.Join(m.root, name, "manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		m.t.Fatal(err)
	}
	fields := map[string]any{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		m.t.Fatal(err)
	}
	mutate(fields)
	body, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		m.t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		m.t.Fatal(err)
	}
}

func (m *purgePackagingMarket) zipStagedDir(name, target string) {
	m.t.Helper()
	dir := filepath.Join(m.root, name)
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		m.t.Fatal(err)
	}
	manifest := map[string]any{}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		m.t.Fatal(err)
	}
	binary, _ := manifest["binary"].(string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			m.t.Fatal(err)
		}
		header := &zip.FileHeader{Name: entry.Name(), Method: zip.Store}
		if entry.Name() == binary {
			header.SetMode(0o755)
		} else {
			header.SetMode(0o644)
		}
		output, err := writer.CreateHeader(header)
		if err != nil {
			m.t.Fatal(err)
		}
		if _, err := output.Write(data); err != nil {
			m.t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		m.t.Fatal(err)
	}
	if err := os.WriteFile(target, buffer.Bytes(), 0o644); err != nil {
		m.t.Fatal(err)
	}
}

func (m *purgePackagingMarket) runReleaseinfo(name string) {
	m.t.Helper()
	cmd := exec.Command(filepath.Join(m.tools, "releaseinfo"), "-plugin", name, "-repo", purgePackagingReleaseRepo, "-dist", m.dist, "-root", m.root)
	if output, err := cmd.CombinedOutput(); err != nil {
		m.t.Fatalf("releaseinfo %s: %v\n%s", name, err, output)
	}
}

func (m *purgePackagingMarket) runGenindex() {
	m.t.Helper()
	if output, err := exec.Command(filepath.Join(m.tools, "genindex"), "-root", m.root, "-o", filepath.Join(m.root, "index.json")).CombinedOutput(); err != nil {
		m.t.Fatalf("genindex: %v\n%s", err, output)
	}
}

func (m *purgePackagingMarket) readIndex() map[string]purgePackagingIndexEntry {
	m.t.Helper()
	raw, err := os.ReadFile(filepath.Join(m.root, "index.json"))
	if err != nil {
		m.t.Fatal(err)
	}
	var catalog struct {
		Plugins []purgePackagingIndexEntry `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		m.t.Fatal(err)
	}
	entries := make(map[string]purgePackagingIndexEntry, len(catalog.Plugins))
	for _, entry := range catalog.Plugins {
		entries[entry.ProviderType] = entry
	}
	return entries
}

func (m *purgePackagingMarket) publish(name, version string) {
	m.t.Helper()
	m.patchManifest(name, func(fields map[string]any) {
		fields["version"] = version
	})
	m.zipStagedDir(name, m.assetPath(name))
	m.runReleaseinfo(name)
	m.runGenindex()
}

func (m *purgePackagingMarket) assetPath(name string) string {
	return filepath.Join(m.dist, fmt.Sprintf("%s_linux_%s.zip", name, runtime.GOARCH))
}

func (m *purgePackagingMarket) handler() http.Handler {
	serve := func(w http.ResponseWriter, r *http.Request, path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/certimate-go/plugins/refs/heads/main/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plugins":[]}` + "\n"))
	})
	mux.HandleFunc("/"+purgePackagingSourceRepo+"/refs/heads/main/index.json", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, filepath.Join(m.root, "index.json"))
	})
	mux.HandleFunc("/"+purgePackagingSourceRepo+"/refs/heads/main/market/details/", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, filepath.Join(m.root, "market", "details", filepath.Base(r.URL.Path)))
	})
	mux.HandleFunc("/"+purgePackagingReleaseRepo+"/releases/download/", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, filepath.Join(m.dist, filepath.Base(r.URL.Path)))
	})
	return mux
}

type purgeMarketCard struct {
	ProviderType string `json:"provider_type"`
	Capability   string `json:"capability"`
	Version      string `json:"version"`
	CanInstall   bool   `json:"canInstall"`
	CanUpdate    bool   `json:"canUpdate"`
	CanDelete    bool   `json:"canDelete"`
	Source       struct {
		ID string `json:"id"`
	} `json:"source"`
}

type purgeMarketJob struct {
	State  string `json:"state"`
	Stage  string `json:"stage"`
	Error  string `json:"error"`
	Source struct {
		ID string `json:"id"`
	} `json:"source"`
}

type purgeMarketClient struct {
	t    *testing.T
	core *purgeCore
}

func (c purgeMarketClient) call(ctx context.Context, method, path string, body any, result any) (int, string) {
	c.t.Helper()
	var envelope struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := c.core.api.request(ctx, method, "/api/plugin/market"+path, body, &envelope); err != nil {
		c.t.Fatal(err)
	}
	if result != nil {
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			c.t.Fatalf("%s %s: %v", method, path, err)
		}
	}
	return envelope.Code, envelope.Msg
}

func (c purgeMarketClient) ok(ctx context.Context, method, path string, body any, result any) {
	c.t.Helper()
	if code, msg := c.call(ctx, method, path, body, result); code != 0 {
		c.t.Fatalf("%s %s: code=%d message=%s", method, path, code, msg)
	}
}

func (c purgeMarketClient) page(ctx context.Context, query string) []purgeMarketCard {
	c.t.Helper()
	var result struct {
		Items []purgeMarketCard `json:"items"`
	}
	c.ok(ctx, http.MethodGet, query, nil, &result)
	return result.Items
}

func (c purgeMarketClient) install(ctx context.Context, provider, source string) purgeMarketJob {
	c.t.Helper()
	var job purgeMarketJob
	c.ok(ctx, http.MethodPost, "/install", map[string]string{"providerType": provider, "source": source}, &job)
	return job
}

func (c purgeMarketClient) waitJob(ctx context.Context, provider, source, wantState string) purgeMarketJob {
	c.t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		var job purgeMarketJob
		c.ok(ctx, http.MethodGet, "/install/status/"+provider, nil, &job)
		if job.Source.ID != source {
			c.t.Fatalf("job source for %s: %+v", provider, job)
		}
		if job.State == wantState {
			return job
		}
		if job.State == "failed" {
			c.t.Fatalf("job for %s failed at stage %q: %s", provider, job.Stage, job.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}
	c.t.Fatalf("job for %s did not reach %s", provider, wantState)
	return purgeMarketJob{}
}

func (c purgeMarketClient) waitJobFailure(ctx context.Context, provider, source string) purgeMarketJob {
	c.t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		var job purgeMarketJob
		c.ok(ctx, http.MethodGet, "/install/status/"+provider, nil, &job)
		if job.Source.ID != source {
			c.t.Fatalf("job source for %s: %+v", provider, job)
		}
		if job.State == "failed" {
			return job
		}
		if job.State == "installed" {
			c.t.Fatalf("job for %s unexpectedly installed", provider)
		}
		time.Sleep(50 * time.Millisecond)
	}
	c.t.Fatalf("job for %s did not fail", provider)
	return purgeMarketJob{}
}

func (c purgeMarketClient) card(ctx context.Context, provider, source string) purgeMarketCard {
	c.t.Helper()
	for _, card := range c.page(ctx, "?providerType="+provider+"&source="+source+"&perPage=100") {
		if card.ProviderType == provider && card.Source.ID == source {
			return card
		}
	}
	c.t.Fatalf("missing market card for %s on source %s", provider, source)
	return purgeMarketCard{}
}

func purgePackagingInstalledVersion(t *testing.T, core *purgeCore, provider string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(core.pluginDir, provider, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	version, _ := manifest["version"].(string)
	return version
}

func TestPurgePackaging(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge packaging verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runPurgePackagingInDocker(t)
		return
	}
	runPurgePackagingScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runPurgePackagingScenarios(t *testing.T, fixturesDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	market := newPurgePackagingMarket(t, fixturesDir)

	t.Run("generated_index_carries_capability_and_details", func(t *testing.T) {
		entries := market.readIndex()
		if len(entries) != len(purgePackagingPurgers)+len(purgePackagingOldSamples)+1 {
			t.Fatalf("index has %d entries, want %d", len(entries), len(purgePackagingPurgers)+len(purgePackagingOldSamples)+1)
		}
		for _, provider := range purgePackagingPurgers {
			entry, ok := entries[provider]
			if !ok {
				t.Fatalf("index missing purge entry %s", provider)
			}
			if entry.Capability != "purge" || entry.ProtocolVersion != 4 || entry.Version != "0.1.0" {
				t.Fatalf("purge entry %s: %+v", provider, entry)
			}
			if entry.Detail == nil || entry.Detail.Repo != purgePackagingReleaseRepo || len(entry.Detail.Platforms) != 5 {
				t.Fatalf("purge entry %s detail: %+v", provider, entry.Detail)
			}
			detailPath := filepath.Join(market.root, "market", "details", entry.Detail.SHA256+".json")
			raw, err := os.ReadFile(detailPath)
			if err != nil {
				t.Fatalf("detail for %s: %v", provider, err)
			}
			if fmt.Sprintf("%x", sha256.Sum256(raw)) != entry.Detail.SHA256 {
				t.Fatalf("detail checksum mismatch for %s", provider)
			}
		}
		if capability := entries["mockdns"].Capability; capability != "dns" {
			t.Fatalf("mockdns capability %q", capability)
		}
		if capability := entries["mocknotify"].Capability; capability != "notify" {
			t.Fatalf("mocknotify capability %q", capability)
		}
		if capability := entries["baiducloud-cert"].Capability; capability != "deploy" {
			t.Fatalf("baiducloud-cert capability %q", capability)
		}
		if entry := entries[purgePackagingProtocolPlugin]; entry.Capability != "purge" || entry.ProtocolVersion != 3 {
			t.Fatalf("protocol fixture entry: %+v", entry)
		}
	})

	newB4CloudProxy(t, []string{"raw.githubusercontent.com", "github.com"}, market.handler())
	core := startPurgeCore(t, fixturesDir, []string{})
	client := purgeMarketClient{t: t, core: core}

	var sourceID string
	t.Run("market_page_reads_purge_entries", func(t *testing.T) {
		var status struct {
			ID string `json:"id"`
		}
		client.ok(ctx, http.MethodPost, "/sources", map[string]string{"repository": "https://github.com/" + purgePackagingSourceRepo}, &status)
		if status.ID == "" {
			t.Fatal("market source was not assigned an id")
		}
		sourceID = status.ID

		purgeCards := map[string]purgeMarketCard{}
		for _, card := range client.page(ctx, "?capability=purge&perPage=100") {
			if card.Source.ID != sourceID {
				t.Fatalf("purge card from unexpected source: %+v", card)
			}
			if card.Capability != "purge" {
				t.Fatalf("purge card capability: %+v", card)
			}
			purgeCards[card.ProviderType] = card
		}
		for _, provider := range append(append([]string{}, purgePackagingPurgers...), purgePackagingProtocolPlugin) {
			card, ok := purgeCards[provider]
			if !ok {
				t.Fatalf("market page missing purge card %s", provider)
			}
			if !card.CanInstall || card.CanUpdate || card.CanDelete {
				t.Fatalf("purge card %s actions: %+v", provider, card)
			}
		}
		if len(purgeCards) != len(purgePackagingPurgers)+1 {
			t.Fatalf("capability=purge returned %d cards, want %d", len(purgeCards), len(purgePackagingPurgers)+1)
		}

		deployProviders := map[string]bool{}
		for _, card := range client.page(ctx, "?capability=deploy&perPage=100") {
			deployProviders[card.ProviderType] = true
		}
		if !deployProviders["baiducloud-cert"] || deployProviders[purgePackagingUCloud] {
			t.Fatalf("capability=deploy selection wrong: %v", deployProviders)
		}
		dnsProviders := map[string]bool{}
		for _, card := range client.page(ctx, "?capability=dns&perPage=100") {
			dnsProviders[card.ProviderType] = true
		}
		if !dnsProviders["mockdns"] {
			t.Fatalf("capability=dns selection missing mockdns: %v", dnsProviders)
		}
		notifyProviders := map[string]bool{}
		for _, card := range client.page(ctx, "?capability=notify&perPage=100") {
			notifyProviders[card.ProviderType] = true
		}
		if !notifyProviders["mocknotify"] {
			t.Fatalf("capability=notify selection missing mocknotify: %v", notifyProviders)
		}
	})

	t.Run("install_all_sixteen_and_old_categories", func(t *testing.T) {
		for _, provider := range purgePackagingPurgers {
			client.install(ctx, provider, sourceID)
			client.waitJob(ctx, provider, sourceID, "installed")
			core.assertCatalogEntry(provider, "purge", true)
		}
		for provider, capability := range map[string]string{"mockdns": "dns", "mocknotify": "notify", "baiducloud-cert": "deploy"} {
			client.install(ctx, provider, sourceID)
			client.waitJob(ctx, provider, sourceID, "installed")
			core.assertCatalogEntry(provider, capability, true)
		}
		if version := purgePackagingInstalledVersion(t, core, purgePackagingUCloud); version != "0.1.0" {
			t.Fatalf("installed ucloud version %q, want 0.1.0", version)
		}
	})

	t.Run("protocol_mismatch_rejected_core_usable", func(t *testing.T) {
		client.install(ctx, purgePackagingProtocolPlugin, sourceID)
		job := client.waitJobFailure(ctx, purgePackagingProtocolPlugin, sourceID)
		if !strings.Contains(job.Error, "protocol") {
			t.Fatalf("protocol-mismatch job: %+v", job)
		}
		core.assertCatalogEntry(purgePackagingProtocolPlugin, "purge", false)
		core.assertCatalogEntry(purgePackagingUCloud, "purge", true)
		if card := client.card(ctx, purgePackagingUCloud, sourceID); !card.CanDelete || card.CanUpdate {
			t.Fatalf("card after protocol rejection: %+v", card)
		}
	})

	t.Run("missing_provider_install_reports_error", func(t *testing.T) {
		client.install(ctx, "purge-missing-provider", sourceID)
		job := client.waitJobFailure(ctx, "purge-missing-provider", sourceID)
		if !strings.Contains(job.Error, "purge-missing-provider") {
			t.Fatalf("missing provider job: %+v", job)
		}
	})

	t.Run("market_installed_plugin_runs_real_workflow", func(t *testing.T) {
		fixture := newPurgeUCloudFixture(t)
		fixture.seed(910, "trust", 30)
		fixture.seed(911, "trust", -6)
		fixture.seed(912, "purchase", 10)

		result := core.runPurgeWorkflow(ctx, purgeWorkflowRequest{
			name:           "Purge market installed ucloud",
			provider:       purgePackagingUCloud,
			accessProvider: "ucloud",
			accessConfig:   map[string]any{"privateKey": purgeUCloudPrivateKey, "publicKey": purgeUCloudPublicKey, "projectId": purgeUCloudProjectId},
			providerConfig: map[string]any{"endpoint": fixture.endpoint},
			expiredDays:    7,
		}, purgeUCloudPrivateKey)
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if strings.Join(fixture.deletes, ",") != "trust:910,purchase:912" {
			t.Fatalf("deleted certificates %v", fixture.deletes)
		}
	})

	t.Run("broken_update_fails_then_recovers", func(t *testing.T) {
		market.publish(purgePackagingUCloud, "0.2.0")
		if err := os.WriteFile(market.assetPath(purgePackagingUCloud), []byte("bytes that no longer match the published checksum"), 0o644); err != nil {
			t.Fatal(err)
		}
		client.ok(ctx, http.MethodPost, "/sources/"+sourceID+"/refresh", nil, nil)
		if card := client.card(ctx, purgePackagingUCloud, sourceID); !card.CanUpdate || card.Version != "0.2.0" {
			t.Fatalf("card before update: %+v", card)
		}
		code, msg := client.call(ctx, http.MethodPost, "/update/"+purgePackagingUCloud, map[string]string{"source": sourceID}, nil)
		if code == 0 || !strings.Contains(msg, "checksum") {
			t.Fatalf("checksum-mismatch update: code=%d message=%s", code, msg)
		}
		if version := purgePackagingInstalledVersion(t, core, purgePackagingUCloud); version != "0.1.0" {
			t.Fatalf("failed update changed installed version to %q", version)
		}
		core.assertCatalogEntry(purgePackagingUCloud, "purge", true)

		if err := os.WriteFile(market.assetPath(purgePackagingUCloud), []byte("checksummed bytes that are not a zip archive"), 0o644); err != nil {
			t.Fatal(err)
		}
		market.runReleaseinfo(purgePackagingUCloud)
		market.runGenindex()
		client.ok(ctx, http.MethodPost, "/sources/"+sourceID+"/refresh", nil, nil)
		code, msg = client.call(ctx, http.MethodPost, "/update/"+purgePackagingUCloud, map[string]string{"source": sourceID}, nil)
		if code == 0 {
			t.Fatalf("corrupt-archive update unexpectedly succeeded: %+v", msg)
		}
		if version := purgePackagingInstalledVersion(t, core, purgePackagingUCloud); version != "0.1.0" {
			t.Fatalf("corrupt update changed installed version to %q", version)
		}
		core.assertCatalogEntry(purgePackagingUCloud, "purge", true)

		market.publish(purgePackagingUCloud, "0.2.0")
		client.ok(ctx, http.MethodPost, "/sources/"+sourceID+"/refresh", nil, nil)
		client.ok(ctx, http.MethodPost, "/update/"+purgePackagingUCloud, map[string]string{"source": sourceID}, nil)
		if version := purgePackagingInstalledVersion(t, core, purgePackagingUCloud); version != "0.2.0" {
			t.Fatalf("recovered update version %q, want 0.2.0", version)
		}
		core.assertCatalogEntry(purgePackagingUCloud, "purge", true)
	})

	t.Run("uninstall_breaks_purge_and_reinstall_restores", func(t *testing.T) {
		fixture := newPurgeUCloudFixture(t)
		fixture.seed(920, "trust", 30)

		request := purgeWorkflowRequest{
			name:           "Purge market uninstall ucloud",
			provider:       purgePackagingUCloud,
			accessProvider: "ucloud",
			accessConfig:   map[string]any{"privateKey": purgeUCloudPrivateKey, "publicKey": purgeUCloudPublicKey, "projectId": purgeUCloudProjectId},
			providerConfig: map[string]any{"endpoint": fixture.endpoint},
			expiredDays:    7,
		}

		client.ok(ctx, http.MethodDelete, "/"+purgePackagingUCloud+"?source="+sourceID, nil, nil)
		core.assertCatalogEntry(purgePackagingUCloud, "purge", false)
		broken := core.runPurgeWorkflow(ctx, request, purgeUCloudPrivateKey)
		if broken.status != "failed" || !strings.Contains(broken.runError, purgePackagingUCloud) {
			t.Fatalf("uninstalled plugin run: status=%s error=%s", broken.status, broken.runError)
		}
		if len(fixture.deletes) != 0 {
			t.Fatalf("uninstalled plugin still deleted %v", fixture.deletes)
		}

		client.install(ctx, purgePackagingUCloud, sourceID)
		client.waitJob(ctx, purgePackagingUCloud, sourceID, "installed")
		core.assertCatalogEntry(purgePackagingUCloud, "purge", true)
		restored := core.runPurgeWorkflow(ctx, request, purgeUCloudPrivateKey)
		if restored.status != "succeeded" {
			t.Fatalf("restored run status %s: %s", restored.status, restored.runError)
		}
		if strings.Join(fixture.deletes, ",") != "trust:920" {
			t.Fatalf("restored run deleted %v", fixture.deletes)
		}
	})
}

func runPurgePackagingInDocker(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	run := func(env []string, program string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, program, args...)
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", program, args, err, out)
		}
		return string(out)
	}
	arch := strings.TrimSpace(run(nil, "docker", "info", "--format", "{{.Architecture}}"))
	switch arch {
	case "aarch64":
		arch = "arm64"
	case "x86_64":
		arch = "amd64"
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	env := []string{"GOOS=linux", "GOARCH=" + arch, "CGO_ENABLED=0"}
	buildB4CloudPackages(t, dir, append(append([]string{}, purgePackagingPurgers...), purgePackagingOldSamples...), "linux", arch)
	run(env, "go", "-C", root, "build", "-o", filepath.Join(dir, "genindex"), "./cmd/genindex")
	run(env, "go", "-C", root, "build", "-o", filepath.Join(dir, "releaseinfo"), "./cmd/releaseinfo")
	run(env, "go", "-C", root, "test", "-c", "-o", filepath.Join(dir, "purge-packaging.test"), "./integration")
	binary, err := filepath.Abs(os.Getenv("CERTIMATE_B1_CORE_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	output := run(nil, "docker", "run", "--rm", "--network=none",
		"--mount", "type=bind,src="+dir+",dst=/fixtures,readonly",
		"--mount", "type=bind,src="+binary+",dst=/core,readonly",
		"-e", "CERTIMATE_B1_CORE_BINARY=/core",
		"-e", "CERTIMATE_PURGE_BINARIES=/fixtures",
		image, "/fixtures/purge-packaging.test", "-test.run=^TestPurgePackaging$", "-test.v", "-test.timeout=10m")
	t.Logf("Linux purge packaging output:\n%s", output)
}
