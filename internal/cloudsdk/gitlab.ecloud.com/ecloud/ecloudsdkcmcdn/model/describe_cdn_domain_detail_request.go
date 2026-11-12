package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnDomainDetailRequest struct {
	DescribeCdnDomainDetailPath *DescribeCdnDomainDetailPath `json:"describeCdnDomainDetailPath,omitempty"`
}

func (s DescribeCdnDomainDetailRequest) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnDomainDetailRequest) GoString() string {
	return s.String()
}

func (s DescribeCdnDomainDetailRequest) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnDomainDetailRequest) SetDescribeCdnDomainDetailPath(v *DescribeCdnDomainDetailPath) *DescribeCdnDomainDetailRequest {
	s.DescribeCdnDomainDetailPath = v
	return s
}

type DescribeCdnDomainDetailRequestBuilder struct {
	s *DescribeCdnDomainDetailRequest
}

func NewDescribeCdnDomainDetailRequestBuilder() *DescribeCdnDomainDetailRequestBuilder {
	s := &DescribeCdnDomainDetailRequest{}
	b := &DescribeCdnDomainDetailRequestBuilder{s: s}
	return b
}

func (b *DescribeCdnDomainDetailRequestBuilder) DescribeCdnDomainDetailPath(v *DescribeCdnDomainDetailPath) *DescribeCdnDomainDetailRequestBuilder {
	b.s.DescribeCdnDomainDetailPath = v
	return b
}

func (b *DescribeCdnDomainDetailRequestBuilder) Build() *DescribeCdnDomainDetailRequest {
	return b.s
}
