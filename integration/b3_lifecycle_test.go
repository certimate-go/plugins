package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

type workflowRequestBarrier struct {
	mu      sync.Mutex
	entered chan struct{}
	release chan struct{}
}

type workflowLifecycleTarget struct {
	failureOnly bool
	barrier     *workflowRequestBarrier
	verify      func(*plugin.DeployRequest)
}

func (b *workflowRequestBarrier) arm() (<-chan struct{}, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entered = make(chan struct{})
	b.release = make(chan struct{})
	release := b.release
	return b.entered, sync.OnceFunc(func() { close(release) })
}

func (b *workflowRequestBarrier) wait(ctx context.Context, prepare func() error) error {
	b.mu.Lock()
	entered, release := b.entered, b.release
	b.entered = nil
	b.mu.Unlock()
	if entered == nil {
		return nil
	}
	if prepare != nil {
		if err := prepare(); err != nil {
			return err
		}
	}
	close(entered)
	select {
	case <-release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *workflowRequestBarrier) waitHTTP(r *http.Request) error {
	return b.wait(r.Context(), func() error {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	})
}

func verifyWorkflowReloadLifecycle(t *testing.T, ctx context.Context, api *b1WorkflowAPI, provider, packageDir, accessID string, request *plugin.DeployRequest, deploy, deploySuccess func(context.Context, *plugin.DeployRequest) (*plugin.DeployResult, error), target workflowLifecycleTarget) {
	t.Helper()
	manifestPath := filepath.Join(packageDir, "manifest.json")
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.WriteFile(manifestPath, original, 0o600); err != nil {
			t.Error(err)
		}
	}()
	var manifest map[string]any
	if err := json.Unmarshal(original, &manifest); err != nil {
		t.Fatal(err)
	}
	writeManifest := func() {
		t.Helper()
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reload := func(wantError bool) {
		t.Helper()
		for {
			var result struct {
				Code int `json:"code"`
				Data struct {
					Errors []string `json:"errors"`
				} `json:"data"`
			}
			if err := api.request(ctx, http.MethodPost, "/api/plugin/reload", map[string]any{}, &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Data.Errors) == 1 && result.Data.Errors[0] == "reload already in progress" {
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(50 * time.Millisecond):
					continue
				}
			}
			if result.Code != 0 || (len(result.Data.Errors) != 0) != wantError {
				t.Fatalf("reload error=%t: %+v", wantError, result)
			}
			return
		}
	}
	assertCatalog := func(description string, present bool) {
		t.Helper()
		var catalog struct {
			Code int `json:"code"`
			Data []struct {
				ProviderType string `json:"providerType"`
				Description  string `json:"description"`
				Source       string `json:"source"`
			} `json:"data"`
		}
		if err := api.request(ctx, http.MethodGet, "/api/plugin-catalog", nil, &catalog); err != nil {
			t.Fatal(err)
		}
		if catalog.Code != 0 {
			t.Fatalf("catalog returned error %d", catalog.Code)
		}
		found := false
		for _, entry := range catalog.Data {
			if entry.ProviderType == provider && entry.Source == "plugin" {
				found = true
				if entry.Description != description {
					t.Fatalf("catalog did not publish the expected package metadata: %q", entry.Description)
				}
			}
		}
		if found != present {
			t.Fatalf("catalog registration=%t, want %t", found, present)
		}
	}
	assertDeploy := func(scenario string) {
		t.Helper()
		if _, err := deploySuccess(ctx, request); err != nil {
			t.Fatalf("%s: %v", scenario, err)
		}
		target.verify(request)
		t.Logf("LIFECYCLE PASS %s %s access=%s", provider, scenario, accessID)
	}
	entered, release := target.barrier.arm()
	defer release()
	result := make(chan error, 1)
	go func() {
		_, err := deploySuccess(ctx, request)
		result <- err
	}()
	select {
	case <-entered:
	case err := <-result:
		t.Fatalf("deployment finished before target barrier: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("deployment never reached the target barrier")
	}
	originalDescription, _ := manifest["description"].(string)
	updatedDescription := "Lifecycle reload " + provider
	manifest["description"] = updatedDescription
	manifest["version"] = "0.1.1-lifecycle"
	writeManifest()
	reload(false)
	assertCatalog(updatedDescription, true)
	select {
	case err := <-result:
		t.Fatalf("reload terminated the blocked deployment: %v", err)
	default:
	}
	release()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("deployment did not survive reload: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("deployment did not finish after target release")
	}
	target.verify(request)
	t.Logf("LIFECYCLE PASS %s reload_during_inflight_deployment access=%s", provider, accessID)
	assertDeploy("replacement_runtime_saved_workflow")
	invalidBinary := filepath.Join(packageDir, "invalid-binary")
	if err := os.WriteFile(invalidBinary, []byte("invalid plugin executable\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest["binary"] = "invalid-binary"
	writeManifest()
	reload(true)
	assertCatalog(updatedDescription, true)
	assertDeploy("invalid_package_keeps_working_runtime")
	if err := os.WriteFile(manifestPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	reload(false)
	assertCatalog(originalDescription, true)
	assertDeploy("restored_package_saved_workflow")
	var originalAccess map[string]any
	accessPath := "/api/collections/access/records/" + accessID
	if err := api.request(ctx, http.MethodGet, accessPath, nil, &originalAccess); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(t.TempDir(), provider)
	if err := os.Rename(packageDir, parked); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := os.Stat(parked); err == nil {
			if err := os.Rename(parked, packageDir); err != nil {
				t.Error(err)
			}
		}
	}()
	reload(false)
	assertCatalog("", false)
	if _, err := deploy(ctx, request); err == nil || !strings.Contains(err.Error(), "provider '"+provider+"' not registered") {
		t.Fatalf("uninstalled saved workflow did not report the missing provider: %v", err)
	}
	var editedAccess map[string]any
	if err := api.request(ctx, http.MethodPatch, accessPath, map[string]any{"name": "Retained " + provider, "config": originalAccess["config"]}, &editedAccess); err != nil {
		t.Fatalf("retained built-in access is not editable after uninstall: %v", err)
	}
	if editedAccess["id"] != accessID || editedAccess["name"] != "Retained "+provider || !reflect.DeepEqual(editedAccess["config"], originalAccess["config"]) || editedAccess["provider"] != originalAccess["provider"] {
		t.Fatal("editing retained access changed its identity, provider, or configuration")
	}
	t.Logf("LIFECYCLE PASS %s uninstall_keeps_builtin_access_editable access=%s", provider, accessID)
	if err := os.Rename(parked, packageDir); err != nil {
		t.Fatal(err)
	}
	reload(false)
	assertCatalog(originalDescription, true)
	assertDeploy("reinstall_preserves_saved_workflow")
}
