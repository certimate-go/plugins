package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/notifier"
)

type accessConfig struct {
	BotToken  string `json:"botToken"`
	ChannelID string `json:"channelId"`
}

func (*server) Process(ctx context.Context, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error) {
	var config accessConfig
	if err := notifier.DecodeAccess(req, &config); err != nil {
		return nil, err
	}
	config.ChannelID = notifier.Destination(req, "channelId", config.ChannelID)
	client := notifier.NewClient().SetHeader("Authorization", "Bearer "+config.BotToken)

	request := client.R().
		SetContext(ctx).
		SetBody(map[string]any{
			"token":   config.BotToken,
			"channel": config.ChannelID,
			"text":    req.Notify.Subject + "\n" + req.Notify.Message,
		})
	resp, err := request.Post("https://slack.com/api/chat.postMessage")
	if err != nil {
		return nil, fmt.Errorf("slack api error: failed to send request: %w", err)
	} else if resp.IsError() {
		return nil, fmt.Errorf("slack api error: unexpected status code: %d (resp: %s)", resp.StatusCode(), resp.String())
	}

	return &plugin.Result{Notify: &plugin.NotifyResult{}}, nil
}
