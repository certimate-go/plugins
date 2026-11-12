package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnDomainDetailResponseReqHeaderDetails struct {
	Type *int32 `json:"type,omitempty"`

	Value *string `json:"value,omitempty"`
}

func (s DescribeCdnDomainDetailResponseReqHeaderDetails) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnDomainDetailResponseReqHeaderDetails) GoString() string {
	return s.String()
}

func (s DescribeCdnDomainDetailResponseReqHeaderDetails) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnDomainDetailResponseReqHeaderDetails) SetType(v int32) *DescribeCdnDomainDetailResponseReqHeaderDetails {
	s.Type = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseReqHeaderDetails) SetValue(v string) *DescribeCdnDomainDetailResponseReqHeaderDetails {
	s.Value = &v
	return s
}

type DescribeCdnDomainDetailResponseReqHeaderDetailsBuilder struct {
	s *DescribeCdnDomainDetailResponseReqHeaderDetails
}

func NewDescribeCdnDomainDetailResponseReqHeaderDetailsBuilder() *DescribeCdnDomainDetailResponseReqHeaderDetailsBuilder {
	s := &DescribeCdnDomainDetailResponseReqHeaderDetails{}
	b := &DescribeCdnDomainDetailResponseReqHeaderDetailsBuilder{s: s}
	return b
}

func (b *DescribeCdnDomainDetailResponseReqHeaderDetailsBuilder) Type(v int32) *DescribeCdnDomainDetailResponseReqHeaderDetailsBuilder {
	b.s.Type = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseReqHeaderDetailsBuilder) Value(v string) *DescribeCdnDomainDetailResponseReqHeaderDetailsBuilder {
	b.s.Value = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseReqHeaderDetailsBuilder) Build() *DescribeCdnDomainDetailResponseReqHeaderDetails {
	return b.s
}
