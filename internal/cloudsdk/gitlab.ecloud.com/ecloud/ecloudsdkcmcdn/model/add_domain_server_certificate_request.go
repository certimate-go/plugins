package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type AddDomainServerCertificateRequest struct {
	AddDomainServerCertificateBody *AddDomainServerCertificateBody `json:"addDomainServerCertificateBody,omitempty"`
}

func (s AddDomainServerCertificateRequest) String() string {
	return utils.Beautify(s)
}

func (s AddDomainServerCertificateRequest) GoString() string {
	return s.String()
}

func (s AddDomainServerCertificateRequest) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *AddDomainServerCertificateRequest) SetAddDomainServerCertificateBody(v *AddDomainServerCertificateBody) *AddDomainServerCertificateRequest {
	s.AddDomainServerCertificateBody = v
	return s
}

type AddDomainServerCertificateRequestBuilder struct {
	s *AddDomainServerCertificateRequest
}

func NewAddDomainServerCertificateRequestBuilder() *AddDomainServerCertificateRequestBuilder {
	s := &AddDomainServerCertificateRequest{}
	b := &AddDomainServerCertificateRequestBuilder{s: s}
	return b
}

func (b *AddDomainServerCertificateRequestBuilder) AddDomainServerCertificateBody(v *AddDomainServerCertificateBody) *AddDomainServerCertificateRequestBuilder {
	b.s.AddDomainServerCertificateBody = v
	return b
}

func (b *AddDomainServerCertificateRequestBuilder) Build() *AddDomainServerCertificateRequest {
	return b.s
}
