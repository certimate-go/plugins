package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/notifier"
)

type accessConfig struct {
	ServerURL   string `json:"serverUrl"`
	AccessToken string `json:"accessToken"`
	RoomID      string `json:"roomId"`
}

func (*server) Notify(ctx context.Context, req *plugin.NotifyRequest) (*plugin.NotifyResult, error) {
	var config accessConfig
	if err := notifier.DecodeAccess(req, &config); err != nil {
		return nil, err
	}
	roomID := notifier.Destination(req, "roomId", config.RoomID)
	if strings.TrimSpace(roomID) == "" {
		return nil, fmt.Errorf("matrix: roomId is required")
	}
	serverURL, err := url.Parse(config.ServerURL)
	if err != nil || (serverURL.Scheme != "http" && serverURL.Scheme != "https") || serverURL.Hostname() == "" {
		return nil, fmt.Errorf("matrix: invalid serverUrl")
	}
	if config.AccessToken == "" {
		return nil, fmt.Errorf("matrix: accessToken is required")
	}

	client := notifier.NewClient()
	baseURL := resolveHomeserver(ctx, client, strings.TrimSuffix(serverURL.String(), "/"))
	client.SetBaseURL(baseURL).SetAuthToken(config.AccessToken)
	if _, err := client.R().SetContext(ctx).Get("/_matrix/client/versions"); err != nil {
		return nil, fmt.Errorf("matrix: failed to probe client API: %w", err)
	}

	random := make([]byte, 8)
	_, _ = rand.Read(random)
	transactionID := fmt.Sprintf("certimate_%d_%s", time.Now().UnixNano(), hex.EncodeToString(random))
	path := fmt.Sprintf("/_matrix/client/v3/rooms/%s/send/m.room.message/%s", url.PathEscape(roomID), transactionID)
	resp, err := client.R().
		SetContext(ctx).
		SetBody(map[string]string{"msgtype": "m.text", "body": req.Subject + "\n\n" + req.Message}).
		Put(path)
	if err != nil {
		return nil, fmt.Errorf("matrix: failed to send message: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("matrix: send message returned HTTP %d", resp.StatusCode())
	}
	return &plugin.NotifyResult{}, nil
}

func resolveHomeserver(ctx context.Context, client *resty.Client, serverURL string) string {
	var discovery struct {
		Homeserver struct {
			BaseURL string `json:"base_url"`
		} `json:"m.homeserver"`
	}
	resp, err := client.R().SetContext(ctx).SetResult(&discovery).Get(serverURL + "/.well-known/matrix/client")
	if err == nil && resp.IsSuccess() && strings.TrimSpace(discovery.Homeserver.BaseURL) != "" {
		return strings.TrimSuffix(discovery.Homeserver.BaseURL, "/")
	}
	return serverURL
}
