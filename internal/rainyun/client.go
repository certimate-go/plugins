package rainyun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

type Client struct {
	http *resty.Client
}

func NewClient(apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("sdkerr: unset apiKey")
	}

	client := resty.New().
		SetBaseURL("https://api.v2.rainyun.com").
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", "Certimate/RainYun").
		SetHeader("X-API-Key", apiKey)
	return &Client{http: client}, nil
}

type apiResponse[T any] struct {
	Code int `json:"code"`
	Data T   `json:"data"`
}

func send[T any](req *resty.Request, method, path string) (T, error) {
	var result apiResponse[T]
	response, err := req.Execute(method, path)
	if err != nil {
		return result.Data, fmt.Errorf("sdkerr: failed to send request: %w", err)
	}
	if response.IsError() {
		return result.Data, fmt.Errorf("sdkerr: unexpected status code: %d", response.StatusCode())
	}
	if len(response.Body()) == 0 {
		return result.Data, nil
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return result.Data, fmt.Errorf("sdkerr: failed to unmarshal response: %w", err)
	}
	if result.Code/100 != 2 {
		return result.Data, fmt.Errorf("sdkerr: api error: code='%d'", result.Code)
	}
	return result.Data, nil
}

func (c *Client) Bind(ctx context.Context, instanceID, certID int64, domain string) error {
	body := struct {
		CertID  int64    `json:"cert_id"`
		Domains []string `json:"domains"`
	}{certID, []string{domain}}
	req := c.http.R().SetContext(ctx).SetBody(body)
	_, err := send[any](req, http.MethodPost, fmt.Sprintf("/product/rcdn/instance/%d/ssl_bind", instanceID))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'rcdn.InstanceSslBind': %w", err)
	}
	return nil
}
