package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type AddDomainServerCertificateResponse struct {
	RequestId *string `json:"requestId,omitempty"`

	ErrorMessage *string `json:"errorMessage,omitempty"`

	ErrorCode *string `json:"errorCode,omitempty"`

	State *string `json:"state,omitempty"`

	Body *string `json:"body,omitempty"`
}

func (s AddDomainServerCertificateResponse) String() string {
	return utils.Beautify(s)
}

func (s AddDomainServerCertificateResponse) GoString() string {
	return s.String()
}

func (s AddDomainServerCertificateResponse) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *AddDomainServerCertificateResponse) SetRequestId(v string) *AddDomainServerCertificateResponse {
	s.RequestId = &v
	return s
}

func (s *AddDomainServerCertificateResponse) SetErrorMessage(v string) *AddDomainServerCertificateResponse {
	s.ErrorMessage = &v
	return s
}

func (s *AddDomainServerCertificateResponse) SetErrorCode(v string) *AddDomainServerCertificateResponse {
	s.ErrorCode = &v
	return s
}

func (s *AddDomainServerCertificateResponse) SetState(v string) *AddDomainServerCertificateResponse {
	s.State = &v
	return s
}

func (s *AddDomainServerCertificateResponse) SetBody(v string) *AddDomainServerCertificateResponse {
	s.Body = &v
	return s
}

type AddDomainServerCertificateResponseBuilder struct {
	s *AddDomainServerCertificateResponse
}

func NewAddDomainServerCertificateResponseBuilder() *AddDomainServerCertificateResponseBuilder {
	s := &AddDomainServerCertificateResponse{}
	b := &AddDomainServerCertificateResponseBuilder{s: s}
	return b
}

func (b *AddDomainServerCertificateResponseBuilder) RequestId(v string) *AddDomainServerCertificateResponseBuilder {
	b.s.RequestId = &v
	return b
}

func (b *AddDomainServerCertificateResponseBuilder) ErrorMessage(v string) *AddDomainServerCertificateResponseBuilder {
	b.s.ErrorMessage = &v
	return b
}

func (b *AddDomainServerCertificateResponseBuilder) ErrorCode(v string) *AddDomainServerCertificateResponseBuilder {
	b.s.ErrorCode = &v
	return b
}

func (b *AddDomainServerCertificateResponseBuilder) State(v string) *AddDomainServerCertificateResponseBuilder {
	b.s.State = &v
	return b
}

func (b *AddDomainServerCertificateResponseBuilder) Body(v string) *AddDomainServerCertificateResponseBuilder {
	b.s.Body = &v
	return b
}

func (b *AddDomainServerCertificateResponseBuilder) Build() *AddDomainServerCertificateResponse {
	return b.s
}
