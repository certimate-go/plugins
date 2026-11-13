package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/notifier"
)

type accessConfig struct {
	WebhookURL    string `json:"webhookUrl"`
	CustomPayload string `json:"customPayload,omitempty"`
}

func (*server) Notify(ctx context.Context, req *plugin.NotifyRequest) (*plugin.NotifyResult, error) {
	var config accessConfig
	if err := notifier.DecodeAccess(req, &config); err != nil {
		return nil, err
	}
	client := notifier.NewClient()

	webhookURL, err := url.Parse(config.WebhookURL)
	if err != nil {
		return nil, fmt.Errorf("wecom api error: invalid webhook url: %w", err)
	} else {
		const hostname = "qyapi.weixin.qq.com"
		if webhookURL.Hostname() != hostname {
			slog.Default().Warn(fmt.Sprintf("the webhook url hostname is not '%s', please make sure it is correct", hostname))
		}
	}

	webhookData, err := notifier.WebhookPayload(req, config.CustomPayload, map[string]any{
		"msgtype": "text",
		"text": map[string]string{
			"content": req.Subject + "\n\n" + req.Message,
		},
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		ErrorCode    int    `json:"errcode"`
		ErrorMessage string `json:"errmsg"`
	}
	request := client.R().
		SetContext(ctx).
		SetBody(webhookData)
	resp, err := request.Post(webhookURL.String())
	if err != nil {
		return nil, fmt.Errorf("wecom api error: failed to send request: %w", err)
	} else if resp.IsError() {
		return nil, fmt.Errorf("wecom api error: unexpected status code: %d (resp: %s)", resp.StatusCode(), resp.String())
	} else if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("wecom api error: %w (resp: %s)", err, string(resp.Body()))
	} else if result.ErrorCode != 0 {
		return nil, fmt.Errorf("wecom api error: errcode='%d', errmsg='%s'", result.ErrorCode, result.ErrorMessage)
	}

	return &plugin.NotifyResult{}, nil
}
