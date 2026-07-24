package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
)

type webhookDeployer struct{}

func (*webhookDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "other",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*webhookDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
	return &plugin.ConfigSchema{
		AccessSchemaJSON: accessSchemaJSON(),
		DeploySchemaJSON: deploySchemaJSON(),
		I18n:             i18nResources(),
	}, nil
}

type accessConfig struct {
	URL                      string `json:"url"`
	Method                   string `json:"method,omitempty"`
	HeadersString            string `json:"headers,omitempty"`
	DataString               string `json:"data,omitempty"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}

type extendedConfig struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

func (d *webhookDeployer) Deploy(_ context.Context, req *plugin.DeployRequest) (*plugin.DeployResult, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	if access.URL == "" {
		return nil, fmt.Errorf("access config missing url")
	}

	var extended extendedConfig
	if req.ExtendedConfigJSON != "" {
		if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
			return nil, fmt.Errorf("invalid deploy config: %w", err)
		}
	}
	method := strings.ToUpper(strings.TrimSpace(extended.Method))
	if method == "" {
		method = strings.ToUpper(strings.TrimSpace(access.Method))
	}
	if method == "" {
		method = http.MethodPost
	}

	target := strings.TrimRight(access.URL, "/") + "/" + strings.TrimLeft(extended.Path, "/")

	body, _ := json.Marshal(map[string]string{
		"certificatePem": req.CertificatePEM,
		"privateKeyPem":  req.PrivateKeyPEM,
	})

	httpReq, err := http.NewRequestWithContext(context.Background(), method, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range parseHeaders(access.HeadersString) {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	if access.AllowInsecureConnections {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	extendedData, _ := json.Marshal(map[string]any{"statusCode": resp.StatusCode, "target": target})
	return &plugin.DeployResult{ExtendedDataJSON: string(extendedData)}, nil
}

func parseHeaders(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return out
}
