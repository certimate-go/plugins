package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

type b1WorkflowAPI struct {
	baseURL string
	token   string
	client  *http.Client
}

func (a *b1WorkflowAPI) request(ctx context.Context, method, path string, body, result any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", a.token)
	}
	res, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var problem struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(res.Body).Decode(&problem)
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, res.StatusCode, problem.Message)
	}
	if result == nil {
		_, err = io.Copy(io.Discard, res.Body)
		return err
	}
	decoder := json.NewDecoder(res.Body)
	decoder.UseNumber()
	return decoder.Decode(result)
}

func (a *b1WorkflowAPI) inspectLogs(ctx context.Context, runID, runError string, secrets []string) error {
	var text strings.Builder
	text.WriteString(runError)
	var collect func(any)
	collect = func(value any) {
		switch value := value.(type) {
		case string:
			text.WriteString(value)
			text.WriteByte('\n')
		case []any:
			for _, item := range value {
				collect(item)
			}
		case map[string]any:
			for _, item := range value {
				collect(item)
			}
		}
	}
	count := 0
	for page := 1; ; page++ {
		var logs struct {
			Items      []map[string]any `json:"items"`
			TotalPages int              `json:"totalPages"`
		}
		path := fmt.Sprintf("/api/collections/workflow_logs/records?filter=%s&perPage=200&page=%d", url.QueryEscape("runRef='"+runID+"'"), page)
		if err := a.request(ctx, http.MethodGet, path, nil, &logs); err != nil {
			return fmt.Errorf("could not inspect persisted workflow logs")
		}
		for _, record := range logs.Items {
			collect(record)
		}
		count += len(logs.Items)
		if page >= logs.TotalPages {
			break
		}
	}
	if count == 0 {
		return fmt.Errorf("completed workflow has no persisted logs to inspect")
	}
	for index, secret := range secrets {
		if secret != "" && strings.Contains(text.String(), secret) {
			return fmt.Errorf("secret %d leaked in persisted workflow logs or run error", index)
		}
	}
	return nil
}

func newB1WorkflowDeployer(t *testing.T, binaryDir, provider string, targets ...workflowLifecycleTarget) func(context.Context, *plugin.DeployRequest) (*plugin.DeployResult, error) {
	t.Helper()
	binary := os.Getenv("CERTIMATE_B1_CORE_BINARY")
	if binary == "" {
		t.Fatal("CERTIMATE_B1_CORE_BINARY is required for workflow verification")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	logFile, err := os.Create(filepath.Join(dataDir, "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logFile.Close() })
	baseline := os.Getenv("CERTIMATE_B1_BASELINE") == "1"
	pluginDir := t.TempDir()
	if !baseline {
		if err := os.CopyFS(filepath.Join(pluginDir, provider), os.DirFS(filepath.Join(binaryDir, provider))); err != nil {
			t.Fatal(err)
		}
	}
	env := append(os.Environ(), "CERTIMATE_PLUGIN_DIR="+pluginDir)
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancelSetup()
	for _, args := range [][]string{
		{"migrate", "up", "--dir", dataDir},
		{"superuser", "upsert", "b1@example.test", "B1-fixture-password123!", "--dir", dataDir},
	} {
		cmd := exec.CommandContext(setupCtx, binary, args...)
		cmd.Env = env
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Run(); err != nil {
			t.Fatalf("core setup %s: %v", args[0], err)
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
	api := &b1WorkflowAPI{baseURL: "http://" + address, client: &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{}}}
	var cmd *exec.Cmd
	var exited chan struct{}
	var processErr error
	stopCore := func() {
		if cmd == nil {
			return
		}
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
		cmd = nil
		api.client.CloseIdleConnections()
	}
	t.Cleanup(stopCore)
	startCore := func() {
		readyCtx, cancelReady := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancelReady()
		cmd = exec.Command(binary, "serve", "--http", address, "--dir", dataDir)
		cmd.Env = env
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		exited = make(chan struct{})
		go func() {
			processErr = cmd.Wait()
			close(exited)
		}()
		for {
			err = api.request(readyCtx, http.MethodGet, "/api/health", nil, nil)
			if err == nil {
				break
			}
			select {
			case <-exited:
				t.Fatalf("core exited before readiness: %v", processErr)
			case <-readyCtx.Done():
				t.Fatalf("core did not become ready: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
		}
		var auth struct {
			Token string `json:"token"`
		}
		if err := api.request(readyCtx, http.MethodPost, "/api/collections/_superusers/auth-with-password", map[string]string{"identity": "b1@example.test", "password": "B1-fixture-password123!"}, &auth); err != nil {
			t.Fatal(err)
		}
		if auth.Token == "" {
			t.Fatal("core authentication returned no token")
		}
		api.token = auth.Token
	}
	startCore()
	assertCatalog := func(ctx context.Context, wantLoaded bool) {
		var catalog struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data []struct {
				ProviderType string `json:"providerType"`
				Source       string `json:"source"`
			} `json:"data"`
		}
		if err := api.request(ctx, http.MethodGet, "/api/plugin-catalog", nil, &catalog); err != nil {
			t.Fatal(err)
		}
		if catalog.Code != 0 {
			t.Fatalf("plugin catalog: code=%d message=%s", catalog.Code, catalog.Msg)
		}
		loaded := false
		for _, entry := range catalog.Data {
			if entry.ProviderType == provider && entry.Source == "plugin" {
				loaded = true
			}
		}
		if loaded != wantLoaded {
			t.Fatalf("%s plugin catalog registration=%t, want %t", provider, loaded, wantLoaded)
		}
	}
	assertCatalog(setupCtx, !baseline)
	if !baseline {
		var schema struct {
			Data struct {
				Schema struct {
					Columns []struct {
						Name         string `json:"name"`
						NumericValue string `json:"numericValue"`
						InputType    string `json:"inputType"`
					} `json:"columns"`
				} `json:"schema"`
			} `json:"data"`
		}
		if err := api.request(setupCtx, http.MethodGet, "/api/provider-schemas/"+provider+"?category=deploy", nil, &schema); err != nil {
			t.Fatal(err)
		}
		identifierFields := map[string][]string{
			"cdnfly":    {"siteId", "certificateId"},
			"lecdn":     {"certificateId", "clientId"},
			"mohua-mvh": {"hostId", "domainId"},
		}
		for _, name := range identifierFields[provider] {
			found := false
			for _, column := range schema.Data.Schema.Columns {
				if column.Name != name {
					continue
				}
				found = true
				numericValue := "integer"
				inputType := "number"
				if provider == "mohua-mvh" {
					numericValue, inputType = "positiveInteger", ""
				}
				if column.NumericValue != numericValue || column.InputType != inputType {
					t.Fatalf("schema endpoint lost saved identifier behavior for %s.%s", provider, name)
				}
			}
			if !found {
				t.Fatalf("schema endpoint omitted %s.%s", provider, name)
			}
		}
	}
	accessProvider := strings.SplitN(provider, "-", 2)[0]
	accessID := ""
	workflowID := ""
	var lastSuccess *plugin.DeployRequest
	replayOnly := false
	deploy := func(ctx context.Context, request *plugin.DeployRequest) (*plugin.DeployResult, error) {
		decode := func(raw, kind string) (map[string]any, error) {
			if !json.Valid([]byte(raw)) {
				return nil, fmt.Errorf("invalid %s config JSON", kind)
			}
			var values map[string]any
			decoder := json.NewDecoder(strings.NewReader(raw))
			decoder.UseNumber()
			if err := decoder.Decode(&values); err != nil {
				return nil, fmt.Errorf("invalid %s config: %w", kind, err)
			}
			return values, nil
		}
		accessConfig, err := decode(request.AccessConfigJSON, "access")
		if err != nil {
			return nil, err
		}
		deployConfig, err := decode(request.ExtendedConfigJSON, "deploy")
		if err != nil {
			return nil, err
		}
		workflow := struct {
			ID string `json:"id"`
		}{ID: workflowID}
		if !replayOnly {
			var access struct {
				ID string `json:"id"`
			}
			accessMethod, accessPath := http.MethodPost, "/api/collections/access/records"
			if accessID != "" {
				accessMethod, accessPath = http.MethodPatch, accessPath+"/"+accessID
			}
			if err := api.request(ctx, accessMethod, accessPath, map[string]any{"name": "B1 " + provider, "provider": accessProvider, "config": accessConfig}, &access); err != nil {
				return nil, err
			}
			if access.ID == "" {
				return nil, fmt.Errorf("core did not persist the access record")
			}
			accessID = access.ID
			node := func(id, kind string, config map[string]any) map[string]any {
				return map[string]any{"id": id, "type": kind, "data": map[string]any{"name": id, "config": config}}
			}
			graph := map[string]any{"nodes": []any{
				node("start", "start", map[string]any{}),
				node("upload", "bizUpload", map[string]any{"source": "form", "certificate": request.CertificatePEM, "privateKey": request.PrivateKeyPEM}),
				node("deploy", "bizDeploy", map[string]any{"certificateOutputNodeId": "upload", "provider": provider, "providerAccessId": access.ID, "providerConfig": deployConfig, "skipOnLastSucceeded": false}),
				node("end", "end", map[string]any{}),
			}}
			workflowMethod, workflowPath := http.MethodPost, "/api/collections/workflow/records"
			if workflowID != "" {
				workflowMethod, workflowPath = http.MethodPatch, workflowPath+"/"+workflowID
			}
			if err := api.request(ctx, workflowMethod, workflowPath, map[string]any{
				"name": "B1 " + provider, "description": "Isolated B1 plugin verification", "trigger": "manual", "enabled": true,
				"hasContent": true, "hasDraft": false, "graphContent": graph, "graphDraft": graph,
			}, &workflow); err != nil {
				return nil, err
			}
			if workflow.ID == "" {
				return nil, fmt.Errorf("core did not persist the workflow record")
			}
			workflowID = workflow.ID
		}
		var started struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				RunID string `json:"runId"`
			} `json:"data"`
		}
		if err := api.request(ctx, http.MethodPost, "/api/workflows/"+workflow.ID+"/runs", map[string]string{"trigger": "manual"}, &started); err != nil {
			return nil, err
		}
		if started.Code != 0 || started.Data.RunID == "" {
			return nil, fmt.Errorf("workflow trigger returned no run ID: code=%d message=%s", started.Code, started.Msg)
		}
		completed := false
		defer func() {
			if completed {
				return
			}
			cancelCtx, cancelRequest := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelRequest()
			_ = api.request(cancelCtx, http.MethodPost, "/api/workflows/"+workflow.ID+"/runs/"+started.Data.RunID+"/cancel", nil, nil)
		}()
		pollCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		secrets := []string{request.PrivateKeyPEM, base64.StdEncoding.EncodeToString([]byte(request.PrivateKeyPEM))}
		for key, value := range accessConfig {
			name := strings.ToLower(key)
			if name == "accesstokenid" {
				continue
			}
			if strings.Contains(name, "password") || strings.Contains(name, "secret") || strings.Contains(name, "token") || name == "apikey" || name == "accesskey" || name == "privatekey" {
				if secret, ok := value.(string); ok && secret != "" {
					secrets = append(secrets, secret, base64.StdEncoding.EncodeToString([]byte(secret)))
				}
			}
		}
		for {
			var run struct {
				Status string `json:"status"`
				Error  string `json:"error"`
			}
			if err := api.request(pollCtx, http.MethodGet, "/api/collections/workflow_run/records/"+started.Data.RunID, nil, &run); err != nil {
				return nil, err
			}
			if !baseline && (run.Status == "succeeded" || run.Status == "failed" || run.Status == "canceled") {
				if err := api.inspectLogs(pollCtx, started.Data.RunID, run.Error, secrets); err != nil {
					completed = true
					t.Error(err)
					return nil, err
				}
			}
			switch run.Status {
			case "succeeded":
				completed = true
				if !replayOnly {
					copy := *request
					lastSuccess = &copy
				}
				t.Logf("%s workflow %s run %s succeeded", provider, workflow.ID, started.Data.RunID)
				return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
			case "failed", "canceled":
				completed = true
				t.Logf("%s workflow %s run %s %s", provider, workflow.ID, started.Data.RunID, run.Status)
				return nil, fmt.Errorf("workflow %s: %s", run.Status, run.Error)
			case "pending", "processing":
			default:
				return nil, fmt.Errorf("unexpected workflow status %q", run.Status)
			}
			select {
			case <-pollCtx.Done():
				return nil, pollCtx.Err()
			case <-time.After(25 * time.Millisecond):
			}
		}
	}
	if !baseline && (len(targets) == 0 || !targets[0].failureOnly) {
		t.Cleanup(func() {
			if lastSuccess == nil {
				t.Fatal("lifecycle verification requires a successful saved workflow")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			if _, err := deploy(ctx, lastSuccess); err != nil {
				t.Fatalf("restore successful workflow before lifecycle verification: %v", err)
			}
			replayOnly = true
			persistedAccessID, persistedWorkflowID := accessID, workflowID
			deploySuccess := func(ctx context.Context, request *plugin.DeployRequest) (*plugin.DeployResult, error) {
				if len(targets) == 0 {
					return deploy(ctx, request)
				}
				certificate, err := xcert.ParseCertificateFromPEM(request.CertificatePEM)
				if err != nil {
					return nil, err
				}
				_, chain := pem.Decode([]byte(request.CertificatePEM))
				request.CertificatePEM, request.PrivateKeyPEM, _ = makeCertificate(t, certificate.DNSNames, false)
				request.CertificatePEM += string(chain)
				type savedWorkflow struct {
					ID           string         `json:"id"`
					GraphContent map[string]any `json:"graphContent"`
					GraphDraft   map[string]any `json:"graphDraft"`
					HasContent   bool           `json:"hasContent"`
					HasDraft     bool           `json:"hasDraft"`
				}
				workflowPath := "/api/collections/workflow/records/" + workflowID
				accessPath := "/api/collections/access/records/" + accessID
				var saved savedWorkflow
				var savedAccess map[string]any
				if err := api.request(ctx, http.MethodGet, workflowPath, nil, &saved); err != nil {
					return nil, err
				}
				if err := api.request(ctx, http.MethodGet, accessPath, nil, &savedAccess); err != nil {
					return nil, err
				}
				for _, graph := range []map[string]any{saved.GraphContent, saved.GraphDraft} {
					nodes, ok := graph["nodes"].([]any)
					if !ok {
						return nil, fmt.Errorf("saved workflow has no graph nodes")
					}
					updatedUploads := 0
					for _, raw := range nodes {
						node, ok := raw.(map[string]any)
						if !ok || node["id"] != "upload" || node["type"] != "bizUpload" {
							continue
						}
						data, _ := node["data"].(map[string]any)
						config, _ := data["config"].(map[string]any)
						if config == nil {
							return nil, fmt.Errorf("saved upload node has no configuration")
						}
						config["certificate"] = request.CertificatePEM
						config["privateKey"] = request.PrivateKeyPEM
						updatedUploads++
					}
					if updatedUploads != 1 {
						return nil, fmt.Errorf("saved workflow has %d upload nodes, want one", updatedUploads)
					}
				}
				if err := api.request(ctx, http.MethodPatch, workflowPath, map[string]any{"graphContent": saved.GraphContent, "graphDraft": saved.GraphDraft}, nil); err != nil {
					return nil, err
				}
				var updated savedWorkflow
				var updatedAccess map[string]any
				if err := api.request(ctx, http.MethodGet, workflowPath, nil, &updated); err != nil {
					return nil, err
				}
				if err := api.request(ctx, http.MethodGet, accessPath, nil, &updatedAccess); err != nil {
					return nil, err
				}
				if !reflect.DeepEqual(saved, updated) || !reflect.DeepEqual(savedAccess, updatedAccess) {
					return nil, fmt.Errorf("fresh certificate update changed the saved graph, workflow state, or access record")
				}
				result, err := deploy(ctx, request)
				if accessID != persistedAccessID || workflowID != persistedWorkflowID {
					t.Fatal("fresh certificate deployment replaced the saved records")
				}
				return result, err
			}
			assertRun := func(scenario string, wantSuccess bool) {
				t.Helper()
				invoke := deploy
				if wantSuccess {
					invoke = deploySuccess
				}
				_, err := invoke(ctx, lastSuccess)
				if wantSuccess && err != nil {
					t.Fatalf("%s: %v", scenario, err)
				}
				if wantSuccess && len(targets) != 0 {
					targets[0].verify(lastSuccess)
				}
				if !wantSuccess && (err == nil || !strings.Contains(err.Error(), "provider '"+provider+"' not registered")) {
					t.Fatalf("%s: expected missing provider error, got %v", scenario, err)
				}
				if accessID != persistedAccessID || workflowID != persistedWorkflowID {
					t.Fatal("lifecycle verification replaced the saved records")
				}
				t.Logf("LIFECYCLE PASS %s %s workflow=%s", provider, scenario, workflowID)
			}
			packageDir := filepath.Join(pluginDir, provider)
			if len(targets) != 0 {
				verifyWorkflowReloadLifecycle(t, ctx, api, provider, packageDir, accessID, lastSuccess, deploy, deploySuccess, targets[0])
			}
			parkedDir := filepath.Join(t.TempDir(), provider)
			stopCore()
			if err := os.Rename(packageDir, parkedDir); err != nil {
				t.Fatal(err)
			}
			startCore()
			assertCatalog(ctx, false)
			assertRun("missing_plugin", false)
			stopCore()
			if err := os.Rename(parkedDir, packageDir); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(packageDir, "manifest.json")
			original, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(original, &manifest); err != nil {
				t.Fatal(err)
			}
			manifest["protocol_version"] = plugin.ProtocolVersion + 1
			incompatible, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, incompatible, 0o600); err != nil {
				t.Fatal(err)
			}
			startCore()
			assertCatalog(ctx, false)
			assertRun("incompatible_plugin", false)
			stopCore()
			if err := os.WriteFile(manifestPath, original, 0o600); err != nil {
				t.Fatal(err)
			}
			startCore()
			assertCatalog(ctx, true)
			assertRun("compatible_plugin_restored", true)
			stopCore()
			startCore()
			assertCatalog(ctx, true)
			assertRun("core_restart_saved_workflow", true)
		})
	}
	return deploy
}
