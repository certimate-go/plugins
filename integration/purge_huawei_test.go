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
	purgeHuaweiAK        = "fixture-huawei-ak"
	purgeHuaweiSK        = "fixture-huawei-sk"
	purgeHuaweiEPSID     = "fixture-huawei-eps"
	purgeHuaweiRegion    = "cn-north-4"
	purgeHuaweiProjectID = "fixture-huawei-project"
	purgeHuaweiDomainID  = "fixture-huawei-domain"
	purgeHuaweiELBHost   = "elb.cn-north-4.myhuaweicloud.com"
	purgeHuaweiSCMHost   = "scm.cn-north-4.myhuaweicloud.com"
	purgeHuaweiWAFHost   = "waf.cn-north-4.myhuaweicloud.com"
	purgeHuaweiIAMHost   = "iam.cn-north-4.myhuaweicloud.com"
	purgeHuaweiCST       = 8 * 60 * 60
)

var purgeHuaweiPlugins = []string{"huaweicloud-elb-purger", "huaweicloud-scm-purger", "huaweicloud-waf-purger"}

var purgeHuaweiHosts = []string{purgeHuaweiELBHost, purgeHuaweiSCMHost, purgeHuaweiWAFHost, purgeHuaweiIAMHost}

type purgeHuaweiCert struct {
	id       string
	notAfter time.Time
}

type purgeHuaweiService struct {
	certs    map[string]*purgeHuaweiCert
	order    []string
	deletes  []string
	lists    int
	pageSize int
	denyID   string
	listFail bool
}

func (s *purgeHuaweiService) seed(id string, daysToExpiry float64) {
	if _, exists := s.certs[id]; !exists {
		s.order = append(s.order, id)
	}
	s.certs[id] = &purgeHuaweiCert{
		id:       id,
		notAfter: time.Now().Add(time.Duration(-daysToExpiry * 24 * float64(time.Hour))),
	}
}

func (s *purgeHuaweiService) live() []*purgeHuaweiCert {
	items := make([]*purgeHuaweiCert, 0, len(s.order))
	for _, id := range s.order {
		if cert, exists := s.certs[id]; exists {
			items = append(items, cert)
		}
	}
	return items
}

func (s *purgeHuaweiService) remaining() []string {
	items := make([]string, 0, len(s.certs))
	for _, id := range s.order {
		if _, exists := s.certs[id]; exists {
			items = append(items, id)
		}
	}
	return items
}

type purgeHuaweiFixture struct {
	mu   sync.Mutex
	mode string

	elb purgeHuaweiService
	scm purgeHuaweiService
	waf purgeHuaweiService

	projectQueries []string
	elbMarkers     []string
	scmOffsets     []int
	scmEPSQueries  []string
	wafPages       []int
	wafEPSQueries  []string
}

func newPurgeHuaweiFixture(t *testing.T) *purgeHuaweiFixture {
	t.Helper()
	f := &purgeHuaweiFixture{
		elb: purgeHuaweiService{certs: map[string]*purgeHuaweiCert{}},
		scm: purgeHuaweiService{certs: map[string]*purgeHuaweiCert{}},
		waf: purgeHuaweiService{certs: map[string]*purgeHuaweiCert{}},
	}
	startB7HuaweiTarget(t, purgeHuaweiHosts, http.HandlerFunc(f.serve))
	return f
}

func (f *purgeHuaweiFixture) writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", "fixture")
	_ = json.NewEncoder(w).Encode(body)
}

func (f *purgeHuaweiFixture) apiError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", "fixture")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error_code": code, "error_msg": message})
}

func (f *purgeHuaweiFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	authorization := r.Header.Get("Authorization")
	if !strings.Contains(authorization, "Access="+purgeHuaweiAK+",") && !strings.Contains(authorization, "Credential="+purgeHuaweiAK+"/") {
		f.apiError(w, http.StatusUnauthorized, "APIG.0301", "unexpected access key")
		return
	}

	switch r.Host {
	case purgeHuaweiIAMHost:
		f.serveIAM(w, r)
	case purgeHuaweiELBHost:
		f.serveELB(w, r)
	case purgeHuaweiSCMHost:
		f.serveSCM(w, r)
	case purgeHuaweiWAFHost:
		f.serveWAF(w, r)
	default:
		http.Error(w, "unexpected fixture host "+r.Host, http.StatusBadRequest)
	}
}

func (f *purgeHuaweiFixture) serveIAM(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/v3/auth/domains":
		f.writeJSON(w, map[string]any{
			"domains": []map[string]any{{"id": purgeHuaweiDomainID, "name": "fixture-domain", "enabled": true}},
		})
	case r.URL.Path == "/v3/projects":
		if name := r.URL.Query().Get("name"); name != purgeHuaweiRegion {
			f.apiError(w, http.StatusBadRequest, "IAM.1001", "project name "+name)
			return
		}
		f.projectQueries = append(f.projectQueries, r.URL.Query().Encode())
		f.writeJSON(w, map[string]any{
			"projects": []map[string]any{{
				"id":        purgeHuaweiProjectID,
				"name":      purgeHuaweiRegion,
				"domain_id": purgeHuaweiDomainID,
				"parent_id": purgeHuaweiDomainID,
				"enabled":   true,
				"is_domain": false,
			}},
		})
	default:
		f.apiError(w, http.StatusNotFound, "IAM.1000", "unexpected iam request "+r.URL.Path)
	}
}

func (f *purgeHuaweiFixture) serviceError(w http.ResponseWriter, service *purgeHuaweiService, id string) bool {
	if service.denyID == id {
		f.apiError(w, http.StatusInternalServerError, "FIXTURE.DELETE_DENIED", "fixture delete denied")
		return true
	}
	if _, exists := service.certs[id]; !exists {
		f.apiError(w, http.StatusNotFound, "FIXTURE.NOT_FOUND", "unknown certificate")
		return true
	}
	delete(service.certs, id)
	service.deletes = append(service.deletes, id)
	return false
}

func (f *purgeHuaweiFixture) serveELB(w http.ResponseWriter, r *http.Request) {
	if f.mode == "auth-failure" {
		f.apiError(w, http.StatusUnauthorized, "APIG.0301", "fixture auth-failure")
		return
	}

	path := "/v3/" + purgeHuaweiProjectID + "/elb/certificates"
	switch {
	case r.Method == http.MethodGet && r.URL.Path == path:
		if f.elb.listFail {
			f.apiError(w, http.StatusInternalServerError, "ELB.00000000", "fixture list failure")
			return
		}
		query := r.URL.Query()
		if query.Get("limit") != "2000" {
			f.apiError(w, http.StatusBadRequest, "ELB.00000001", "limit "+query.Get("limit"))
			return
		}
		types := query["type"]
		if len(types) != 2 || !contains(types, "server") || !contains(types, "server_sm") {
			f.apiError(w, http.StatusBadRequest, "ELB.00000001", fmt.Sprintf("type %v", types))
			return
		}
		f.elb.lists++
		live := f.elb.live()
		page := live
		nextMarker := any(nil)
		if f.elb.pageSize > 0 {
			index := 0
			if marker := query.Get("marker"); marker != "" {
				if _, err := fmt.Sscanf(marker, "elb-page-%d", &index); err != nil {
					f.apiError(w, http.StatusBadRequest, "ELB.00000001", "bad marker "+marker)
					return
				}
			}
			start := index * f.elb.pageSize
			if start > len(live) {
				start = len(live)
			}
			end := start + f.elb.pageSize
			if end > len(live) {
				end = len(live)
			}
			page = live[start:end]
			if end < len(live) {
				nextMarker = fmt.Sprintf("elb-page-%d", index+1)
			}
		}
		certificates := make([]map[string]any, 0, len(page))
		for _, cert := range page {
			certificates = append(certificates, map[string]any{
				"id":          cert.id,
				"expire_time": cert.notAfter.UTC().Format(time.RFC3339),
			})
		}
		f.writeJSON(w, map[string]any{
			"request_id":   "fixture",
			"page_info":    map[string]any{"previous_marker": "", "current_count": len(page), "next_marker": nextMarker},
			"certificates": certificates,
		})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, path+"/"):
		id := strings.TrimPrefix(r.URL.Path, path+"/")
		if f.serviceError(w, &f.elb, id) {
			return
		}
		f.writeJSON(w, map[string]any{"request_id": "fixture"})
	default:
		f.apiError(w, http.StatusNotFound, "ELB.00000002", "unexpected elb request "+r.Method+" "+r.URL.Path)
	}
}

func (f *purgeHuaweiFixture) serveSCM(w http.ResponseWriter, r *http.Request) {
	if f.mode == "auth-failure" {
		f.apiError(w, http.StatusUnauthorized, "APIG.0301", "fixture auth-failure")
		return
	}

	cst := time.FixedZone("CST", purgeHuaweiCST)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v3/scm/certificates":
		if f.scm.listFail {
			f.apiError(w, http.StatusInternalServerError, "SCM.XXXX", "fixture list failure")
			return
		}
		query := r.URL.Query()
		if query.Get("limit") != "50" || query.Get("sort_dir") != "DESC" || query.Get("sort_key") != "certExpiredTime" {
			f.apiError(w, http.StatusBadRequest, "SCM.XXX1", fmt.Sprintf("list params %v", query.Encode()))
			return
		}
		f.scm.lists++
		f.scmEPSQueries = append(f.scmEPSQueries, query.Get("enterprise_project_id"))
		offset := 0
		if raw := query.Get("offset"); raw != "" {
			fmt.Sscanf(raw, "%d", &offset)
			f.scmOffsets = append(f.scmOffsets, offset)
		} else {
			f.scmOffsets = append(f.scmOffsets, 0)
		}
		live := f.scm.live()
		start := offset
		if start > len(live) {
			start = len(live)
		}
		end := start + 50
		if end > len(live) {
			end = len(live)
		}
		certificates := make([]map[string]any, 0)
		for _, cert := range live[start:end] {
			certificates = append(certificates, map[string]any{
				"id":          cert.id,
				"name":        cert.id,
				"expire_time": cert.notAfter.In(cst).Format(time.DateTime) + ".0",
			})
		}
		f.writeJSON(w, map[string]any{
			"total_count":  len(f.scm.order),
			"certificates": certificates,
		})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v3/scm/certificates/"):
		id := strings.TrimPrefix(r.URL.Path, "/v3/scm/certificates/")
		if f.serviceError(w, &f.scm, id) {
			return
		}
		f.writeJSON(w, map[string]any{})
	default:
		f.apiError(w, http.StatusNotFound, "SCM.XXX2", "unexpected scm request "+r.Method+" "+r.URL.Path)
	}
}

func (f *purgeHuaweiFixture) serveWAF(w http.ResponseWriter, r *http.Request) {
	if f.mode == "auth-failure" {
		f.apiError(w, http.StatusUnauthorized, "APIG.0301", "fixture auth-failure")
		return
	}

	path := "/v1/" + purgeHuaweiProjectID + "/waf/certificate"
	switch {
	case r.Method == http.MethodGet && r.URL.Path == path:
		if f.waf.listFail {
			f.apiError(w, http.StatusInternalServerError, "WAF.0000", "fixture list failure")
			return
		}
		query := r.URL.Query()
		if query.Get("exp_status") != "1" || query.Get("pagesize") != "100" {
			f.apiError(w, http.StatusBadRequest, "WAF.0001", fmt.Sprintf("list params %v", query.Encode()))
			return
		}
		f.waf.lists++
		f.wafEPSQueries = append(f.wafEPSQueries, query.Get("enterprise_project_id"))
		page := 1
		if raw := query.Get("page"); raw != "" {
			fmt.Sscanf(raw, "%d", &page)
		}
		f.wafPages = append(f.wafPages, page)
		live := f.waf.live()
		start := (page - 1) * 100
		if start > len(live) {
			start = len(live)
		}
		end := start + 100
		if end > len(live) {
			end = len(live)
		}
		items := make([]map[string]any, 0)
		for _, cert := range live[start:end] {
			items = append(items, map[string]any{
				"id":          cert.id,
				"expire_time": cert.notAfter.UnixMilli(),
			})
		}
		f.writeJSON(w, map[string]any{"total": len(f.waf.order), "items": items})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, path+"/"):
		if eps := r.URL.Query().Get("enterprise_project_id"); eps != purgeHuaweiEPSID {
			f.apiError(w, http.StatusBadRequest, "WAF.0002", "delete enterprise_project_id "+eps)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, path+"/")
		if f.serviceError(w, &f.waf, id) {
			return
		}
		f.writeJSON(w, map[string]any{"id": id})
	default:
		f.apiError(w, http.StatusNotFound, "WAF.0003", "unexpected waf request "+r.Method+" "+r.URL.Path)
	}
}

func (f *purgeHuaweiFixture) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

func TestPurgeWorkflowHuawei(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runPurgeHuaweiInDocker(t)
		return
	}
	runPurgeHuaweiScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runPurgeHuaweiScenarios(t *testing.T, binaryDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fixture := newPurgeHuaweiFixture(t)
	core := startPurgeCore(t, binaryDir, purgeHuaweiPlugins)

	for _, provider := range purgeHuaweiPlugins {
		core.assertCatalogEntry(provider, "purge", true)
	}
	if envelope, err := core.fetchSchemaEnvelope("huaweicloud-elb-purger", "purge"); err != nil {
		t.Fatalf("purge schema endpoint: %v", err)
	} else if envelope["provider"] != "huaweicloud-elb-purger" || envelope["category"] != "purge" {
		t.Fatalf("unexpected purge schema envelope: %v", envelope)
	}

	accessConfig := map[string]any{
		"accessKeyId":         purgeHuaweiAK,
		"secretAccessKey":     purgeHuaweiSK,
		"enterpriseProjectId": purgeHuaweiEPSID,
	}
	elbConfig := map[string]any{"region": purgeHuaweiRegion}
	wafConfig := map[string]any{"region": purgeHuaweiRegion}
	scmConfig := map[string]any{}

	request := func(name, provider string, providerConfig map[string]any) purgeWorkflowRequest {
		return purgeWorkflowRequest{
			name:           name,
			provider:       provider,
			accessProvider: "huaweicloud",
			accessConfig:   accessConfig,
			providerConfig: providerConfig,
			expiredDays:    7,
		}
	}
	run := func(req purgeWorkflowRequest) purgeWorkflowResult {
		return core.runPurgeWorkflow(ctx, req, purgeHuaweiSK)
	}
	assertDeletes := func(t *testing.T, service *purgeHuaweiService, want []string) {
		t.Helper()
		if strings.Join(service.deletes, ",") != strings.Join(want, ",") {
			t.Fatalf("deleted certificates %v, want %v", service.deletes, want)
		}
	}

	t.Run("elb_success_rfc3339_and_keystone_project", func(t *testing.T) {
		fixture.elb.seed("elb-old-30d", 30)
		fixture.elb.seed("elb-old-9d", 9)
		fixture.elb.seed("elb-new-2d", 2)
		fixture.elb.seed("elb-future", -30)
		projectsBefore := len(fixture.projectQueries)

		result := run(request("Purge huawei elb success", "huaweicloud-elb-purger", elbConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, &fixture.elb, []string{"elb-old-30d", "elb-old-9d"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		if len(fixture.projectQueries)-projectsBefore < 1 {
			t.Fatal("elb purge never resolved the region project via keystone")
		}
	})

	t.Run("elb_multipage_marker", func(t *testing.T) {
		fixture.mu.Lock()
		fixture.elb.pageSize = 2
		fixture.mu.Unlock()
		defer func() {
			fixture.mu.Lock()
			fixture.elb.pageSize = 0
			fixture.mu.Unlock()
		}()
		for i := 0; i < 5; i++ {
			fixture.elb.seed(fmt.Sprintf("elb-page-fresh-%d", i), 2)
		}
		fixture.elb.seed("elb-page-old", 30)
		listsBefore := fixture.elb.lists
		fixture.mu.Lock()
		total := len(fixture.elb.certs)
		fixture.mu.Unlock()
		wantLists := (total + 1) / 2

		result := run(request("Purge huawei elb multipage", "huaweicloud-elb-purger", elbConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.elb.lists-listsBefore != wantLists {
			t.Fatalf("list calls %d, want %d (2 per page)", fixture.elb.lists-listsBefore, wantLists)
		}
		if !contains(fixture.elb.deletes, "elb-page-old") {
			t.Fatalf("multipage deletes %v, want elb-page-old present", fixture.elb.deletes)
		}
	})

	t.Run("elb_delete_failure_and_repair", func(t *testing.T) {
		fixture.elb.seed("elb-pf-a", 20)
		fixture.elb.seed("elb-pf-b", 20)
		fixture.mu.Lock()
		fixture.elb.denyID = "elb-pf-b"
		fixture.mu.Unlock()

		req := request("Purge huawei elb delete failure", "huaweicloud-elb-purger", elbConfig)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}

		fixture.mu.Lock()
		fixture.elb.denyID = ""
		fixture.mu.Unlock()
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		if !contains(fixture.elb.deletes, "elb-pf-a") || !contains(fixture.elb.deletes, "elb-pf-b") {
			t.Fatalf("repair deleted %v", fixture.elb.deletes)
		}
	})

	t.Run("elb_auth_failure", func(t *testing.T) {
		fixture.elb.seed("elb-auth", 30)
		deletesBefore := len(fixture.elb.deletes)
		fixture.setMode("auth-failure")
		result := run(request("Purge huawei elb auth failure", "huaweicloud-elb-purger", elbConfig))
		fixture.setMode("")
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
		if len(fixture.elb.deletes) != deletesBefore {
			t.Fatalf("auth failure still deleted %v", fixture.elb.deletes[deletesBefore:])
		}
	})

	t.Run("elb_list_failure", func(t *testing.T) {
		fixture.elb.seed("elb-list-fail", 30)
		deletesBefore := len(fixture.elb.deletes)
		fixture.mu.Lock()
		fixture.elb.listFail = true
		fixture.mu.Unlock()
		result := run(request("Purge huawei elb list failure", "huaweicloud-elb-purger", elbConfig))
		fixture.mu.Lock()
		fixture.elb.listFail = false
		fixture.mu.Unlock()
		if result.status != "failed" {
			t.Fatalf("list failure status %s, want failed", result.status)
		}
		if len(fixture.elb.deletes) != deletesBefore {
			t.Fatalf("list failure still deleted %v", fixture.elb.deletes[deletesBefore:])
		}
	})

	t.Run("scm_success_cst_dates_and_enterprise_project", func(t *testing.T) {
		fixture.scm.seed("scm-old-30d", 30)
		fixture.scm.seed("scm-old-8d", 8)
		fixture.scm.seed("scm-new-1d", 1)
		fixture.scm.seed("scm-future", -30)
		epsBefore := len(fixture.scmEPSQueries)

		result := run(request("Purge huawei scm success", "huaweicloud-scm-purger", scmConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, &fixture.scm, []string{"scm-old-30d", "scm-old-8d"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		for _, eps := range fixture.scmEPSQueries[epsBefore:] {
			if eps != purgeHuaweiEPSID {
				t.Fatalf("scm list enterprise_project_id %q, want %q", eps, purgeHuaweiEPSID)
			}
		}
	})

	t.Run("scm_multipage_offset", func(t *testing.T) {
		for i := 0; i < 55; i++ {
			fixture.scm.seed(fmt.Sprintf("scm-page-fresh-%02d", i), 2)
		}
		fixture.scm.seed("scm-page-old", 30)
		listsBefore := fixture.scm.lists
		offsetsBefore := len(fixture.scmOffsets)

		result := run(request("Purge huawei scm multipage", "huaweicloud-scm-purger", scmConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.scm.lists-listsBefore != 2 {
			t.Fatalf("list calls %d, want 2 (50 per page)", fixture.scm.lists-listsBefore)
		}
		offsets := fixture.scmOffsets[offsetsBefore:]
		if len(offsets) < 2 || offsets[0] != 0 || offsets[1] != 50 {
			t.Fatalf("scm offsets %v, want [0 50]", offsets)
		}
		if !contains(fixture.scm.deletes, "scm-page-old") {
			t.Fatalf("multipage deletes %v, want scm-page-old present", fixture.scm.deletes)
		}
	})

	t.Run("scm_delete_failure_and_repair", func(t *testing.T) {
		fixture.scm.seed("scm-pf-a", 20)
		fixture.scm.seed("scm-pf-b", 20)
		fixture.mu.Lock()
		fixture.scm.denyID = "scm-pf-b"
		fixture.mu.Unlock()

		req := request("Purge huawei scm delete failure", "huaweicloud-scm-purger", scmConfig)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}

		fixture.mu.Lock()
		fixture.scm.denyID = ""
		fixture.mu.Unlock()
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		if !contains(fixture.scm.deletes, "scm-pf-a") || !contains(fixture.scm.deletes, "scm-pf-b") {
			t.Fatalf("repair deleted %v", fixture.scm.deletes)
		}
	})

	t.Run("waf_success_unixmilli_and_enterprise_project", func(t *testing.T) {
		fixture.waf.seed("waf-old-30d", 30)
		fixture.waf.seed("waf-old-10d", 10)
		fixture.waf.seed("waf-new-1d", 1)
		fixture.waf.seed("waf-future", -30)
		epsBefore := len(fixture.wafEPSQueries)
		projectsBefore := len(fixture.projectQueries)

		result := run(request("Purge huawei waf success", "huaweicloud-waf-purger", wafConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertDeletes(t, &fixture.waf, []string{"waf-old-30d", "waf-old-10d"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		for _, eps := range fixture.wafEPSQueries[epsBefore:] {
			if eps != purgeHuaweiEPSID {
				t.Fatalf("waf list enterprise_project_id %q, want %q", eps, purgeHuaweiEPSID)
			}
		}
		if len(fixture.projectQueries)-projectsBefore < 1 {
			t.Fatal("waf purge never resolved the region project via keystone")
		}
	})

	t.Run("waf_multipage_page_numbers", func(t *testing.T) {
		for i := 0; i < 120; i++ {
			fixture.waf.seed(fmt.Sprintf("waf-page-fresh-%03d", i), 2)
		}
		fixture.waf.seed("waf-page-old", 30)
		listsBefore := fixture.waf.lists
		pagesBefore := len(fixture.wafPages)

		result := run(request("Purge huawei waf multipage", "huaweicloud-waf-purger", wafConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.waf.lists-listsBefore != 2 {
			t.Fatalf("list calls %d, want 2 (100 per page)", fixture.waf.lists-listsBefore)
		}
		pages := fixture.wafPages[pagesBefore:]
		if len(pages) < 2 || pages[0] != 1 || pages[1] != 2 {
			t.Fatalf("waf pages %v, want [1 2]", pages)
		}
		if !contains(fixture.waf.deletes, "waf-page-old") {
			t.Fatalf("multipage deletes %v, want waf-page-old present", fixture.waf.deletes)
		}
	})

	t.Run("scm_list_failure", func(t *testing.T) {
		fixture.scm.seed("scm-list-fail", 30)
		deletesBefore := len(fixture.scm.deletes)
		fixture.mu.Lock()
		fixture.scm.listFail = true
		fixture.mu.Unlock()
		result := run(request("Purge huawei scm list failure", "huaweicloud-scm-purger", scmConfig))
		fixture.mu.Lock()
		fixture.scm.listFail = false
		fixture.mu.Unlock()
		if result.status != "failed" {
			t.Fatalf("list failure status %s, want failed", result.status)
		}
		if len(fixture.scm.deletes) != deletesBefore {
			t.Fatalf("list failure still deleted %v", fixture.scm.deletes[deletesBefore:])
		}
	})

	t.Run("waf_list_failure", func(t *testing.T) {
		fixture.waf.seed("waf-list-fail", 30)
		deletesBefore := len(fixture.waf.deletes)
		fixture.mu.Lock()
		fixture.waf.listFail = true
		fixture.mu.Unlock()
		result := run(request("Purge huawei waf list failure", "huaweicloud-waf-purger", wafConfig))
		fixture.mu.Lock()
		fixture.waf.listFail = false
		fixture.mu.Unlock()
		if result.status != "failed" {
			t.Fatalf("list failure status %s, want failed", result.status)
		}
		if len(fixture.waf.deletes) != deletesBefore {
			t.Fatalf("list failure still deleted %v", fixture.waf.deletes[deletesBefore:])
		}
	})

	t.Run("waf_delete_failure_and_repair", func(t *testing.T) {
		fixture.waf.seed("waf-pf-a", 20)
		fixture.waf.seed("waf-pf-b", 20)
		fixture.mu.Lock()
		fixture.waf.denyID = "waf-pf-b"
		fixture.mu.Unlock()

		req := request("Purge huawei waf delete failure", "huaweicloud-waf-purger", wafConfig)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}

		fixture.mu.Lock()
		fixture.waf.denyID = ""
		fixture.mu.Unlock()
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		if !contains(fixture.waf.deletes, "waf-pf-a") || !contains(fixture.waf.deletes, "waf-pf-b") {
			t.Fatalf("repair deleted %v", fixture.waf.deletes)
		}
	})
}

func runPurgeHuaweiInDocker(t *testing.T) {
	t.Helper()
	extraArgs := make([]string, 0, len(purgeHuaweiHosts)*2)
	for _, host := range purgeHuaweiHosts {
		extraArgs = append(extraArgs, "--add-host", host+":127.0.0.1")
	}
	runPurgeFamilyInDocker(t, purgeHuaweiPlugins, "purge-huawei.test", "^TestPurgeWorkflowHuawei$", extraArgs...)
}
