package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bcecert "github.com/baidubce/bce-sdk-go/services/cert"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

const (
	purgeBaiduAccessKey    = "fixture-purge-access"
	purgeBaiduSecretKey    = "fixture-purge-secret"
	purgePurgerProvider    = "baiducloud-cert-purger"
	purgeLegacyProvider    = "baiducloud-cert"
	purgePluginProcessName = "baiducloud-cert-purger"
)

var purgeBaiduPlugins = []string{purgePurgerProvider, purgeLegacyProvider}

type purgeBaiduCert struct {
	id         string
	name       string
	notAfter   time.Time
	associated bool
}

type purgeBaiduFixture struct {
	barrier workflowRequestBarrier
	mu      sync.Mutex
	mode    string

	denyDeleteID string
	certs        map[string]*purgeBaiduCert
	order        []string
	deletes      []string
	lists        int
	deleteCalls  int
	uploads      map[string]string

	hold         atomic.Bool
	holdEntered  chan struct{}
	holdAborted  chan struct{}
	holdReleased chan struct{}
}

func newPurgeBaiduFixture(t *testing.T) *purgeBaiduFixture {
	t.Helper()
	f := &purgeBaiduFixture{
		certs:   map[string]*purgeBaiduCert{},
		uploads: map[string]string{},
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:80")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(f.serve), ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })
	return f
}

func (f *purgeBaiduFixture) armHold() (entered, aborted <-chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.holdEntered = make(chan struct{})
	f.holdAborted = make(chan struct{})
	f.holdReleased = make(chan struct{})
	f.hold.Store(true)
	return f.holdEntered, f.holdAborted
}

func (f *purgeBaiduFixture) releaseHold() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.holdReleased != nil {
		select {
		case <-f.holdReleased:
		default:
			close(f.holdReleased)
		}
	}
}

func (f *purgeBaiduFixture) seed(id string, daysToExpiry float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seedLocked(id, daysToExpiry, false)
}

func (f *purgeBaiduFixture) seedAssociated(id string, daysToExpiry float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seedLocked(id, daysToExpiry, true)
}

func (f *purgeBaiduFixture) seedLocked(id string, daysToExpiry float64, associated bool) {
	if _, exists := f.certs[id]; !exists {
		f.order = append(f.order, id)
	}
	f.certs[id] = &purgeBaiduCert{id: id, name: id, notAfter: time.Now().Add(time.Duration(-daysToExpiry * 24 * float64(time.Hour))), associated: associated}
}

func (f *purgeBaiduFixture) snapshot() (certs []string, deletes []string, lists int, deleteCalls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.order {
		if _, exists := f.certs[id]; exists {
			certs = append(certs, id)
		}
	}
	return certs, append([]string{}, f.deletes...), f.lists, f.deleteCalls
}

func (f *purgeBaiduFixture) setDenyDelete(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.denyDeleteID = id
}

func (f *purgeBaiduFixture) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

func (f *purgeBaiduFixture) uploadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.uploads)
}

func (f *purgeBaiduFixture) serve(w http.ResponseWriter, r *http.Request) {
	if err := f.barrier.wait(r.Context(), nil); err != nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(result any) { _ = json.NewEncoder(w).Encode(result) }
	fail := func(status int, code, message string) {
		w.WriteHeader(status)
		write(map[string]string{"code": code, "message": message, "requestId": "fixture"})
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "bce-auth-v1/"+purgeBaiduAccessKey+"/") {
		fail(http.StatusUnauthorized, "InvalidAccessKeyId", "unexpected access key")
		return
	}
	if f.mode == "auth-failure" {
		fail(http.StatusForbidden, "AccessDenied", "fixture auth-failure")
		return
	}
	if r.Host != "certificate.baidubce.com" {
		fail(http.StatusNotFound, "UnexpectedRequest", fmt.Sprintf("unexpected fixture host %s", r.Host))
		return
	}
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == "/v1/certificate/detail":
		f.lists++
		result := bcecert.ListCertDetailResult{Certs: []bcecert.CertificateDetailMeta{}}
		for _, id := range f.order {
			cert, exists := f.certs[id]
			if !exists {
				continue
			}
			result.Certs = append(result.Certs, bcecert.CertificateDetailMeta{
				CertId: cert.id, CertName: cert.name, CertStopTime: cert.notAfter.Format(time.RFC3339),
			})
		}
		write(result)
		return
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/v1/certificate/"):
		f.deleteCalls++
		id := strings.TrimPrefix(path, "/v1/certificate/")
		cert, ok := f.certs[id]
		if !ok {
			fail(http.StatusNotFound, "ResourceNotFoundException", "unknown certificate")
			return
		}
		if f.denyDeleteID == id {
			fail(http.StatusForbidden, "AccessDenied", "fixture delete denied")
			return
		}
		if cert.associated {
			fail(http.StatusConflict, "OperationNotAllowedException", "certificate is associated")
			return
		}
		if f.hold.CompareAndSwap(true, false) {
			entered, aborted, released := f.holdEntered, f.holdAborted, f.holdReleased
			f.mu.Unlock()
			close(entered)
			select {
			case <-r.Context().Done():
				close(aborted)
			case <-released:
			case <-time.After(60 * time.Second):
			}
			f.mu.Lock()
			return
		}
		delete(f.certs, id)
		f.deletes = append(f.deletes, id)
		write(struct{}{})
		return
	case r.Method == http.MethodPost && path == "/v1/certificate":
		var upload struct {
			CertName        string `json:"certName"`
			CertServerData  string `json:"certServerData"`
			CertPrivateData string `json:"certPrivateData"`
		}
		if json.NewDecoder(r.Body).Decode(&upload) != nil || upload.CertServerData == "" || upload.CertPrivateData == "" {
			fail(http.StatusBadRequest, "InvalidCertificate", "missing certificate data")
			return
		}
		leaf, err := xcert.ParseCertificateFromPEM(upload.CertServerData)
		if err != nil {
			fail(http.StatusBadRequest, "InvalidCertificate", "invalid uploaded certificate")
			return
		}
		id := fmt.Sprintf("cert-%d", len(f.uploads)+1)
		f.uploads[id] = upload.CertServerData
		f.seedLocked(id, time.Since(leaf.NotAfter).Hours()/24, false)
		write(bcecert.CreateCertResult{CertId: id, CertName: upload.CertName})
		return
	}
	fail(http.StatusNotFound, "UnexpectedRequest", fmt.Sprintf("unexpected fixture request %s %s%s", r.Method, r.Host, path))
}

func TestPurgeWorkflowBaidu(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runPurgeBaiduInDocker(t)
		return
	}
	runPurgeBaiduScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runPurgeBaiduScenarios(t *testing.T, binaryDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	fixture := newPurgeBaiduFixture(t)
	core := startPurgeCore(t, binaryDir, purgeBaiduPlugins)

	core.assertCatalogEntry(purgePurgerProvider, "purge", true)
	core.assertCatalogEntry(purgeLegacyProvider, "deploy", true)
	if envelope, err := core.fetchSchemaEnvelope(purgePurgerProvider, "purge"); err != nil {
		t.Fatalf("purge schema endpoint: %v", err)
	} else if envelope["provider"] != purgePurgerProvider || envelope["category"] != "purge" {
		t.Fatalf("unexpected purge schema envelope: %v", envelope)
	}
	if envelope, err := core.fetchSchemaEnvelope(purgePurgerProvider, ""); err != nil {
		t.Fatalf("purge schema endpoint without category: %v", err)
	} else if envelope["category"] != "purge" {
		t.Fatalf("schema endpoint without category lost the purge envelope: %v", envelope)
	}

	accessConfig := map[string]any{"accessKeyId": purgeBaiduAccessKey, "secretAccessKey": purgeBaiduSecretKey}
	sharedAccessID := core.createAccess(ctx, "baiducloud", accessConfig)
	secrets := []string{purgeBaiduSecretKey}

	baseRequest := func(provider string, expiredDays any) purgeWorkflowRequest {
		return purgeWorkflowRequest{
			name:         "Purge baiducloud " + provider,
			provider:     provider,
			accessID:     sharedAccessID,
			accessConfig: accessConfig,
			expiredDays:  expiredDays,
		}
	}

	run := func(t *testing.T, req purgeWorkflowRequest) purgeWorkflowResult {
		t.Helper()
		return core.runPurgeWorkflow(ctx, req, secrets...)
	}

	t.Run("shared_access_deploy_and_canonical_threshold", func(t *testing.T) {
		issuer, _, _ := makeCertificate(t, []string{"issuer.example.com"}, true)
		certPEM, keyPEM, _ := makeCertificate(t, []string{"purge-deploy.example.com"}, false)
		certPEM += issuer

		fixture.seed("expired-30d", 30)
		fixture.seed("expired-10d", 10)
		fixture.seed("expired-2d", 2)

		req := baseRequest(purgePurgerProvider, float64(7))
		req.name = "Purge shared access"
		req.deployNode = true
		req.deployProvider = purgeLegacyProvider
		req.certificatePEM = certPEM
		req.privateKeyPEM = keyPEM
		result := run(t, req)
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}

		_, deletes, _, _ := fixture.snapshot()
		if strings.Join(deletes, ",") != "expired-30d,expired-10d" {
			t.Fatalf("deleted certificates %v, want [expired-30d expired-10d]", deletes)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		certs, _, _, _ := fixture.snapshot()
		for _, keep := range []string{"expired-2d", "cert-1"} {
			if !contains(certs, keep) {
				t.Fatalf("certificate %s was removed unexpectedly; remaining %v", keep, certs)
			}
		}
		if fixture.uploadCount() != 1 {
			t.Fatalf("deploy node uploads=%d, want 1", fixture.uploadCount())
		}
	})

	t.Run("legacy_id_and_reimport", func(t *testing.T) {
		issuer, _, _ := makeCertificate(t, []string{"issuer.example.com"}, true)
		certPEM, keyPEM, _ := makeCertificate(t, []string{"purge-legacy.example.com"}, false)
		certPEM += issuer

		fixture.seed("legacy-expired-9d", 9)
		fixture.seed("legacy-valid", -30)

		req := baseRequest(purgeLegacyProvider, float64(7))
		req.name = "Purge legacy published"
		req.deployNode = true
		req.deployProvider = purgeLegacyProvider
		req.certificatePEM = certPEM
		req.privateKeyPEM = keyPEM
		graph := buildPurgeGraph(req, sharedAccessID)
		workflowID := core.importWorkflow(ctx, req.name, graph)
		result := core.runWorkflow(ctx, workflowID, secrets)
		if result.status != "succeeded" {
			t.Fatalf("legacy workflow status %s: %s", result.status, result.runError)
		}
		_, deletes, _, _ := fixture.snapshot()
		if !contains(deletes, "legacy-expired-9d") || contains(deletes, "legacy-valid") {
			t.Fatalf("legacy run deleted %v", deletes)
		}
		if fixture.uploadCount() != 2 {
			t.Fatalf("legacy deploy node uploads=%d, want 2", fixture.uploadCount())
		}

		before, beforeDraft := core.workflowGraphs(ctx, workflowID)
		reimported := core.importWorkflow(ctx, "Purge legacy imported", graph)
		reimportResult := core.runWorkflow(ctx, reimported, secrets)
		if reimportResult.status != "succeeded" {
			t.Fatalf("reimported workflow status %s: %s", reimportResult.status, reimportResult.runError)
		}
		core.assertGraphsUnchanged(ctx, workflowID, before, beforeDraft)
		if _, deletes, _, _ := fixture.snapshot(); strings.Count(strings.Join(deletes, ","), "legacy-expired-9d") != 1 {
			t.Fatalf("reimported legacy run deleted %v", deletes)
		}
	})

	t.Run("nested_blocks_survive_roundtrip", func(t *testing.T) {
		fixture.seed("nested-expired-8d", 8)

		req := baseRequest(purgeLegacyProvider, float64(7))
		req.name = "Purge nested"
		req.nested = true
		result := run(t, req)
		if result.status != "succeeded" {
			t.Fatalf("nested workflow status %s: %s", result.status, result.runError)
		}
		if _, deletes, _, _ := fixture.snapshot(); !contains(deletes, "nested-expired-8d") {
			t.Fatalf("nested run deleted %v", deletes)
		}

		content, draft := core.workflowGraphs(ctx, result.workflowID)
		imported := core.importWorkflow(ctx, "Purge nested imported", content)
		importedResult := core.runWorkflow(ctx, imported, secrets)
		if importedResult.status != "succeeded" {
			t.Fatalf("imported nested workflow status %s: %s", importedResult.status, importedResult.runError)
		}
		if !hasNestedPurgeNode(content) || !hasNestedPurgeNode(draft) {
			t.Fatal("saved nested graph lost its branch block structure")
		}
		core.assertGraphsUnchanged(ctx, result.workflowID, content, draft)
	})

	t.Run("invalid_thresholds_never_reach_vendor", func(t *testing.T) {
		invalid := []any{float64(-1), float64(1.5), "abc", "99999999999999999999", true}
		_, _, listsBefore, deletesBefore := fixture.snapshot()
		for _, raw := range invalid {
			req := baseRequest(purgePurgerProvider, raw)
			req.name = fmt.Sprintf("Purge invalid %v", raw)
			result := run(t, req)
			if result.status != "failed" {
				t.Fatalf("invalid expiredDays %v produced status %s", raw, result.status)
			}
		}
		_, _, lists, deleteCalls := fixture.snapshot()
		if lists != listsBefore || deleteCalls != deletesBefore {
			t.Fatalf("invalid thresholds reached the vendor: lists %d→%d, delete calls %d→%d", listsBefore, lists, deletesBefore, deleteCalls)
		}
	})

	t.Run("zero_and_empty_thresholds_purge_only_expired", func(t *testing.T) {
		fixture.seed("expired-halfday", 0.5)
		fixture.seed("valid-30d", -30)

		for _, raw := range []any{nil, float64(0), ""} {
			req := baseRequest(purgePurgerProvider, raw)
			req.name = "Purge zero threshold"
			result := run(t, req)
			if result.status != "succeeded" {
				t.Fatalf("threshold %v status %s: %s", raw, result.status, result.runError)
			}
		}
		_, deletes, _, _ := fixture.snapshot()
		if !contains(deletes, "expired-halfday") || contains(deletes, "valid-30d") {
			t.Fatalf("zero-threshold runs deleted %v", deletes)
		}
	})

	t.Run("no_candidates_is_success_with_zero_count", func(t *testing.T) {
		fixture.seed("valid-90d", -90)
		req := baseRequest(purgePurgerProvider, float64(7))
		req.name = "Purge no candidates"
		result := run(t, req)
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("zero-candidate log count=%d found=%t, want 0", count, ok)
		}
		if _, deletes, _, _ := fixture.snapshot(); contains(deletes, "valid-90d") {
			t.Fatalf("zero-candidate run deleted %v", deletes)
		}
	})

	t.Run("associated_skips_not_counted", func(t *testing.T) {
		fixture.seedAssociated("associated-expired-12d", 12)
		fixture.seed("deletable-expired-12d", 12)

		req := baseRequest(purgePurgerProvider, float64(7))
		req.name = "Purge skip associated"
		result := run(t, req)
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 1 {
			t.Fatalf("skip run log count=%d found=%t, want 1", count, ok)
		}
		certs, deletes, _, _ := fixture.snapshot()
		if !contains(deletes, "deletable-expired-12d") {
			t.Fatalf("deletable certificate was not purged: %v", deletes)
		}
		if !contains(certs, "associated-expired-12d") {
			t.Fatal("associated certificate was removed despite the skip error")
		}
	})

	t.Run("partial_failure_then_permission_recovery", func(t *testing.T) {
		fixture.seed("partial-first", 20)
		fixture.seed("partial-second", 20)
		fixture.setDenyDelete("partial-second")

		req := baseRequest(purgePurgerProvider, float64(7))
		req.name = "Purge partial failure"
		result := run(t, req)
		if result.status != "failed" {
			t.Fatalf("partial failure status %s, want failed", result.status)
		}
		_, deletes, _, _ := fixture.snapshot()
		if !contains(deletes, "partial-first") || contains(deletes, "partial-second") {
			t.Fatalf("partial failure deleted %v", deletes)
		}

		fixture.setDenyDelete("")
		rerun := run(t, req)
		if rerun.status != "succeeded" {
			t.Fatalf("recovery status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("recovery log count=%d found=%t, want 1 (no double counting)", count, ok)
		}
		_, deletes, _, _ = fixture.snapshot()
		if !contains(deletes, "partial-second") || strings.Count(strings.Join(deletes, ","), "partial-first") != 1 {
			t.Fatalf("recovery deleted %v", deletes)
		}
	})

	t.Run("cancel_blocked_delete", func(t *testing.T) {
		fixture.seed("cancel-a", 15)
		fixture.seed("cancel-b", 15)

		req := baseRequest(purgePurgerProvider, float64(7))
		req.name = "Purge cancel"
		_, _, _, deleteCallsBase := fixture.snapshot()
		entered, _ := fixture.armHold()
		started := startPurgeRun(t, core, req)
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("fixture did not receive the held delete request")
		}
		var canceled struct {
			Code int `json:"code"`
		}
		if err := core.api.request(ctx, http.MethodPost, "/api/workflows/"+started.workflowID+"/runs/"+started.runID+"/cancel", nil, &canceled); err != nil {
			t.Fatal(err)
		}
		if canceled.Code != 0 {
			t.Fatalf("cancel endpoint returned code %d", canceled.Code)
		}
		waitPurgeRun(t, core, ctx, started, "canceled")
		if _, ok := core.purgeLogCount(ctx, started.runID); ok {
			t.Fatal("canceled run must not report a purge completion")
		}

		fixture.releaseHold()
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
			certs, _, _, _ := fixture.snapshot()
			if !contains(certs, "cancel-a") {
				break
			}
		}
		time.Sleep(2 * time.Second)
		certs, deletes, _, deleteCalls := fixture.snapshot()
		if contains(certs, "cancel-a") {
			t.Logf("cancellation aborted the held delete; cancel-a remains and will be purged by the retry")
		}
		if contains(deletes, "cancel-b") || !contains(certs, "cancel-b") {
			t.Fatalf("canceled run issued further deletes after cancel: deletes %v, remaining %v", deletes, certs)
		}
		if deleteCalls-deleteCallsBase != 2 {
			t.Fatalf("canceled run issued %d delete requests, want 2 (associated skip + the held one)", deleteCalls-deleteCallsBase)
		}

		retry := run(t, req)
		if retry.status != "succeeded" {
			t.Fatalf("post-cancel retry status %s: %s", retry.status, retry.runError)
		}
		certs, _, _, _ = fixture.snapshot()
		if contains(certs, "cancel-a") || contains(certs, "cancel-b") {
			t.Fatalf("post-cancel retry left %v behind", certs)
		}
	})

	t.Run("plugin_process_crash", func(t *testing.T) {
		fixture.seed("crash-a", 15)

		req := baseRequest(purgePurgerProvider, float64(7))
		req.name = "Purge crash"
		entered, _ := fixture.armHold()
		started := startPurgeRun(t, core, req)
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("fixture did not receive the held delete request")
		}
		killCtx, cancelKill := context.WithTimeout(ctx, 10*time.Second)
		defer cancelKill()
		if err := killPluginProcesses(killCtx, purgePluginProcessName); err != nil {
			t.Fatal(err)
		}
		waitPurgeRun(t, core, ctx, started, "failed")
		if _, deletes, _, _ := fixture.snapshot(); contains(deletes, "crash-a") {
			t.Fatal("crashed run must not report successful deletions")
		}

		retry := run(t, req)
		if retry.status != "succeeded" {
			t.Fatalf("post-crash retry status %s: %s", retry.status, retry.runError)
		}
		if _, deletes, _, _ := fixture.snapshot(); !contains(deletes, "crash-a") {
			t.Fatalf("post-crash retry deleted %v", deletes)
		}
	})

	t.Run("uninstall_and_reinstall", func(t *testing.T) {
		fixture.seed("uninstall-expired-11d", 11)

		req := baseRequest(purgeLegacyProvider, float64(7))
		req.name = "Purge uninstall"
		result := run(t, req)
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}

		content, draft := core.workflowGraphs(ctx, result.workflowID)
		restore := core.parkPlugin(purgePurgerProvider)
		core.restart()
		core.assertCatalogEntry(purgePurgerProvider, "purge", false)
		if _, err := core.fetchSchemaEnvelope(purgePurgerProvider, "purge"); err == nil {
			t.Fatal("schema endpoint still serves the uninstalled purge plugin")
		}
		missing := run(t, req)
		if missing.status != "failed" {
			t.Fatalf("missing plugin status %s, want failed", missing.status)
		}
		if !strings.Contains(missing.runError, purgePurgerProvider) {
			t.Fatalf("missing plugin error does not name the plugin: %s", missing.runError)
		}
		core.assertGraphsUnchanged(ctx, result.workflowID, content, draft)

		restore()
		core.restart()
		core.assertCatalogEntry(purgePurgerProvider, "purge", true)
		if _, err := core.fetchSchemaEnvelope(purgePurgerProvider, "purge"); err != nil {
			t.Fatalf("schema endpoint did not recover after reinstall: %v", err)
		}
		recovered := run(t, req)
		if recovered.status != "succeeded" {
			t.Fatalf("recovered status %s: %s", recovered.status, recovered.runError)
		}
		core.assertGraphsUnchanged(ctx, result.workflowID, content, draft)
		if _, deletes, _, _ := fixture.snapshot(); !contains(deletes, "uninstall-expired-11d") {
			t.Fatalf("recovered run deleted %v", deletes)
		}
	})

	t.Run("auth_failure_is_visible", func(t *testing.T) {
		fixture.setMode("auth-failure")
		req := baseRequest(purgePurgerProvider, float64(7))
		req.name = "Purge auth failure"
		result := run(t, req)
		fixture.setMode("")
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
	})
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func hasNestedPurgeNode(graph map[string]any) bool {
	nodes, ok := graph["nodes"].([]any)
	if !ok {
		return false
	}
	for _, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if node["type"] != "branchBlock" {
			continue
		}
		blocks, ok := node["blocks"].([]any)
		if !ok {
			return false
		}
		for _, blockRaw := range blocks {
			block, ok := blockRaw.(map[string]any)
			if ok && block["type"] == "bizPurge" {
				return true
			}
		}
	}
	return false
}

func startPurgeRun(t *testing.T, core *purgeCore, req purgeWorkflowRequest) purgeWorkflowResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	accessID := req.accessID
	if accessID == "" {
		accessID = core.createAccess(ctx, req.accessProvider, req.accessConfig)
	}
	workflowID := core.importWorkflow(ctx, req.name, buildPurgeGraph(req, accessID))
	var started struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	if err := core.api.request(ctx, http.MethodPost, "/api/workflows/"+workflowID+"/runs", map[string]string{"trigger": "manual"}, &started); err != nil {
		t.Fatal(err)
	}
	if started.Code != 0 || started.Data.RunID == "" {
		t.Fatalf("workflow trigger returned no run ID: code=%d message=%s", started.Code, started.Msg)
	}
	return purgeWorkflowResult{workflowID: workflowID, runID: started.Data.RunID}
}

func waitPurgeRun(t *testing.T, core *purgeCore, ctx context.Context, run purgeWorkflowResult, wantStatus string) {
	t.Helper()
	for {
		var current struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := core.api.request(ctx, http.MethodGet, "/api/collections/workflow_run/records/"+run.runID, nil, &current); err != nil {
			t.Fatal(err)
		}
		switch current.Status {
		case "succeeded", "failed", "canceled":
			if current.Status != wantStatus {
				t.Fatalf("run %s status %s, want %s: %s", run.runID, current.Status, wantStatus, current.Error)
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("run %s did not finish", run.runID)
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func runPurgeBaiduInDocker(t *testing.T) {
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
	buildB4CloudPackages(t, dir, purgeBaiduPlugins, "linux", arch)
	run(env, "go", "-C", root, "test", "-c", "-o", filepath.Join(dir, "purge-baidu.test"), "./integration")
	binary, err := filepath.Abs(os.Getenv("CERTIMATE_B1_CORE_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	output := run(nil, "docker", "run", "--rm", "--network=none",
		"--add-host=certificate.baidubce.com:127.0.0.1",
		"--mount", "type=bind,src="+dir+",dst=/fixtures,readonly",
		"--mount", "type=bind,src="+binary+",dst=/core,readonly",
		"-e", "CERTIMATE_B1_CORE_BINARY=/core",
		"-e", "CERTIMATE_PURGE_BINARIES=/fixtures",
		image, "/fixtures/purge-baidu.test", "-test.run=^TestPurgeWorkflowBaidu$", "-test.v", "-test.timeout=10m")
	t.Logf("Linux purge workflow output:\n%s", output)
}
