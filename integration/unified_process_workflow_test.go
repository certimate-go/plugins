package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	unifiedNotifyProvider = "ntfy"
	unifiedTopic          = "unified-alerts"
	unifiedSubject        = "Unified workflow notification"
	unifiedMessage        = "deploy, notify and purge through one plugin service"
)

var unifiedPlugins = []string{unifiedNotifyProvider, purgeLegacyProvider, purgePurgerProvider}

func TestUnifiedProcessWorkflow(t *testing.T) {
	if os.Getenv("CERTIMATE_B1_CORE_BINARY") == "" {
		t.Skip("set CERTIMATE_B1_CORE_BINARY to the Certimate executable for unified workflow verification")
	}
	if runtime.GOOS != "linux" || os.Getenv("CERTIMATE_PURGE_BINARIES") == "" {
		runUnifiedWorkflowInDocker(t)
		return
	}
	runUnifiedWorkflowScenarios(t, os.Getenv("CERTIMATE_PURGE_BINARIES"))
}

func runUnifiedWorkflowScenarios(t *testing.T, binaryDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	fixture := newPurgeBaiduFixture(t)
	core := startPurgeCore(t, binaryDir, unifiedPlugins)

	core.assertCatalogEntry(unifiedNotifyProvider, "notify", true)
	core.assertCatalogEntry(purgeLegacyProvider, "deploy", true)
	core.assertCatalogEntry(purgePurgerProvider, "purge", true)

	type unifiedNotification struct {
		Topic   string `json:"topic"`
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	delivered := make(chan unifiedNotification, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload unifiedNotification
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || r.Method != http.MethodPost || r.URL.Path != "/push" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		delivered <- payload
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	issuer, _, _ := makeCertificate(t, []string{"issuer.example.com"}, true)
	certPEM, keyPEM, _ := makeCertificate(t, []string{"unified.example.com"}, false)
	certPEM += issuer

	fixture.seed("unified-expired-30d", 30)
	fixture.seed("unified-expired-10d", 10)
	fixture.seed("unified-expired-2d", 2)

	baiduAccessID := core.createAccess(ctx, "baiducloud", map[string]any{"accessKeyId": purgeBaiduAccessKey, "secretAccessKey": purgeBaiduSecretKey})
	ntfyAccessID := core.createAccess(ctx, unifiedNotifyProvider, map[string]any{"serverUrl": upstream.URL + "/push", "topic": unifiedTopic})

	graph := buildUnifiedGraph(baiduAccessID, ntfyAccessID, certPEM, keyPEM, float64(7))
	workflowID := core.importWorkflow(ctx, "Unified process workflow", graph)
	result := core.runWorkflow(ctx, workflowID, []string{purgeBaiduSecretKey})
	if result.status != "succeeded" {
		t.Fatalf("unified workflow status %s: %s", result.status, result.runError)
	}

	certs, deletes, _, _ := fixture.snapshot()
	if strings.Join(deletes, ",") != "unified-expired-30d,unified-expired-10d" {
		t.Fatalf("purge node deleted %v, want [unified-expired-30d unified-expired-10d]", deletes)
	}
	for _, keep := range []string{"unified-expired-2d", "cert-1"} {
		if !contains(certs, keep) {
			t.Fatalf("certificate %s was removed unexpectedly; remaining %v", keep, certs)
		}
	}
	if fixture.uploadCount() != 1 {
		t.Fatalf("deploy node uploads=%d, want 1", fixture.uploadCount())
	}
	if count, ok := core.purgeLogCount(ctx, result.runID); !ok || count != 2 {
		t.Fatalf("purge completed log count=%d found=%t, want 2", count, ok)
	}

	select {
	case got := <-delivered:
		if got.Topic != unifiedTopic || got.Title != unifiedSubject || got.Message != unifiedMessage {
			t.Fatalf("unexpected notification payload: %+v", got)
		}
	default:
		t.Fatal("notify node did not deliver a notification")
	}

	waitForPluginRelease(t, unifiedPlugins...)
	certsAfter, deletesAfter, listsAfter, deleteCallsAfter := fixture.snapshot()
	time.Sleep(2 * time.Second)
	certsLater, deletesLater, listsLater, deleteCallsLater := fixture.snapshot()
	if fmt.Sprint(certsAfter) != fmt.Sprint(certsLater) || fmt.Sprint(deletesAfter) != fmt.Sprint(deletesLater) || listsAfter != listsLater || deleteCallsAfter != deleteCallsLater {
		t.Fatal("vendor fixture state changed after the workflow completed")
	}
}

func buildUnifiedGraph(baiduAccessID, ntfyAccessID, certPEM, keyPEM string, expiredDays any) map[string]any {
	node := func(id, kind string, config map[string]any) map[string]any {
		return map[string]any{"id": id, "type": kind, "data": map[string]any{"name": id, "config": config}}
	}
	upload := node("upload", "bizUpload", map[string]any{"source": "form", "certificate": certPEM, "privateKey": keyPEM})
	deploy := node("deploy", "bizDeploy", map[string]any{"certificateOutputNodeId": "upload", "provider": purgeLegacyProvider, "providerAccessId": baiduAccessID, "providerConfig": map[string]any{}, "skipOnLastSucceeded": false})
	notify := node("notify", "bizNotify", map[string]any{"provider": unifiedNotifyProvider, "providerAccessId": ntfyAccessID, "providerConfig": map[string]any{}, "subject": unifiedSubject, "message": unifiedMessage, "skipOnAllPrevSkipped": false})
	purge := node("purge", "bizPurge", map[string]any{"provider": purgePurgerProvider, "providerAccessId": baiduAccessID, "providerConfig": map[string]any{}, "expiredDays": expiredDays})
	nodes := []any{node("start", "start", map[string]any{}), upload, deploy, notify, purge, node("end", "end", map[string]any{})}
	return map[string]any{"nodes": nodes}
}

func countPluginProcesses(names ...string) (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		for _, name := range names {
			if strings.Contains(string(raw), name) {
				count++
				break
			}
		}
	}
	return count, nil
}

func waitForPluginRelease(t *testing.T, names ...string) {
	t.Helper()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if count, _ := countPluginProcesses(names...); count == 0 {
			return
		}
	}
	if count, _ := countPluginProcesses(names...); count != 0 {
		t.Fatalf("%d plugin processes still running after workflow completion", count)
	}
}

func runUnifiedWorkflowInDocker(t *testing.T) {
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
	buildB4CloudPackages(t, dir, unifiedPlugins, "linux", arch)
	run(env, "go", "-C", root, "test", "-c", "-o", filepath.Join(dir, "unified-process.test"), "./integration")
	binary, err := filepath.Abs(os.Getenv("CERTIMATE_B1_CORE_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	image := os.Getenv("CERTIMATE_E2E_IMAGE")
	if image == "" {
		image = "golang:1.26-bookworm"
	}
	output := run(nil, "docker", "run", "--rm", "--network=none",
		"--add-host=certificate.baidubce.com:127.0.0.1",
		"--mount", "type=bind,src="+dir+",dst=/fixtures,readonly",
		"--mount", "type=bind,src="+binary+",dst=/core,readonly",
		"-e", "CERTIMATE_B1_CORE_BINARY=/core",
		"-e", "CERTIMATE_PURGE_BINARIES=/fixtures",
		image, "/fixtures/unified-process.test", "-test.run=^TestUnifiedProcessWorkflow$", "-test.v", "-test.timeout=10m")
	t.Logf("Linux unified workflow output:\n%s", output)
}
