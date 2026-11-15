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
	purgeAzureTenantID     = "11111111-1111-1111-1111-111111111111"
	purgeAzureClientID     = "fixture-azure-client-id"
	purgeAzureClientSecret = "fixture-azure-client-secret"
	purgeAzureWrongSecret  = "fixture-azure-wrong-secret"
	purgeAzureVaultName    = "fixture-vault"
	purgeAzureToken        = "fixture-azure-bearer-token"
	purgeAzureVaultHost    = "fixture-vault.vault.azure.net"
	purgeAzureLoginHost    = "login.microsoftonline.com"
	purgeAzureAPIVersion   = "2025-07-01"
	purgeAzureScope        = "https://vault.azure.net/.default"
)

var purgeAzurePlugins = []string{"azure-keyvault-purger"}

var purgeAzureHosts = []string{purgeAzureVaultHost, purgeAzureLoginHost}

type purgeAzureCert struct {
	name         string
	expires      int64
	noAttributes bool
	noExpiry     bool
}

type purgeAzureFixture struct {
	mu   sync.Mutex
	mode string

	certs      map[string]*purgeAzureCert
	order      []string
	deletes    []string
	lists      int
	pageSize   int
	nextLinks  []string
	tokens     int
	challenges int
	scopes     []string
	secrets    []string
	vanishName string
	denyName   string
	listFail   bool
}

func newPurgeAzureFixture(t *testing.T) *purgeAzureFixture {
	t.Helper()
	f := &purgeAzureFixture{certs: map[string]*purgeAzureCert{}}
	newB4CloudProxy(t, purgeAzureHosts, http.HandlerFunc(f.serve))
	return f
}

func (f *purgeAzureFixture) seed(name string, daysToExpiry float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.certs[name]; !exists {
		f.order = append(f.order, name)
	}
	f.certs[name] = &purgeAzureCert{
		name:    name,
		expires: time.Now().Add(time.Duration(-daysToExpiry * 24 * float64(time.Hour))).Unix(),
	}
}

func (f *purgeAzureFixture) seedExotic(name string, noAttributes, noExpiry bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.certs[name]; !exists {
		f.order = append(f.order, name)
	}
	notAfter := time.Now().Add(-30 * 24 * time.Hour).Unix()
	f.certs[name] = &purgeAzureCert{name: name, expires: notAfter, noAttributes: noAttributes, noExpiry: noExpiry}
}

func (f *purgeAzureFixture) live() []*purgeAzureCert {
	items := make([]*purgeAzureCert, 0, len(f.order))
	for _, name := range f.order {
		if cert, exists := f.certs[name]; exists {
			items = append(items, cert)
		}
	}
	return items
}

func (f *purgeAzureFixture) remaining() []string {
	items := make([]string, 0, len(f.certs))
	for _, name := range f.order {
		if _, exists := f.certs[name]; exists {
			items = append(items, name)
		}
	}
	return items
}

func (f *purgeAzureFixture) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *purgeAzureFixture) keyVaultError(w http.ResponseWriter, status int, code, message string) {
	f.writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func (f *purgeAzureFixture) challenge(w http.ResponseWriter) {
	f.challenges++
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer authorization="https://%s/%s", resource="https://vault.azure.net"`, purgeAzureLoginHost, purgeAzureTenantID))
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func (f *purgeAzureFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.Host {
	case purgeAzureLoginHost:
		f.serveLogin(w, r)
	case purgeAzureVaultHost:
		f.serveVault(w, r)
	default:
		http.Error(w, "unexpected fixture host "+r.Host, http.StatusBadRequest)
	}
}

func (f *purgeAzureFixture) serveLogin(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/common/discovery/instance":
		f.writeJSON(w, http.StatusOK, map[string]any{
			"tenant_discovery_endpoint": fmt.Sprintf("https://%s/%s/v2.0/.well-known/openid-configuration", purgeAzureLoginHost, purgeAzureTenantID),
			"api-version":               "1.1",
			"metadata": []map[string]any{{
				"preferred_network": purgeAzureLoginHost,
				"preferred_cache":   "login.windows.net",
				"aliases":           []string{purgeAzureLoginHost, "login.windows.net"},
			}},
		})
	case r.URL.Path == "/"+purgeAzureTenantID+"/v2.0/.well-known/openid-configuration":
		f.writeJSON(w, http.StatusOK, map[string]any{
			"token_endpoint":         fmt.Sprintf("https://%s/%s/oauth2/v2.0/token", purgeAzureLoginHost, purgeAzureTenantID),
			"issuer":                 fmt.Sprintf("https://%s/%s/v2.0", purgeAzureLoginHost, purgeAzureTenantID),
			"authorization_endpoint": fmt.Sprintf("https://%s/%s/oauth2/v2.0/authorize", purgeAzureLoginHost, purgeAzureTenantID),
			"tenant_region_scope":    "NA",
		})
	case strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token") && r.Method == http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid token request", http.StatusBadRequest)
			return
		}
		if tenant := strings.Trim(strings.TrimSuffix(r.URL.Path, "/oauth2/v2.0/token"), "/"); tenant != purgeAzureTenantID {
			f.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_tenant", "error_description": "unexpected tenant " + tenant})
			return
		}
		if grant := r.PostForm.Get("grant_type"); grant != "client_credentials" {
			f.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "error_description": "grant_type " + grant})
			return
		}
		if clientID := r.PostForm.Get("client_id"); clientID != purgeAzureClientID {
			f.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client", "error_description": "unexpected client " + clientID})
			return
		}
		secret := r.PostForm.Get("client_secret")
		f.secrets = append(f.secrets, secret)
		f.scopes = append(f.scopes, r.PostForm.Get("scope"))
		if f.mode == "auth-failure" || secret != purgeAzureClientSecret {
			f.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client", "error_description": "fixture rejected the client secret"})
			return
		}
		f.tokens++
		f.writeJSON(w, http.StatusOK, map[string]any{
			"token_type":     "Bearer",
			"expires_in":     3599,
			"ext_expires_in": 3599,
			"access_token":   purgeAzureToken,
		})
	default:
		http.Error(w, "unexpected login request "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

func (f *purgeAzureFixture) serveVault(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+purgeAzureToken {
		f.challenge(w)
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/certificates":
		if f.mode == "auth-failure" {
			f.challenge(w)
			return
		}
		if f.listFail {
			f.keyVaultError(w, http.StatusForbidden, "Forbidden", "fixture list failure")
			return
		}
		if version := r.URL.Query().Get("api-version"); version != purgeAzureAPIVersion {
			f.keyVaultError(w, http.StatusBadRequest, "BadParameter", "api-version "+version)
			return
		}
		f.lists++
		live := f.live()
		page := live
		nextLink := ""
		if f.pageSize > 0 {
			index := 0
			if token := r.URL.Query().Get("$skiptoken"); token != "" {
				if _, err := fmt.Sscanf(token, "az-page-%d", &index); err != nil {
					f.keyVaultError(w, http.StatusBadRequest, "BadParameter", "bad skiptoken "+token)
					return
				}
			}
			start := index * f.pageSize
			if start > len(live) {
				start = len(live)
			}
			end := start + f.pageSize
			if end > len(live) {
				end = len(live)
			}
			page = live[start:end]
			if end < len(live) {
				nextLink = fmt.Sprintf("https://%s/certificates?api-version=%s&$skiptoken=az-page-%d", purgeAzureVaultHost, purgeAzureAPIVersion, index+1)
			}
		}
		items := make([]map[string]any, 0, len(page))
		for _, cert := range page {
			item := map[string]any{
				"id": fmt.Sprintf("https://%s/certificates/%s/current", purgeAzureVaultHost, cert.name),
			}
			if !cert.noAttributes {
				attributes := map[string]any{"enabled": true}
				if !cert.noExpiry {
					attributes["exp"] = cert.expires
				}
				item["attributes"] = attributes
			}
			items = append(items, item)
		}
		response := map[string]any{"value": items}
		if nextLink != "" {
			response["nextLink"] = nextLink
			f.nextLinks = append(f.nextLinks, nextLink)
		}
		f.writeJSON(w, http.StatusOK, response)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/certificates/"):
		name := strings.TrimPrefix(r.URL.Path, "/certificates/")
		if name == f.vanishName {
			f.keyVaultError(w, http.StatusNotFound, "CertificateNotFound", "certificate vanished between list and delete")
			return
		}
		if name == f.denyName {
			f.keyVaultError(w, http.StatusInternalServerError, "KeyVaultInternalError", "fixture delete denied")
			return
		}
		if _, exists := f.certs[name]; !exists {
			f.keyVaultError(w, http.StatusNotFound, "CertificateNotFound", "unknown certificate")
			return
		}
		delete(f.certs, name)
		f.deletes = append(f.deletes, name)
		f.writeJSON(w, http.StatusOK, map[string]any{"id": fmt.Sprintf("https://%s/certificates/%s/current", purgeAzureVaultHost, name), "recoveryId": "fixture"})
	default:
		f.keyVaultError(w, http.StatusNotFound, "BadRoute", "unexpected vault request "+r.Method+" "+r.URL.Path)
	}
}

func (f *purgeAzureFixture) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

func (f *purgeAzureFixture) reset() {
	f.certs = map[string]*purgeAzureCert{}
	f.order = nil
	f.deletes = nil
	f.lists = 0
	f.pageSize = 0
	f.nextLinks = nil
	f.tokens = 0
	f.challenges = 0
	f.scopes = nil
	f.secrets = nil
	f.vanishName = ""
	f.denyName = ""
	f.listFail = false
}

func TestPurgeWorkflowAzure(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runPurgeAzureInDocker(t)
		return
	}
	runPurgeAzureScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runPurgeAzureScenarios(t *testing.T, binaryDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fixture := newPurgeAzureFixture(t)
	core := startPurgeCore(t, binaryDir, purgeAzurePlugins)

	core.assertCatalogEntry("azure-keyvault-purger", "purge", true)
	if envelope, err := core.fetchSchemaEnvelope("azure-keyvault-purger", "purge"); err != nil {
		t.Fatalf("purge schema endpoint: %v", err)
	} else if envelope["provider"] != "azure-keyvault-purger" || envelope["category"] != "purge" {
		t.Fatalf("unexpected purge schema envelope: %v", envelope)
	}

	accessConfig := map[string]any{"tenantId": purgeAzureTenantID, "clientId": purgeAzureClientID, "clientSecret": purgeAzureClientSecret}
	wrongSecretAccess := map[string]any{"tenantId": purgeAzureTenantID, "clientId": purgeAzureClientID, "clientSecret": purgeAzureWrongSecret}
	providerConfig := map[string]any{"keyvaultName": purgeAzureVaultName}

	request := func(name string, accessConfig map[string]any) purgeWorkflowRequest {
		return purgeWorkflowRequest{
			name:           name,
			provider:       "azure-keyvault-purger",
			accessProvider: "azure",
			accessConfig:   accessConfig,
			providerConfig: providerConfig,
			expiredDays:    7,
		}
	}
	run := func(req purgeWorkflowRequest) purgeWorkflowResult {
		return core.runPurgeWorkflow(ctx, req, purgeAzureClientSecret, purgeAzureToken)
	}

	t.Run("success_threshold_epoch_seconds", func(t *testing.T) {
		fixture.reset()
		fixture.seed("az-old-30d", 30)
		fixture.seed("az-old-8d", 8)
		fixture.seed("az-new-1d", 1)
		fixture.seed("az-future", -30)
		challengesBefore := fixture.challenges
		tokensBefore := fixture.tokens

		result := run(request("Purge azure keyvault success", accessConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if strings.Join(fixture.deletes, ",") != "az-old-30d,az-old-8d" {
			t.Fatalf("deleted certificates %v, want [az-old-30d az-old-8d]", fixture.deletes)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
			t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
		}
		if fixture.challenges-challengesBefore != 1 || fixture.tokens-tokensBefore != 1 {
			t.Fatalf("challenge=%d token=%d fetches, want 1 each", fixture.challenges-challengesBefore, fixture.tokens-tokensBefore)
		}
		for _, scope := range fixture.scopes {
			if !strings.Contains(scope, purgeAzureScope) {
				t.Fatalf("token scope %q, want it to include %q", scope, purgeAzureScope)
			}
		}
	})

	t.Run("missing_expiry_attributes_skipped", func(t *testing.T) {
		fixture.reset()
		fixture.seedExotic("az-no-attributes", true, false)
		fixture.seedExotic("az-no-expiry", false, true)

		result := run(request("Purge azure keyvault missing expiry", accessConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("missing-expiry log count=%d found=%t, want 0", count, ok)
		}
		if !contains(fixture.remaining(), "az-no-attributes") || !contains(fixture.remaining(), "az-no-expiry") {
			t.Fatalf("certificates without expiry were purged: %v", fixture.remaining())
		}
	})

	t.Run("multipage_list", func(t *testing.T) {
		fixture.reset()
		fixture.mu.Lock()
		fixture.pageSize = 2
		fixture.mu.Unlock()
		defer func() {
			fixture.mu.Lock()
			fixture.pageSize = 0
			fixture.mu.Unlock()
		}()
		fixture.seed("az-page-fresh-1", 2)
		fixture.seed("az-page-fresh-2", 2)
		fixture.seed("az-page-fresh-3", 2)
		fixture.seed("az-page-fresh-4", 2)
		fixture.seed("az-page-old", 30)
		listsBefore := fixture.lists
		fixture.mu.Lock()
		total := len(fixture.certs)
		fixture.mu.Unlock()
		wantLists := (total + 1) / 2

		result := run(request("Purge azure keyvault multipage", accessConfig))
		if result.status != "succeeded" {
			t.Fatalf("workflow status %s: %s", result.status, result.runError)
		}
		if fixture.lists-listsBefore != wantLists {
			t.Fatalf("list calls %d, want %d (2 per page)", fixture.lists-listsBefore, wantLists)
		}
		if !contains(fixture.deletes, "az-page-old") {
			t.Fatalf("multipage deletes %v, want az-page-old present", fixture.deletes)
		}
	})

	t.Run("notfound_skips", func(t *testing.T) {
		fixture.reset()
		fixture.seed("az-vanish", 15)
		fixture.mu.Lock()
		fixture.vanishName = "az-vanish"
		fixture.mu.Unlock()

		result := run(request("Purge azure keyvault notfound", accessConfig))
		fixture.mu.Lock()
		fixture.vanishName = ""
		fixture.mu.Unlock()
		if result.status != "succeeded" {
			t.Fatalf("status %s: %s", result.status, result.runError)
		}
		if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 0 {
			t.Fatalf("notfound log count=%d found=%t, want 0", count, ok)
		}
		if !contains(fixture.remaining(), "az-vanish") {
			t.Fatal("vanished certificate was removed despite the skip error")
		}
	})

	t.Run("delete_failure_and_repair", func(t *testing.T) {
		fixture.reset()
		fixture.seed("az-pf-a", 20)
		fixture.seed("az-pf-b", 20)
		fixture.mu.Lock()
		fixture.denyName = "az-pf-b"
		fixture.mu.Unlock()

		req := request("Purge azure keyvault delete failure", accessConfig)
		result := run(req)
		if result.status != "failed" {
			t.Fatalf("delete failure status %s, want failed", result.status)
		}

		fixture.mu.Lock()
		fixture.denyName = ""
		fixture.mu.Unlock()
		rerun := run(req)
		if rerun.status != "succeeded" {
			t.Fatalf("repair status %s: %s", rerun.status, rerun.runError)
		}
		if count, ok := core.purgeLogCount(ctx, rerun.runID); !ok || count != 1 {
			t.Fatalf("repair log count=%d found=%t, want 1", count, ok)
		}
		if strings.Count(strings.Join(fixture.deletes, ","), "az-pf-a") != 1 || !contains(fixture.deletes, "az-pf-b") {
			t.Fatalf("repair deleted %v", fixture.deletes)
		}
	})

	t.Run("list_failure", func(t *testing.T) {
		fixture.reset()
		fixture.seed("az-list-fail", 30)
		deletesBefore := len(fixture.deletes)
		fixture.mu.Lock()
		fixture.listFail = true
		fixture.mu.Unlock()
		result := run(request("Purge azure keyvault list failure", accessConfig))
		fixture.mu.Lock()
		fixture.listFail = false
		fixture.mu.Unlock()
		if result.status != "failed" {
			t.Fatalf("list failure status %s, want failed", result.status)
		}
		if len(fixture.deletes) != deletesBefore {
			t.Fatalf("list failure still deleted %v", fixture.deletes[deletesBefore:])
		}
	})

	t.Run("auth_failure_no_deletes", func(t *testing.T) {
		fixture.reset()
		fixture.seed("az-auth", 30)
		deletesBefore := len(fixture.deletes)
		challengesBefore := fixture.challenges

		result := run(request("Purge azure keyvault wrong secret", wrongSecretAccess))
		if result.status != "failed" {
			t.Fatalf("auth failure status %s, want failed", result.status)
		}
		if len(fixture.deletes) != deletesBefore {
			t.Fatalf("auth failure still deleted %v", fixture.deletes[deletesBefore:])
		}
		if fixture.challenges-challengesBefore < 1 {
			t.Fatal("keyvault challenge was never issued")
		}
		for _, secret := range fixture.secrets {
			if secret == purgeAzureClientSecret {
				t.Fatal("wrong-secret workflow sent the valid secret")
			}
		}
	})
}

func runPurgeAzureInDocker(t *testing.T) {
	t.Helper()
	runPurgeFamilyInDocker(t, purgeAzurePlugins, "purge-azure.test", "^TestPurgeWorkflowAzure$")
}
