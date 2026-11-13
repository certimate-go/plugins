package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"

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
	client := notifier.NewClient().SetPreRequestHook(func(_ *resty.Client, req *http.Request) error {
		if config.Secret != "" {
			timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())

			h := hmac.New(sha256.New, []byte(config.Secret))
			h.Write([]byte(fmt.Sprintf("%s\n%s", timestamp, config.Secret)))
			sign := base64.StdEncoding.EncodeToString(h.Sum(nil))

			qs := req.URL.Query()
			qs.Set("timestamp", timestamp)
			qs.Set("sign", sign)
			req.URL.RawQuery = qs.Encode()
		}

		return nil
	})

	webhookURL, err := url.Parse(config.WebhookURL)
	if err != nil {
		return nil, fmt.Errorf("dingtalk api error: invalid webhook url: %w", err)
	} else {
		const hostname = "oapi.dingtalk.com"
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
		return nil, fmt.Errorf("dingtalk api error: failed to send request: %w", err)
	} else if resp.IsError() {
		return nil, fmt.Errorf("dingtalk api error: unexpected status code: %d (resp: %s)", resp.StatusCode(), resp.String())
	} else if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("dingtalk api error: %w (resp: %s)", err, resp.String())
	} else if result.ErrorCode != 0 {
		return nil, fmt.Errorf("dingtalk api error: errcode='%d', errmsg='%s'", result.ErrorCode, result.ErrorMessage)
	}

	return &plugin.NotifyResult{}, nil
}
