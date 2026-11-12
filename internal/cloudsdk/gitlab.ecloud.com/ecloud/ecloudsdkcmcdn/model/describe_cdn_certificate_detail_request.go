package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnCertificateDetailRequest struct {
	DescribeCdnCertificateDetailPath *DescribeCdnCertificateDetailPath `json:"describeCdnCertificateDetailPath,omitempty"`
}

func (s DescribeCdnCertificateDetailRequest) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnCertificateDetailRequest) GoString() string {
	return s.String()
}

func (s DescribeCdnCertificateDetailRequest) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnCertificateDetailRequest) SetDescribeCdnCertificateDetailPath(v *DescribeCdnCertificateDetailPath) *DescribeCdnCertificateDetailRequest {
	s.DescribeCdnCertificateDetailPath = v
	return s
}

type DescribeCdnCertificateDetailRequestBuilder struct {
	s *DescribeCdnCertificateDetailRequest
}

func NewDescribeCdnCertificateDetailRequestBuilder() *DescribeCdnCertificateDetailRequestBuilder {
	s := &DescribeCdnCertificateDetailRequest{}
	b := &DescribeCdnCertificateDetailRequestBuilder{s: s}
	return b
}

func (b *DescribeCdnCertificateDetailRequestBuilder) DescribeCdnCertificateDetailPath(v *DescribeCdnCertificateDetailPath) *DescribeCdnCertificateDetailRequestBuilder {
	b.s.DescribeCdnCertificateDetailPath = v
	return b
}

func (b *DescribeCdnCertificateDetailRequestBuilder) Build() *DescribeCdnCertificateDetailRequest {
	return b.s
}
