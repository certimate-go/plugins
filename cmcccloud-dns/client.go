package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-acme/lego/v5/platform/env"
)

const endpoint = "https://ecloud.10086.cn/api/query/openapi/apim/request/sdk"

var cloudClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: env.GetOrDefaultSecond("CMCCCLOUD_CONNECT_TIMEOUT", 30*time.Second),
		}).DialContext,
		ResponseHeaderTimeout: env.GetOrDefaultSecond("CMCCCLOUD_READ_TIMEOUT", 30*time.Second),
	},
	Timeout: env.GetOrDefaultSecond("CMCCCLOUD_CONNECT_TIMEOUT", 30*time.Second) +
		env.GetOrDefaultSecond("CMCCCLOUD_READ_TIMEOUT", 30*time.Second),
}

type accessConfig struct {
	AccessKeyID     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
}

type cloudRequest struct {
	Product       string `json:"product"`
	Version       string `json:"version"`
	SDKVersion    string `json:"sdkVersion"`
	Language      string `json:"language"`
	API           string `json:"api"`
	PoolID        string `json:"poolId"`
	AccessKey     string `json:"accessKey,omitempty"`
	SecretKey     string `json:"secretKey,omitempty"`
	BodyParameter any    `json:"bodyParameter"`
}

type cloudResponse struct {
	State string          `json:"state"`
	Body  json.RawMessage `json:"body"`
}

func callDNS(ctx context.Context, accessJSON, action string, body any) (json.RawMessage, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(accessJSON), &access); err != nil {
		return nil, errors.New("invalid CMCC Cloud access configuration")
	}
	data, err := json.Marshal(cloudRequest{
		Product:       "clouddns",
		Version:       "v1",
		SDKVersion:    "1.0.1",
		Language:      "Golang",
		API:           action,
		PoolID:        "CIDC-CORE-00",
		AccessKey:     access.AccessKeyID,
		SecretKey:     access.AccessKeySecret,
		BodyParameter: body,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := cloudClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CMCC Cloud %s: %w", action, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("CMCC Cloud %s returned HTTP %d", action, res.StatusCode)
	}
	var gateway struct {
		Body struct {
			ResponseBody string `json:"responseBody"`
		} `json:"body"`
	}
	if err := json.NewDecoder(res.Body).Decode(&gateway); err != nil {
		return nil, fmt.Errorf("invalid CMCC Cloud %s gateway response", action)
	}
	var result cloudResponse
	if err := json.Unmarshal([]byte(gateway.Body.ResponseBody), &result); err != nil {
		return nil, fmt.Errorf("invalid CMCC Cloud %s response", action)
	}
	if result.State != "OK" {
		return nil, fmt.Errorf("CMCC Cloud %s failed", action)
	}
	return result.Body, nil
}
