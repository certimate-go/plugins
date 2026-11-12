package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/position"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type ListLoadBalanceHTTPSListenerPath struct {
	position.Path

	LoadBalanceId *string `json:"loadBalanceId,omitempty"`
}

func (s ListLoadBalanceHTTPSListenerPath) String() string {
	return utils.Beautify(s)
}

func (s ListLoadBalanceHTTPSListenerPath) GoString() string {
	return s.String()
}

func (s ListLoadBalanceHTTPSListenerPath) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *ListLoadBalanceHTTPSListenerPath) SetLoadBalanceId(v string) *ListLoadBalanceHTTPSListenerPath {
	s.LoadBalanceId = &v
	return s
}

type ListLoadBalanceHTTPSListenerPathBuilder struct {
	s *ListLoadBalanceHTTPSListenerPath
}

func NewListLoadBalanceHTTPSListenerPathBuilder() *ListLoadBalanceHTTPSListenerPathBuilder {
	s := &ListLoadBalanceHTTPSListenerPath{}
	b := &ListLoadBalanceHTTPSListenerPathBuilder{s: s}
	return b
}

func (b *ListLoadBalanceHTTPSListenerPathBuilder) LoadBalanceId(v string) *ListLoadBalanceHTTPSListenerPathBuilder {
	b.s.LoadBalanceId = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerPathBuilder) Build() *ListLoadBalanceHTTPSListenerPath {
	return b.s
}
