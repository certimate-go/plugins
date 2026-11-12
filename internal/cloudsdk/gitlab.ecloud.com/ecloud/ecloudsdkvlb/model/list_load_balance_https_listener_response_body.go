package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type ListLoadBalanceHTTPSListenerResponseBody struct {
	Total *int32 `json:"total,omitempty"`

	Content *[]ListLoadBalanceHTTPSListenerResponseContent `json:"content,omitempty"`
}

func (s ListLoadBalanceHTTPSListenerResponseBody) String() string {
	return utils.Beautify(s)
}

func (s ListLoadBalanceHTTPSListenerResponseBody) GoString() string {
	return s.String()
}

func (s ListLoadBalanceHTTPSListenerResponseBody) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *ListLoadBalanceHTTPSListenerResponseBody) SetTotal(v int32) *ListLoadBalanceHTTPSListenerResponseBody {
	s.Total = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseBody) SetContent(v []ListLoadBalanceHTTPSListenerResponseContent) *ListLoadBalanceHTTPSListenerResponseBody {
	s.Content = &v
	return s
}

type ListLoadBalanceHTTPSListenerResponseBodyBuilder struct {
	s *ListLoadBalanceHTTPSListenerResponseBody
}

func NewListLoadBalanceHTTPSListenerResponseBodyBuilder() *ListLoadBalanceHTTPSListenerResponseBodyBuilder {
	s := &ListLoadBalanceHTTPSListenerResponseBody{}
	b := &ListLoadBalanceHTTPSListenerResponseBodyBuilder{s: s}
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseBodyBuilder) Total(v int32) *ListLoadBalanceHTTPSListenerResponseBodyBuilder {
	b.s.Total = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseBodyBuilder) Content(v []ListLoadBalanceHTTPSListenerResponseContent) *ListLoadBalanceHTTPSListenerResponseBodyBuilder {
	b.s.Content = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseBodyBuilder) Build() *ListLoadBalanceHTTPSListenerResponseBody {
	return b.s
}
