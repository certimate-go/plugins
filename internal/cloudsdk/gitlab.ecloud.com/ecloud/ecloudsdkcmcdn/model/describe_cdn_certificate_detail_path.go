package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/position"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnCertificateDetailPath struct {
	position.Path

	UniqueId *string `json:"uniqueId,omitempty"`
}

func (s DescribeCdnCertificateDetailPath) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnCertificateDetailPath) GoString() string {
	return s.String()
}

func (s DescribeCdnCertificateDetailPath) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnCertificateDetailPath) SetUniqueId(v string) *DescribeCdnCertificateDetailPath {
	s.UniqueId = &v
	return s
}

type DescribeCdnCertificateDetailPathBuilder struct {
	s *DescribeCdnCertificateDetailPath
}

func NewDescribeCdnCertificateDetailPathBuilder() *DescribeCdnCertificateDetailPathBuilder {
	s := &DescribeCdnCertificateDetailPath{}
	b := &DescribeCdnCertificateDetailPathBuilder{s: s}
	return b
}

func (b *DescribeCdnCertificateDetailPathBuilder) UniqueId(v string) *DescribeCdnCertificateDetailPathBuilder {
	b.s.UniqueId = &v
	return b
}

func (b *DescribeCdnCertificateDetailPathBuilder) Build() *DescribeCdnCertificateDetailPath {
	return b.s
}
