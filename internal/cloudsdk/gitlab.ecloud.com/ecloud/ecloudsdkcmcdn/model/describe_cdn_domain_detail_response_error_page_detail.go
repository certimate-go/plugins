package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnDomainDetailResponseErrorPageDetail struct {
	Link *string `json:"link,omitempty"`

	Type *int32 `json:"type,omitempty"`
}

func (s DescribeCdnDomainDetailResponseErrorPageDetail) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnDomainDetailResponseErrorPageDetail) GoString() string {
	return s.String()
}

func (s DescribeCdnDomainDetailResponseErrorPageDetail) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnDomainDetailResponseErrorPageDetail) SetLink(v string) *DescribeCdnDomainDetailResponseErrorPageDetail {
	s.Link = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseErrorPageDetail) SetType(v int32) *DescribeCdnDomainDetailResponseErrorPageDetail {
	s.Type = &v
	return s
}

type DescribeCdnDomainDetailResponseErrorPageDetailBuilder struct {
	s *DescribeCdnDomainDetailResponseErrorPageDetail
}

func NewDescribeCdnDomainDetailResponseErrorPageDetailBuilder() *DescribeCdnDomainDetailResponseErrorPageDetailBuilder {
	s := &DescribeCdnDomainDetailResponseErrorPageDetail{}
	b := &DescribeCdnDomainDetailResponseErrorPageDetailBuilder{s: s}
	return b
}

func (b *DescribeCdnDomainDetailResponseErrorPageDetailBuilder) Link(v string) *DescribeCdnDomainDetailResponseErrorPageDetailBuilder {
	b.s.Link = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseErrorPageDetailBuilder) Type(v int32) *DescribeCdnDomainDetailResponseErrorPageDetailBuilder {
	b.s.Type = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseErrorPageDetailBuilder) Build() *DescribeCdnDomainDetailResponseErrorPageDetail {
	return b.s
}
