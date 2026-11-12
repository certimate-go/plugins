package netlify

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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

	if opts.ApiToken == "" {
		return nil, fmt.Errorf("sdkerr: unset apiToken")
	}

	httper := resty.New().
		SetBaseURL("https://api.netlify.com/api/v1").
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", "Bearer "+opts.ApiToken).
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", "Certimate-Plugin")

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
		var urlError *url.Error
		if errors.As(err, &urlError) {
			err = urlError.Err
		}
		return resp, fmt.Errorf("sdkerr: failed to send request: %w", err)
	} else if resp.IsError() {
		return resp, fmt.Errorf("sdkerr: unexpected status code: %d", resp.StatusCode())
	}

	return resp, nil
}

func (c *Client) doRequestWithResult(req *resty.Request, res sdkResponse) (*resty.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("sdkerr: nil request")
	}

	resp, err := c.doRequest(req)
	if err != nil {
		if resp != nil {
			json.Unmarshal(resp.Body(), &res)
		}
		return resp, err
	}

	if len(resp.Body()) != 0 {
		if err := json.Unmarshal(resp.Body(), &res); err != nil {
			return resp, fmt.Errorf("sdkerr: failed to unmarshal response: %w", err)
		} else {
			if rCode := res.GetCode(); rCode != 0 {
				return resp, fmt.Errorf("sdkerr: api error: code='%d'", rCode)
			}
		}
	}

	return resp, nil
}
