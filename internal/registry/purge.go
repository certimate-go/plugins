package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type CDNPurge struct {
	Path       string `json:"path"`
	Outcome    string `json:"outcome"`
	ID         string `json:"id,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

func MarketReleaseTag(epoch uint32, generation uint64) (string, error) {
	if epoch == 0 || generation == 0 {
		return "", errors.New("market release tag requires an epoch and generation")
	}
	return fmt.Sprintf("v%d.%d.0", epoch, generation), nil
}

func purgeMarketPaths(ctx context.Context, client *http.Client, purgeOrigin string, paths []string) []CDNPurge {
	attempts := make([]CDNPurge, 0, len(paths))
	for _, repoPath := range paths {
		attempt := requestPurge(ctx, client, purgeOrigin+marketCDNPath+repoPath, repoPath)
		attempts = append(attempts, attempt)
		if attempt.Outcome != "finished" {
			break
		}
	}
	return attempts
}

func requestPurge(ctx context.Context, client *http.Client, address, repoPath string) CDNPurge {
	attempt := CDNPurge{Path: repoPath, Outcome: "rejected"}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return attempt
	}
	response, err := client.Do(request)
	if err != nil {
		return attempt
	}
	defer response.Body.Close()
	attempt.HTTPStatus = response.StatusCode
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusOK || err != nil {
		return attempt
	}
	attempt.Outcome, attempt.ID = classifyPurge(body, marketCDNPath+repoPath)
	return attempt
}

func classifyPurge(body []byte, requestPath string) (string, string) {
	var decoded struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Paths  map[string]struct {
			Throttled *bool `json:"throttled"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil || decoded.Status != "finished" {
		return "rejected", ""
	}
	entry, ok := decoded.Paths[requestPath]
	if !ok || entry.Throttled == nil {
		return "rejected", decoded.ID
	}
	if *entry.Throttled {
		return "throttled", decoded.ID
	}
	return "finished", decoded.ID
}
