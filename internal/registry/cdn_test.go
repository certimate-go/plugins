package registry

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/certimate-go/plugins/internal/providerrelease"
)

func TestObserveReleaseRequiresEveryCompleteZIP(t *testing.T) {
	const content = "sample ZIP fixture bytes"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	release := Version{
		Record: providerrelease.Record{
			ProviderType: "volcengine-dns", Repository: "certimate-go/plugin-provider-volcengine-dns-acmedns01challenger",
			Version: "1.2.3", Tag: "v1.2.3", BuildRunID: "123", SourceCommit: strings.Repeat("1", 40),
		},
		ReleaseCommit: strings.Repeat("a", 40),
		Manifest: plugin.Manifest{
			ProviderType: "volcengine-dns", AccessProviderType: "volcengine", Capability: plugin.CapabilityDns,
			Version: "1.2.3", Binary: "volcengine-dns.exe", ProtocolVersion: 4,
		},
	}
	prefix := "/gh/" + release.Repository + "@" + release.ReleaseCommit + "/"
	paths := make(map[string]bool)
	for _, platform := range providerrelease.Targets() {
		artifact := providerrelease.Artifact{Target: platform, Path: "build/" + platform.ArchiveName(), SHA256: digest, Size: int64(len(content))}
		release.Artifacts = append(release.Artifacts, artifact)
		paths[prefix+artifact.Path] = true
	}
	finalPath := prefix + release.Artifacts[len(release.Artifacts)-1].Path
	for _, scenario := range []struct {
		name    string
		code    int
		content string
	}{
		{"all-verified", http.StatusOK, content},
		{"last-platform-pending", http.StatusNotFound, "not found"},
		{"partial-response", http.StatusPartialContent, content},
		{"wrong-size", http.StatusOK, content[:len(content)-1]},
		{"same-size-wrong-digest", http.StatusOK, strings.Repeat("x", len(content))},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				if request.Method != http.MethodGet || !paths[request.URL.Path] || request.Header.Get("Accept-Encoding") != "identity" {
					t.Errorf("unexpected download request: %s %s", request.Method, request.URL.Path)
				}
				if request.URL.Path == finalPath {
					writer.WriteHeader(scenario.code)
					fmt.Fprint(writer, scenario.content)
					return
				}
				fmt.Fprint(writer, content)
			}))
			defer server.Close()
			observation, err := observeRelease(t.Context(), server.Client(), server.URL, release)
			if scenario.name == "all-verified" {
				if err != nil || observation.ReleaseCommit != release.ReleaseCommit || observation.Version != release.Version {
					t.Fatalf("missing readiness evidence: %+v, %v", observation, err)
				}
			} else if err == nil || observation.ReleaseCommit != "" {
				t.Fatalf("incomplete release yielded a readiness observation: %+v, %v", observation, err)
			}
			if count := requests.Load(); count != 8 {
				t.Fatalf("want all eight downloads attempted, got %d", count)
			}
		})
	}
}

func TestDownloadRejectsOversizeChunkedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.(http.Flusher).Flush()
		io.WriteString(writer, "oversize")
	}))
	defer server.Close()
	err := checkDownload(t.Context(), server.Client(), server.URL, 4, fmt.Sprintf("%x", sha256.Sum256([]byte("over"))))
	if err == nil {
		t.Fatal("accepted longer response with valid prefix")
	}
}

func TestDownloadReportsRedirectWithoutFollowingIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "https://raw.githubusercontent.com/fixture/provider/archive.zip", http.StatusMovedPermanently)
	}))
	defer server.Close()
	result, err := measureDownload(t.Context(), cdnHTTPClient(), server.URL, 10, strings.Repeat("a", 64))
	if err == nil || result.Status != http.StatusMovedPermanently || result.RedirectHost != "raw.githubusercontent.com" ||
		result.Size != 0 || result.SHA256 != "" {
		t.Fatalf("unexpected redirect observation: %+v, %v", result, err)
	}
}
