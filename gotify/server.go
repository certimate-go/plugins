package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/pluginhttp"
)

type accessConfig struct {
	ServerURL string `json:"serverUrl"`
	Token     string `json:"token"`
}

func (*server) Process(ctx context.Context, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid Gotify access configuration: %w", err)
	}
	if access.Token == "" {
		return nil, errors.New("Gotify application token is required")
	}
	endpoint, err := pluginhttp.ParseServerURL(access.ServerURL)
	if err != nil {
		return nil, err
	}
	payload := struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	}{Title: req.Notify.Subject, Message: req.Notify.Message}
	if err := pluginhttp.SendJSON(ctx, http.MethodPost, endpoint.JoinPath("message"), "X-Gotify-Key", access.Token, payload); err != nil {
		return nil, fmt.Errorf("Gotify publish failed: %w", err)
	}
	return &plugin.Result{Notify: &plugin.NotifyResult{}}, nil
}
