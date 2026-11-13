package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/notifier"
)

type accessConfig struct {
	WebhookURL    string `json:"webhookUrl"`
	Secret        string `json:"secret"`
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
		return nil, fmt.Errorf("lark api error: invalid webhook url: %w", err)
	} else {
		const hostname = "open.larksuite.com"
		const hostnameCN = "open.feishu.cn"
		if webhookURL.Hostname() != hostname && webhookURL.Hostname() != hostnameCN {
			slog.Default().Warn(fmt.Sprintf("the webhook url hostname is not '%s' or '%s', please make sure it is correct", hostname, hostnameCN))
		}
	}

	webhookData, err := notifier.WebhookPayload(req, config.CustomPayload, map[string]any{
		"msg_type": "text",
		"content": map[string]string{
			"text": req.Subject + "\n\n" + req.Message,
		},
	})
	if err != nil {
		return nil, err
	}

	if config.Secret != "" {
		timestamp := fmt.Sprintf("%d", time.Now().Unix())

		stringToSign := fmt.Sprintf("%s\n%s", timestamp, config.Secret)

		h := hmac.New(sha256.New, []byte(stringToSign))
		sign := base64.StdEncoding.EncodeToString(h.Sum(nil))

		webhookData["timestamp"] = timestamp
		webhookData["sign"] = sign
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
	}
	request := client.R().
		SetContext(ctx).
		SetBody(webhookData)
	resp, err := request.Post(webhookURL.String())
	if err != nil {
		return nil, fmt.Errorf("lark api error: failed to send request: %w", err)
	} else if resp.IsError() {
		return nil, fmt.Errorf("lark api error: unexpected status code: %d (resp: %s)", resp.StatusCode(), resp.String())
	} else if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("lark api error: %w (resp: %s)", err, resp.String())
	} else if result.Code != 0 {
		return nil, fmt.Errorf("lark api error: code='%d', msg='%s'", result.Code, result.Message)
	}

	return &plugin.NotifyResult{}, nil
}
