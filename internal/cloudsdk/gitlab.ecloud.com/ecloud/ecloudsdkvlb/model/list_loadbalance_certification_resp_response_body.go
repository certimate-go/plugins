package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type ListLoadbalanceCertificationRespResponseBody struct {
	Total *int32 `json:"total,omitempty"`

	Content *[]ListLoadbalanceCertificationRespResponseContent `json:"content,omitempty"`
}

func (s ListLoadbalanceCertificationRespResponseBody) String() string {
	return utils.Beautify(s)
}

func (s ListLoadbalanceCertificationRespResponseBody) GoString() string {
	return s.String()
}

func (s ListLoadbalanceCertificationRespResponseBody) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *ListLoadbalanceCertificationRespResponseBody) SetTotal(v int32) *ListLoadbalanceCertificationRespResponseBody {
	s.Total = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseBody) SetContent(v []ListLoadbalanceCertificationRespResponseContent) *ListLoadbalanceCertificationRespResponseBody {
	s.Content = &v
	return s
}

type ListLoadbalanceCertificationRespResponseBodyBuilder struct {
	s *ListLoadbalanceCertificationRespResponseBody
}

func NewListLoadbalanceCertificationRespResponseBodyBuilder() *ListLoadbalanceCertificationRespResponseBodyBuilder {
	s := &ListLoadbalanceCertificationRespResponseBody{}
	b := &ListLoadbalanceCertificationRespResponseBodyBuilder{s: s}
	return b
}

func (b *ListLoadbalanceCertificationRespResponseBodyBuilder) Total(v int32) *ListLoadbalanceCertificationRespResponseBodyBuilder {
	b.s.Total = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseBodyBuilder) Content(v []ListLoadbalanceCertificationRespResponseContent) *ListLoadbalanceCertificationRespResponseBodyBuilder {
	b.s.Content = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseBodyBuilder) Build() *ListLoadbalanceCertificationRespResponseBody {
	return b.s
}
