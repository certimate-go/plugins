package bunny

import (
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

type Client struct {
	rc *resty.Client
}

func NewClient(optFns ...OptionsFunc) (*Client, error) {
	opts := &Options{}
	for _, fn := range optFns {
		fn(opts)
	}

	if opts.ApiKey == "" {
		return nil, fmt.Errorf("sdkerr: unset apiKey")
	}

	httper := resty.New().
		SetBaseURL("https://api.bunny.net").
		SetHeader("Accept", "application/json").
		SetHeader("AccessKey", opts.ApiKey).
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", "Certimate-Plugin/bunny-cdn")

	return &Client{rc: httper}, nil
}

func (c *Client) SetTimeout(timeout time.Duration) *Client {
	c.rc.SetTimeout(timeout)
	return c
}

func (c *Client) newRequest(method string, path string) (*resty.Request, error) {
	if method == "" {
		return nil, fmt.Errorf("sdkerr: unset method")
	}
	if path == "" {
		return nil, fmt.Errorf("sdkerr: unset path")
	}

	req := c.rc.R()
	req.Method = method
	req.URL = path
	return req, nil
}

func (c *Client) doRequest(req *resty.Request) (*resty.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("sdkerr: nil request")
	}

	resp, err := req.Send()
	if err != nil {
		return resp, fmt.Errorf("sdkerr: failed to send request: %w", err)
	} else if resp.IsError() {
		return resp, fmt.Errorf("sdkerr: unexpected status code: %d", resp.StatusCode())
	}

	return resp, nil
}
