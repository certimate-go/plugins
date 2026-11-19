package registry

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/certimate-go/plugins/internal/providerrelease"
)

func TestPublicationAdvancesWhileAnUnrelatedReleaseIsPending(t *testing.T) {
	dir := t.TempDir()
	git := func(arguments ...string) string {
		t.Helper()
		command := exec.CommandContext(t.Context(), "git", append([]string{
			"-C", dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test",
		}, arguments...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	write := func(name string, body any) {
		t.Helper()
		file := filepath.Join(dir, "registry", name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	save := func(parent string) string {
		t.Helper()
		git("add", "registry")
		args := []string{"commit-tree", git("write-tree"), "-m", "Registry source fixture"}
		if parent != "" {
			args = append(args, "-p", parent)
		}
		commit := git(args...)
		git("update-ref", "refs/heads/main", commit)
		return commit
	}

	alpha := newPublicationVersion("alpha", "1.0.0", "a")
	beta := newPublicationVersion("beta", "1.0.0", "b")
	activeBeta := newPublicationVersion("beta", "1.1.0", "c")
	var icon bytes.Buffer
	if err := png.Encode(&icon, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	beta.Icon = icon.Bytes()
	pendingBeta := newPublicationVersion("beta", "1.2.0", "d")
	write("_policy/admissions.json", map[string]string{"alpha": alpha.Repository, "beta": beta.Repository})
	write("_policy/withdrawals.json", []Withdrawal{})
	write("alpha/v1.0.0.json", alpha)
	write("beta/v1.0.0.json", beta)
	write("beta/v1.1.0.json", activeBeta)
	firstSource := save("")
	var mu sync.Mutex
	cdn := make(map[string][]byte)
	const packageBytes = "immutable provider ZIP fixture"
	for _, version := range []Version{alpha, beta, activeBeta} {
		for _, zip := range version.Artifacts {
			cdn["/gh/"+version.Repository+"@"+version.ReleaseCommit+"/"+zip.Path] = []byte(packageBytes)
		}
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		body, ok := cdn[strings.TrimPrefix(request.URL.Path, "/mirror")]
		mu.Unlock()
		if !ok {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Age", "17")
		writer.Header().Set("Cache-Control", "public, max-age=1800")
		writer.Header().Set("X-Cache", "HIT")
		writer.Write(body)
	}))
	defer server.Close()
	observe := func(ctx context.Context, version Version) (ReadyRelease, error) {
		return observeRelease(ctx, server.Client(), server.URL, version)
	}
	seed := bytes.Repeat([]byte{0x24}, ed25519.SeedSize)
	keys := map[string]ed25519.PublicKey{"fixture": ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)}
	now := time.Now().UTC().Truncate(time.Second)
	options := PublishOptions{
		Checkout: dir, SourceCommit: firstSource, Epoch: 1, KeyID: "fixture", Seed: seed, PublicKeys: keys,
		Now: now, Lifetime: 48 * time.Hour, RenewBefore: 24 * time.Hour,
	}
	model := releaseGit{t.Context(), dir}
	index := func(commit string) ([]byte, IndexPayload) {
		t.Helper()
		body, err := model.blob(commit, marketIndexPath, MaxIndexEnvelope)
		if err != nil {
			t.Fatal(err)
		}
		payload, _, err := VerifyIndex(body, keys)
		if err != nil {
			t.Fatal(err)
		}
		return body, payload
	}
	first, err := stagePublication(t.Context(), options, observe)
	if err != nil || !first.Changed || first.KeyID != options.KeyID || first.Generation != 1 || len(first.Pending) != 0 {
		t.Fatalf("bootstrap publication: %+v, %v", first, err)
	}
	firstEnvelope, firstPayload := index(first.Commit)
	if len(firstPayload.Plugins) != 2 || firstPayload.Plugins[1].Version != activeBeta.Version {
		t.Fatalf("bootstrap omitted registrations: %+v", firstPayload)
	}
	if got := git("rev-parse", "refs/heads/main"); got != firstSource {
		t.Fatalf("market staging changed registry main: %s", got)
	}
	options.MarketCommit = first.Commit
	options.Now = now.Add(time.Hour)
	write("beta/v1.2.0.json", pendingBeta)
	withdrawAlpha, withdrawBeta := Identity{"alpha", "1.0.0"}, Identity{"beta", "1.1.0"}
	write("_policy/withdrawals.json", []Withdrawal{
		{withdrawAlpha, "withdraw broken deployment"},
		{withdrawBeta, "use the previous release"},
	})
	options.SourceCommit = save(firstSource)
	second, err := stagePublication(t.Context(), options, observe)
	if err != nil || !second.Changed || second.Generation != 2 || len(second.Pending) != 1 {
		t.Fatalf("withdrawal while another provider propagates: %+v, %v", second, err)
	}
	_, secondPayload := index(second.Commit)
	if len(secondPayload.Plugins) != 1 || secondPayload.Plugins[0].Version != "1.0.0" || secondPayload.Plugins[0].ProviderType != "beta" {
		t.Fatalf("withdrawal or pending version was handled incorrectly: %+v", secondPayload.Plugins)
	}
	if git("rev-list", "-n1", "--parents", second.Commit) != second.Commit+" "+first.Commit {
		t.Fatal("market commit is not an atomic fast-forward from the previous snapshot")
	}
	oldDetailPath := "registry/market/details/" + firstPayload.Plugins[0].Detail.SHA256 + ".json"
	if _, err := model.blob(second.Commit, oldDetailPath, maxRecordBytes); err != nil {
		t.Fatal("withdrawal deleted data still referenced by cached earlier indexes")
	}
	options.MarketCommit = second.Commit
	noChange, err := stagePublication(t.Context(), options, observe)
	if err != nil || noChange.Changed || noChange.Commit != second.Commit || noChange.KeyID != options.KeyID {
		t.Fatalf("rerun before renewal replaced the same snapshot: %+v, %v", noChange, err)
	}
	options.Now = now.Add(26 * time.Hour)
	renewed, err := stagePublication(t.Context(), options, observe)
	if err != nil || !renewed.Changed || renewed.Generation != 3 || !renewed.ExpiresAt.After(second.ExpiresAt) {
		t.Fatalf("lease renewal: %+v, %v", renewed, err)
	}
	renewedEnvelope, renewedPayload := index(renewed.Commit)
	const marketPath = "/gh/certimate-go/plugins@market/"
	setCDN := func(path string, body []byte) {
		mu.Lock()
		defer mu.Unlock()
		cdn[marketPath+path] = body
	}
	verifyCDN := func(want string) {
		t.Helper()
		result, err := observePublishedIndex(t.Context(), dir, renewed.Commit, keys, options.Now, server.Client(), server.URL)
		if err != nil || result.KeyID != options.KeyID || result.Generation != renewed.Generation {
			t.Fatalf("invalid publication observation: %+v, %v", result, err)
		}
		if want == "" && len(result.Pending) != 0 || want != "" && (len(result.Pending) != 1 || !strings.Contains(result.Pending[0], want)) {
			t.Fatalf("unexpected CDN propagation state %q: %+v", want, result)
		}
	}
	setCDN(marketIndexPath, firstEnvelope)
	verifyCDN("signed index not observable")
	mirror := MirrorAuditOptions{
		Checkout: dir, MarketCommit: renewed.Commit, BaseURL: server.URL + "/mirror/",
		Network: "fixture region", Withdrawn: []Identity{withdrawBeta, withdrawAlpha, withdrawBeta}, PublicKeys: keys, Now: options.Now,
	}
	audit := func(ready bool, probes int) MirrorAudit {
		t.Helper()
		report, err := auditMirror(t.Context(), mirror, server.Client())
		if err != nil || report.Ready != ready || len(report.Probes) != probes ||
			report.KeyID != options.KeyID || report.Generation != renewed.Generation || report.SourceCommit != renewed.SourceCommit {
			t.Fatalf("unexpected mirror audit: %+v, %v", report, err)
		}
		return report
	}
	stale := audit(false, 1)
	if stale.Probes[0].Error == "" {
		t.Fatal("stale signed index did not leave failure evidence")
	}
	if len(stale.Withdrawals) != 2 || stale.Withdrawals[0].Confirmed || stale.Withdrawals[1].Confirmed {
		t.Fatalf("an old index confirmed withdrawal: %+v", stale.Withdrawals)
	}
	invalid := mirror
	invalid.Withdrawn = []Identity{{"nonexistent", "1.0.0"}}
	if _, err := auditMirror(t.Context(), invalid, server.Client()); err == nil || !strings.Contains(err.Error(), "missing from the signed source") {
		t.Fatalf("a version missing from withdrawal policy was audited: %v", err)
	}
	inconsistent, err := SignIndex(Snapshot{Catalog: firstPayload.Catalog}, renewedPayload.IndexMetadata, options.KeyID, options.Seed)
	if err != nil {
		t.Fatal(err)
	}
	invalid = mirror
	invalid.MarketCommit, err = model.marketCommit(renewed.Commit, Snapshot{}, inconsistent, renewedPayload.IndexMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auditMirror(t.Context(), invalid, server.Client()); err == nil || !strings.Contains(err.Error(), "still listed in the expected index") {
		t.Fatalf("a signed index listing a withdrawn version was audited: %v", err)
	}
	setCDN(marketIndexPath, renewedEnvelope)
	removal := mirror
	removal.Withdrawn = []Identity{withdrawAlpha}
	removed, err := auditMirror(t.Context(), removal, server.Client())
	if err != nil || !removed.Ready || len(removed.Probes) != 1 || len(removed.Providers) != 0 ||
		!slices.Equal(removed.Withdrawals, []WithdrawalAudit{{Identity: withdrawAlpha, Confirmed: true}}) {
		t.Fatalf("withdrawal without a replacement: %+v, %v", removed, err)
	}
	setCDN("registry/market/icons/"+renewedPayload.Plugins[0].Icon.SHA256+".png", beta.Icon)
	verifyCDN("market/details/")
	detail := "registry/market/details/" + renewedPayload.Plugins[0].Detail.SHA256 + ".json"
	body, err := model.blob(renewed.Commit, detail, maxRecordBytes)
	if err != nil {
		t.Fatal(err)
	}
	setCDN(detail, body)
	verifyCDN("")
	report := audit(true, 11)
	if !slices.Equal(report.Providers, []string{"beta"}) || !slices.Equal(report.Withdrawals, []WithdrawalAudit{
		{Identity: withdrawAlpha, Confirmed: true},
		{Identity: withdrawBeta, ReplacementVersion: "1.0.0", Confirmed: true},
	}) {
		t.Fatalf("missing withdrawal or replacement evidence: %+v", report)
	}
	for _, observation := range report.Probes {
		if observation.Size != observation.ExpectedSize || observation.SHA256 != observation.ExpectedSHA ||
			observation.Error != "" || observation.Age != "17" || observation.CacheStatus != "HIT" ||
			!strings.HasPrefix(observation.URL, mirror.BaseURL+"gh/") {
			t.Fatalf("missing verified mirror response metrics: %+v", observation)
		}
	}
	t.Run("rotate signing key and recover from an old epoch", func(t *testing.T) {
		rotation := options
		rotation.MarketCommit = renewed.Commit
		rotation.Now = options.Now.Add(time.Minute)
		rotation.KeyID, rotation.Seed = "routine", bytes.Repeat([]byte{0x35}, ed25519.SeedSize)
		routineKey := ed25519.NewKeyFromSeed(rotation.Seed).Public().(ed25519.PublicKey)
		rotation.PublicKeys = map[string]ed25519.PublicKey{rotation.KeyID: routineKey}
		if _, err := stagePublication(t.Context(), rotation, observe); err == nil || !strings.Contains(err.Error(), "verify current market branch index") {
			t.Fatalf("rotation proceeded without trusting the protected market tip: %v", err)
		}
		rotation.PublicKeys[options.KeyID] = keys[options.KeyID]
		routine, err := stagePublication(t.Context(), rotation, observe)
		if err != nil || routine.Epoch != renewed.Epoch || routine.Generation != renewed.Generation+1 || routine.KeyID != rotation.KeyID {
			t.Fatalf("routine rotation: %+v, %v", routine, err)
		}
		rotation.MarketCommit = routine.Commit
		rotation.Epoch++
		rotation.KeyID, rotation.Seed = "recovery", bytes.Repeat([]byte{0x46}, ed25519.SeedSize)
		recoveryKey := ed25519.NewKeyFromSeed(rotation.Seed).Public().(ed25519.PublicKey)
		rotation.PublicKeys = map[string]ed25519.PublicKey{routine.KeyID: routineKey, rotation.KeyID: recoveryKey}
		recovery, err := stagePublication(t.Context(), rotation, observe)
		if err != nil || recovery.Epoch != routine.Epoch+1 || recovery.Generation != 1 || recovery.KeyID != rotation.KeyID {
			t.Fatalf("emergency rotation: %+v, %v", recovery, err)
		}
		if git("rev-list", "-n1", "--parents", recovery.Commit) != recovery.Commit+" "+routine.Commit {
			t.Fatal("emergency rotation did not fast-forward the protected market branch")
		}
		body, err := model.blob(recovery.Commit, marketIndexPath, MaxIndexEnvelope)
		if err != nil {
			t.Fatal(err)
		}
		payload, keyID, err := VerifyIndex(body, map[string]ed25519.PublicKey{rotation.KeyID: recoveryKey})
		if err != nil || keyID != rotation.KeyID || !sameCatalog(renewedPayload.Catalog, payload.Catalog) {
			t.Fatalf("emergency rotation changed the approved catalog: %+v, %v", payload, err)
		}
		rotation.MarketCommit = recovery.Commit
		rotation.Epoch--
		if _, err := stagePublication(t.Context(), rotation, observe); err == nil || !strings.Contains(err.Error(), "older epoch") {
			t.Fatalf("previous epoch could overwrite the recovered catalog: %v", err)
		}
	})
	missingZIP := "/gh/" + beta.Repository + "@" + beta.ReleaseCommit + "/" + beta.Artifacts[0].Path
	mu.Lock()
	delete(cdn, missingZIP)
	mu.Unlock()
	partial := audit(false, 11)
	if len(partial.Withdrawals) != 2 || !partial.Withdrawals[1].Confirmed {
		t.Fatal("a missing fallback ZIP concealed the observed removal from the signed index")
	}
	var failed int
	for _, observation := range partial.Probes {
		if observation.Error != "" {
			if observation.URL != server.URL+"/mirror"+missingZIP || observation.Status != http.StatusNotFound {
				t.Fatalf("unexpected transfer failure: %+v", observation)
			}
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("expected one missing platform ZIP, got %d", failed)
	}

	git("update-ref", "refs/heads/market", renewed.Commit)
	older := t.TempDir()
	git("clone", "--branch", "main", dir, older)
	git("-C", older, "checkout", "--detach", firstSource)
	options.Checkout, options.SourceCommit, options.MarketCommit = older, firstSource, renewed.Commit
	if _, err := stagePublication(t.Context(), options, observe); err == nil || !strings.Contains(err.Error(), "older source") {
		t.Fatalf("old registry source could claim a new publication generation: %v", err)
	}
}

func newPublicationVersion(provider, version, hash string) Version {
	result := Version{
		Record: providerrelease.Record{
			ProviderType: provider, Repository: "certimate-go/plugin-provider-" + provider, Version: version, Tag: "v" + version,
			BuildRunID: "1", SourceCommit: strings.Repeat("1", 40),
		},
		ReleaseCommit: strings.Repeat(hash, 40),
		Manifest: plugin.Manifest{
			ProviderType: provider, AccessProviderType: "volcengine", Capability: plugin.CapabilityDns, ProtocolVersion: plugin.ProtocolVersion,
			Version: version, Binary: provider + ".exe",
		},
	}
	content := []byte("immutable provider ZIP fixture")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	for _, target := range providerrelease.Targets() {
		result.Artifacts = append(result.Artifacts, providerrelease.Artifact{
			Target: target, Path: "build/" + target.ArchiveName(), Size: int64(len(content)), SHA256: digest,
		})
	}
	return result
}
