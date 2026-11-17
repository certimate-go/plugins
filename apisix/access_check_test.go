package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
)

const testAPIKey = "unit-test-api-key"

type accessCheckFixture struct {
	t *testing.T

	mu          sync.Mutex
	mode        string
	method      string
	path        string
	query       string
	apiKeyHeard string
}

func (f *accessCheckFixture) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

func (f *accessCheckFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.method = r.Method
	f.path = r.URL.Path
	f.query = r.URL.RawQuery
	f.apiKeyHeard = r.Header.Get("X-API-KEY")
	mode := f.mode
	f.mu.Unlock()

	if r.Method != http.MethodGet {
		f.t.Errorf("mutating request %s %s reached the admin API", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	switch mode {
	case "hang":
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
		return
	case "unauthorized":
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error_msg":"failed to check token"}`))
		return
	case "forbidden":
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error_msg":"failed to check token"}`))
		return
	case "server-error":
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error_msg":"etcd cluster is unavailable"}`))
		return
	case "garbage":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html>not json`))
		return
	case "redirect":
		w.Header().Set("Location", "https://elsewhere.example.com/admin")
		w.WriteHeader(http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"code":0,"data":{"list":[],"total":0}}`))
}

func accessConfigJSON(t *testing.T, serverURL string) string {
	t.Helper()
	return accessConfigJSONWithExtras(t, serverURL, nil)
}

func accessConfigJSONWithExtras(t *testing.T, serverURL string, extras map[string]any) string {
	t.Helper()
	values := map[string]any{"serverUrl": serverURL, "apiKey": testAPIKey}
	xmaps.CopyTo(extras, values)
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func runCheckAccess(t *testing.T, ctx context.Context, configJSON string) *plugin.AccessCheckResult {
	t.Helper()
	res, err := (&pluginDeployer{}).CheckAccess(ctx, &plugin.AccessCheckRequest{AccessConfigJSON: configJSON}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("CheckAccess returned an error instead of a verdict: %v", err)
	}
	if verr := res.Validate(); verr != nil {
		t.Fatalf("result is not wire-valid: %v", verr)
	}
	for _, diag := range res.Diagnostics {
		if strings.Contains(diag, testAPIKey) {
			t.Fatalf("api key leaked into diagnostics: %q", diag)
		}
	}
	return res
}

func assertResult(t *testing.T, res *plugin.AccessCheckResult, status, reasonCode, connectStatus, authStatus string) *plugin.AccessCheckResult {
	t.Helper()
	if res.Status != status {
		t.Errorf("status = %q, want %q", res.Status, status)
	}
	if res.ReasonCode != reasonCode {
		t.Errorf("reasonCode = %q, want %q", res.ReasonCode, reasonCode)
	}
	wantItems := []plugin.AccessCheckItem{{ID: "connect", Status: connectStatus}, {ID: "auth", Status: authStatus}}
	if len(res.Items) != len(wantItems) {
		t.Fatalf("items = %v, want %v", res.Items, wantItems)
	}
	for i, want := range wantItems {
		if res.Items[i] != want {
			t.Errorf("items[%d] = %+v, want %+v", i, res.Items[i], want)
		}
	}
	return res
}

func assertFixtureSawProbe(t *testing.T, f *accessCheckFixture) *accessCheckFixture {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.method != http.MethodGet || f.path != "/apisix/admin/ssls" {
		t.Errorf("probe was %s %s, want GET /apisix/admin/ssls", f.method, f.path)
	}
	if f.apiKeyHeard != testAPIKey {
		t.Errorf("probe sent X-API-KEY %q, want %q", f.apiKeyHeard, testAPIKey)
	}
	if !strings.Contains(f.query, "page=1") || !strings.Contains(f.query, "page_size=1") {
		t.Errorf("probe query = %q, want a small page size", f.query)
	}
	return f
}

func TestCheckAccessPassed(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	res := runCheckAccess(t, context.Background(), accessConfigJSON(t, server.URL))
	assertResult(t, res, plugin.AccessCheckStatusPassed, "", plugin.AccessCheckItemStatusPassed, plugin.AccessCheckItemStatusPassed)
	assertFixtureSawProbe(t, fixture)
	joined := strings.Join(res.Diagnostics, " ")
	if !strings.Contains(joined, "127.0.0.1") || !strings.Contains(joined, "X-API-KEY") {
		t.Errorf("diagnostics missing endpoint host or auth kind: %v", res.Diagnostics)
	}
}

func TestCheckAccessPassedWithInsecureFlagAsString(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	res := runCheckAccess(t, context.Background(), accessConfigJSONWithExtras(t, server.URL, map[string]any{"allowInsecureConnections": "true"}))
	assertResult(t, res, plugin.AccessCheckStatusPassed, "", plugin.AccessCheckItemStatusPassed, plugin.AccessCheckItemStatusPassed)
}

func TestCheckAccessAuthRejected(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	fixture.setMode("unauthorized")
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	res := runCheckAccess(t, context.Background(), accessConfigJSON(t, server.URL))
	assertResult(t, res, plugin.AccessCheckStatusFailed, accessCheckReasonAuthRejected, plugin.AccessCheckItemStatusPassed, plugin.AccessCheckItemStatusFailed)
	assertFixtureSawProbe(t, fixture)
}

func TestCheckAccessProbeForbidden(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	fixture.setMode("forbidden")
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	res := runCheckAccess(t, context.Background(), accessConfigJSON(t, server.URL))
	assertResult(t, res, plugin.AccessCheckStatusInconclusive, accessCheckReasonProbeForbidden, plugin.AccessCheckItemStatusPassed, plugin.AccessCheckItemStatusSkipped)
}

func TestCheckAccessRemoteErrorOnServerFailure(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	fixture.setMode("server-error")
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	res := runCheckAccess(t, context.Background(), accessConfigJSON(t, server.URL))
	assertResult(t, res, plugin.AccessCheckStatusInconclusive, accessCheckReasonRemoteError, plugin.AccessCheckItemStatusPassed, plugin.AccessCheckItemStatusSkipped)
}

func TestCheckAccessRemoteErrorOnUnparseableBody(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	fixture.setMode("garbage")
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	res := runCheckAccess(t, context.Background(), accessConfigJSON(t, server.URL))
	assertResult(t, res, plugin.AccessCheckStatusInconclusive, accessCheckReasonRemoteError, plugin.AccessCheckItemStatusPassed, plugin.AccessCheckItemStatusSkipped)
}

func TestCheckAccessRemoteErrorOnRedirect(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	fixture.setMode("redirect")
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	res := runCheckAccess(t, context.Background(), accessConfigJSON(t, server.URL))
	assertResult(t, res, plugin.AccessCheckStatusInconclusive, accessCheckReasonRemoteError, plugin.AccessCheckItemStatusPassed, plugin.AccessCheckItemStatusSkipped)
}

func TestCheckAccessConnectRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := server.URL
	server.Close()

	res := runCheckAccess(t, context.Background(), accessConfigJSON(t, deadURL))
	assertResult(t, res, plugin.AccessCheckStatusFailed, accessCheckReasonConnectFailed, plugin.AccessCheckItemStatusFailed, plugin.AccessCheckItemStatusSkipped)
}

func TestCheckAccessTimeout(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	fixture.setMode("hang")
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	res := runCheckAccess(t, ctx, accessConfigJSON(t, server.URL))
	assertResult(t, res, plugin.AccessCheckStatusInconclusive, accessCheckReasonTimeout, plugin.AccessCheckItemStatusSkipped, plugin.AccessCheckItemStatusSkipped)
}

func TestCheckAccessCanceled(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := runCheckAccess(t, ctx, accessConfigJSON(t, server.URL))
	assertResult(t, res, plugin.AccessCheckStatusInconclusive, accessCheckReasonCanceled, plugin.AccessCheckItemStatusSkipped, plugin.AccessCheckItemStatusSkipped)
}

func TestCheckAccessTLSFailureOnSelfSigned(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	server := httptest.NewTLSServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	res := runCheckAccess(t, context.Background(), accessConfigJSON(t, server.URL))
	assertResult(t, res, plugin.AccessCheckStatusFailed, accessCheckReasonTLSFailed, plugin.AccessCheckItemStatusFailed, plugin.AccessCheckItemStatusSkipped)
}

func TestCheckAccessTLSAllowedWithInsecureConnections(t *testing.T) {
	fixture := &accessCheckFixture{t: t}
	server := httptest.NewTLSServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	configJSON := accessConfigJSONWithExtras(t, server.URL, map[string]any{"allowInsecureConnections": true})
	res := runCheckAccess(t, context.Background(), configJSON)
	assertResult(t, res, plugin.AccessCheckStatusPassed, "", plugin.AccessCheckItemStatusPassed, plugin.AccessCheckItemStatusPassed)
	assertFixtureSawProbe(t, fixture)
}

func TestCheckAccessConfigInvalid(t *testing.T) {
	cases := []struct {
		name       string
		configJSON string
		fieldName  string
	}{
		{"missing serverUrl", `{"apiKey":"` + testAPIKey + `"}`, "serverUrl"},
		{"missing apiKey", `{"serverUrl":"http://127.0.0.1:9180/"}`, "apiKey"},
		{"malformed JSON", `{serverUrl`, "malformed"},
		{"empty JSON", ``, "malformed"},
		{"serverUrl without scheme", `{"serverUrl":"127.0.0.1:9180","apiKey":"` + testAPIKey + `"}`, "serverUrl"},
		{"serverUrl not a URL", `{"serverUrl":"not a url","apiKey":"` + testAPIKey + `"}`, "serverUrl"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := runCheckAccess(t, context.Background(), c.configJSON)
			assertResult(t, res, plugin.AccessCheckStatusFailed, accessCheckReasonConfigInvalid, plugin.AccessCheckItemStatusSkipped, plugin.AccessCheckItemStatusSkipped)
			if !strings.Contains(strings.Join(res.Diagnostics, " "), c.fieldName) {
				t.Errorf("diagnostics %v do not name the offending field %q", res.Diagnostics, c.fieldName)
			}
		})
	}
}
