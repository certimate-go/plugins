package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnDomainDetailResponseRasLinkDetail struct {
	ListType *int32 `json:"list_type,omitempty"`

	ReferrerNull *bool `json:"referrer_null,omitempty"`

	DomainList *string `json:"domainList,omitempty"`
}

func (s DescribeCdnDomainDetailResponseRasLinkDetail) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnDomainDetailResponseRasLinkDetail) GoString() string {
	return s.String()
}

func (s DescribeCdnDomainDetailResponseRasLinkDetail) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnDomainDetailResponseRasLinkDetail) SetListType(v int32) *DescribeCdnDomainDetailResponseRasLinkDetail {
	s.ListType = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseRasLinkDetail) SetReferrerNull(v bool) *DescribeCdnDomainDetailResponseRasLinkDetail {
	s.ReferrerNull = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseRasLinkDetail) SetDomainList(v string) *DescribeCdnDomainDetailResponseRasLinkDetail {
	s.DomainList = &v
	return s
}

type DescribeCdnDomainDetailResponseRasLinkDetailBuilder struct {
	s *DescribeCdnDomainDetailResponseRasLinkDetail
}

func NewDescribeCdnDomainDetailResponseRasLinkDetailBuilder() *DescribeCdnDomainDetailResponseRasLinkDetailBuilder {
	s := &DescribeCdnDomainDetailResponseRasLinkDetail{}
	b := &DescribeCdnDomainDetailResponseRasLinkDetailBuilder{s: s}
	return b
}

func (b *DescribeCdnDomainDetailResponseRasLinkDetailBuilder) ListType(v int32) *DescribeCdnDomainDetailResponseRasLinkDetailBuilder {
	b.s.ListType = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseRasLinkDetailBuilder) ReferrerNull(v bool) *DescribeCdnDomainDetailResponseRasLinkDetailBuilder {
	b.s.ReferrerNull = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseRasLinkDetailBuilder) DomainList(v string) *DescribeCdnDomainDetailResponseRasLinkDetailBuilder {
	b.s.DomainList = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseRasLinkDetailBuilder) Build() *DescribeCdnDomainDetailResponseRasLinkDetail {
	return b.s
}
