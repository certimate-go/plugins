package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	purgeVolcAccessKey    = "fixture-volc-ak"
	purgeVolcSecretKey    = "fixture-volc-sk"
	purgeVolcProject      = "project-fixture"
	purgeVolcOtherProject = "project-other"
	purgeVolcHost         = "open.volcengineapi.com"
	purgeVolcAPIVersion   = "2024-10-01"
)

var purgeVolcPlugins = []string{"volcengine-certcenter-purger"}

var purgeVolcCST = time.FixedZone("CST", 8*60*60)

type purgeVolcCert struct {
	id       string
	project  string
	notAfter string
}

type purgeVolcFixture struct {
	mu sync.Mutex

	certs   map[string]*purgeVolcCert
	order   []string
	deletes []string
	lists   int

	vanishCodes map[string]string
	denyID      string
	listFail    bool
}

func newPurgeVolcFixture(t *testing.T) *purgeVolcFixture {
	t.Helper()
	f := &purgeVolcFixture{certs: map[string]*purgeVolcCert{}, vanishCodes: map[string]string{}}
	newB4CloudProxy(t, []string{purgeVolcHost}, http.HandlerFunc(f.serve))
	return f
}

func (f *purgeVolcFixture) seed(id string, daysToExpiry float64) {
	f.seedProject(id, purgeVolcProject, time.Now().In(purgeVolcCST).Add(time.Duration(-daysToExpiry*24*float64(time.Hour))).Format(time.DateTime))
}

func (f *purgeVolcFixture) seedProject(id, project, notAfter string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.certs[id]; !exists {
		f.order = append(f.order, id)
	}
	f.certs[id] = &purgeVolcCert{id: id, project: project, notAfter: notAfter}
}

func (f *purgeVolcFixture) remaining() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	items := make([]string, 0, len(f.certs))
	for _, id := range f.order {
		if _, exists := f.certs[id]; exists {
			items = append(items, id)
		}
	}
	return items
}

func (f *purgeVolcFixture) reply(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ResponseMetadata": map[string]string{"RequestId": "fixture"},
		"Result":           result,
	})
}

func (f *purgeVolcFixture) fail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ResponseMetadata": map[string]any{
			"RequestId": "fixture",
			"Error":     map[string]string{"Code": code, "Message": code},
		},
	})
}

func (f *purgeVolcFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if r.URL.Path != "/" || r.Method != http.MethodPost || r.URL.Query().Get("Version") != purgeVolcAPIVersion {
		f.fail(w, http.StatusBadRequest, "FixtureUnexpectedRequest")
		return
	}
	if !strings.Contains(r.Header.Get("Authorization"), "Credential="+purgeVolcAccessKey+"/") {
		f.fail(w, http.StatusBadRequest, "FixtureAccessDenied")
		return
	}

	var body struct {
		ProjectName string
		PageNumber  int
		PageSize    int
		InstanceId  string
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.fail(w, http.StatusBadRequest, "FixtureInvalidBody")
		return
	}

	switch r.URL.Query().Get("Action") {
	case "CertificateGetInstanceList":
		if f.listFail {
			f.fail(w, http.StatusBadRequest, "FixtureListDenied")
			return
		}
		f.lists++
		project := body.ProjectName
		if project == "" {
			project = "default"
		}
		ids := make([]string, 0, len(f.order))
		for _, id := range f.order {
			if cert, exists := f.certs[id]; exists && cert.project == project {
				ids = append(ids, id)
			}
		}
		size := body.PageSize
		if size < 1 {
			size = 100
		}
		start := min((body.PageNumber-1)*size, len(ids))
		if start < 0 {
			start = 0
		}
		end := min(start+size, len(ids))
		instances := make([]map[string]any, 0, end-start)
		for _, id := range ids[start:end] {
			instances = append(instances, map[string]any{"InstanceId": id, "NotAfter": f.certs[id].notAfter})
		}
		f.reply(w, map[string]any{"Instances": instances, "TotalCount": len(ids), "PageNumber": body.PageNumber, "PageSize": size})
	case "CertificateDeleteInstance":
		if code, vanish := f.vanishCodes[body.InstanceId]; vanish {
			f.fail(w, http.StatusNotFound, code)
			return
		}
		if body.InstanceId == f.denyID {
			f.fail(w, http.StatusBadRequest, "Delete.CertificateDenied")
			return
		}
		if _, exists := f.certs[body.InstanceId]; !exists {
			f.fail(w, http.StatusNotFound, "NotFound.CertificateInstance")
			return
		}
		delete(f.certs, body.InstanceId)
		f.deletes = append(f.deletes, body.InstanceId)
		f.reply(w, map[string]any{})
	default:
		f.fail(w, http.StatusBadRequest, "FixtureUnexpectedAction")
	}
}

func (f *purgeVolcFixture) reset() {
	f.certs = map[string]*purgeVolcCert{}
	f.order = nil
	f.deletes = nil
	f.lists = 0
	f.vanishCodes = map[string]string{}
	f.denyID = ""
	f.listFail = false
}

func TestPurgeWorkflowVolcengine(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runPurgeVolcInDocker(t)
		return
	}
	runPurgeVolcScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runPurgeVolcScenarios(t *testing.T, binaryDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fixture := newPurgeVolcFixture(t)
	core := startPurgeCore(t, binaryDir, purgeVolcPlugins)

	core.assertCatalogEntry("volcengine-certcenter-purger", "purge", true)
	if envelope, err := core.fetchSchemaEnvelope("volcengine-certcenter-purger", "purge"); err != nil {
		t.Fatalf("purge schema endpoint: %v", err)
	} else if envelope["provider"] != "volcengine-certcenter-purger" || envelope["category"] != "purge" {
		t.Fatalf("unexpected purge schema envelope: %v", envelope)
	}

	accessConfig := map[string]any{"accessKeyId": purgeVolcAccessKey, "secretAccessKey": purgeVolcSecretKey, "projectName": purgeVolcProject}

	request := func(name, provider string, providerConfig map[string]any) purgeWorkflowRequest {
		return purgeWorkflowRequest{
			name:           name,
			provider:       provider,
			accessProvider: "volcengine",
			accessConfig:   accessConfig,
			providerConfig: providerConfig,
			expiredDays:    7,
		}
	}
	run := func(req purgeWorkflowRequest) purgeWorkflowResult {
		return core.runPurgeWorkflow(ctx, req, purgeVolcSecretKey)
	}
	assertDeletes := func(t *testing.T, want ...string) {
		t.Helper()
		if strings.Join(fixture.deletes, ",") != strings.Join(want, ",") {
			t.Fatalf("deleted certificates %v, want %v", fixture.deletes, want)
		}
	}

	t.Run("success_legacy_and_new_ids", func(t *testing.T) {
		for _, provider := range []string{"volcengine-certcenter", "volcengine-certcenter-purger"} {
			fixture.reset()
			fixture.seed("ve-old-30d", 30)
			fixture.seed("ve-old-8d", 8)
			fixture.seed("ve-new-1d", -1)
			fixture.seed("ve-future", -30)

			result := run(request("Purge volcengine certcenter "+provider, provider, map[string]any{"region": "cn-beijing"}))
			if result.status != "succeeded" {
				t.Fatalf("provider %s workflow status %s: %s", provider, result.status, result.runError)
			}
			assertDeletes(t, "ve-old-30d", "ve-old-8d")
			if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
				t.Fatalf("provider %s purge log count=%d found=%t, want 2", provider, count, ok)
			}
			if !contains(fixture.remaining(), "ve-new-1d") || !contains(fixture.remaining(), "ve-future") {
				t.Fatalf("provider %s kept certificates %v", provider, fixture.remaining())
			}
		}
	})

	t.Run("region_default", func(t *testing.T) {
		fixture.reset()
		fixture.seed("ve-default-region", 20)

		result := run(request("Purge volcengine certcenter default region", "volcengine-certcenter-purger", map[string]any{}))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, "ve-default-region")
	})

	t.Run("multipage_list", func(t *testing.T) {
		fixture.reset()
		fixture.mu.Lock()
		for i := 1; i <= 101; i++ {
			id := fmt.Sprintf("ve-page-new-%d", i)
			if i == 1 {
				id = "ve-page-old"
			}
			days := -30.0
			if i == 1 {
				days = 30
			}
			notAfter := time.Now().In(purgeVolcCST).Add(time.Duration(-days * 24 * float64(time.Hour))).Format(time.DateTime)
			fixture.certs[id] = &purgeVolcCert{id: id, project: purgeVolcProject, notAfter: notAfter}
			fixture.order = append(fixture.order, id)
		}
		fixture.mu.Unlock()

		result := run(request("Purge volcengine certcenter multipage", "volcengine-certcenter-purger", map[string]any{"region": "cn-beijing"}))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.lists != 2 {
			t.Fatalf("list calls %d, want 2 (100 per page then remainder)", fixture.lists)
		}
		assertDeletes(t, "ve-page-old")
	})

	t.Run("unparseable_date_skipped", func(t *testing.T) {
		fixture.reset()
		fixture.seedProject("ve-bad-date", purgeVolcProject, "not-a-timestamp")
		fixture.seed("ve-old-30d", 30)

		result := run(request("Purge volcengine certcenter bad date", "volcengine-certcenter-purger", map[string]any{"region": "cn-beijing"}))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, "ve-old-30d")
		if !contains(fixture.remaining(), "ve-bad-date") {
			t.Fatal("certificate with unparseable date was purged")
		}
	})

	t.Run("other_project_records_preserved", func(t *testing.T) {
		fixture.reset()
		fixture.seed("ve-mine-old", 30)
		fixture.seedProject("ve-other-old", purgeVolcOtherProject, time.Now().In(purgeVolcCST).Add(-30*24*time.Hour).Format(time.DateTime))

		result := run(request("Purge volcengine certcenter project scope", "volcengine-certcenter-purger", map[string]any{"region": "cn-beijing"}))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, "ve-mine-old")
		if !contains(fixture.remaining(), "ve-other-old") {
			t.Fatal("certificate in another project was purged")
		}
	})

	t.Run("vanish_skips", func(t *testing.T) {
		fixture.reset()
		fixture.seed("ve-vanish-a", 15)
		fixture.seed("ve-vanish-b", 15)
		fixture.seed("ve-keep-target", 20)
		fixture.mu.Lock()
		fixture.vanishCodes["ve-vanish-a"] = "NotFound.CertificateInstance"
		fixture.vanishCodes["ve-vanish-b"] = "Delete.Certificate"
		fixture.mu.Unlock()

		result := run(request("Purge volcengine certcenter vanish", "volcengine-certcenter-purger", map[string]any{"region": "cn-beijing"}))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, "ve-keep-target")
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 1 {
			t.Fatalf("purge log count=%d found=%t, want 1", count, ok)
		}
		if !contains(fixture.remaining(), "ve-vanish-a") || !contains(fixture.remaining(), "ve-vanish-b") {
			t.Fatal("vanished certificates were removed despite the skip codes")
		}
	})

	t.Run("delete_failure_and_repair", func(t *testing.T) {
		fixture.reset()
		fixture.seed("ve-deny-a", 20)
		fixture.seed("ve-deny-b", 20)
		fixture.mu.Lock()
		fixture.denyID = "ve-deny-b"
		fixture.mu.Unlock()

		req := request("Purge volcengine certcenter delete failure", "volcengine-certcenter-purger", map[string]any{"region": "cn-beijing"})
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}
		assertDeletes(t, "ve-deny-a")

		fixture.mu.Lock()
		fixture.denyID = ""
		fixture.mu.Unlock()
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		assertDeletes(t, "ve-deny-a", "ve-deny-b")
	})

	t.Run("list_failure", func(t *testing.T) {
		fixture.reset()
		fixture.seed("ve-list-fail", 30)
		fixture.mu.Lock()
		fixture.listFail = true
		fixture.mu.Unlock()

		result := run(request("Purge volcengine certcenter list failure", "volcengine-certcenter-purger", map[string]any{"region": "cn-beijing"}))
		fixture.mu.Lock()
		fixture.listFail = false
		fixture.mu.Unlock()
		if result.status != "failed" {
			t.Fatalf("list failure status %s, want failed", result.status)
		}
		if len(fixture.deletes) != 0 {
			t.Fatalf("list failure still deleted %v", fixture.deletes)
		}
	})
}

func runPurgeVolcInDocker(t *testing.T) {
	t.Helper()
	runPurgeFamilyInDocker(t, purgeVolcPlugins, "purge-volcengine.test", "^TestPurgeWorkflowVolcengine$")
}
