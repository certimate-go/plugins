package registry

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"time"
)

const defaultCDN = "https://cdn.jsdelivr.net"

func ObserveDefaultCDN(ctx context.Context, version Version) (ReadyRelease, error) {
	return observeRelease(ctx, cdnHTTPClient(), defaultCDN, version)
}

func cdnHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func ObservePublishedIndex(ctx context.Context, checkout, commit string, keys map[string]ed25519.PublicKey, now time.Time) (Publication, error) {
	return observePublishedIndex(ctx, checkout, commit, keys, now, cdnHTTPClient(), defaultCDN)
}

func observePublishedIndex(ctx context.Context, checkout, commit string, keys map[string]ed25519.PublicKey, now time.Time, client *http.Client, origin string) (Publication, error) {
	git := releaseGit{ctx, checkout}
	index, payload, keyID, err := readPublishedIndex(git, commit, keys, now)
	if err != nil {
		return Publication{}, err
	}
	result := Publication{Commit: commit, SourceCommit: payload.SourceCommit, KeyID: keyID, Epoch: payload.Epoch, Generation: payload.Generation, ExpiresAt: payload.ExpiresAt}
	base := origin + "/gh/certimate-go/plugins@market/"
	if err := checkDownload(ctx, client, base+marketIndexPath, int64(len(index)), fmt.Sprintf("%x", sha256.Sum256(index))); err != nil {
		result.Pending = append(result.Pending, fmt.Sprintf("signed index not observable: %v", err))
		return result, nil
	}
	assets := make(map[string]string)
	for _, provider := range payload.Plugins {
		if !digestPattern.MatchString(provider.Detail.SHA256) {
			return Publication{}, fmt.Errorf("invalid signed detail digest for %s", provider.ProviderType)
		}
		assets["registry/market/details/"+provider.Detail.SHA256+".json"] = provider.Detail.SHA256
		if icon := provider.Icon; icon != nil {
			if !digestPattern.MatchString(icon.SHA256) || (icon.Format != "png" && icon.Format != "jpeg") {
				return Publication{}, fmt.Errorf("invalid signed icon for %s", provider.ProviderType)
			}
			assets["registry/market/icons/"+icon.SHA256+"."+icon.Format] = icon.SHA256
		}
	}
	for _, path := range slices.Sorted(maps.Keys(assets)) {
		digest := assets[path]
		body, err := git.blob(commit, path, maxRecordBytes)
		if err != nil {
			return Publication{}, err
		}
		if fmt.Sprintf("%x", sha256.Sum256(body)) != digest {
			return Publication{}, fmt.Errorf("market tree has incorrect signed asset %s", path)
		}
		if err := checkDownload(ctx, client, base+path, int64(len(body)), digest); err != nil {
			result.Pending = append(result.Pending, fmt.Sprintf("%s: %v", path, err))
		}
	}
	return result, nil
}

func readPublishedIndex(git releaseGit, commit string, keys map[string]ed25519.PublicKey, now time.Time) ([]byte, IndexPayload, string, error) {
	if !commitPattern.MatchString(commit) {
		return nil, IndexPayload{}, "", fmt.Errorf("invalid published Git commit")
	}
	index, err := git.blob(commit, marketIndexPath, MaxIndexEnvelope)
	if err != nil {
		return nil, IndexPayload{}, "", err
	}
	payload, keyID, err := VerifyIndex(index, keys)
	if err != nil {
		return nil, IndexPayload{}, "", err
	}
	if !now.Before(payload.ExpiresAt) {
		return nil, IndexPayload{}, "", fmt.Errorf("published index has expired")
	}
	if payload.IssuedAt.After(now.Add(5 * time.Minute)) {
		return nil, IndexPayload{}, "", fmt.Errorf("published index was issued in the future")
	}
	return index, payload, keyID, nil
}

func observeRelease(ctx context.Context, client *http.Client, origin string, version Version) (ReadyRelease, error) {
	if err := version.Validate(); err != nil {
		return ReadyRelease{}, err
	}
	for _, artifact := range version.Artifacts {
		address := origin + "/gh/" + version.Repository + "@" + version.ReleaseCommit + "/" + artifact.Path
		if err := checkDownload(ctx, client, address, artifact.Size, artifact.SHA256); err != nil {
			return ReadyRelease{}, fmt.Errorf("CDN artifact %q not ready: %w", artifact.Path, err)
		}
	}
	return ReadyRelease{Identity{version.ProviderType, version.Version}, version.ReleaseCommit}, nil
}

func checkDownload(ctx context.Context, client *http.Client, address string, size int64, digest string) error {
	_, err := measureDownload(ctx, client, address, size, digest)
	return err
}

type DownloadObservation struct {
	URL          string `json:"url"`
	ExpectedSize int64  `json:"expected_size"`
	ExpectedSHA  string `json:"expected_sha256"`
	Status       int    `json:"status,omitempty"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256,omitempty"`
	ElapsedMS    int64  `json:"elapsed_ms"`
	Age          string `json:"age,omitempty"`
	CacheControl string `json:"cache_control,omitempty"`
	CacheStatus  string `json:"cache_status,omitempty"`
	RedirectHost string `json:"redirect_host,omitempty"`
	Error        string `json:"error,omitempty"`
}

func measureDownload(ctx context.Context, client *http.Client, address string, size int64, digest string) (observation DownloadObservation, err error) {
	start := time.Now()
	observation = DownloadObservation{URL: address, ExpectedSize: size, ExpectedSHA: digest}
	defer func() {
		observation.ElapsedMS = time.Since(start).Milliseconds()
		if err != nil {
			observation.Error = err.Error()
		}
	}()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return observation, err
	}
	request.Header.Set("Accept-Encoding", "identity")
	response, err := client.Do(request)
	if err != nil {
		return observation, err
	}
	defer response.Body.Close()
	observation.Status = response.StatusCode
	observation.Age = response.Header.Get("Age")
	observation.CacheControl = response.Header.Get("Cache-Control")
	for _, key := range []string{"Cache-Status", "EO-Cache-Status", "CF-Cache-Status", "X-Cache"} {
		if status := response.Header.Get(key); status != "" {
			observation.CacheStatus = status
			break
		}
	}
	if response.StatusCode != http.StatusOK {
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			if location, err := response.Location(); err == nil && location.Hostname() != "" {
				observation.RedirectHost = location.Hostname()
				return observation, fmt.Errorf("got HTTP status %d redirecting to %s", response.StatusCode, observation.RedirectHost)
			}
		}
		return observation, fmt.Errorf("got HTTP status %d", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != size {
		return observation, fmt.Errorf("invalid content length %d", response.ContentLength)
	}
	hash := sha256.New()
	observation.Size, err = io.Copy(hash, io.LimitReader(response.Body, size+1))
	if err != nil {
		return observation, err
	}
	observation.SHA256 = fmt.Sprintf("%x", hash.Sum(nil))
	if observation.Size != size || observation.SHA256 != digest {
		return observation, fmt.Errorf("download size or SHA-256 does not match the published file")
	}
	return observation, nil
}
