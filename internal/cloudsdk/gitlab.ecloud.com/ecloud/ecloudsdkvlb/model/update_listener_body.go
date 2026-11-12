package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/position"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type UpdateListenerBody struct {
	position.Body

	SniUp *bool `json:"sniUp,omitempty"`

	GroupType *string `json:"groupType,omitempty"`

	KeepAliveTimeout *int32 `json:"keepAliveTimeout,omitempty"`

	TlsUp *bool `json:"tlsUp,omitempty"`

	ProxyProtocol *bool `json:"proxyProtocol,omitempty"`

	RedirectToListenerId *string `json:"redirectToListenerId,omitempty"`

	SniContainerIds []string `json:"sniContainerIds,omitempty"`

	Transparent *bool `json:"transparent,omitempty"`

	GroupName *string `json:"groupName,omitempty"`

	Name *string `json:"name,omitempty"`

	PoolId *string `json:"poolId,omitempty"`

	GroupEnabled *bool `json:"groupEnabled,omitempty"`

	Http2 *bool `json:"http2,omitempty"`

	CaContainerId *string `json:"caContainerId,omitempty"`

	Id *string `json:"id,omitempty"`

	RedirectEnabled *bool `json:"redirectEnabled,omitempty"`

	ControlGroupId *string `json:"controlGroupId,omitempty"`

	DefaultTlsContainerId *string `json:"defaultTlsContainerId,omitempty"`

	MutualAuthenticationUp *bool `json:"mutualAuthenticationUp,omitempty"`

	TlsCustomizeProtocolId *int64 `json:"tlsCustomizeProtocolId,omitempty"`
}

func (s UpdateListenerBody) String() string {
	return utils.Beautify(s)
}

func (s UpdateListenerBody) GoString() string {
	return s.String()
}

func (s UpdateListenerBody) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *UpdateListenerBody) SetSniUp(v bool) *UpdateListenerBody {
	s.SniUp = &v
	return s
}

func (s *UpdateListenerBody) SetGroupType(v string) *UpdateListenerBody {
	s.GroupType = &v
	return s
}

func (s *UpdateListenerBody) SetKeepAliveTimeout(v int32) *UpdateListenerBody {
	s.KeepAliveTimeout = &v
	return s
}

func (s *UpdateListenerBody) SetTlsUp(v bool) *UpdateListenerBody {
	s.TlsUp = &v
	return s
}

func (s *UpdateListenerBody) SetProxyProtocol(v bool) *UpdateListenerBody {
	s.ProxyProtocol = &v
	return s
}

func (s *UpdateListenerBody) SetRedirectToListenerId(v string) *UpdateListenerBody {
	s.RedirectToListenerId = &v
	return s
}

func (s *UpdateListenerBody) SetSniContainerIds(v []string) *UpdateListenerBody {
	s.SniContainerIds = v
	return s
}

func (s *UpdateListenerBody) SetTransparent(v bool) *UpdateListenerBody {
	s.Transparent = &v
	return s
}

func (s *UpdateListenerBody) SetGroupName(v string) *UpdateListenerBody {
	s.GroupName = &v
	return s
}

func (s *UpdateListenerBody) SetName(v string) *UpdateListenerBody {
	s.Name = &v
	return s
}

func (s *UpdateListenerBody) SetPoolId(v string) *UpdateListenerBody {
	s.PoolId = &v
	return s
}

func (s *UpdateListenerBody) SetGroupEnabled(v bool) *UpdateListenerBody {
	s.GroupEnabled = &v
	return s
}

func (s *UpdateListenerBody) SetHttp2(v bool) *UpdateListenerBody {
	s.Http2 = &v
	return s
}

func (s *UpdateListenerBody) SetCaContainerId(v string) *UpdateListenerBody {
	s.CaContainerId = &v
	return s
}

func (s *UpdateListenerBody) SetId(v string) *UpdateListenerBody {
	s.Id = &v
	return s
}

func (s *UpdateListenerBody) SetRedirectEnabled(v bool) *UpdateListenerBody {
	s.RedirectEnabled = &v
	return s
}

func (s *UpdateListenerBody) SetControlGroupId(v string) *UpdateListenerBody {
	s.ControlGroupId = &v
	return s
}

func (s *UpdateListenerBody) SetDefaultTlsContainerId(v string) *UpdateListenerBody {
	s.DefaultTlsContainerId = &v
	return s
}

func (s *UpdateListenerBody) SetMutualAuthenticationUp(v bool) *UpdateListenerBody {
	s.MutualAuthenticationUp = &v
	return s
}

func (s *UpdateListenerBody) SetTlsCustomizeProtocolId(v int64) *UpdateListenerBody {
	s.TlsCustomizeProtocolId = &v
	return s
}

type UpdateListenerBodyBuilder struct {
	s *UpdateListenerBody
}

func NewUpdateListenerBodyBuilder() *UpdateListenerBodyBuilder {
	s := &UpdateListenerBody{}
	b := &UpdateListenerBodyBuilder{s: s}
	return b
}

func (b *UpdateListenerBodyBuilder) SniUp(v bool) *UpdateListenerBodyBuilder {
	b.s.SniUp = &v
	return b
}

func (b *UpdateListenerBodyBuilder) GroupType(v string) *UpdateListenerBodyBuilder {
	b.s.GroupType = &v
	return b
}

func (b *UpdateListenerBodyBuilder) KeepAliveTimeout(v int32) *UpdateListenerBodyBuilder {
	b.s.KeepAliveTimeout = &v
	return b
}

func (b *UpdateListenerBodyBuilder) TlsUp(v bool) *UpdateListenerBodyBuilder {
	b.s.TlsUp = &v
	return b
}

func (b *UpdateListenerBodyBuilder) ProxyProtocol(v bool) *UpdateListenerBodyBuilder {
	b.s.ProxyProtocol = &v
	return b
}

func (b *UpdateListenerBodyBuilder) RedirectToListenerId(v string) *UpdateListenerBodyBuilder {
	b.s.RedirectToListenerId = &v
	return b
}

func (b *UpdateListenerBodyBuilder) SniContainerIds(v []string) *UpdateListenerBodyBuilder {
	b.s.SniContainerIds = v
	return b
}

func (b *UpdateListenerBodyBuilder) Transparent(v bool) *UpdateListenerBodyBuilder {
	b.s.Transparent = &v
	return b
}

func (b *UpdateListenerBodyBuilder) GroupName(v string) *UpdateListenerBodyBuilder {
	b.s.GroupName = &v
	return b
}

func (b *UpdateListenerBodyBuilder) Name(v string) *UpdateListenerBodyBuilder {
	b.s.Name = &v
	return b
}

func (b *UpdateListenerBodyBuilder) PoolId(v string) *UpdateListenerBodyBuilder {
	b.s.PoolId = &v
	return b
}

func (b *UpdateListenerBodyBuilder) GroupEnabled(v bool) *UpdateListenerBodyBuilder {
	b.s.GroupEnabled = &v
	return b
}

func (b *UpdateListenerBodyBuilder) Http2(v bool) *UpdateListenerBodyBuilder {
	b.s.Http2 = &v
	return b
}

func (b *UpdateListenerBodyBuilder) CaContainerId(v string) *UpdateListenerBodyBuilder {
	b.s.CaContainerId = &v
	return b
}

func (b *UpdateListenerBodyBuilder) Id(v string) *UpdateListenerBodyBuilder {
	b.s.Id = &v
	return b
}

func (b *UpdateListenerBodyBuilder) RedirectEnabled(v bool) *UpdateListenerBodyBuilder {
	b.s.RedirectEnabled = &v
	return b
}

func (b *UpdateListenerBodyBuilder) ControlGroupId(v string) *UpdateListenerBodyBuilder {
	b.s.ControlGroupId = &v
	return b
}

func (b *UpdateListenerBodyBuilder) DefaultTlsContainerId(v string) *UpdateListenerBodyBuilder {
	b.s.DefaultTlsContainerId = &v
	return b
}

func (b *UpdateListenerBodyBuilder) MutualAuthenticationUp(v bool) *UpdateListenerBodyBuilder {
	b.s.MutualAuthenticationUp = &v
	return b
}

func (b *UpdateListenerBodyBuilder) TlsCustomizeProtocolId(v int64) *UpdateListenerBodyBuilder {
	b.s.TlsCustomizeProtocolId = &v
	return b
}

func (b *UpdateListenerBodyBuilder) Build() *UpdateListenerBody {
	return b.s
}
