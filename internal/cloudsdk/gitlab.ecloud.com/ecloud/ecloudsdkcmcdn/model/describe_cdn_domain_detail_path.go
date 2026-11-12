package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/position"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnDomainDetailPath struct {
	position.Path

	DomainId *string `json:"domainId,omitempty"`
}

func (s DescribeCdnDomainDetailPath) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnDomainDetailPath) GoString() string {
	return s.String()
}

func (s DescribeCdnDomainDetailPath) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnDomainDetailPath) SetDomainId(v string) *DescribeCdnDomainDetailPath {
	s.DomainId = &v
	return s
}

type DescribeCdnDomainDetailPathBuilder struct {
	s *DescribeCdnDomainDetailPath
}

func NewDescribeCdnDomainDetailPathBuilder() *DescribeCdnDomainDetailPathBuilder {
	s := &DescribeCdnDomainDetailPath{}
	b := &DescribeCdnDomainDetailPathBuilder{s: s}
	return b
}

func (b *DescribeCdnDomainDetailPathBuilder) DomainId(v string) *DescribeCdnDomainDetailPathBuilder {
	b.s.DomainId = &v
	return b
}

func (b *DescribeCdnDomainDetailPathBuilder) Build() *DescribeCdnDomainDetailPath {
	return b.s
}
