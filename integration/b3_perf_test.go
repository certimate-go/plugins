package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type b3PerfTarget struct {
	access, config map[string]any
	reset          func()
	verify         func(string, string) (int, int)
}

type b3PerfSample struct {
	ElapsedMS          float64        `json:"elapsedMs"`
	RSSBytes           uint64         `json:"rssBytes"`
	PIDs               []int          `json:"pids"`
	ProcessRSS         map[int]uint64 `json:"processRSSBytes"`
	ProcessExecutables map[int]string `json:"processExecutables"`
}

type b3PerfRun struct {
	Provider              string         `json:"provider"`
	Side                  string         `json:"side"`
	Kind                  string         `json:"kind"`
	Iteration             int            `json:"iteration"`
	Milliseconds          float64        `json:"milliseconds"`
	RunID                 string         `json:"runId"`
	Writes                int            `json:"writes"`
	Bindings              int            `json:"bindings"`
	PeakRSSBytes          uint64         `json:"sampledPeakRSSBytes,omitempty"`
	CgroupMemoryPeakBytes uint64         `json:"cgroupMemoryPeakBytes,omitempty"`
	CPUStartStat          string         `json:"cpuStartStat,omitempty"`
	CPUStat               string         `json:"cpuStat,omitempty"`
	HarnessCPUStartStat   string         `json:"harnessCpuStartStat,omitempty"`
	HarnessCPUStat        string         `json:"harnessCpuStat,omitempty"`
	Samples               []b3PerfSample `json:"samples,omitempty"`
}

type b3PerfCore struct {
	api        *b1WorkflowAPI
	workflowID string
	cgroup     string
	pid        int
	cmd        *exec.Cmd
	log        *os.File
}

func TestB3WorkflowPerformance(t *testing.T) {
	root := os.Getenv("CERTIMATE_B3_PERF_ROOT")
	if root == "" {
		t.Skip("set CERTIMATE_B3_PERF_ROOT in an isolated Linux cgroup v2 container")
	}
	if runtime.GOOS != "linux" {
		t.Fatal("Linux cgroup v2 is required")
	}
	artifactPrefix := "all-"
	phases := []struct {
		kind  string
		count int
	}{{"cold", 1}, {"warm", 20}, {"diagnostic", 5}}
	if os.Getenv("CERTIMATE_B3_PERF_MEMORY_ONLY") == "1" {
		artifactPrefix = "all-memory-"
		phases = phases[2:]
	}
	cert := b3PerfRead(t, filepath.Join(root, "certificate.pem"))
	key := b3PerfRead(t, filepath.Join(root, "leaf-key.pem"))
	providers := append(append(append([]string{}, b3EdgeProviders...), b3CloudProviders...), b3PlatformProviders...)
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			target := b3PerfFixture(t, provider)
			var runs []b3PerfRun
			defer func() {
				data, err := json.MarshalIndent(runs, "", "  ")
				if err != nil {
					t.Error(err)
					return
				}
				if err := os.WriteFile(filepath.Join(root, artifactPrefix+provider+".json"), data, 0o600); err != nil {
					t.Error(err)
				}
			}()
			cores := map[string]*b3PerfCore{}
			for _, side := range []string{"before", "after"} {
				core, startup := b3PerfStart(t, root, provider, side, artifactPrefix)
				cores[side] = core
				runs = append(runs, b3PerfRun{Provider: provider, Side: side, Kind: "startup", Milliseconds: startup})
				core.seed(t, provider, target, cert, key)
			}
			for _, phase := range phases {
				for iteration := 0; iteration < phase.count; iteration++ {
					order := []string{"before", "after"}
					if iteration%2 == 1 {
						order = []string{"after", "before"}
					}
					for _, side := range order {
						core := cores[side]
						target.reset()
						var samples []b3PerfSample
						stop, done := make(chan struct{}), make(chan struct{})
						stopSampling := func() {}
						if phase.kind == "diagnostic" {
							var finish sync.Once
							stopSampling = func() { finish.Do(func() { close(stop); <-done }) }
							t.Cleanup(stopSampling)
							go func() {
								defer close(done)
								start := time.Now()
								for {
									sample := b3PerfRSS(core.cgroup)
									sample.ElapsedMS = float64(time.Since(start)) / float64(time.Millisecond)
									samples = append(samples, sample)
									select {
									case <-stop:
										return
									case <-time.After(time.Millisecond):
									}
								}
							}()
						}
						cpuStart := ""
						harnessCPUStart := ""
						if phase.kind == "diagnostic" {
							cpuStart = b3PerfRead(t, filepath.Join(core.cgroup, "cpu.stat"))
							harnessCPUStart = b3PerfRead(t, "/sys/fs/cgroup/harness/cpu.stat")
						}
						started := time.Now()
						runID := core.run(t)
						elapsed := float64(time.Since(started)) / float64(time.Millisecond)
						stopSampling()
						writes, bindings := target.verify(cert, key)
						row := b3PerfRun{Provider: provider, Side: side, Kind: phase.kind, Iteration: iteration + 1, Milliseconds: elapsed, RunID: runID, Writes: writes, Bindings: bindings, Samples: samples}
						if writes != 1 {
							t.Fatalf("expected exactly one target write, got %d", writes)
						}
						childObserved := false
						for _, sample := range samples {
							if sample.RSSBytes > row.PeakRSSBytes {
								row.PeakRSSBytes = sample.RSSBytes
							}
							if len(sample.PIDs) > 1 {
								childObserved = true
							}
						}
						if phase.kind == "diagnostic" {
							if side == "after" && !childObserved {
								t.Fatal("RSS sampler missed plugin process")
							}
							peak, err := strconv.ParseUint(strings.TrimSpace(b3PerfRead(t, filepath.Join(core.cgroup, "memory.peak"))), 10, 64)
							if err != nil {
								t.Fatal(err)
							}
							row.CgroupMemoryPeakBytes = peak
							row.CPUStartStat = cpuStart
							row.CPUStat = b3PerfRead(t, filepath.Join(core.cgroup, "cpu.stat"))
							row.HarnessCPUStartStat = harnessCPUStart
							row.HarnessCPUStat = b3PerfRead(t, "/sys/fs/cgroup/harness/cpu.stat")
						}
						deadline := time.Now().Add(2 * time.Second)
						for {
							pids := b3PerfRSS(core.cgroup).PIDs
							if len(pids) == 1 && pids[0] == core.pid {
								break
							}
							if time.Now().After(deadline) {
								t.Fatalf("residual processes after workflow: %v", pids)
							}
							time.Sleep(time.Millisecond)
						}
						runs = append(runs, row)
					}
				}
			}
			for _, side := range []string{"before", "after"} {
				if err := cores[side].stop(); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("PERF PASS %s: %d recorded runs, all target writes verified", provider, len(runs))
		})
	}
}

func b3PerfRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func b3PerfRSS(cgroup string) b3PerfSample {
	data, _ := os.ReadFile(filepath.Join(cgroup, "cgroup.procs"))
	sample := b3PerfSample{PIDs: []int{}, ProcessRSS: map[int]uint64{}, ProcessExecutables: map[int]string{}}
	for _, word := range strings.Fields(string(data)) {
		pid, err := strconv.Atoi(word)
		if err != nil {
			continue
		}
		imageBefore, _ := os.Readlink(filepath.Join("/proc", word, "exe"))
		raw, err := os.ReadFile(filepath.Join("/proc", word, "statm"))
		if err != nil {
			continue
		}
		fields := strings.Fields(string(raw))
		if len(fields) < 2 {
			continue
		}
		pages, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		imageAfter, _ := os.Readlink(filepath.Join("/proc", word, "exe"))
		if imageBefore == imageAfter {
			sample.ProcessExecutables[pid] = imageAfter
		} else {
			sample.ProcessExecutables[pid] = "changed-during-sample"
		}
		sample.ProcessRSS[pid] = pages * uint64(os.Getpagesize())
		sample.RSSBytes += sample.ProcessRSS[pid]
		sample.PIDs = append(sample.PIDs, pid)
	}
	return sample
}

func b3PerfStart(t *testing.T, root, provider, side, artifactPrefix string) (*b3PerfCore, float64) {
	t.Helper()
	workRoot := filepath.Join(root, "all-work")
	if err := os.MkdirAll(workRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	dataDir, err := os.MkdirTemp(workRoot, provider+"-"+side+"-")
	if err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(dataDir, "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if side == "after" {
		if err := os.CopyFS(filepath.Join(pluginDir, provider), os.DirFS(filepath.Join(root, "all-plugins", provider))); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(root, "..", "core-before-linux")
	if side == "after" {
		binary = filepath.Join(root, "..", "core-final-linux")
	}
	log, err := os.Create(filepath.Join(root, artifactPrefix+provider+"-"+side+".log"))
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "CERTIMATE_PLUGIN_DIR="+pluginDir)
	for _, args := range [][]string{{"migrate", "up", "--dir", dataDir}, {"superuser", "upsert", "perf@example.test", "Perf-fixture-password123!", "--dir", dataDir}} {
		cmd := exec.Command(binary, args...)
		cmd.Env, cmd.Stdout, cmd.Stderr = env, log, log
		if err := cmd.Run(); err != nil {
			t.Fatalf("setup %s: %v", args[0], err)
		}
	}
	cgroup := filepath.Join("/sys/fs/cgroup", "b3-perf-"+provider+"-"+side)
	if err := os.Mkdir(cgroup, 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	core := &b3PerfCore{api: &b1WorkflowAPI{baseURL: "http://" + address, client: &http.Client{Timeout: time.Second * 30, Transport: &http.Transport{}}}, cgroup: cgroup, log: log}
	core.cmd = exec.Command("sh", "-c", `echo $$ > "$1/cgroup.procs"; shift; exec "$@"`, "b3-perf", cgroup, binary, "serve", "--http", address, "--dir", dataDir)
	core.cmd.Env, core.cmd.Stdout, core.cmd.Stderr = env, log, log
	started := time.Now()
	if err := core.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	core.pid = core.cmd.Process.Pid
	t.Cleanup(func() {
		if err := core.stop(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for {
		if core.api.request(ctx, "GET", "/api/health", nil, nil) == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(5 * time.Millisecond)
	}
	elapsed := float64(time.Since(started)) / float64(time.Millisecond)
	var auth struct {
		Token string `json:"token"`
	}
	if err := core.api.request(ctx, "POST", "/api/collections/_superusers/auth-with-password", map[string]string{"identity": "perf@example.test", "password": "Perf-fixture-password123!"}, &auth); err != nil {
		t.Fatal(err)
	}
	core.api.token = auth.Token
	if auth.Token == "" {
		t.Fatal("missing auth token")
	}
	return core, elapsed
}

func (c *b3PerfCore) stop() error {
	if c.cmd == nil {
		return nil
	}
	_ = c.cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return err
		}
	case <-time.After(10 * time.Second):
		_ = c.cmd.Process.Kill()
		<-done
		return fmt.Errorf("core failed to stop gracefully")
	}
	c.cmd = nil
	c.api.client.CloseIdleConnections()
	c.log.Close()
	pids := b3PerfRSS(c.cgroup).PIDs
	if len(pids) != 0 {
		return fmt.Errorf("remaining cgroup processes: %v", pids)
	}
	return os.Remove(c.cgroup)
}

func (c *b3PerfCore) seed(t *testing.T, provider string, target b3PerfTarget, cert, key string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var access struct {
		ID string `json:"id"`
	}
	if err := c.api.request(ctx, "POST", "/api/collections/access/records", map[string]any{"name": "perf", "provider": strings.Split(provider, "-")[0], "config": target.access}, &access); err != nil {
		t.Fatal(err)
	}
	node := func(id, kind string, config map[string]any) map[string]any {
		return map[string]any{"id": id, "type": kind, "data": map[string]any{"name": id, "config": config}}
	}
	graph := map[string]any{"nodes": []any{
		node("start", "start", map[string]any{}),
		node("upload", "bizUpload", map[string]any{"source": "form", "certificate": cert, "privateKey": key}),
		node("deploy", "bizDeploy", map[string]any{"certificateOutputNodeId": "upload", "provider": provider, "providerAccessId": access.ID, "providerConfig": target.config, "skipOnLastSucceeded": false}),
		node("end", "end", map[string]any{}),
	}}
	var workflow struct {
		ID string `json:"id"`
	}
	if err := c.api.request(ctx, "POST", "/api/collections/workflow/records", map[string]any{"name": "perf " + provider, "trigger": "manual", "enabled": true, "hasContent": true, "hasDraft": false, "graphContent": graph, "graphDraft": graph}, &workflow); err != nil {
		t.Fatal(err)
	}
	c.workflowID = workflow.ID
	if workflow.ID == "" || access.ID == "" {
		t.Fatal("missing persisted workflow/access")
	}
}

func (c *b3PerfCore) run(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var started struct {
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	if err := c.api.request(ctx, "POST", "/api/workflows/"+c.workflowID+"/runs", map[string]string{"trigger": "manual"}, &started); err != nil {
		t.Fatal(err)
	}
	if started.Data.RunID == "" {
		t.Fatal("missing run ID")
	}
	for {
		var run struct{ Status, Error string }
		if err := c.api.request(ctx, "GET", "/api/collections/workflow_run/records/"+started.Data.RunID, nil, &run); err != nil {
			t.Fatal(err)
		}
		switch run.Status {
		case "succeeded":
			return started.Data.RunID
		case "pending", "processing":
			time.Sleep(time.Millisecond)
		default:
			t.Fatalf("workflow %s: %s", run.Status, run.Error)
		}
	}
}

func b3PerfFixture(t *testing.T, provider string) b3PerfTarget {
	t.Helper()
	contains := func(values []string) bool {
		for _, value := range values {
			if value == provider {
				return true
			}
		}
		return false
	}
	verify := func(gotCert, gotKey, wantCert, wantKey string) {
		t.Helper()
		if strings.TrimSpace(gotCert) != strings.TrimSpace(wantCert) || strings.TrimSpace(gotKey) != strings.TrimSpace(wantKey) {
			t.Fatal("target certificate chain or private key differs")
		}
	}
	if contains(b3EdgeProviders) {
		f := &b3EdgeFixture{t: t, provider: provider}
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		t.Cleanup(server.Close)
		newB3EdgeProxy(t, f)
		access := map[string]any{"apiKey": "fixture-key", "apiToken": "fixture-token", "accessToken": "fixture-token", "username": "fixture-user", "serverUrl": server.URL, "allowInsecureConnections": true}
		config := map[string]any{}
		switch provider {
		case "bunny-cdn":
			config = map[string]any{"pullZoneId": "42", "hostname": "edge.example.com"}
		case "cloudflare-ssl":
			config = map[string]any{"zoneId": "zone-42", "environment": "staging"}
		case "cpanel":
			config = map[string]any{"deployTarget": "website", "domain": "edge.example.com"}
		}
		return b3PerfTarget{access: access, config: config, reset: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.cert, f.key, f.resource, f.environment = "", "", "", ""
			f.writes = 0
		}, verify: func(cert, key string) (int, int) {
			f.mu.Lock()
			defer f.mu.Unlock()
			verify(f.cert, f.key, cert, key)
			if provider == "cloudflare-ssl" && f.environment != "staging" {
				t.Fatal("wrong target environment")
			}
			binding := 0
			want := map[string]string{"bunny-cdn": "42/edge.example.com", "cloudflare-ssl": "zone-42", "cpanel": "edge.example.com"}[provider]
			if want != "" {
				if f.resource != want {
					t.Fatal("wrong target binding")
				}
				binding = 1
			}
			return f.writes, binding
		}}
	}
	if contains(b3CloudProviders) {
		f := newB3CloudFixture(t, provider)
		access := map[string]any{"apiToken": "fixture-cloud-token", "accessToken": "fixture-cloud-token"}
		config := map[string]any{}
		switch provider {
		case "gcore-cdn":
			config = map[string]any{"resourceId": "17", "certificateId": "41"}
		case "linode-los":
			config = map[string]any{"regionId": "us-east-1", "bucket": "cloud.example.com"}
		case "netlify":
			config = map[string]any{"deployTarget": "website", "siteId": "fixture-site"}
		case "googlecloud-certificatemanager":
			signer, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal(err)
			}
			key := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(signer)}))
			sa, _ := json.Marshal(map[string]string{"type": "service_account", "project_id": "fixture-project", "private_key_id": "fixture-id", "private_key": key, "client_email": "fixture@fixture-project.iam.gserviceaccount.com", "token_uri": "https://oauth2.googleapis.com/token"})
			access = map[string]any{"projectId": "fixture-project", "serviceAccountKey": string(sa)}
		}
		return b3PerfTarget{access: access, config: config, reset: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.cert, f.key, f.name = "", "", ""
			f.writes, f.bindings, f.deletes = 0, 0, 0
			f.certificateID = 41
		}, verify: func(cert, key string) (int, int) {
			f.mu.Lock()
			defer f.mu.Unlock()
			verify(f.cert, f.key, cert, key)
			if provider == "gcore-cdn" && f.bindings != 1 {
				t.Fatal("missing CDN binding")
			}
			return f.writes, f.bindings
		}}
	}
	f := newB3PlatformFixture(t)
	f.provider = provider
	access, config := f.config(provider)
	return b3PerfTarget{access: access, config: config, reset: func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.cert, f.key = "", ""
		f.writes = 0
		f.bindings = map[string]string{}
	}, verify: func(cert, key string) (int, int) {
		f.mu.Lock()
		defer f.mu.Unlock()
		verify(f.cert, f.key, cert, key)
		if provider == "qingcloud-lb" && f.bindings["listener-1"] != f.certID() {
			t.Fatal("missing listener binding")
		}
		return f.writes, len(f.bindings)
	}}
}
