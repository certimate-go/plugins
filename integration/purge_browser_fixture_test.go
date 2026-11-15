package integration

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPurgeBrowserFixture(t *testing.T) {
	if os.Getenv("CERTIMATE_PURGE_BROWSER") != "1" {
		t.Skip("set CERTIMATE_PURGE_BROWSER=1 to keep a purge vendor fixture alive for browser verification")
	}
	address := os.Getenv("CERTIMATE_PURGE_BROWSER_ADDR")
	if address == "" {
		address = "127.0.0.1:80"
	}

	fixture := &purgeBaiduFixture{
		certs:   map[string]*purgeBaiduCert{},
		uploads: map[string]string{},
	}
	fixture.resetBrowserScenario()

	tencent := newPurgeTencentBrowserFixture(t)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/__purge-fixture/tse/") {
				tencent.serveBrowserControl(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/__purge-fixture/") {
				fixture.serveBrowserControl(w, r)
				return
			}
			fixture.serve(w, r)
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go server.Serve(listener)
	defer server.Close()

	t.Logf("purge browser fixture listening on http://%s", address)
	t.Logf("map the vendor host first, e.g. /etc/hosts: 127.0.0.1 certificate.baidubce.com")
	t.Logf("access credentials: accessKeyId=%s secretAccessKey=%s", purgeBaiduAccessKey, purgeBaiduSecretKey)
	t.Logf("tencent tse access credentials: secretId=%s secretKey=%s region=%s gateway=%s", purgeTencentSecretID, purgeTencentSecretKey, purgeTencentTSERegion, purgeTencentTSEGateway)
	t.Logf("tencent tse endpoint override: %s (https via CONNECT proxy, SSL_CERT_FILE=%s)", "custom-tse.endpoint.test", tencent.caPath)
	t.Logf("control endpoints:")
	t.Logf("  GET  /__purge-fixture/status")
	t.Logf("  POST /__purge-fixture/reset")
	t.Logf("  POST /__purge-fixture/seed?id=expired-15d&days=15[&associated=1]")
	t.Logf("  POST /__purge-fixture/deny?id=deny-10d   (empty id clears the denial)")
	t.Logf("  POST /__purge-fixture/mode?name=auth-failure|block|normal")
	t.Logf("  POST /__purge-fixture/release            (unblocks a held delete)")
	t.Logf("  GET  /__purge-fixture/tse/status")
	t.Logf("  POST /__purge-fixture/tse/reset")
	t.Logf("  POST /__purge-fixture/tse/seed?gateway=..&id=..&days=..")
	t.Logf("  POST /__purge-fixture/tse/mode?name=auth-failure|normal")

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
	case <-time.After(6 * time.Hour):
	}
}

func (f *purgeBaiduFixture) resetBrowserScenario() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = ""
	f.denyDeleteID = ""
	f.certs = map[string]*purgeBaiduCert{}
	f.order = nil
	f.deletes = nil
	f.lists = 0
	f.deleteCalls = 0
	f.uploads = map[string]string{}
	f.seedLocked("expired-20d", 20, false)
	f.seedLocked("expired-15d", 15, false)
	f.seedLocked("expired-2d", 2, false)
	f.seedLocked("valid-30d", -30, false)
	f.seedLocked("associated-12d", 12, true)
	f.seedLocked("deny-10d", 10, false)
}

func (f *purgeBaiduFixture) serveBrowserControl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	respond := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	switch {
	case r.URL.Path == "/__purge-fixture/status" && r.Method == http.MethodGet:
		f.mu.Lock()
		certs := make([]map[string]any, 0, len(f.certs))
		for _, id := range f.order {
			cert, exists := f.certs[id]
			if !exists {
				continue
			}
			certs = append(certs, map[string]any{"id": cert.id, "notAfter": cert.notAfter.Format(time.RFC3339), "associated": cert.associated})
		}
		payload := map[string]any{
			"certs": certs, "deletes": f.deletes, "uploads": len(f.uploads),
			"lists": f.lists, "deleteCalls": f.deleteCalls, "mode": f.mode, "denyDeleteID": f.denyDeleteID,
		}
		f.mu.Unlock()
		respond(payload)
		return
	case r.URL.Path == "/__purge-fixture/reset" && r.Method == http.MethodPost:
		f.resetBrowserScenario()
		respond(map[string]string{"status": "reset"})
		return
	case r.URL.Path == "/__purge-fixture/seed" && r.Method == http.MethodPost:
		id := r.URL.Query().Get("id")
		days, err := strconv.ParseFloat(r.URL.Query().Get("days"), 64)
		if id == "" || err != nil {
			http.Error(w, "id and numeric days are required", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.seedLocked(id, days, r.URL.Query().Get("associated") == "1")
		f.mu.Unlock()
		respond(map[string]string{"status": "seeded"})
		return
	case r.URL.Path == "/__purge-fixture/deny" && r.Method == http.MethodPost:
		f.mu.Lock()
		f.denyDeleteID = r.URL.Query().Get("id")
		denied := f.denyDeleteID
		f.mu.Unlock()
		respond(map[string]string{"status": "denied", "id": denied})
		return
	case r.URL.Path == "/__purge-fixture/mode" && r.Method == http.MethodPost:
		name := r.URL.Query().Get("name")
		f.mu.Lock()
		switch name {
		case "", "normal":
			f.mode = ""
		case "auth-failure":
			f.mode = "auth-failure"
		case "block":
			f.mode = ""
			f.holdEntered = make(chan struct{})
			f.holdAborted = make(chan struct{})
			f.holdReleased = make(chan struct{})
			f.hold.Store(true)
		default:
			f.mu.Unlock()
			http.Error(w, "unknown mode", http.StatusBadRequest)
			return
		}
		f.mu.Unlock()
		respond(map[string]string{"status": "mode", "mode": name})
		return
	case r.URL.Path == "/__purge-fixture/release" && r.Method == http.MethodPost:
		f.releaseHold()
		respond(map[string]string{"status": "released"})
		return
	}
	http.Error(w, fmt.Sprintf("unknown control request %s %s", r.Method, r.URL.Path), http.StatusNotFound)
}

type purgeTencentBrowserFixture struct {
	*purgeTencentFixture
	caPath string
}

func newPurgeTencentBrowserFixture(t *testing.T) *purgeTencentBrowserFixture {
	t.Helper()
	ca, key, _ := makeCertificate(t, purgeTencentHosts, true)
	pair, err := tls.X509KeyPair([]byte(ca), []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	fixture := &purgeTencentBrowserFixture{
		purgeTencentFixture: &purgeTencentFixture{
			ssl:  purgeTencentService{certs: map[string]*purgeTencentCert{}},
			gaap: purgeTencentService{certs: map[string]*purgeTencentCert{}},
			tse:  map[string]*purgeTencentService{},
		},
	}
	fixture.resetBrowserScenario()

	target, err := net.Listen("tcp", "127.0.0.1:18443")
	if err != nil {
		t.Fatal(err)
	}
	go (&http.Server{
		Handler: http.HandlerFunc(fixture.serve),
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{pair},
		},
		ReadHeaderTimeout: 5 * time.Second,
	}).ServeTLS(target, "", "")

	proxyListener, err := net.Listen("tcp", "127.0.0.1:8118")
	if err != nil {
		t.Fatal(err)
	}
	allowed := make(map[string]bool, len(purgeTencentHosts))
	for _, host := range purgeTencentHosts {
		allowed[host+":443"] = true
	}
	go (&http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodConnect || !allowed[r.Host] {
				http.Error(w, "unexpected proxy destination", http.StatusBadGateway)
				return
			}
			upstream, err := net.DialTimeout("tcp", "127.0.0.1:18443", 5*time.Second)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer upstream.Close()
			client, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer client.Close()
			if _, err := buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
				return
			}
			if err := buffer.Flush(); err != nil {
				return
			}
			done := make(chan struct{})
			go func() {
				_, _ = io.Copy(upstream, buffer)
				_ = upstream.Close()
				close(done)
			}()
			_, _ = io.Copy(client, upstream)
			_ = client.Close()
			<-done
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}).Serve(proxyListener)

	caPath := os.Getenv("CERTIMATE_PURGE_BROWSER_CA")
	if caPath == "" {
		caPath = filepath.Join(os.TempDir(), "fixture-ca.pem")
	}
	if err := os.WriteFile(caPath, []byte(ca), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.caPath = caPath
	return fixture
}

func (f *purgeTencentBrowserFixture) resetBrowserScenario() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = ""
	f.tseRegions = nil
	f.tse = map[string]*purgeTencentService{}
	main := f.tseGateway(purgeTencentTSEGateway)
	main.certs = map[string]*purgeTencentCert{}
	main.order = nil
	main.deletes = nil
	main.lists = 0
	main.denyID = ""
	main.vanishID = ""
	main.boundID = ""
	main.seed("expired-20d", 20)
	main.seed("expired-15d", 15)
	main.seed("valid-30d", -30)
	foreign := f.tseGateway("gw-fixture-other")
	foreign.certs = map[string]*purgeTencentCert{}
	foreign.order = nil
	foreign.deletes = nil
	foreign.lists = 0
	foreign.seed("foreign-10d", 10)
}

func (f *purgeTencentBrowserFixture) statusPayload() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	gateways := map[string]any{}
	for gatewayID, service := range f.tse {
		certs := make([]map[string]any, 0, len(service.order))
		for _, id := range service.order {
			if cert, exists := service.certs[id]; exists {
				certs = append(certs, map[string]any{"id": cert.id, "notAfter": cert.notAfter.Format(time.RFC3339)})
			}
		}
		gateways[gatewayID] = map[string]any{"certs": certs, "deletes": service.deletes, "lists": service.lists}
	}
	return map[string]any{"gateways": gateways, "regions": f.tseRegions, "mode": f.mode}
}

func (f *purgeTencentBrowserFixture) serveBrowserControl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	respond := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	path := strings.TrimPrefix(r.URL.Path, "/__purge-fixture/tse/")
	switch {
	case path == "status" && r.Method == http.MethodGet:
		respond(f.statusPayload())
		return
	case path == "reset" && r.Method == http.MethodPost:
		f.resetBrowserScenario()
		respond(map[string]string{"status": "reset"})
		return
	case path == "seed" && r.Method == http.MethodPost:
		gatewayID := r.URL.Query().Get("gateway")
		if gatewayID == "" {
			gatewayID = purgeTencentTSEGateway
		}
		id := r.URL.Query().Get("id")
		days, err := strconv.ParseFloat(r.URL.Query().Get("days"), 64)
		if id == "" || err != nil {
			http.Error(w, "id and numeric days are required", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.tseGateway(gatewayID).seed(id, days)
		f.mu.Unlock()
		respond(map[string]string{"status": "seeded"})
		return
	case path == "mode" && r.Method == http.MethodPost:
		f.setMode(r.URL.Query().Get("name"))
		respond(map[string]string{"status": "mode", "mode": f.mode})
		return
	}
	http.Error(w, fmt.Sprintf("unknown control request %s %s", r.Method, r.URL.Path), http.StatusNotFound)
}
