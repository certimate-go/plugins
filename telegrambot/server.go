package main

import (
	"context"
	"fmt"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/notifier"
)

type accessConfig struct {
	BotToken string `json:"botToken"`
	ChatID   string `json:"chatId"`
}

func (*server) Notify(ctx context.Context, req *plugin.NotifyRequest) (*plugin.NotifyResult, error) {
	var config accessConfig
	if err := notifier.DecodeAccess(req, &config); err != nil {
		return nil, err
	}
	config.ChatID = notifier.Destination(req, "chatId", config.ChatID)
	client := notifier.NewClient()

	request := client.R().
		SetContext(ctx).
		SetBody(map[string]any{
			"chat_id": config.ChatID,
			"text":    req.Subject + "\n" + req.Message,
		})
	resp, err := request.Post(fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.BotToken))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("telegram api error: failed to send request")
	} else if resp.IsError() {
		return nil, fmt.Errorf("telegram api error: unexpected status code: %d (resp: %s)", resp.StatusCode(), resp.String())
	}

	return &plugin.NotifyResult{}, nil
}
