package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

type apisixAccessCheckFixture struct {
	t  *testing.T
	mu sync.Mutex
}

func (f *apisixAccessCheckFixture) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		f.t.Errorf("mutating request %s %s reached the admin API", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-API-KEY") != "fixture-key" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error_msg": "failed to check token"})
		return
	}
	if r.URL.Path != "/apisix/admin/ssls" {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error_msg": "not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"list": []any{}, "total": 0}})
}

func buildApisixPlugin(t *testing.T, dir string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "apisix")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(out, "apisix"), "./apisix")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build apisix: %v\n%s", err, output)
	}
	for _, name := range []string{"manifest.json", "apisix.svg"} {
		data, err := os.ReadFile(filepath.Join(root, "apisix", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestApisixAccessCheckSubprocess(t *testing.T) {
	dir := os.Getenv("CERTIMATE_ACCESSCHECK_BINARIES")
	if dir != "" {
		if !filepath.IsAbs(dir) {
			root, err := filepath.Abs("..")
			if err != nil {
				t.Fatal(err)
			}
			dir = filepath.Join(root, dir)
		}
	} else {
		dir = t.TempDir()
		buildApisixPlugin(t, dir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := plugin.PluginConfig{PluginDir: dir, StartTimeout: 15 * time.Second, ProcessTimeout: 15 * time.Second}
	discovered, failures := plugin.Discover(ctx, cfg)
	if len(failures) != 0 || len(discovered) != 1 || discovered[0].Manifest.ProviderType != "apisix" {
		t.Fatalf("discovery count=%d failures=%v", len(discovered), failures)
	}
	dp := discovered[0]

	manager := plugin.NewManager(cfg, slog.New(slog.DiscardHandler))
	meta, _, err := manager.Bootstrap(ctx, dp)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.SupportsAccessCheck {
		t.Fatal("apisix plugin did not advertise access check support over the wire")
	}

	fixture := &apisixAccessCheckFixture{t: t}
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)

	session, err := manager.StartSession(ctx, dp)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	check := func(t *testing.T, configJSON string) *plugin.AccessCheckResult {
		t.Helper()
		res, err := session.CheckAccess(ctx, &plugin.AccessCheckRequest{AccessConfigJSON: configJSON})
		if err != nil {
			t.Fatalf("session.CheckAccess failed: %v", err)
		}
		t.Logf("access check result: %s", formatAccessCheckResult(res))
		return res
	}

	t.Run("passed", func(t *testing.T) {
		res := check(t, fmt.Sprintf(`{"serverUrl":%q,"apiKey":"fixture-key"}`, server.URL))
		if res.Status != plugin.AccessCheckStatusPassed || res.ReasonCode != "" {
			t.Fatalf("status=%q reasonCode=%q, want passed with no reason code", res.Status, res.ReasonCode)
		}
		assertAccessCheckItems(t, res, plugin.AccessCheckItemStatusPassed, plugin.AccessCheckItemStatusPassed)
		if strings.Contains(strings.Join(res.Diagnostics, " "), "fixture-key") {
			t.Fatal("api key leaked into diagnostics")
		}
	})

	t.Run("wrong-key", func(t *testing.T) {
		res := check(t, fmt.Sprintf(`{"serverUrl":%q,"apiKey":"wrong-key"}`, server.URL))
		if res.Status != plugin.AccessCheckStatusFailed || res.ReasonCode != "auth_rejected" {
			t.Fatalf("status=%q reasonCode=%q, want failed/auth_rejected", res.Status, res.ReasonCode)
		}
		assertAccessCheckItems(t, res, plugin.AccessCheckItemStatusPassed, plugin.AccessCheckItemStatusFailed)
	})

	t.Run("config-invalid", func(t *testing.T) {
		res := check(t, `{"serverUrl":"","apiKey":""}`)
		if res.Status != plugin.AccessCheckStatusFailed || res.ReasonCode != "config_invalid" {
			t.Fatalf("status=%q reasonCode=%q, want failed/config_invalid", res.Status, res.ReasonCode)
		}
		assertAccessCheckItems(t, res, plugin.AccessCheckItemStatusSkipped, plugin.AccessCheckItemStatusSkipped)
	})

	t.Run("hanging-admin-api", func(t *testing.T) {
		hangServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
		}))
		defer hangServer.Close()

		timeoutCtx, stop := context.WithTimeout(ctx, 500*time.Millisecond)
		defer stop()
		res, err := session.CheckAccess(timeoutCtx, &plugin.AccessCheckRequest{AccessConfigJSON: fmt.Sprintf(`{"serverUrl":%q,"apiKey":"fixture-key"}`, hangServer.URL)})
		if err != nil {
			t.Logf("hanging admin api surfaced as a transport error: %v", err)
			return
		}
		t.Logf("access check result: %s", formatAccessCheckResult(res))
		if res.Status == plugin.AccessCheckStatusPassed {
			t.Fatalf("hanging admin api reported passed: %+v", res)
		}
		if res.ReasonCode != "timeout" && res.ReasonCode != "canceled" {
			t.Fatalf("reasonCode=%q, want timeout or canceled", res.ReasonCode)
		}
	})
}

func assertAccessCheckItems(t *testing.T, res *plugin.AccessCheckResult, connect, auth string) {
	t.Helper()
	want := []plugin.AccessCheckItem{{ID: "connect", Status: connect}, {ID: "auth", Status: auth}}
	if len(res.Items) != len(want) {
		t.Fatalf("items = %v, want %v", res.Items, want)
	}
	for i, item := range want {
		if res.Items[i] != item {
			t.Fatalf("items[%d] = %+v, want %+v", i, res.Items[i], item)
		}
	}
}

func formatAccessCheckResult(res *plugin.AccessCheckResult) string {
	data, err := json.Marshal(res)
	if err != nil {
		return fmt.Sprintf("%#v", res)
	}
	return string(data)
}
