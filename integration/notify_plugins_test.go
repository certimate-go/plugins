package integration

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExternalNotificationPlugins(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	api := buildAndStartNotificationCore(t, ctx, "ntfy", "gotify")
	var catalog struct {
		Data []struct {
			ProviderType string
			Capability   string
			OwnsAccess   bool
		}
	}
	if err := api.request(ctx, http.MethodGet, "/api/plugin-catalog", nil, &catalog); err != nil {
		t.Fatal(err)
	}
	loaded := make(map[string]bool)
	for _, entry := range catalog.Data {
		loaded[entry.ProviderType] = entry.Capability == "notify" && entry.OwnsAccess
	}
	for _, provider := range []struct {
		name, path, authHeader, authPrefix, topic string
	}{
		{"ntfy", "/push", "Authorization", "Bearer ", "certimate-alerts"},
		{"gotify", "/push/message", "X-Gotify-Key", "", ""},
	} {
		t.Run(provider.name, func(t *testing.T) {
			if !loaded[provider.name] {
				t.Fatalf("plugin %s missing from notification catalog", provider.name)
			}
			for _, category := range []string{"access", "notify"} {
				var result struct {
					Data struct {
						Provider string
						Category string
					}
				}
				if err := api.request(ctx, http.MethodGet, "/api/provider-schemas/"+provider.name+"?category="+category, nil, &result); err != nil {
					t.Fatal(err)
				}
				if result.Data.Provider != provider.name || result.Data.Category != category {
					t.Fatalf("unexpected %s schema: %+v", category, result.Data)
				}
			}
			delivered := make(chan string, 8)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload struct {
					Title, Message, Topic string
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || r.Method != http.MethodPost || r.URL.Path != provider.path ||
					r.Header.Get("Content-Type") != "application/json" || payload.Topic != provider.topic ||
					payload.Title != "[Certimate] Notification Testing" || payload.Message != "Welcome to use Certimate!" {
					t.Errorf("unexpected request %s %s: %+v (%v)", r.Method, r.URL.Path, payload, err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				authorization := r.Header.Get(provider.authHeader)
				delivered <- authorization
				if authorization != provider.authPrefix+"fixture-valid" && (provider.name != "ntfy" || authorization != "") {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()

			config := map[string]string{"serverUrl": upstream.URL + "/push"}
			if provider.topic != "" {
				config["topic"] = provider.topic
			}
			for _, token := range []string{"fixture-valid", "fixture-rejected", "fixture-valid"} {
				config["token"] = token
				checkNotificationPush(t, ctx, api, provider.name, config, token != "fixture-rejected")
				select {
				case got := <-delivered:
					if got != provider.authPrefix+token {
						t.Errorf("unexpected provider authorization header: %q", got)
					}
				default:
					t.Fatal("plugin did not contact provider")
				}
			}
			if provider.name == "ntfy" {
				delete(config, "token")
				checkNotificationPush(t, ctx, api, provider.name, config, true)
				select {
				case got := <-delivered:
					if got != "" {
						t.Errorf("unexpected authorization for anonymous ntfy publish: %q", got)
					}
				default:
					t.Fatal("ntfy did not publish anonymously")
				}
			}
		})
	}
}

func buildAndStartNotificationCore(t *testing.T, ctx context.Context, names ...string) *b1WorkflowAPI {
	t.Helper()
	core := filepath.Join("..", "..", "certimate")
	if _, err := os.Stat(filepath.Join(core, "go.mod")); os.IsNotExist(err) {
		t.Skip("requires the sibling certimate checkout")
	} else if err != nil {
		t.Fatal(err)
	}
	plugins := t.TempDir()
	binary := filepath.Join(t.TempDir(), "certimate")
	build := func(dir, output string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "go", "build", "-o", output, ".")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", dir, err, out)
		}
	}
	build(core, binary)
	for _, name := range names {
		source := filepath.Join("..", name)
		stage := filepath.Join(plugins, name)
		if err := os.Mkdir(stage, 0o755); err != nil {
			t.Fatal(err)
		}
		build(source, filepath.Join(stage, name))
		manifest, err := os.ReadFile(filepath.Join(source, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stage, "manifest.json"), manifest, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return startNotificationCore(t, ctx, binary, plugins)
}

func startNotificationCore(t *testing.T, ctx context.Context, binary, plugins string) *b1WorkflowAPI {
	t.Helper()
	const email = "notification-plugin@example.test"
	const password = "Notification-fixture-password123!"
	dataDir := t.TempDir()
	logFile, err := os.Create(filepath.Join(dataDir, "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logFile.Close() })
	env := append(os.Environ(), "CERTIMATE_PLUGIN_DIR="+plugins)
	for _, args := range [][]string{{"migrate", "up"}, {"superuser", "upsert", email, password}} {
		cmd := exec.CommandContext(ctx, binary, append(args, "--dir", dataDir)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("core setup: %v\n%s", err, out)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary, "serve", "--http", address, "--dir", dataDir)
	cmd.Env, cmd.Stdout, cmd.Stderr = env, logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
		if t.Failed() {
			log, _ := os.ReadFile(logFile.Name())
			t.Log(string(log))
		}
	})
	api := &b1WorkflowAPI{baseURL: "http://" + address, client: &http.Client{Timeout: 5 * time.Second}}
	for api.request(ctx, http.MethodGet, "/api/health", nil, nil) != nil {
		select {
		case err := <-exited:
			exited <- err
			t.Fatalf("core exited before readiness: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
	var auth struct{ Token string }
	if err := api.request(ctx, http.MethodPost, "/api/collections/_superusers/auth-with-password", map[string]string{"identity": email, "password": password}, &auth); err != nil {
		t.Fatal(err)
	}
	api.token = auth.Token
	return api
}

func checkNotificationPush(t *testing.T, ctx context.Context, api *b1WorkflowAPI, provider string, config map[string]string, wantSuccess bool) {
	t.Helper()
	var access struct{ ID string }
	data := map[string]any{"name": provider, "provider": provider, "reserve": "notif", "config": config}
	if err := api.request(ctx, http.MethodPost, "/api/collections/access/records", data, &access); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Code int
		Msg  string
	}
	if err := api.request(ctx, http.MethodPost, "/api/notifications/test", map[string]string{"provider": provider, "accessId": access.ID}, &result); err != nil {
		t.Fatal(err)
	}
	if (result.Code == 0) != wantSuccess || (!wantSuccess && !strings.Contains(result.Msg, "HTTP 401")) {
		t.Fatalf("notification response = %+v, expected success=%t", result, wantSuccess)
	}
}
