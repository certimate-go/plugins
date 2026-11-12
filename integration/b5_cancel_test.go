package integration

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func TestB5WorkflowCancellation(t *testing.T) {
	binary := os.Getenv("CERTIMATE_B5_CANCEL_CORE")
	packages := os.Getenv("CERTIMATE_B5_CANCEL_PLUGINS")
	if binary == "" || packages == "" {
		t.Skip("set CERTIMATE_B5_CANCEL_CORE and CERTIMATE_B5_CANCEL_PLUGINS for real workflow verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	provider := "ucloud-uewaf"
	target := &b5UCloudTarget{provider: provider, uploads: map[string]b5UCloudCertificate{}}
	var hold atomic.Bool
	entered, aborted := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hold.CompareAndSwap(true, false) {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				return
			}
			_ = r.Body.Close()
			close(entered)
			select {
			case <-r.Context().Done():
				close(aborted)
			case <-ctx.Done():
			}
			return
		}
		target.serve(w, r)
	}))
	defer func() { cancel(); server.Close() }()
	target.url = server.URL
	dataDir, pluginDir := t.TempDir(), t.TempDir()
	if err := os.CopyFS(filepath.Join(pluginDir, provider), os.DirFS(filepath.Join(packages, provider))); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.Create(filepath.Join(dataDir, "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	env := append(os.Environ(), "CERTIMATE_PLUGIN_DIR="+pluginDir)
	for _, args := range [][]string{{"migrate", "up", "--dir", dataDir}, {"superuser", "upsert", "cancel@example.test", "B5-cancel-fixture123!", "--dir", dataDir}} {
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env, cmd.Stdout, cmd.Stderr = env, logFile, logFile
		if err := cmd.Run(); err != nil {
			t.Fatalf("core setup: %v", err)
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
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
		if t.Failed() {
			data, _ := os.ReadFile(filepath.Join(dataDir, "core.log"))
			t.Log(string(data))
		}
	}()
	api := &b1WorkflowAPI{baseURL: "http://" + address, client: &http.Client{Timeout: 5 * time.Second}}
	for api.request(ctx, http.MethodGet, "/api/health", nil, nil) != nil {
		select {
		case <-ctx.Done():
			t.Fatal("core did not become ready")
		case <-exited:
			t.Fatal("core exited before readiness")
		case <-time.After(25 * time.Millisecond):
		}
	}
	var auth struct {
		Token string `json:"token"`
	}
	if err := api.request(ctx, http.MethodPost, "/api/collections/_superusers/auth-with-password", map[string]string{"identity": "cancel@example.test", "password": "B5-cancel-fixture123!"}, &auth); err != nil {
		t.Fatal(err)
	}
	api.token = auth.Token
	var access struct {
		ID string `json:"id"`
	}
	if err := api.request(ctx, http.MethodPost, "/api/collections/access/records", map[string]any{"name": "Cancellation UCloud", "provider": "ucloud", "config": map[string]string{"publicKey": b5UCloudPublicKey, "privateKey": b5UCloudPrivateKey, "projectId": "project-fixture"}}, &access); err != nil {
		t.Fatal(err)
	}
	certificate, key, _ := makeCertificate(t, []string{b5UCloudDomain}, false)
	issuer, _, _ := makeCertificate(t, []string{"issuer.example.com"}, true)
	certificate += issuer
	uploadConfig := map[string]any{"source": "form", "certificate": certificate, "privateKey": key}
	node := func(id, kind string, config map[string]any) map[string]any {
		return map[string]any{"id": id, "type": kind, "data": map[string]any{"name": id, "config": config}}
	}
	graph := map[string]any{"nodes": []any{
		node("start", "start", map[string]any{}),
		node("upload", "bizUpload", uploadConfig),
		node("deploy", "bizDeploy", map[string]any{"certificateOutputNodeId": "upload", "provider": provider, "providerAccessId": access.ID, "providerConfig": map[string]string{"endpoint": server.URL, "domain": b5UCloudDomain}, "skipOnLastSucceeded": false}),
		node("end", "end", map[string]any{}),
	}}
	var workflow struct {
		ID string `json:"id"`
	}
	if err := api.request(ctx, http.MethodPost, "/api/collections/workflow/records", map[string]any{"name": "B5 cancellation", "trigger": "manual", "enabled": true, "hasContent": true, "hasDraft": false, "graphContent": graph, "graphDraft": graph}, &workflow); err != nil {
		t.Fatal(err)
	}
	startRun := func() string {
		t.Helper()
		var started struct {
			Code int `json:"code"`
			Data struct {
				RunID string `json:"runId"`
			} `json:"data"`
		}
		if err := api.request(ctx, http.MethodPost, "/api/workflows/"+workflow.ID+"/runs", map[string]string{"trigger": "manual"}, &started); err != nil {
			t.Fatal(err)
		}
		if started.Code != 0 || started.Data.RunID == "" {
			t.Fatalf("workflow did not start: %+v", started)
		}
		return started.Data.RunID
	}
	waitStatus := func(id, expected string) {
		t.Helper()
		for {
			var run struct {
				Status string `json:"status"`
				Error  string `json:"error"`
			}
			if err := api.request(ctx, http.MethodGet, "/api/collections/workflow_run/records/"+id, nil, &run); err != nil {
				t.Fatal(err)
			}
			switch run.Status {
			case "succeeded", "failed", "canceled":
				if run.Status != expected {
					t.Fatalf("run %s status %s, want %s: %s", id, run.Status, expected, run.Error)
				}
				return
			}
			select {
			case <-ctx.Done():
				t.Fatalf("run %s did not finish", id)
			case <-time.After(25 * time.Millisecond):
			}
		}
	}
	initial := startRun()
	waitStatus(initial, "succeeded")
	target.verify(t, &plugin.DeployRequest{CertificatePEM: certificate, PrivateKeyPEM: key})
	newCertificate, newKey, _ := makeCertificate(t, []string{b5UCloudDomain}, false)
	newCertificate += issuer
	uploadConfig["certificate"], uploadConfig["privateKey"] = newCertificate, newKey
	if err := api.request(ctx, http.MethodPatch, "/api/collections/workflow/records/"+workflow.ID, map[string]any{"graphContent": graph, "graphDraft": graph}, nil); err != nil {
		t.Fatal(err)
	}
	hold.Store(true)
	interrupted := startRun()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("target did not receive held request")
	}
	var canceled struct {
		Code int `json:"code"`
	}
	if err := api.request(ctx, http.MethodPost, "/api/workflows/"+workflow.ID+"/runs/"+interrupted+"/cancel", nil, &canceled); err != nil {
		t.Fatal(err)
	}
	if canceled.Code != 0 {
		t.Fatalf("cancel endpoint returned code %d", canceled.Code)
	}
	select {
	case <-aborted:
	case <-time.After(10 * time.Second):
		t.Fatal("workflow cancellation did not close the target HTTP request")
	}
	waitStatus(interrupted, "canceled")
	target.verify(t, &plugin.DeployRequest{CertificatePEM: certificate, PrivateKeyPEM: key})
	target.mu.Lock()
	writes, bindings := len(target.uploads), target.bindings
	target.mu.Unlock()
	if writes != 1 || bindings != 1 {
		t.Fatalf("cancellation changed target writes/bindings: %d/%d", writes, bindings)
	}
	retry := startRun()
	waitStatus(retry, "succeeded")
	target.verify(t, &plugin.DeployRequest{CertificatePEM: newCertificate, PrivateKeyPEM: newKey})
	waitStatus(interrupted, "canceled")
	t.Logf("CANCEL PASS %s workflow=%s initial=%s canceled=%s retry=%s; held HTTP request disconnected, old binding preserved, new chain and key deployed on retry", provider, workflow.ID, initial, interrupted, retry)
}
