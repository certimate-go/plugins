package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	purgeTencentSecretID   = "fixture-purge-tc-id"
	purgeTencentSecretKey  = "fixture-purge-tc-key"
	purgeTencentProjectID  = uint64(2000)
	purgeTencentTSERegion  = "ap-guangzhou"
	purgeTencentTSEGateway = "gw-fixture-main"
	purgeTencentCST        = 8 * 60 * 60
)

var (
	purgeTencentPlugins = []string{"tencentcloud-ssl-purger", "tencentcloud-gaap-purger", "tencentcloud-tse-purger"}
	purgeTencentHosts   = []string{
		"custom-ssl.endpoint.test",
		"custom-gaap.endpoint.test",
		"custom-tse.endpoint.test",
	}
)

type purgeTencentCert struct {
	id       string
	notAfter time.Time
}

func (c *purgeTencentCert) cstDateTime() string {
	return c.notAfter.In(time.FixedZone("CST", purgeTencentCST)).Format(time.DateTime)
}

type purgeTencentService struct {
	certs    map[string]*purgeTencentCert
	order    []string
	deletes  []string
	lists    int
	denyID   string
	vanishID string
	boundID  string
}

func (s *purgeTencentService) seed(id string, daysToExpiry float64) {
	if _, exists := s.certs[id]; !exists {
		s.order = append(s.order, id)
	}
	s.certs[id] = &purgeTencentCert{id: id, notAfter: time.Now().Add(time.Duration(-daysToExpiry * 24 * float64(time.Hour)))}
}

func (s *purgeTencentService) live() []*purgeTencentCert {
	items := make([]*purgeTencentCert, 0, len(s.order))
	for _, id := range s.order {
		if cert, exists := s.certs[id]; exists {
			items = append(items, cert)
		}
	}
	return items
}

func (s *purgeTencentService) remaining() []string {
	ids := make([]string, 0, len(s.certs))
	for _, id := range s.order {
		if _, exists := s.certs[id]; exists {
			ids = append(ids, id)
		}
	}
	return ids
}

type purgeTencentFixture struct {
	mu   sync.Mutex
	mode string

	ssl        purgeTencentService
	gaap       purgeTencentService
	tse        map[string]*purgeTencentService
	tseRegions []string
	sslProject []uint64
}

func newPurgeTencentFixture(t *testing.T) *purgeTencentFixture {
	t.Helper()
	f := &purgeTencentFixture{
		ssl:  purgeTencentService{certs: map[string]*purgeTencentCert{}},
		gaap: purgeTencentService{certs: map[string]*purgeTencentCert{}},
		tse:  map[string]*purgeTencentService{},
	}
	newB4CloudProxy(t, purgeTencentHosts, http.HandlerFunc(f.serve))
	return f
}

func (f *purgeTencentFixture) tseGateway(gatewayID string) *purgeTencentService {
	service, exists := f.tse[gatewayID]
	if !exists {
		service = &purgeTencentService{certs: map[string]*purgeTencentCert{}}
		f.tse[gatewayID] = service
	}
	return service
}

func (f *purgeTencentFixture) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

func (f *purgeTencentFixture) writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (f *purgeTencentFixture) apiError(w http.ResponseWriter, code, message string) {
	f.writeJSON(w, map[string]any{"Response": map[string]any{"Error": map[string]string{"Code": code, "Message": message}, "RequestId": "fixture"}})
}

func (f *purgeTencentFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !strings.Contains(r.Header.Get("Authorization"), "Credential="+purgeTencentSecretID+"/") {
		f.apiError(w, "AuthFailure.InvalidSecretId", "unexpected secret id")
		return
	}
	if f.mode == "auth-failure" {
		f.apiError(w, "AuthFailure.SignatureFailure", "fixture auth-failure")
		return
	}

	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	bodyString := func(key string) string {
		if value, ok := body[key].(string); ok {
			return value
		}
		return ""
	}
	bodyFloat := func(key string) float64 {
		if value, ok := body[key].(float64); ok {
			return value
		}
		return 0
	}
	bodyFloats := func(key string) []float64 {
		raw, ok := body[key].([]any)
		if !ok {
			return nil
		}
		values := make([]float64, 0, len(raw))
		for _, item := range raw {
			if value, ok := item.(float64); ok {
				values = append(values, value)
			}
		}
		return values
	}

	switch r.Host {
	case "custom-ssl.endpoint.test":
		f.serveSSL(w, r.Header.Get("X-TC-Action"), bodyString, bodyFloat, bodyFloats)
	case "custom-gaap.endpoint.test":
		f.serveGAAP(w, r.Header.Get("X-TC-Action"), bodyString, bodyFloat)
	case "custom-tse.endpoint.test":
		f.serveTSE(w, r.Header.Get("X-TC-Action"), r.Header.Get("X-TC-Region"), bodyString, bodyFloat)
	default:
		f.apiError(w, "InvalidEndpoint", fmt.Sprintf("unexpected fixture host %s", r.Host))
	}
}

func (f *purgeTencentFixture) page(live []*purgeTencentCert, offset, limit float64) []*purgeTencentCert {
	if limit < 1 {
		limit = 100
	}
	start := int(offset)
	if start > len(live) {
		start = len(live)
	}
	end := start + int(limit)
	if end > len(live) {
		end = len(live)
	}
	return live[start:end]
}

func (f *purgeTencentFixture) deleteCertificate(w http.ResponseWriter, service *purgeTencentService, id string, notFoundCode, boundCode, denyCode string) {
	if _, exists := service.certs[id]; !exists {
		f.apiError(w, notFoundCode, "unknown certificate")
		return
	}
	if service.vanishID == id {
		f.apiError(w, notFoundCode, "certificate vanished between list and delete")
		return
	}
	if service.boundID == id {
		f.apiError(w, boundCode, "certificate is bound")
		return
	}
	if service.denyID == id {
		f.apiError(w, denyCode, "fixture delete denied")
		return
	}
	delete(service.certs, id)
	service.deletes = append(service.deletes, id)
	f.writeJSON(w, map[string]any{"Response": map[string]any{"RequestId": "fixture"}})
}

func (f *purgeTencentFixture) serveSSL(w http.ResponseWriter, action string, bodyString func(string) string, bodyFloat func(string) float64, bodyFloats func(string) []float64) {
	switch action {
	case "DescribeCertificates":
		if certificateType := bodyString("CertificateType"); certificateType != "SVR" {
			f.apiError(w, "InvalidParameter", "CertificateType "+certificateType)
			return
		}
		if isSM := bodyFloat("IsSM"); isSM != 0 {
			f.apiError(w, "InvalidParameter", "IsSM must be 0")
			return
		}
		if statuses := bodyFloats("CertificateStatus"); len(statuses) != 1 || statuses[0] != 3 {
			f.apiError(w, "InvalidParameter", fmt.Sprintf("CertificateStatus %v, want [3]", statuses))
			return
		}
		f.sslProject = append(f.sslProject, uint64(bodyFloat("ProjectId")))
		f.ssl.lists++
		items := make([]map[string]any, 0)
		for _, cert := range f.page(f.ssl.live(), bodyFloat("Offset"), bodyFloat("Limit")) {
			items = append(items, map[string]any{"CertificateId": cert.id, "CertEndTime": cert.cstDateTime()})
		}
		f.writeJSON(w, map[string]any{"Response": map[string]any{"Certificates": items, "TotalCount": len(f.ssl.order), "RequestId": "fixture"}})
	case "DeleteCertificate":
		f.deleteCertificate(w, &f.ssl, bodyString("CertificateId"), "FailedOperation.CertificateNotFound", "FailedOperation.BoundResources", "InternalError.RequestError")
	default:
		f.apiError(w, "InvalidAction", "unexpected ssl action "+action)
	}
}

func (f *purgeTencentFixture) serveGAAP(w http.ResponseWriter, action string, bodyString func(string) string, bodyFloat func(string) float64) {
	switch action {
	case "DescribeCertificates":
		if certificateType := bodyFloat("CertificateType"); certificateType != 2 {
			f.apiError(w, "InvalidParameter", "CertificateType must be 2")
			return
		}
		f.gaap.lists++
		items := make([]map[string]any, 0)
		for _, cert := range f.page(f.gaap.live(), bodyFloat("Offset"), bodyFloat("Limit")) {
			items = append(items, map[string]any{"CertificateId": cert.id, "EndTime": cert.notAfter.Unix()})
		}
		f.writeJSON(w, map[string]any{"Response": map[string]any{"CertificateSet": items, "TotalCount": len(f.gaap.order), "RequestId": "fixture"}})
	case "DeleteCertificate":
		f.deleteCertificate(w, &f.gaap, bodyString("CertificateId"), "ResourceNotFound", "FailedOperation.CertificateIsUsing", "InternalError.RequestError")
	default:
		f.apiError(w, "InvalidAction", "unexpected gaap action "+action)
	}
}

func (f *purgeTencentFixture) serveTSE(w http.ResponseWriter, action, region string, bodyString func(string) string, bodyFloat func(string) float64) {
	gatewayID := bodyString("GatewayId")
	service := f.tseGateway(gatewayID)
	switch action {
	case "DescribeCloudNativeAPIGatewayCertificates":
		if gatewayID == "" {
			f.apiError(w, "InvalidParameter", "GatewayId required")
			return
		}
		f.tseRegions = append(f.tseRegions, region)
		service.lists++
		items := make([]map[string]any, 0)
		for _, cert := range f.page(service.live(), bodyFloat("Offset"), bodyFloat("Limit")) {
			items = append(items, map[string]any{"Id": cert.id, "CertId": cert.id, "ExpireTime": cert.cstDateTime()})
		}
		f.writeJSON(w, map[string]any{"Response": map[string]any{
			"Result":    map[string]any{"Total": len(service.order), "CertificatesList": items},
			"RequestId": "fixture",
		}})
	case "DeleteCloudNativeAPIGatewayCertificate":
		f.deleteCertificate(w, service, bodyString("Id"), "ResourceNotFound.ResourceNotFound", "FailedOperation.CertificateIsUsing", "InternalError.RequestError")
	default:
		f.apiError(w, "InvalidAction", "unexpected tse action "+action)
	}
}

func bodyAnyFloats(bodyFloat func(string) float64, key string) ([]float64, bool) {
	_ = bodyFloat
	_ = key
	return nil, false
}

func TestPurgeWorkflowTencentCloud(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runPurgeTencentInDocker(t)
		return
	}
	runPurgeTencentScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runPurgeTencentScenarios(t *testing.T, binaryDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fixture := newPurgeTencentFixture(t)
	core := startPurgeCore(t, binaryDir, purgeTencentPlugins)

	for _, provider := range purgeTencentPlugins {
		core.assertCatalogEntry(provider, "purge", true)
	}
	if envelope, err := core.fetchSchemaEnvelope("tencentcloud-tse-purger", "purge"); err != nil {
		t.Fatalf("purge schema endpoint: %v", err)
	} else if envelope["provider"] != "tencentcloud-tse-purger" || envelope["category"] != "purge" {
		t.Fatalf("unexpected purge schema envelope: %v", envelope)
	}

	accessConfig := map[string]any{"secretId": purgeTencentSecretID, "secretKey": purgeTencentSecretKey, "projectId": purgeTencentProjectID}
	accessID := core.createAccess(ctx, "tencentcloud", accessConfig)
	secrets := []string{purgeTencentSecretKey}

	request := func(name, provider string, providerConfig map[string]any, expiredDays float64) purgeWorkflowRequest {
		return purgeWorkflowRequest{
			name:           name,
			provider:       provider,
			accessID:       accessID,
			accessProvider: "tencentcloud",
			accessConfig:   accessConfig,
			providerConfig: providerConfig,
			expiredDays:    expiredDays,
		}
	}
	run := func(req purgeWorkflowRequest) purgeWorkflowResult {
		return core.runPurgeWorkflow(ctx, req, secrets...)
	}
	assertState := func(t *testing.T, service *purgeTencentService, wantDeletes []string, wantRemaining []string) {
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

	sslConfig := map[string]any{"endpoint": "custom-ssl.endpoint.test"}
	gaapConfig := map[string]any{"endpoint": "custom-gaap.endpoint.test"}
	tseConfig := map[string]any{"endpoint": "custom-tse.endpoint.test", "region": purgeTencentTSERegion, "serviceType": "cloudnative", "gatewayId": purgeTencentTSEGateway}

	t.Run("ssl_success_cst_dates_threshold", func(t *testing.T) {
		fixture.ssl.seed("ssl-old-30d", 30)
		fixture.ssl.seed("ssl-old-9d", 9)
		fixture.ssl.seed("ssl-new-2d", 2)
		fixture.ssl.seed("ssl-future", -30)
		projectsBefore := len(fixture.sslProject)

		result := run(request("Purge tencent ssl success", "tencentcloud-ssl-purger", sslConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertState(t, &fixture.ssl, []string{"ssl-old-30d", "ssl-old-9d"}, []string{"ssl-new-2d", "ssl-future"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		for _, project := range fixture.sslProject[projectsBefore:] {
			if project != purgeTencentProjectID {
				t.Fatalf("ssl request carried projectId %d, want %d", project, purgeTencentProjectID)
			}
		}
	})

	t.Run("ssl_multipage_list", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fixture.ssl.seed(fmt.Sprintf("ssl-p1-%03d", i), 2)
		}
		fixture.ssl.seed("ssl-p2-old", 30)
		listsBefore := fixture.ssl.lists

		result := run(request("Purge tencent ssl multipage", "tencentcloud-ssl-purger", sslConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.ssl.lists-listsBefore != 2 {
			t.Fatalf("list calls %d, want 2 (100 per page)", fixture.ssl.lists-listsBefore)
		}
		if !contains(fixture.ssl.deletes, "ssl-p2-old") {
			t.Fatalf("multipage deletes %v, want ssl-p2-old from page 2", fixture.ssl.deletes)
		}
	})

	t.Run("ssl_bound_skips", func(t *testing.T) {
		fixture.ssl.seed("ssl-bound-12d", 12)
		fixture.ssl.seed("ssl-deletable-12d", 12)
		fixture.ssl.boundID = "ssl-bound-12d"

		result := run(request("Purge tencent ssl bound", "tencentcloud-ssl-purger", sslConfig, 7))
		fixture.ssl.boundID = ""
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 1 {
			t.Fatalf("bound-skip log count=%d found=%t, want 1", count, ok)
		}
		if !contains(fixture.ssl.deletes, "ssl-deletable-12d") {
			t.Fatalf("deletable certificate was not purged: %v", fixture.ssl.deletes)
		}
		if !contains(fixture.ssl.remaining(), "ssl-bound-12d") {
			t.Fatal("bound certificate was removed despite the skip error")
		}
	})

	t.Run("ssl_auth_failure", func(t *testing.T) {
		fixture.ssl.seed("ssl-auth", 30)
		deletesBefore := len(fixture.ssl.deletes)
		fixture.setMode("auth-failure")
		result := run(request("Purge tencent ssl auth failure", "tencentcloud-ssl-purger", sslConfig, 7))
		fixture.setMode("")
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
		if len(fixture.ssl.deletes) != deletesBefore {
			t.Fatalf("auth failure still deleted %v", fixture.ssl.deletes[deletesBefore:])
		}
	})

	t.Run("ssl_delete_failure_and_repair", func(t *testing.T) {
		fixture.ssl.seed("ssl-pf-a", 20)
		fixture.ssl.seed("ssl-pf-b", 20)
		fixture.ssl.denyID = "ssl-pf-b"

		req := request("Purge tencent ssl delete failure", "tencentcloud-ssl-purger", sslConfig, 7)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}

		fixture.ssl.denyID = ""
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		if strings.Count(strings.Join(fixture.ssl.deletes, ","), "ssl-pf-a") != 1 || !contains(fixture.ssl.deletes, "ssl-pf-b") {
			t.Fatalf("repair deleted %v", fixture.ssl.deletes)
		}
	})

	t.Run("ssl_notfound_skips", func(t *testing.T) {
		fixture.ssl.seed("ssl-vanish", 15)
		fixture.ssl.vanishID = "ssl-vanish"

		result := run(request("Purge tencent ssl notfound", "tencentcloud-ssl-purger", sslConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("notfound log count=%d found=%t, want 0", count, ok)
		}
	})

	t.Run("gaap_success_unix_seconds_threshold", func(t *testing.T) {
		fixture.gaap.seed("gaap-old-30d", 30)
		fixture.gaap.seed("gaap-old-10d", 10)
		fixture.gaap.seed("gaap-new-2d", 2)
		fixture.gaap.seed("gaap-future", -30)

		result := run(request("Purge tencent gaap success", "tencentcloud-gaap-purger", gaapConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertState(t, &fixture.gaap, []string{"gaap-old-30d", "gaap-old-10d"}, []string{"gaap-new-2d", "gaap-future"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
	})

	t.Run("gaap_in_use_skips", func(t *testing.T) {
		fixture.gaap.seed("gaap-inuse-12d", 12)
		fixture.gaap.seed("gaap-deletable-12d", 12)
		fixture.gaap.boundID = "gaap-inuse-12d"

		result := run(request("Purge tencent gaap in use", "tencentcloud-gaap-purger", gaapConfig, 7))
		fixture.gaap.boundID = ""
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 1 {
			t.Fatalf("in-use-skip log count=%d found=%t, want 1", count, ok)
		}
		if !contains(fixture.gaap.remaining(), "gaap-inuse-12d") {
			t.Fatal("in-use certificate was removed despite the skip error")
		}
	})

	t.Run("gaap_auth_failure", func(t *testing.T) {
		fixture.gaap.seed("gaap-auth", 30)
		fixture.setMode("auth-failure")
		result := run(request("Purge tencent gaap auth failure", "tencentcloud-gaap-purger", gaapConfig, 7))
		fixture.setMode("")
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
	})

	t.Run("gaap_delete_failure_and_repair", func(t *testing.T) {
		fixture.gaap.seed("gaap-pf-a", 20)
		fixture.gaap.seed("gaap-pf-b", 20)
		fixture.gaap.denyID = "gaap-pf-b"

		req := request("Purge tencent gaap delete failure", "tencentcloud-gaap-purger", gaapConfig, 7)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}

		fixture.gaap.denyID = ""
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		if strings.Count(strings.Join(fixture.gaap.deletes, ","), "gaap-pf-a") != 1 || !contains(fixture.gaap.deletes, "gaap-pf-b") {
			t.Fatalf("repair deleted %v", fixture.gaap.deletes)
		}
	})

	t.Run("gaap_notfound_skips", func(t *testing.T) {
		fixture.gaap.seed("gaap-vanish", 15)
		fixture.gaap.vanishID = "gaap-vanish"

		result := run(request("Purge tencent gaap notfound", "tencentcloud-gaap-purger", gaapConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("notfound log count=%d found=%t, want 0", count, ok)
		}
	})

	t.Run("tse_success_cst_dates_gateway_scope", func(t *testing.T) {
		main := fixture.tseGateway(purgeTencentTSEGateway)
		main.seed("tse-old-30d", 30)
		main.seed("tse-old-8d", 8)
		main.seed("tse-new-1d", 1)
		main.seed("tse-future", -30)

		foreign := fixture.tseGateway("gw-fixture-other")
		foreign.seed("tse-foreign-20d", 20)
		regionsBefore := len(fixture.tseRegions)

		result := run(request("Purge tencent tse success", "tencentcloud-tse-purger", tseConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		assertState(t, main, []string{"tse-old-30d", "tse-old-8d"}, []string{"tse-new-1d", "tse-future"})
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		for _, region := range fixture.tseRegions[regionsBefore:] {
			if region != purgeTencentTSERegion {
				t.Fatalf("tse request carried region %q, want %q", region, purgeTencentTSERegion)
			}
		}
		if foreign.lists != 0 || !contains(foreign.remaining(), "tse-foreign-20d") {
			t.Fatalf("tse purge crossed gateway boundaries: lists=%d remaining=%v", foreign.lists, foreign.remaining())
		}
	})

	t.Run("tse_multipage_list", func(t *testing.T) {
		main := fixture.tseGateway(purgeTencentTSEGateway)
		for i := 0; i < 100; i++ {
			main.seed(fmt.Sprintf("tse-p1-%03d", i), 2)
		}
		main.seed("tse-p2-old", 30)
		listsBefore := main.lists

		result := run(request("Purge tencent tse multipage", "tencentcloud-tse-purger", tseConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if main.lists-listsBefore != 2 {
			t.Fatalf("list calls %d, want 2 (100 per page)", main.lists-listsBefore)
		}
		if !contains(main.deletes, "tse-p2-old") {
			t.Fatalf("multipage deletes %v", main.deletes)
		}
	})

	t.Run("tse_missing_gateway_fails", func(t *testing.T) {
		config := map[string]any{"endpoint": "custom-tse.endpoint.test", "region": purgeTencentTSERegion, "serviceType": "cloudnative"}
		result := run(request("Purge tencent tse missing gateway", "tencentcloud-tse-purger", config, 7))
		if result.status != "failed" {
			t.Fatalf("missing gatewayId status %s, want failed", result.status)
		}
		if !strings.Contains(result.runError, "gatewayId") {
			t.Fatalf("missing gatewayId error does not explain the cause: %s", result.runError)
		}
	})

	t.Run("tse_unsupported_service_type_fails", func(t *testing.T) {
		config := map[string]any{"endpoint": "custom-tse.endpoint.test", "region": purgeTencentTSERegion, "serviceType": "bogus", "gatewayId": purgeTencentTSEGateway}
		result := run(request("Purge tencent tse bogus service", "tencentcloud-tse-purger", config, 7))
		if result.status != "failed" {
			t.Fatalf("unsupported serviceType status %s, want failed", result.status)
		}
		if !strings.Contains(result.runError, "service type") {
			t.Fatalf("unsupported serviceType error does not explain the cause: %s", result.runError)
		}
	})

	t.Run("tse_auth_failure", func(t *testing.T) {
		fixture.tseGateway(purgeTencentTSEGateway).seed("tse-auth", 30)
		fixture.setMode("auth-failure")
		result := run(request("Purge tencent tse auth failure", "tencentcloud-tse-purger", tseConfig, 7))
		fixture.setMode("")
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
	})

	t.Run("tse_delete_failure_and_repair", func(t *testing.T) {
		main := fixture.tseGateway(purgeTencentTSEGateway)
		main.seed("tse-pf-a", 20)
		main.seed("tse-pf-b", 20)
		main.denyID = "tse-pf-b"

		req := request("Purge tencent tse delete failure", "tencentcloud-tse-purger", tseConfig, 7)
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
		if !contains(main.deletes, "tse-pf-a") || !contains(main.deletes, "tse-pf-b") {
			t.Fatalf("repair deleted %v", main.deletes)
		}
	})

	t.Run("tse_notfound_skips", func(t *testing.T) {
		main := fixture.tseGateway(purgeTencentTSEGateway)
		main.seed("tse-vanish", 15)
		main.vanishID = "tse-vanish"

		result := run(request("Purge tencent tse notfound", "tencentcloud-tse-purger", tseConfig, 7))
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("notfound log count=%d found=%t, want 0", count, ok)
		}
	})
}

func runPurgeTencentInDocker(t *testing.T) {
	t.Helper()
	runPurgeFamilyInDocker(t, purgeTencentPlugins, "purge-tencentcloud.test", "^TestPurgeWorkflowTencentCloud$")
}
