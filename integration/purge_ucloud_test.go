package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"
)

const (
	purgeUCloudPublicKey    = "fixture-ucloud-public"
	purgeUCloudPrivateKey   = "fixture-ucloud-private"
	purgeUCloudProjectId    = "project-fixture"
	purgeUCloudOtherProject = "project-other"
)

var purgeUCloudPlugins = []string{"ucloud-ussl-purger"}

type purgeUCloudCert struct {
	id       int
	mode     string
	project  string
	notAfter int64
}

type purgeUCloudFixture struct {
	mu sync.Mutex

	endpoint string
	certs    map[int]*purgeUCloudCert
	order    []int
	deletes  []string
	lists    map[string]int

	vanishID     int
	denyID       int
	listFailCode int
}

func newPurgeUCloudFixture(t *testing.T) *purgeUCloudFixture {
	t.Helper()
	f := &purgeUCloudFixture{certs: map[int]*purgeUCloudCert{}, lists: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	f.endpoint = server.URL
	return f
}

func (f *purgeUCloudFixture) seed(id int, mode string, daysToExpiry float64) {
	f.seedProject(id, mode, purgeUCloudProjectId, time.Now().Add(time.Duration(-daysToExpiry*24*float64(time.Hour))).UnixMilli())
}

func (f *purgeUCloudFixture) seedProject(id int, mode, project string, notAfter int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.certs[id]; !exists {
		f.order = append(f.order, id)
	}
	f.certs[id] = &purgeUCloudCert{id: id, mode: mode, project: project, notAfter: notAfter}
}

func (f *purgeUCloudFixture) remaining() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	items := make([]int, 0, len(f.certs))
	for _, id := range f.order {
		if _, exists := f.certs[id]; exists {
			items = append(items, id)
		}
	}
	return items
}

func (f *purgeUCloudFixture) writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (f *purgeUCloudFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payload := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.writeJSON(w, map[string]any{"RetCode": 400, "Message": "invalid json body"})
		return
	}
	value := func(key string) string {
		if v, ok := payload[key]; ok {
			return fmt.Sprint(v)
		}
		return ""
	}
	action := r.URL.Query().Get("Action")
	if action == "" {
		action = value("Action")
	}
	reply := func(result map[string]any) {
		if _, ok := result["RetCode"]; !ok {
			result["RetCode"] = 0
		}
		f.writeJSON(w, result)
	}
	fail := func(code int, message string) {
		reply(map[string]any{"RetCode": code, "Message": message})
	}

	signature := value("Signature")
	delete(payload, "Signature")
	if value("PublicKey") != purgeUCloudPublicKey || auth.CalculateSignature(payload, purgeUCloudPrivateKey).Sign != signature {
		fail(161, "invalid fixture authentication")
		return
	}
	if value("ProjectId") != purgeUCloudProjectId {
		fail(230, "unexpected project "+value("ProjectId"))
		return
	}

	switch action {
	case "GetCertificateList":
		if f.listFailCode != 0 {
			fail(f.listFailCode, "fixture list failure")
			return
		}
		mode := value("Mode")
		f.lists[mode]++
		page := 1
		if _, err := fmt.Sscanf(value("Page"), "%d", &page); err != nil || page < 1 {
			fail(400, "missing pagination")
			return
		}
		size := 1000
		if _, err := fmt.Sscanf(value("PageSize"), "%d", &size); err != nil || size < 1 {
			fail(400, "missing pagination")
			return
		}
		ids := make([]int, 0, len(f.order))
		for _, id := range f.order {
			cert, exists := f.certs[id]
			if exists && cert.mode == mode && cert.project == purgeUCloudProjectId {
				ids = append(ids, id)
			}
		}
		start := min((page-1)*size, len(ids))
		end := min(start+size, len(ids))
		list := make([]map[string]any, 0, end-start)
		for _, id := range ids[start:end] {
			list = append(list, map[string]any{
				"CertificateID": f.certs[id].id,
				"Mode":          f.certs[id].mode,
				"NotAfter":      f.certs[id].notAfter,
			})
		}
		reply(map[string]any{"TotalCount": len(ids), "CertificateList": list})
	case "DeleteSSLCertificate":
		id := 0
		if _, err := fmt.Sscanf(value("CertificateID"), "%d", &id); err != nil {
			fail(400, "missing certificate id")
			return
		}
		if id == f.vanishID {
			fail(80011, "certificate not found")
			return
		}
		if id == f.denyID {
			fail(230, "fixture delete denied")
			return
		}
		cert, exists := f.certs[id]
		if !exists {
			fail(80011, "unknown certificate")
			return
		}
		if mode := value("CertificateMode"); mode != cert.mode {
			fail(400, "unexpected certificate mode "+mode)
			return
		}
		delete(f.certs, id)
		f.deletes = append(f.deletes, fmt.Sprintf("%s:%d", cert.mode, id))
		reply(map[string]any{})
	default:
		fail(400, "unexpected fixture action "+action)
	}
}

func (f *purgeUCloudFixture) reset() {
	f.certs = map[int]*purgeUCloudCert{}
	f.order = nil
	f.deletes = nil
	f.lists = map[string]int{}
	f.vanishID = 0
	f.denyID = 0
	f.listFailCode = 0
}

func TestPurgeWorkflowUCloud(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runPurgeUCloudInDocker(t)
		return
	}
	runPurgeUCloudScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runPurgeUCloudScenarios(t *testing.T, binaryDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fixture := newPurgeUCloudFixture(t)
	core := startPurgeCore(t, binaryDir, purgeUCloudPlugins)

	core.assertCatalogEntry("ucloud-ussl-purger", "purge", true)
	if envelope, err := core.fetchSchemaEnvelope("ucloud-ussl-purger", "purge"); err != nil {
		t.Fatalf("purge schema endpoint: %v", err)
	} else if envelope["provider"] != "ucloud-ussl-purger" || envelope["category"] != "purge" {
		t.Fatalf("unexpected purge schema envelope: %v", envelope)
	}

	accessConfig := map[string]any{"privateKey": purgeUCloudPrivateKey, "publicKey": purgeUCloudPublicKey, "projectId": purgeUCloudProjectId}
	wrongKeyAccess := map[string]any{"privateKey": "wrong-private", "publicKey": purgeUCloudPublicKey, "projectId": purgeUCloudProjectId}

	request := func(name, provider string, access map[string]any) purgeWorkflowRequest {
		return purgeWorkflowRequest{
			name:           name,
			provider:       provider,
			accessProvider: "ucloud",
			accessConfig:   access,
			providerConfig: map[string]any{"endpoint": fixture.endpoint},
			expiredDays:    7,
		}
	}
	run := func(req purgeWorkflowRequest) purgeWorkflowResult {
		return core.runPurgeWorkflow(ctx, req, purgeUCloudPrivateKey)
	}
	assertDeletes := func(t *testing.T, want ...string) {
		t.Helper()
		if strings.Join(fixture.deletes, ",") != strings.Join(want, ",") {
			t.Fatalf("deleted certificates %v, want %v", fixture.deletes, want)
		}
	}

	t.Run("success_legacy_and_new_ids", func(t *testing.T) {
		for _, provider := range []string{"ucloud-ussl", "ucloud-ussl-purger"} {
			fixture.reset()
			fixture.seed(1, "trust", 30)
			fixture.seed(2, "trust", -6)
			fixture.seed(3, "purchase", 8)
			fixture.seed(4, "purchase", -30)

			result := run(request("Purge ucloud ussl "+provider, provider, accessConfig))
			if result.status != "succeeded" {
				t.Fatalf("provider %s workflow status %s: %s", provider, result.status, result.runError)
			}
			assertDeletes(t, "trust:1", "purchase:3")
			if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
				t.Fatalf("provider %s purge log count=%d found=%t, want 2", provider, count, ok)
			}
			for _, kept := range []int{2, 4} {
				if !containsInt(fixture.remaining(), kept) {
					t.Fatalf("provider %s kept certificates %v, want %d present", provider, fixture.remaining(), kept)
				}
			}
		}
	})

	t.Run("multipage_list", func(t *testing.T) {
		fixture.reset()
		fixture.mu.Lock()
		for i := 1; i <= 1001; i++ {
			days := -30.0
			if i == 1 {
				days = 30
			}
			fixture.certs[i] = &purgeUCloudCert{id: i, mode: "trust", project: purgeUCloudProjectId, notAfter: time.Now().Add(time.Duration(-days * 24 * float64(time.Hour))).UnixMilli()}
			fixture.order = append(fixture.order, i)
		}
		fixture.mu.Unlock()
		fixture.seed(2001, "purchase", 9)

		result := run(request("Purge ucloud ussl multipage", "ucloud-ussl-purger", accessConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.lists["trust"] != 2 || fixture.lists["purchase"] != 1 {
			t.Fatalf("list calls trust=%d purchase=%d, want 2 and 1", fixture.lists["trust"], fixture.lists["purchase"])
		}
		assertDeletes(t, "trust:1", "purchase:2001")
	})

	t.Run("zero_notafter_treated_as_epoch", func(t *testing.T) {
		fixture.reset()
		fixture.seedProject(5, "trust", purgeUCloudProjectId, 0)
		fixture.seed(6, "trust", -6)

		result := run(request("Purge ucloud ussl zero date", "ucloud-ussl-purger", accessConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, "trust:5")
	})

	t.Run("other_project_records_preserved", func(t *testing.T) {
		fixture.reset()
		fixture.seed(7, "trust", 30)
		fixture.seedProject(8, "trust", purgeUCloudOtherProject, time.Now().Add(-30*24*time.Hour).UnixMilli())

		result := run(request("Purge ucloud ussl project scope", "ucloud-ussl-purger", accessConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, "trust:7")
		if !containsInt(fixture.remaining(), 8) {
			t.Fatal("certificate in another project was purged")
		}
	})

	t.Run("vanish_skips", func(t *testing.T) {
		fixture.reset()
		fixture.seed(9, "trust", 15)
		fixture.seed(10, "trust", 20)
		fixture.mu.Lock()
		fixture.vanishID = 9
		fixture.mu.Unlock()

		result := run(request("Purge ucloud ussl vanish", "ucloud-ussl-purger", accessConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, "trust:10")
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 1 {
			t.Fatalf("purge log count=%d found=%t, want 1", count, ok)
		}
		if !containsInt(fixture.remaining(), 9) {
			t.Fatal("vanished certificate was removed despite the skip code")
		}
	})

	t.Run("delete_failure_and_repair", func(t *testing.T) {
		fixture.reset()
		fixture.seed(11, "trust", 20)
		fixture.seed(12, "trust", 20)
		fixture.mu.Lock()
		fixture.denyID = 12
		fixture.mu.Unlock()

		req := request("Purge ucloud ussl delete failure", "ucloud-ussl-purger", accessConfig)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}
		assertDeletes(t, "trust:11")

		fixture.mu.Lock()
		fixture.denyID = 0
		fixture.mu.Unlock()
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		assertDeletes(t, "trust:11", "trust:12")
	})

	t.Run("list_failure", func(t *testing.T) {
		fixture.reset()
		fixture.seed(13, "trust", 30)
		fixture.mu.Lock()
		fixture.listFailCode = 230
		fixture.mu.Unlock()

		result := run(request("Purge ucloud ussl list failure", "ucloud-ussl-purger", accessConfig))
		fixture.mu.Lock()
		fixture.listFailCode = 0
		fixture.mu.Unlock()
		if result.status != "failed" {
			t.Fatalf("list failure status %s, want failed", result.status)
		}
		if len(fixture.deletes) != 0 {
			t.Fatalf("list failure still deleted %v", fixture.deletes)
		}
	})

	t.Run("auth_failure_no_deletes", func(t *testing.T) {
		fixture.reset()
		fixture.seed(14, "trust", 30)

		result := run(request("Purge ucloud ussl wrong key", "ucloud-ussl-purger", wrongKeyAccess))
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
		if len(fixture.deletes) != 0 {
			t.Fatalf("auth failure still deleted %v", fixture.deletes)
		}
		if !containsInt(fixture.remaining(), 14) {
			t.Fatal("certificate vanished during auth failure")
		}
	})
}

func runPurgeUCloudInDocker(t *testing.T) {
	t.Helper()
	runPurgeFamilyInDocker(t, purgeUCloudPlugins, "purge-ucloud.test", "^TestPurgeWorkflowUCloud$")
}

func containsInt(items []int, want int) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
