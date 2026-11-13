package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/notifier"
)

type accessConfig struct {
	ServerURL string `json:"serverUrl"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	ChannelID string `json:"channelId"`
}

func (*server) Notify(ctx context.Context, req *plugin.NotifyRequest) (*plugin.NotifyResult, error) {
	var config accessConfig
	if err := notifier.DecodeAccess(req, &config); err != nil {
		return nil, err
	}
	config.ChannelID = notifier.Destination(req, "channelId", config.ChannelID)
	client := notifier.NewClient()

	serverUrl := strings.TrimSuffix(config.ServerURL, "/")

	loginReq := client.R().
		SetContext(ctx).
		SetBody(map[string]any{
			"login_id": config.Username,
			"password": config.Password,
		})
	loginResp, err := loginReq.Post(fmt.Sprintf("%s/api/v4/users/login", serverUrl))
	if err != nil {
		return nil, fmt.Errorf("mattermost api error: failed to send request: %w", err)
	} else if loginResp.IsError() {
		return nil, fmt.Errorf("mattermost api error: unexpected status code: %d (resp: %s)", loginResp.StatusCode(), loginResp.String())
	} else if loginResp.Header().Get("Token") == "" {
		return nil, fmt.Errorf("mattermost api error: received empty login token")
	}

	postReq := client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+loginResp.Header().Get("Token")).
		SetBody(map[string]any{
			"channel_id": config.ChannelID,
			"props": map[string]any{
				"attachments": []map[string]any{
					{
						"title": req.Subject,
						"text":  req.Message,
					},
				},
			},
		})
	postResp, err := postReq.Post(fmt.Sprintf("%s/api/v4/posts", serverUrl))
	if err != nil {
		return nil, fmt.Errorf("mattermost api error: failed to send request: %w", err)
	} else if postResp.IsError() {
		return nil, fmt.Errorf("mattermost api error: unexpected status code: %d (resp: %s)", postResp.StatusCode(), postResp.String())
	}

	return &plugin.NotifyResult{}, nil
}
