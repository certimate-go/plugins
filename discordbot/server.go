package main

import (
	"context"
	"fmt"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/notifier"
)

type accessConfig struct {
	BotToken  string `json:"botToken"`
	ChannelID string `json:"channelId"`
}

func (*server) Notify(ctx context.Context, req *plugin.NotifyRequest) (*plugin.NotifyResult, error) {
	var config accessConfig
	if err := notifier.DecodeAccess(req, &config); err != nil {
		return nil, err
	}
	config.ChannelID = notifier.Destination(req, "channelId", config.ChannelID)
	client := notifier.NewClient().SetHeader("Authorization", "Bot "+config.BotToken)

	request := client.R().
		SetContext(ctx).
		SetBody(map[string]any{
			"content": req.Subject + "\n" + req.Message,
		})
	resp, err := request.Post(fmt.Sprintf("https://discord.com/api/v9/channels/%s/messages", config.ChannelID))
	if err != nil {
		return nil, fmt.Errorf("discord api error: failed to send request: %w", err)
	} else if resp.IsError() {
		return nil, fmt.Errorf("discord api error: unexpected status code: %d (resp: %s)", resp.StatusCode(), resp.String())
	}

	return &plugin.NotifyResult{}, nil
}
