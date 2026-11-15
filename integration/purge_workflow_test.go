package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

type purgeCore struct {
	t           *testing.T
	binary      string
	dataDir     string
	pluginDir   string
	pluginNames []string
	logPath     string

	api     *b1WorkflowAPI
	cmd     *exec.Cmd
	exited  chan struct{}
	waitErr error
}

func purgeCoreBinary(t *testing.T) string {
	t.Helper()
	binary := os.Getenv("CERTIMATE_B1_CORE_BINARY")
	if binary == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for purge workflow verification")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatal(err)
	}
	return binary
}

func startPurgeCore(t *testing.T, binaryDir string, pluginNames []string) *purgeCore {
	t.Helper()
	c := &purgeCore{
		t:           t,
		binary:      purgeCoreBinary(t),
		dataDir:     t.TempDir(),
		pluginDir:   t.TempDir(),
		pluginNames: pluginNames,
	}
	logFile, err := os.Create(filepath.Join(c.dataDir, "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	c.logPath = logFile.Name()
	t.Cleanup(func() {
		_ = logFile.Close()
		if c.t.Failed() {
			if data, readErr := os.ReadFile(c.logPath); readErr == nil {
				t.Logf("core log:\n%s", data)
			}
		}
	})
	for _, name := range pluginNames {
		if err := os.CopyFS(filepath.Join(c.pluginDir, name), os.DirFS(filepath.Join(binaryDir, name))); err != nil {
			t.Fatal(err)
		}
	}
	c.api = &b1WorkflowAPI{baseURL: "", client: &http.Client{Timeout: 10 * time.Second}}
	c.start()
	return c
}

func (c *purgeCore) start() {
	c.t.Helper()
	setupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c.api.token = ""
	env := append(os.Environ(), "CERTIMATE_PLUGIN_DIR="+c.pluginDir)
	logFile, err := os.OpenFile(c.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		c.t.Fatal(err)
	}
	defer logFile.Close()
	for _, args := range [][]string{
		{"migrate", "up", "--dir", c.dataDir},
		{"superuser", "upsert", "purge@example.test", "Purge-fixture-password123!", "--dir", c.dataDir},
	} {
		cmd := exec.CommandContext(setupCtx, c.binary, args...)
		cmd.Env = env
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Run(); err != nil {
			c.t.Fatalf("core setup %s: %v", args[0], err)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		c.t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		c.t.Fatal(err)
	}
	c.api.baseURL = "http://" + address
	cmd := exec.Command(c.binary, "serve", "--http", address, "--dir", c.dataDir)
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		c.t.Fatal(err)
	}
	c.cmd = cmd
	c.exited = make(chan struct{})
	go func() {
		c.waitErr = cmd.Wait()
		close(c.exited)
	}()
	for {
		err := c.api.request(setupCtx, http.MethodGet, "/api/health", nil, nil)
		if err == nil {
			break
		}
		select {
		case <-c.exited:
			c.t.Fatalf("core exited before readiness: %v", c.waitErr)
		case <-setupCtx.Done():
			c.t.Fatalf("core did not become ready: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
	var auth struct {
		Token string `json:"token"`
	}
	if err := c.api.request(setupCtx, http.MethodPost, "/api/collections/_superusers/auth-with-password", map[string]string{"identity": "purge@example.test", "password": "Purge-fixture-password123!"}, &auth); err != nil {
		c.t.Fatal(err)
	}
	if auth.Token == "" {
		c.t.Fatal("core authentication returned no token")
	}
	c.api.token = auth.Token
}

func (c *purgeCore) stop() {
	c.t.Helper()
	if c.cmd == nil {
		return
	}
	_ = c.cmd.Process.Signal(os.Interrupt)
	select {
	case <-c.exited:
	case <-time.After(10 * time.Second):
		_ = c.cmd.Process.Kill()
		<-c.exited
	}
	c.cmd = nil
	c.api.client.CloseIdleConnections()
}

func (c *purgeCore) restart() {
	c.t.Helper()
	c.stop()
	c.start()
}

func (c *purgeCore) parkPlugin(name string) func() {
	c.t.Helper()
	parked := filepath.Join(c.t.TempDir(), name)
	if err := os.Rename(filepath.Join(c.pluginDir, name), parked); err != nil {
		c.t.Fatal(err)
	}
	return func() {
		if err := os.Rename(parked, filepath.Join(c.pluginDir, name)); err != nil {
			c.t.Fatal(err)
		}
	}
}

func (c *purgeCore) assertCatalogEntry(provider, capability string, wantLoaded bool) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var catalog struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			ProviderType string `json:"providerType"`
			Capability   string `json:"capability"`
			Source       string `json:"source"`
		} `json:"data"`
	}
	if err := c.api.request(ctx, http.MethodGet, "/api/plugin-catalog", nil, &catalog); err != nil {
		c.t.Fatal(err)
	}
	if catalog.Code != 0 {
		c.t.Fatalf("plugin catalog: code=%d message=%s", catalog.Code, catalog.Msg)
	}
	loaded := false
	for _, entry := range catalog.Data {
		if entry.ProviderType == provider && entry.Source == "plugin" {
			loaded = true
			if entry.Capability != capability {
				c.t.Fatalf("plugin %s catalog capability=%q, want %q", provider, entry.Capability, capability)
			}
		}
	}
	if loaded != wantLoaded {
		c.t.Fatalf("plugin %s catalog registration=%t, want %t", provider, loaded, wantLoaded)
	}
}

func (c *purgeCore) fetchSchemaEnvelope(provider string, category string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	path := "/api/provider-schemas/" + provider
	if category != "" {
		path += "?category=" + category
	}
	var payload struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := c.api.request(ctx, http.MethodGet, path, nil, &payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("provider schema endpoint: code=%d message=%s", payload.Code, payload.Msg)
	}
	return payload.Data, nil
}

type purgeWorkflowRequest struct {
	name           string
	provider       string
	accessProvider string
	accessConfig   map[string]any
	accessID       string
	providerConfig map[string]any
	expiredDays    any
	nested         bool
	deployNode     bool
	deployProvider string
	certificatePEM string
	privateKeyPEM  string
}

type purgeWorkflowResult struct {
	workflowID string
	runID      string
	status     string
	runError   string
}

func (c *purgeCore) createAccess(ctx context.Context, provider string, config map[string]any) string {
	c.t.Helper()
	var access struct {
		ID string `json:"id"`
	}
	if err := c.api.request(ctx, http.MethodPost, "/api/collections/access/records", map[string]any{"name": "Purge " + provider, "provider": provider, "config": config}, &access); err != nil {
		c.t.Fatal(err)
	}
	if access.ID == "" {
		c.t.Fatal("core did not persist the access record")
	}
	return access.ID
}

func (c *purgeCore) importWorkflow(ctx context.Context, name string, graph map[string]any) string {
	c.t.Helper()
	var workflow struct {
		ID string `json:"id"`
	}
	payload := map[string]any{
		"name": name, "description": "Isolated purge plugin verification", "trigger": "manual", "enabled": true,
		"hasContent": true, "hasDraft": false, "graphContent": graph, "graphDraft": graph,
	}
	if err := c.api.request(ctx, http.MethodPost, "/api/collections/workflow/records", payload, &workflow); err != nil {
		c.t.Fatal(err)
	}
	if workflow.ID == "" {
		c.t.Fatal("core did not persist the workflow record")
	}
	return workflow.ID
}

func buildPurgeGraph(req purgeWorkflowRequest, accessID string) map[string]any {
	node := func(id, kind string, config map[string]any) map[string]any {
		return map[string]any{"id": id, "type": kind, "data": map[string]any{"name": id, "config": config}}
	}
	purgeConfig := map[string]any{"provider": req.provider, "providerAccessId": accessID, "providerConfig": req.providerConfig}
	if req.expiredDays != nil {
		purgeConfig["expiredDays"] = req.expiredDays
	}
	purgeNode := node("purge", "bizPurge", purgeConfig)
	var nodes []any
	if req.nested {
		branch := map[string]any{
			"id": "branch", "type": "branchBlock",
			"data":   map[string]any{"name": "branch", "config": map[string]any{}},
			"blocks": []any{purgeNode},
		}
		nodes = []any{node("start", "start", map[string]any{}), branch, node("end", "end", map[string]any{})}
	} else {
		nodes = []any{node("start", "start", map[string]any{}), purgeNode, node("end", "end", map[string]any{})}
	}
	if req.deployNode {
		deployConfig := map[string]any{"certificateOutputNodeId": "upload", "provider": req.deployProvider, "providerAccessId": accessID, "providerConfig": map[string]any{}, "skipOnLastSucceeded": false}
		upload := node("upload", "bizUpload", map[string]any{"source": "form", "certificate": req.certificatePEM, "privateKey": req.privateKeyPEM})
		deploy := node("deploy", "bizDeploy", deployConfig)
		nodes = append([]any{node("start", "start", map[string]any{}), upload, deploy}, nodes[1:]...)
	}
	return map[string]any{"nodes": nodes}
}

func (c *purgeCore) runWorkflow(ctx context.Context, workflowID string, secrets []string) purgeWorkflowResult {
	c.t.Helper()
	var started struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	if err := c.api.request(ctx, http.MethodPost, "/api/workflows/"+workflowID+"/runs", map[string]string{"trigger": "manual"}, &started); err != nil {
		c.t.Fatal(err)
	}
	if started.Code != 0 || started.Data.RunID == "" {
		c.t.Fatalf("workflow trigger returned no run ID: code=%d message=%s", started.Code, started.Msg)
	}
	result := purgeWorkflowResult{workflowID: workflowID, runID: started.Data.RunID}
	completed := false
	defer func() {
		if completed {
			return
		}
		cancelCtx, cancelRequest := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelRequest()
		_ = c.api.request(cancelCtx, http.MethodPost, "/api/workflows/"+workflowID+"/runs/"+result.runID+"/cancel", nil, nil)
	}()
	for {
		var run struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := c.api.request(ctx, http.MethodGet, "/api/collections/workflow_run/records/"+result.runID, nil, &run); err != nil {
			c.t.Fatal(err)
		}
		switch run.Status {
		case "succeeded", "failed", "canceled":
			completed = true
			result.status = run.Status
			result.runError = run.Error
			if err := c.api.inspectLogs(ctx, result.runID, run.Error, secrets); err != nil {
				c.t.Error(err)
			}
			return result
		case "pending", "processing":
		default:
			c.t.Fatalf("unexpected workflow status %q", run.Status)
		}
		select {
		case <-ctx.Done():
			c.t.Fatal(ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (c *purgeCore) runPurgeWorkflow(ctx context.Context, req purgeWorkflowRequest, secrets ...string) purgeWorkflowResult {
	c.t.Helper()
	accessProvider := req.accessProvider
	if accessProvider == "" {
		accessProvider = "baiducloud"
	}
	accessID := req.accessID
	if accessID == "" {
		accessID = c.createAccess(ctx, accessProvider, req.accessConfig)
	}
	graph := buildPurgeGraph(req, accessID)
	workflowID := c.importWorkflow(ctx, req.name, graph)
	result := c.runWorkflow(ctx, workflowID, secrets)
	if result.status == "succeeded" {
		c.t.Logf("purge workflow %s run %s succeeded", workflowID, result.runID)
	}
	return result
}

func (c *purgeCore) workflowGraphs(ctx context.Context, workflowID string) (content, draft map[string]any) {
	c.t.Helper()
	var record struct {
		GraphContent map[string]any `json:"graphContent"`
		GraphDraft   map[string]any `json:"graphDraft"`
	}
	if err := c.api.request(ctx, http.MethodGet, "/api/collections/workflow/records/"+workflowID, nil, &record); err != nil {
		c.t.Fatal(err)
	}
	if record.GraphContent == nil || record.GraphDraft == nil {
		c.t.Fatal("saved workflow has no graph content")
	}
	return record.GraphContent, record.GraphDraft
}

func (c *purgeCore) assertGraphsUnchanged(ctx context.Context, workflowID string, wantContent, wantDraft map[string]any) {
	c.t.Helper()
	content, draft := c.workflowGraphs(ctx, workflowID)
	if !reflect.DeepEqual(content, wantContent) || !reflect.DeepEqual(draft, wantDraft) {
		c.t.Fatal("saved workflow graph was modified by reading or running")
	}
}

func (c *purgeCore) purgeLogCount(ctx context.Context, runID string) (int, bool) {
	c.t.Helper()
	var logs struct {
		Items []struct {
			NodeId  string         `json:"nodeId"`
			Message string         `json:"message"`
			Data    map[string]any `json:"data"`
		} `json:"items"`
	}
	path := "/api/collections/workflow_logs/records?filter=" + url.QueryEscape("runRef='"+runID+"'") + "&perPage=200"
	if err := c.api.request(ctx, http.MethodGet, path, nil, &logs); err != nil {
		c.t.Fatal(err)
	}
	for _, record := range logs.Items {
		if record.Message != "purge completed" {
			continue
		}
		if count, ok := record.Data["count"]; ok {
			switch count := count.(type) {
			case float64:
				return int(count), true
			case json.Number:
				value, err := count.Float64()
				if err != nil {
					c.t.Fatal(err)
				}
				return int(value), true
			case string:
				value, err := strconv.Atoi(count)
				if err != nil {
					c.t.Fatal(err)
				}
				return value, true
			}
		}
	}
	for _, record := range logs.Items {
		c.t.Logf("run %s log record: node=%s message=%q data=%v", runID, record.NodeId, record.Message, record.Data)
	}
	return 0, false
}

func runPurgeFamilyInDocker(t *testing.T, plugins []string, testBinary string, runPattern string, extraDockerArgs ...string) {
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
	buildB4CloudPackages(t, dir, plugins, "linux", arch)
	run(env, "go", "-C", root, "test", "-c", "-o", filepath.Join(dir, testBinary), "./integration")
	binary, err := filepath.Abs(os.Getenv("CERTIMATE_B1_CORE_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	dockerArgs := append([]string{"run", "--rm", "--network=none"}, extraDockerArgs...)
	dockerArgs = append(dockerArgs,
		"--mount", "type=bind,src="+dir+",dst=/fixtures,readonly",
		"--mount", "type=bind,src="+binary+",dst=/core,readonly",
		"-e", "CERTIMATE_B1_CORE_BINARY=/core",
		"-e", "CERTIMATE_PURGE_BINARIES=/fixtures",
		image, "/fixtures/"+testBinary, "-test.run="+runPattern, "-test.v", "-test.timeout=10m")
	output := run(nil, "docker", dockerArgs...)
	t.Logf("Linux purge workflow output:\n%s", output)
}

func killPluginProcesses(ctx context.Context, name string) error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	killed := false
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if !strings.Contains(string(raw), name) {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(entry.Name(), "%d", &pid); err != nil {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := proc.Signal(os.Kill); err == nil {
			killed = true
		}
	}
	if !killed {
		return fmt.Errorf("no plugin process found for %q", name)
	}
	return nil
}
