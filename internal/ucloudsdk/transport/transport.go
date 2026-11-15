package transport

import (
	"context"
	"net/http"
)

type contextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func NewContextRoundTripper(ctx context.Context, base http.RoundTripper) http.RoundTripper {
	return &contextTransport{ctx: ctx, base: base}
}

func (t *contextTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if t.ctx != nil {
		req = req.Clone(t.ctx)
	}
	return base.RoundTrip(req)
}
