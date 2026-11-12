package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type ListLoadBalanceHTTPSListenerRequest struct {
	ListLoadBalanceHTTPSListenerQuery *ListLoadBalanceHTTPSListenerQuery `json:"listLoadBalanceHTTPSListenerQuery,omitempty"`

	ListLoadBalanceHTTPSListenerPath *ListLoadBalanceHTTPSListenerPath `json:"listLoadBalanceHTTPSListenerPath,omitempty"`
}

func (s ListLoadBalanceHTTPSListenerRequest) String() string {
	return utils.Beautify(s)
}

func (s ListLoadBalanceHTTPSListenerRequest) GoString() string {
	return s.String()
}

func (s ListLoadBalanceHTTPSListenerRequest) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *ListLoadBalanceHTTPSListenerRequest) SetListLoadBalanceHTTPSListenerQuery(v *ListLoadBalanceHTTPSListenerQuery) *ListLoadBalanceHTTPSListenerRequest {
	s.ListLoadBalanceHTTPSListenerQuery = v
	return s
}

func (s *ListLoadBalanceHTTPSListenerRequest) SetListLoadBalanceHTTPSListenerPath(v *ListLoadBalanceHTTPSListenerPath) *ListLoadBalanceHTTPSListenerRequest {
	s.ListLoadBalanceHTTPSListenerPath = v
	return s
}

type ListLoadBalanceHTTPSListenerRequestBuilder struct {
	s *ListLoadBalanceHTTPSListenerRequest
}

func NewListLoadBalanceHTTPSListenerRequestBuilder() *ListLoadBalanceHTTPSListenerRequestBuilder {
	s := &ListLoadBalanceHTTPSListenerRequest{}
	b := &ListLoadBalanceHTTPSListenerRequestBuilder{s: s}
	return b
}

func (b *ListLoadBalanceHTTPSListenerRequestBuilder) ListLoadBalanceHTTPSListenerQuery(v *ListLoadBalanceHTTPSListenerQuery) *ListLoadBalanceHTTPSListenerRequestBuilder {
	b.s.ListLoadBalanceHTTPSListenerQuery = v
	return b
}

func (b *ListLoadBalanceHTTPSListenerRequestBuilder) ListLoadBalanceHTTPSListenerPath(v *ListLoadBalanceHTTPSListenerPath) *ListLoadBalanceHTTPSListenerRequestBuilder {
	b.s.ListLoadBalanceHTTPSListenerPath = v
	return b
}

func (b *ListLoadBalanceHTTPSListenerRequestBuilder) Build() *ListLoadBalanceHTTPSListenerRequest {
	return b.s
}
