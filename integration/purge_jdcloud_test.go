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
	purgeJDCloudAccessKey = "fixture-jdcloud-ak"
	purgeJDCloudSecretKey = "fixture-jdcloud-sk"
	purgeJDCloudHost      = "ssl.jdcloud-api.com"
)

var purgeJDCloudPlugins = []string{"jdcloud-ssl-purger"}

type purgeJDCloudCert struct {
	id      string
	endTime string
}

type purgeJDCloudFixture struct {
	mu sync.Mutex

	certs   map[string]*purgeJDCloudCert
	order   []string
	deletes []string
	lists   int

	vanishID     string
	denyID       string
	permissionID string
	listFailBody bool
}

func newPurgeJDCloudFixture(t *testing.T) *purgeJDCloudFixture {
	t.Helper()
	f := &purgeJDCloudFixture{certs: map[string]*purgeJDCloudCert{}}
	newB4CloudProxy(t, []string{purgeJDCloudHost}, http.HandlerFunc(f.serve))
	return f
}

func (f *purgeJDCloudFixture) seed(id string, daysToExpiry float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seedLocked(id, time.Now().Add(time.Duration(-daysToExpiry*24*float64(time.Hour))).Format(time.RFC3339))
}

func (f *purgeJDCloudFixture) seedLocked(id string, endTime string) {
	if _, exists := f.certs[id]; !exists {
		f.order = append(f.order, id)
	}
	f.certs[id] = &purgeJDCloudCert{id: id, endTime: endTime}
}

func (f *purgeJDCloudFixture) remaining() []string {
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

func (f *purgeJDCloudFixture) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *purgeJDCloudFixture) errorBody(w http.ResponseWriter, status int, code int, message string) {
	f.writeJSON(w, status, map[string]any{
		"requestId": "fixture",
		"error":     map[string]any{"code": code, "status": "PERMISSION_DENIED", "message": message},
	})
}

func (f *purgeJDCloudFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !strings.Contains(r.Header.Get("Authorization"), "Credential="+purgeJDCloudAccessKey+"/") {
		f.errorBody(w, http.StatusForbidden, 403, "unexpected access key")
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/sslCert":
		if f.listFailBody {
			f.errorBody(w, http.StatusInternalServerError, 500, "fixture list failure")
			return
		}
		f.lists++
		page := 1
		if _, err := fmt.Sscanf(r.URL.Query().Get("pageNumber"), "%d", &page); err != nil || page < 1 {
			f.errorBody(w, http.StatusBadRequest, 400, "missing pagination")
			return
		}
		size := 10
		if _, err := fmt.Sscanf(r.URL.Query().Get("pageSize"), "%d", &size); err != nil || size < 1 {
			f.errorBody(w, http.StatusBadRequest, 400, "missing pagination")
			return
		}
		live := make([]string, 0, len(f.order))
		for _, id := range f.order {
			if _, exists := f.certs[id]; exists {
				live = append(live, id)
			}
		}
		start := min((page-1)*size, len(live))
		end := min(start+size, len(live))
		details := make([]map[string]any, 0, end-start)
		for _, id := range live[start:end] {
			details = append(details, map[string]any{"certId": id, "endTime": f.certs[id].endTime})
		}
		f.writeJSON(w, http.StatusOK, map[string]any{
			"requestId": "fixture",
			"result":    map[string]any{"certListDetails": details, "totalCount": len(live)},
		})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/sslCert/"):
		id := strings.TrimPrefix(r.URL.Path, "/v1/sslCert/")
		if id == f.denyID {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", http.StatusInternalServerError)
				return
			}
			conn, buffer, err := hijacker.Hijack()
			if err == nil {
				_ = buffer.Flush()
				_ = conn.Close()
			}
			return
		}
		if id == f.vanishID || id == f.permissionID {
			f.errorBody(w, http.StatusNotFound, 404, "fixture simulated delete failure")
			return
		}
		if _, exists := f.certs[id]; !exists {
			f.errorBody(w, http.StatusNotFound, 404, "unknown certificate")
			return
		}
		delete(f.certs, id)
		f.deletes = append(f.deletes, id)
		f.writeJSON(w, http.StatusOK, map[string]any{"requestId": "fixture", "result": map[string]any{}})
	default:
		f.errorBody(w, http.StatusNotFound, 404, "unexpected fixture request "+r.Method+" "+r.URL.Path)
	}
}

func (f *purgeJDCloudFixture) reset() {
	f.certs = map[string]*purgeJDCloudCert{}
	f.order = nil
	f.deletes = nil
	f.lists = 0
	f.vanishID = ""
	f.denyID = ""
	f.permissionID = ""
	f.listFailBody = false
}

func TestPurgeWorkflowJDCloud(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runPurgeJDCloudInDocker(t)
		return
	}
	runPurgeJDCloudScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runPurgeJDCloudScenarios(t *testing.T, binaryDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fixture := newPurgeJDCloudFixture(t)
	core := startPurgeCore(t, binaryDir, purgeJDCloudPlugins)

	core.assertCatalogEntry("jdcloud-ssl-purger", "purge", true)
	if envelope, err := core.fetchSchemaEnvelope("jdcloud-ssl-purger", "purge"); err != nil {
		t.Fatalf("purge schema endpoint: %v", err)
	} else if envelope["provider"] != "jdcloud-ssl-purger" || envelope["category"] != "purge" {
		t.Fatalf("unexpected purge schema envelope: %v", envelope)
	}

	accessConfig := map[string]any{"accessKeyId": purgeJDCloudAccessKey, "accessKeySecret": purgeJDCloudSecretKey}

	request := func(name, provider string) purgeWorkflowRequest {
		return purgeWorkflowRequest{
			name:           name,
			provider:       provider,
			accessProvider: "jdcloud",
			accessConfig:   accessConfig,
			providerConfig: map[string]any{},
			expiredDays:    7,
		}
	}
	run := func(req purgeWorkflowRequest) purgeWorkflowResult {
		return core.runPurgeWorkflow(ctx, req, purgeJDCloudSecretKey)
	}
	assertDeletes := func(t *testing.T, want ...string) {
		t.Helper()
		if strings.Join(fixture.deletes, ",") != strings.Join(want, ",") {
			t.Fatalf("deleted certificates %v, want %v", fixture.deletes, want)
		}
	}

	t.Run("success_legacy_and_new_ids", func(t *testing.T) {
		for _, provider := range []string{"jdcloud-ssl", "jdcloud-ssl-purger"} {
			fixture.reset()
			fixture.seed("jd-old-30d", 30)
			fixture.seed("jd-old-8d", 8)
			fixture.seed("jd-new-6d", -6)
			fixture.seed("jd-future", -30)

			result := run(request("Purge jdcloud ssl "+provider, provider))
			if result.status != "succeeded" {
				t.Fatalf("provider %s workflow status %s: %s", provider, result.status, result.runError)
			}
			assertDeletes(t, "jd-old-30d", "jd-old-8d")
			if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
				t.Fatalf("provider %s purge log count=%d found=%t, want 2", provider, count, ok)
			}
			if !contains(fixture.remaining(), "jd-new-6d") || !contains(fixture.remaining(), "jd-future") {
				t.Fatalf("provider %s kept certificates %v", provider, fixture.remaining())
			}
		}
	})

	t.Run("multipage_list", func(t *testing.T) {
		fixture.reset()
		fixture.mu.Lock()
		for i := 1; i <= 11; i++ {
			if i <= 2 {
				fixture.seedLocked(fmt.Sprintf("jd-page-old-%d", i), time.Now().Add(-30*24*time.Hour).Format(time.RFC3339))
			} else {
				fixture.seedLocked(fmt.Sprintf("jd-page-new-%d", i), time.Now().Add(-24*time.Hour).Format(time.RFC3339))
			}
		}
		fixture.mu.Unlock()

		result := run(request("Purge jdcloud ssl multipage", "jdcloud-ssl-purger"))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.lists != 2 {
			t.Fatalf("list calls %d, want 2 (10 per page then remainder)", fixture.lists)
		}
		assertDeletes(t, "jd-page-old-1", "jd-page-old-2")
	})

	t.Run("unparseable_date_skipped", func(t *testing.T) {
		fixture.reset()
		fixture.mu.Lock()
		fixture.seedLocked("jd-bad-date", "not-a-timestamp")
		fixture.seedLocked("jd-old-30d", time.Now().Add(-30*24*time.Hour).Format(time.RFC3339))
		fixture.mu.Unlock()

		result := run(request("Purge jdcloud ssl bad date", "jdcloud-ssl-purger"))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, "jd-old-30d")
		if !contains(fixture.remaining(), "jd-bad-date") {
			t.Fatal("certificate with unparseable date was purged")
		}
	})

	t.Run("delete_error_body_still_counts", func(t *testing.T) {
		fixture.reset()
		fixture.seed("jd-vanish", 15)
		fixture.seed("jd-old-30d", 30)
		fixture.mu.Lock()
		fixture.vanishID = "jd-vanish"
		fixture.mu.Unlock()

		result := run(request("Purge jdcloud ssl vanished", "jdcloud-ssl-purger"))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, "jd-old-30d")
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge log count=%d found=%t, want 2 (error bodies are not transport errors)", count, ok)
		}
		if !contains(fixture.remaining(), "jd-vanish") {
			t.Fatal("vanished certificate disappeared from the fixture despite the error body")
		}
	})

	t.Run("permission_error_body_still_succeeds", func(t *testing.T) {
		fixture.reset()
		fixture.seed("jd-denied", 20)
		fixture.mu.Lock()
		fixture.permissionID = "jd-denied"
		fixture.mu.Unlock()

		result := run(request("Purge jdcloud ssl permission", "jdcloud-ssl-purger"))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 1 {
			t.Fatalf("purge log count=%d found=%t, want 1", count, ok)
		}
		if !contains(fixture.remaining(), "jd-denied") {
			t.Fatal("permission-denied certificate disappeared from the fixture")
		}
	})

	t.Run("transport_failure_and_repair", func(t *testing.T) {
		fixture.reset()
		fixture.seed("jd-deny-a", 20)
		fixture.seed("jd-deny-b", 20)
		fixture.mu.Lock()
		fixture.denyID = "jd-deny-b"
		fixture.mu.Unlock()

		req := request("Purge jdcloud ssl delete failure", "jdcloud-ssl-purger")
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}
		assertDeletes(t, "jd-deny-a")

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
		assertDeletes(t, "jd-deny-a", "jd-deny-b")
	})

	t.Run("list_error_body_zero_count", func(t *testing.T) {
		fixture.reset()
		fixture.seed("jd-list-fail", 30)
		fixture.mu.Lock()
		fixture.listFailBody = true
		fixture.mu.Unlock()

		result := run(request("Purge jdcloud ssl list failure", "jdcloud-ssl-purger"))
		fixture.mu.Lock()
		fixture.listFailBody = false
		fixture.mu.Unlock()
		if result.status != "succeeded" {
			t.Fatalf("list error body status %s, want succeeded (empty result is current vendor behavior)", result.status)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("list error body log count=%d found=%t, want 0", count, ok)
		}
		if len(fixture.deletes) != 0 {
			t.Fatalf("list error body still deleted %v", fixture.deletes)
		}
	})
}

func runPurgeJDCloudInDocker(t *testing.T) {
	t.Helper()
	runPurgeFamilyInDocker(t, purgeJDCloudPlugins, "purge-jdcloud.test", "^TestPurgeWorkflowJDCloud$")
}
