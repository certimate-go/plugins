package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnDomainDetailResponseUrlParameterDetail struct {
	Type *int32 `json:"type,omitempty"`

	Content *string `json:"content,omitempty"`
}

func (s DescribeCdnDomainDetailResponseUrlParameterDetail) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnDomainDetailResponseUrlParameterDetail) GoString() string {
	return s.String()
}

func (s DescribeCdnDomainDetailResponseUrlParameterDetail) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnDomainDetailResponseUrlParameterDetail) SetType(v int32) *DescribeCdnDomainDetailResponseUrlParameterDetail {
	s.Type = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseUrlParameterDetail) SetContent(v string) *DescribeCdnDomainDetailResponseUrlParameterDetail {
	s.Content = &v
	return s
}

type DescribeCdnDomainDetailResponseUrlParameterDetailBuilder struct {
	s *DescribeCdnDomainDetailResponseUrlParameterDetail
}

func NewDescribeCdnDomainDetailResponseUrlParameterDetailBuilder() *DescribeCdnDomainDetailResponseUrlParameterDetailBuilder {
	s := &DescribeCdnDomainDetailResponseUrlParameterDetail{}
	b := &DescribeCdnDomainDetailResponseUrlParameterDetailBuilder{s: s}
	return b
}

func (b *DescribeCdnDomainDetailResponseUrlParameterDetailBuilder) Type(v int32) *DescribeCdnDomainDetailResponseUrlParameterDetailBuilder {
	b.s.Type = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseUrlParameterDetailBuilder) Content(v string) *DescribeCdnDomainDetailResponseUrlParameterDetailBuilder {
	b.s.Content = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseUrlParameterDetailBuilder) Build() *DescribeCdnDomainDetailResponseUrlParameterDetail {
	return b.s
}
