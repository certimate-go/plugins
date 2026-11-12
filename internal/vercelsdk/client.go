package vercel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

type Client struct {
	rc *resty.Client
}

func NewClient(optFns ...OptionsFunc) (*Client, error) {
	options := &Options{}
	for _, fn := range optFns {
		fn(options)
	}

	if options.ApiToken == "" {
		return nil, fmt.Errorf("sdkerr: unset apiToken")
	}

	httper := resty.New().
		SetBaseURL("https://api.vercel.com/v8").
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", "Bearer "+options.ApiToken).
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", "Certimate-Plugin/vercel").
		SetPreRequestHook(func(_ *resty.Client, req *http.Request) error {
			if options.TeamId != "" {
				qs := req.URL.Query()
				qs.Set("teamId", options.TeamId)
				req.URL.RawQuery = qs.Encode()
			}

			return nil
		})

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
			if rError := res.GetError(); rError != nil && rError.Code != "" {
				return resp, fmt.Errorf("sdkerr: api error: code='%s'", rError.Code)
			}
		}
	}

	return resp, nil
}
