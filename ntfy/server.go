package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/pluginhttp"
)

var validTopic = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type accessConfig struct {
	ServerURL string `json:"serverUrl"`
	Topic     string `json:"topic"`
	Token     string `json:"token"`
}

func (*server) Notify(ctx context.Context, req *plugin.NotifyRequest) (*plugin.NotifyResult, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid ntfy access configuration: %w", err)
	}
	endpoint, err := pluginhttp.ParseServerURL(access.ServerURL)
	if err != nil {
		return nil, err
	}
	if !validTopic.MatchString(access.Topic) {
		return nil, errors.New("ntfy topic must contain 1 to 64 letters, digits, hyphens, or underscores")
	}
	credential := ""
	if access.Token != "" {
		credential = "Bearer " + access.Token
	}
	payload := struct {
		Topic   string `json:"topic"`
		Title   string `json:"title"`
		Message string `json:"message"`
	}{Topic: access.Topic, Title: req.Subject, Message: req.Message}
	if err := pluginhttp.SendJSON(ctx, http.MethodPost, endpoint, "Authorization", credential, payload); err != nil {
		return nil, fmt.Errorf("ntfy publish failed: %w", err)
	}
	return &plugin.NotifyResult{}, nil
}
