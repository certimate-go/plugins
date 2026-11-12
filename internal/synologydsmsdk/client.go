package synologydsm

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

type Client struct {
	authApiPath    string
	authApiVersion int

	sid          string
	synoToken    string
	synoTokenMtx sync.Mutex

	rc *resty.Client
}

func NewClient(serverUrl string) (*Client, error) {
	if serverUrl == "" {
		return nil, fmt.Errorf("sdkerr: unset serverUrl")
	}
	if _, err := url.Parse(serverUrl); err != nil {
		return nil, fmt.Errorf("sdkerr: invalid serverUrl: %w", err)
	}

	client := &Client{}
	client.rc = resty.New().
		SetBaseURL(strings.TrimSuffix(serverUrl, "/")).
		SetHeader("User-Agent", "Certimate-Plugin/0.1.0").
		SetPreRequestHook(func(_ *resty.Client, req *http.Request) error {
			if client.synoToken != "" {
				req.Header.Set("X-SYNO-TOKEN", client.synoToken)
			}

			return nil
		})

	return client, nil
}

func (c *Client) SetTimeout(timeout time.Duration) *Client {
	c.rc.SetTimeout(timeout)
	return c
}

func (c *Client) SetTLSConfig(config *tls.Config) *Client {
	c.rc.SetTLSClientConfig(config)
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
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = &url.Error{Op: urlErr.Op, URL: strings.Split(urlErr.URL, "?")[0], Err: urlErr.Err}
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
			if rSuccess := res.GetSuccess(); !rSuccess {
				return resp, fmt.Errorf("sdkerr: api error: code='%d'", res.GetErrorCode())
			}
		}
	}

	return resp, nil
}
