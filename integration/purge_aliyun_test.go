package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	purgeAliyunAccessKey = "fixture-purge-aliyun-ak"
	purgeAliyunSecretKey = "fixture-purge-aliyun-sk"
	purgeAliyunRegion    = "cn-qingdao"
	purgeAliyunSiteID    = int64(200)
	purgeAliyunCST       = 8 * 60 * 60
)

var (
	purgeAliyunPlugins = []string{"aliyun-cas-purger", "aliyun-clb-purger", "aliyun-esa-purger"}
	purgeAliyunHosts   = []string{
		"cas.cn-qingdao.aliyuncs.com",
		"slb.cn-qingdao.aliyuncs.com",
		"esa.cn-qingdao.aliyuncs.com",
	}
)

type purgeAliyunCert struct {
	id       string
	notAfter time.Time
}

type purgeAliyunService struct {
	certs    map[string]*purgeAliyunCert
	order    []string
	deletes  []string
	lists    int
	denyID   string
	vanishID string
}

func (s *purgeAliyunService) seed(id string, daysToExpiry float64) {
	if _, exists := s.certs[id]; !exists {
		s.order = append(s.order, id)
	}
	s.certs[id] = &purgeAliyunCert{id: id, notAfter: time.Now().Add(time.Duration(-daysToExpiry * 24 * float64(time.Hour)))}
}

func (s *purgeAliyunService) live() []*purgeAliyunCert {
	items := make([]*purgeAliyunCert, 0, len(s.order))
	for _, id := range s.order {
		if cert, exists := s.certs[id]; exists {
			items = append(items, cert)
		}
	}
	return items
}

func (s *purgeAliyunService) remaining() []string {
	ids := make([]string, 0, len(s.certs))
	for _, id := range s.order {
		if _, exists := s.certs[id]; exists {
			ids = append(ids, id)
		}
	}
	return ids
}

type purgeAliyunFixture struct {
	mu   sync.Mutex
	mode string

	cas purgeAliyunService
	slb purgeAliyunService
	esa map[int64]*purgeAliyunService

	slbRegions []string
	esaSites   []int64
}

func newPurgeAliyunFixture(t *testing.T) *purgeAliyunFixture {
	t.Helper()
	f := &purgeAliyunFixture{
		cas: purgeAliyunService{certs: map[string]*purgeAliyunCert{}},
		slb: purgeAliyunService{certs: map[string]*purgeAliyunCert{}},
		esa: map[int64]*purgeAliyunService{},
	}
	newB4CloudProxy(t, purgeAliyunHosts, http.HandlerFunc(f.serve))
	return f
}

func (f *purgeAliyunFixture) esaSite(siteID int64) *purgeAliyunService {
	service, exists := f.esa[siteID]
	if !exists {
		service = &purgeAliyunService{certs: map[string]*purgeAliyunCert{}}
		f.esa[siteID] = service
	}
	return service
}

func (f *purgeAliyunFixture) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

func (f *purgeAliyunFixture) writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (f *purgeAliyunFixture) fail(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	f.writeJSON(w, map[string]any{"Code": code, "Message": message, "RequestId": "fixture"})
}

func (f *purgeAliyunFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !strings.HasPrefix(r.Header.Get("Authorization"), "ACS3-HMAC-SHA256 Credential=") {
		f.fail(w, http.StatusUnauthorized, "InvalidAccessKeyId.NotFound", "unexpected authorization scheme; authorization="+r.Header.Get("Authorization"))
		return
	}
	if f.mode == "auth-failure" {
		f.fail(w, http.StatusForbidden, "SignatureDoesNotMatch", "fixture auth-failure")
		return
	}

	query := r.URL.Query()
	if action := r.Header.Get("x-acs-action"); action != "" {
		query.Set("Action", action)
	}
	switch r.Host {
	case "cas.cn-qingdao.aliyuncs.com":
		f.serveCAS(w, query)
	case "slb.cn-qingdao.aliyuncs.com":
		f.serveSLB(w, query)
	case "esa.cn-qingdao.aliyuncs.com":
		f.serveESA(w, query)
	default:
		f.fail(w, http.StatusNotFound, "UnexpectedRequest", fmt.Sprintf("unexpected fixture host %s", r.Host))
	}
}

func (f *purgeAliyunFixture) serveCAS(w http.ResponseWriter, query url.Values) {
	switch query.Get("Action") {
	case "ListCertificates":
		if got := query.Get("CertificateStatus"); got != "expired" {
			f.fail(w, http.StatusBadRequest, "InvalidParameter", "CertificateStatus "+got)
			return
		}
		page, _ := strconv.Atoi(query.Get("CurrentPage"))
		size, _ := strconv.Atoi(query.Get("ShowSize"))
		if page < 1 {
			page = 1
		}
		if size < 1 {
			size = 50
		}
		f.cas.lists++
		live := f.cas.live()
		start := (page - 1) * size
		if start > len(live) {
			start = len(live)
		}
		end := start + size
		if end > len(live) {
			end = len(live)
		}
		items := make([]map[string]any, 0, end-start)
		for _, cert := range live[start:end] {
			items = append(items, map[string]any{
				"CertificateId": cert.id,
				"NotAfter":      cert.notAfter.UnixMilli(),
			})
		}
		f.writeJSON(w, map[string]any{"CertificateList": items, "TotalCount": len(live), "CurrentPage": page, "ShowSize": size, "RequestId": "fixture"})
	case "DeleteUserCertificate":
		id := query.Get("CertId")
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			f.fail(w, http.StatusBadRequest, "InvalidParameter", "CertId must be numeric")
			return
		}
		cert, exists := f.cas.certs[id]
		if !exists || cert == nil {
			f.fail(w, http.StatusNotFound, "NotFound.CertId", "unknown certificate")
			return
		}
		if f.cas.vanishID == id {
			f.fail(w, http.StatusNotFound, "NotFound.CertId", "certificate vanished between list and delete")
			return
		}
		if f.cas.denyID == id {
			f.fail(w, http.StatusForbidden, "Forbidden.RAM", "fixture delete denied")
			return
		}
		delete(f.cas.certs, id)
		f.cas.deletes = append(f.cas.deletes, id)
		f.writeJSON(w, map[string]any{"RequestId": "fixture"})
	default:
		f.fail(w, http.StatusNotFound, "UnexpectedRequest", "unexpected CAS action "+query.Get("Action"))
	}
}

func (f *purgeAliyunFixture) serveSLB(w http.ResponseWriter, query url.Values) {
	switch query.Get("Action") {
	case "DescribeServerCertificates":
		if got := query.Get("RegionId"); got != purgeAliyunRegion {
			f.fail(w, http.StatusBadRequest, "InvalidParameter", "RegionId "+got)
			return
		}
		f.slbRegions = append(f.slbRegions, query.Get("RegionId"))
		f.slb.lists++
		items := make([]map[string]any, 0, len(f.slb.order))
		for _, cert := range f.slb.live() {
			items = append(items, map[string]any{
				"ServerCertificateId": cert.id,
				"ExpireTimeStamp":     cert.notAfter.Unix(),
			})
		}
		f.writeJSON(w, map[string]any{
			"ServerCertificates": map[string]any{"ServerCertificate": items},
			"RequestId":          "fixture",
		})
	case "DeleteServerCertificate":
		id := query.Get("ServerCertificateId")
		if _, exists := f.slb.certs[id]; !exists {
			f.fail(w, http.StatusNotFound, "ServerCertificateId.NotFound", "unknown certificate")
			return
		}
		if f.slb.vanishID == id {
			f.fail(w, http.StatusNotFound, "ServerCertificateId.NotFound", "certificate vanished between list and delete")
			return
		}
		if f.slb.denyID == id {
			f.fail(w, http.StatusForbidden, "Forbidden.RAM", "fixture delete denied")
			return
		}
		delete(f.slb.certs, id)
		f.slb.deletes = append(f.slb.deletes, id)
		f.writeJSON(w, map[string]any{"RequestId": "fixture"})
	default:
		f.fail(w, http.StatusNotFound, "UnexpectedRequest", "unexpected SLB action "+query.Get("Action"))
	}
}

func (f *purgeAliyunFixture) serveESA(w http.ResponseWriter, query url.Values) {
	siteID, _ := strconv.ParseInt(query.Get("SiteId"), 10, 64)
	service := f.esaSite(siteID)
	switch query.Get("Action") {
	case "ListCertificates":
		if siteID == 0 {
			f.fail(w, http.StatusBadRequest, "InvalidParameter", "SiteId required")
			return
		}
		f.esaSites = append(f.esaSites, siteID)
		page, _ := strconv.Atoi(query.Get("PageNumber"))
		size, _ := strconv.Atoi(query.Get("PageSize"))
		if page < 1 {
			page = 1
		}
		if size < 1 {
			size = 10
		}
		service.lists++
		live := service.live()
		start := (page - 1) * size
		if start > len(live) {
			start = len(live)
		}
		end := start + size
		if end > len(live) {
			end = len(live)
		}
		items := make([]map[string]any, 0, end-start)
		for _, cert := range live[start:end] {
			items = append(items, map[string]any{
				"Id":       cert.id,
				"NotAfter": cert.notAfter.In(time.FixedZone("CST", purgeAliyunCST)).Format(time.DateTime),
			})
		}
		f.writeJSON(w, map[string]any{"Result": items, "TotalCount": len(live), "PageNumber": page, "PageSize": size, "SiteId": siteID, "RequestId": "fixture"})
	case "DeleteCertificate":
		if siteID == 0 {
			f.fail(w, http.StatusBadRequest, "InvalidParameter", "SiteId required")
			return
		}
		id := query.Get("Id")
		if _, exists := service.certs[id]; !exists {
			f.fail(w, http.StatusNotFound, "Certificate.NotFound", "unknown certificate for site")
			return
		}
		if service.vanishID == id {
			f.fail(w, http.StatusNotFound, "Certificate.NotFound", "certificate vanished between list and delete")
			return
		}
		if service.denyID == id {
			f.fail(w, http.StatusForbidden, "Forbidden.RAM", "fixture delete denied")
			return
		}
		delete(service.certs, id)
		service.deletes = append(service.deletes, id)
		f.writeJSON(w, map[string]any{"RequestId": "fixture"})
	default:
		f.fail(w, http.StatusNotFound, "UnexpectedRequest", "unexpected ESA action "+query.Get("Action"))
	}
}

func TestPurgeWorkflowAliyun(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runPurgeAliyunInDocker(t)
		return
	}
	runPurgeAliyunScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runPurgeAliyunScenarios(t *testing.T, binaryDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fixture := newPurgeAliyunFixture(t)
	core := startPurgeCore(t, binaryDir, purgeAliyunPlugins)

	for _, provider := range purgeAliyunPlugins {
		core.assertCatalogEntry(provider, "purge", true)
	}
	if envelope, err := core.fetchSchemaEnvelope("aliyun-esa-purger", "purge"); err != nil {
		t.Fatalf("purge schema endpoint: %v", err)
	} else if envelope["provider"] != "aliyun-esa-purger" || envelope["category"] != "purge" {
		t.Fatalf("unexpected purge schema envelope: %v", envelope)
	}

	accessConfig := map[string]any{"accessKeyId": purgeAliyunAccessKey, "accessKeySecret": purgeAliyunSecretKey}
	accessID := core.createAccess(ctx, "aliyun", accessConfig)
	secrets := []string{purgeAliyunSecretKey}

	request := func(name, provider string, providerConfig map[string]any, expiredDays float64) purgeWorkflowRequest {
		return purgeWorkflowRequest{
			name:           name,
			provider:       provider,
			accessID:       accessID,
			accessProvider: "aliyun",
			accessConfig:   accessConfig,
			providerConfig: providerConfig,
			expiredDays:    expiredDays,
		}
	}
	run := func(req purgeWorkflowRequest) purgeWorkflowResult {
		return core.runPurgeWorkflow(ctx, req, secrets...)
	}
	assertState := func(t *testing.T, service *purgeAliyunService, wantDeletes []string, wantRemaining []string) {
		t.Helper()
		if strings.Join(service.deletes, ",") != strings.Join(wantDeletes, ",") {
			t.Fatalf("deleted certificates %v, want %v", service.deletes, wantDeletes)
		}
		got := service.remaining()
		sort.Strings(got)
		sorted := append([]string{}, wantRemaining...)
		sort.Strings(sorted)
		if strings.Join(got, ",") != strings.Join(sorted, ",") {
			t.Fatalf("remaining certificates %v, want %v", got, sorted)
		}
	}

	casConfig := map[string]any{"region": purgeAliyunRegion}
	clbConfig := map[string]any{"region": purgeAliyunRegion}
	esaConfig := map[string]any{"region": purgeAliyunRegion, "siteId": purgeAliyunSiteID}

	t.Run("cas_success_ms_dates_threshold", func(t *testing.T) {
		fixture.cas.seed("1001", 30)
		fixture.cas.seed("1002", 10)
		fixture.cas.seed("1003", 2)
		fixture.cas.seed("1004", -30)

		result := run(request("Purge aliyun cas success", "aliyun-cas-purger", casConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertState(t, &fixture.cas, []string{"1001", "1002"}, []string{"1003", "1004"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
	})

	t.Run("cas_multipage_list", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			fixture.cas.seed(fmt.Sprintf("20%02d", i+1), 2)
		}
		fixture.cas.seed("2101", 30)
		fixture.cas.seed("2102", 30)
		listsBefore := fixture.cas.lists

		result := run(request("Purge aliyun cas multipage", "aliyun-cas-purger", casConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.cas.lists-listsBefore != 2 {
			t.Fatalf("list calls %d, want 2 (50 per page)", fixture.cas.lists-listsBefore)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		if !contains(fixture.cas.deletes, "2101") || !contains(fixture.cas.deletes, "2102") {
			t.Fatalf("multipage deletes %v, want the two expired certificates from page 2", fixture.cas.deletes)
		}
	})

	t.Run("cas_auth_failure", func(t *testing.T) {
		fixture.cas.seed("1101", 30)
		deletesBefore := len(fixture.cas.deletes)
		fixture.setMode("auth-failure")
		result := run(request("Purge aliyun cas auth failure", "aliyun-cas-purger", casConfig, 7))
		fixture.setMode("")
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
		if len(fixture.cas.deletes) != deletesBefore {
			t.Fatalf("auth failure still deleted %v", fixture.cas.deletes[deletesBefore:])
		}
	})

	t.Run("cas_delete_failure_and_repair", func(t *testing.T) {
		fixture.cas.seed("1201", 20)
		fixture.cas.seed("1202", 20)
		fixture.cas.denyID = "1202"

		req := request("Purge aliyun cas delete failure", "aliyun-cas-purger", casConfig, 7)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}
		if !contains(fixture.cas.deletes, "1201") || contains(fixture.cas.deletes, "1202") {
			t.Fatalf("partial failure deleted %v", fixture.cas.deletes)
		}

		fixture.cas.denyID = ""
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1 (no double counting)", count, ok)
		}
		if strings.Count(strings.Join(fixture.cas.deletes, ","), "1201") != 1 || !contains(fixture.cas.deletes, "1202") {
			t.Fatalf("repair deleted %v", fixture.cas.deletes)
		}
	})

	t.Run("cas_notfound_skips", func(t *testing.T) {
		fixture.cas.seed("1301", 15)
		fixture.cas.vanishID = "1301"

		result := run(request("Purge aliyun cas notfound", "aliyun-cas-purger", casConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("notfound log count=%d found=%t, want 0", count, ok)
		}
	})

	t.Run("clb_success_seconds_dates_threshold", func(t *testing.T) {
		fixture.slb.seed("clb-old-30d", 30)
		fixture.slb.seed("clb-old-9d", 9)
		fixture.slb.seed("clb-new-1d", 1)
		fixture.slb.seed("clb-future", -30)

		result := run(request("Purge aliyun clb success", "aliyun-clb-purger", clbConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertState(t, &fixture.slb, []string{"clb-old-30d", "clb-old-9d"}, []string{"clb-new-1d", "clb-future"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		for _, region := range fixture.slbRegions {
			if region != purgeAliyunRegion {
				t.Fatalf("clb request carried region %q, want %q", region, purgeAliyunRegion)
			}
		}
	})

	t.Run("clb_auth_failure", func(t *testing.T) {
		fixture.slb.seed("clb-auth", 30)
		deletesBefore := len(fixture.slb.deletes)
		fixture.setMode("auth-failure")
		result := run(request("Purge aliyun clb auth failure", "aliyun-clb-purger", clbConfig, 7))
		fixture.setMode("")
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
		if len(fixture.slb.deletes) != deletesBefore {
			t.Fatalf("auth failure still deleted %v", fixture.slb.deletes[deletesBefore:])
		}
	})

	t.Run("clb_delete_failure_and_repair", func(t *testing.T) {
		fixture.slb.seed("clb-pf-a", 20)
		fixture.slb.seed("clb-pf-b", 20)
		fixture.slb.denyID = "clb-pf-b"

		req := request("Purge aliyun clb delete failure", "aliyun-clb-purger", clbConfig, 7)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}

		fixture.slb.denyID = ""
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		if strings.Count(strings.Join(fixture.slb.deletes, ","), "clb-pf-a") != 1 || !contains(fixture.slb.deletes, "clb-pf-b") {
			t.Fatalf("repair deleted %v", fixture.slb.deletes)
		}
	})

	t.Run("clb_notfound_skips", func(t *testing.T) {
		fixture.slb.seed("clb-vanish", 15)
		fixture.slb.vanishID = "clb-vanish"

		result := run(request("Purge aliyun clb notfound", "aliyun-clb-purger", clbConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("notfound log count=%d found=%t, want 0", count, ok)
		}
	})

	t.Run("esa_success_cst_dates_threshold_and_site_scope", func(t *testing.T) {
		main := fixture.esaSite(purgeAliyunSiteID)
		main.seed("esa-old-30d", 30)
		main.seed("esa-old-8d", 8)
		main.seed("esa-new-1d", 1)
		main.seed("esa-future", -30)

		foreign := fixture.esaSite(999)
		foreign.seed("esa-foreign-20d", 20)
		sitesBefore := len(fixture.esaSites)

		result := run(request("Purge aliyun esa success", "aliyun-esa-purger", esaConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertState(t, main, []string{"esa-old-30d", "esa-old-8d"}, []string{"esa-new-1d", "esa-future"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		for _, site := range fixture.esaSites[sitesBefore:] {
			if site != purgeAliyunSiteID {
				t.Fatalf("esa list touched foreign site %d", site)
			}
		}
		if !contains(foreign.remaining(), "esa-foreign-20d") {
			t.Fatal("esa purged a certificate on a foreign site")
		}
		if foreign.lists != 0 {
			t.Fatalf("foreign site was listed %d times", foreign.lists)
		}
	})

	t.Run("esa_multipage_list", func(t *testing.T) {
		main := fixture.esaSite(purgeAliyunSiteID)
		for i := 0; i < 10; i++ {
			main.seed(fmt.Sprintf("esa-p1-%d", i), 2)
		}
		main.seed("esa-p2-old", 30)
		main.seed("esa-p2-new", 2)
		listsBefore := main.lists

		result := run(request("Purge aliyun esa multipage", "aliyun-esa-purger", esaConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if main.lists-listsBefore != 2 {
			t.Fatalf("list calls %d, want 2 (10 per page)", main.lists-listsBefore)
		}
		if !contains(main.deletes, "esa-p2-old") {
			t.Fatalf("multipage deletes %v", main.deletes)
		}
	})

	t.Run("esa_missing_siteId_fails", func(t *testing.T) {
		result := run(request("Purge aliyun esa missing site", "aliyun-esa-purger", map[string]any{"region": purgeAliyunRegion}, 7))
		if result.status != "failed" {
			t.Fatalf("missing siteId status %s, want failed", result.status)
		}
		if !strings.Contains(result.runError, "siteId") {
			t.Fatalf("missing siteId error does not explain the cause: %s", result.runError)
		}
	})

	t.Run("esa_auth_failure", func(t *testing.T) {
		fixture.esaSite(purgeAliyunSiteID).seed("esa-auth", 30)
		fixture.setMode("auth-failure")
		result := run(request("Purge aliyun esa auth failure", "aliyun-esa-purger", esaConfig, 7))
		fixture.setMode("")
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
	})

	t.Run("esa_delete_failure_and_repair", func(t *testing.T) {
		main := fixture.esaSite(purgeAliyunSiteID)
		main.seed("esa-pf-a", 20)
		main.seed("esa-pf-b", 20)
		main.denyID = "esa-pf-b"

		req := request("Purge aliyun esa delete failure", "aliyun-esa-purger", esaConfig, 7)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}

		main.denyID = ""
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		if !contains(main.deletes, "esa-pf-a") || !contains(main.deletes, "esa-pf-b") {
			t.Fatalf("repair deleted %v", main.deletes)
		}
	})

	t.Run("esa_notfound_skips", func(t *testing.T) {
		main := fixture.esaSite(purgeAliyunSiteID)
		main.seed("esa-vanish", 15)
		main.vanishID = "esa-vanish"

		result := run(request("Purge aliyun esa notfound", "aliyun-esa-purger", esaConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("notfound log count=%d found=%t, want 0", count, ok)
		}
	})
}

func runPurgeAliyunInDocker(t *testing.T) {
	t.Helper()
	runPurgeFamilyInDocker(t, purgeAliyunPlugins, "purge-aliyun.test", "^TestPurgeWorkflowAliyun$")
}
