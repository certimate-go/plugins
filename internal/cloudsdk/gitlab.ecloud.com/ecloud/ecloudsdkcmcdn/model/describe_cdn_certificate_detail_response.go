package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnCertificateDetailResponse struct {
	RequestId *string `json:"requestId,omitempty"`

	ErrorMessage *string `json:"errorMessage,omitempty"`

	ErrorCode *string `json:"errorCode,omitempty"`

	State *string `json:"state,omitempty"`

	Body *DescribeCdnCertificateDetailResponseBody `json:"body,omitempty"`
}

func (s DescribeCdnCertificateDetailResponse) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnCertificateDetailResponse) GoString() string {
	return s.String()
}

func (s DescribeCdnCertificateDetailResponse) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnCertificateDetailResponse) SetRequestId(v string) *DescribeCdnCertificateDetailResponse {
	s.RequestId = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponse) SetErrorMessage(v string) *DescribeCdnCertificateDetailResponse {
	s.ErrorMessage = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponse) SetErrorCode(v string) *DescribeCdnCertificateDetailResponse {
	s.ErrorCode = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponse) SetState(v string) *DescribeCdnCertificateDetailResponse {
	s.State = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponse) SetBody(v *DescribeCdnCertificateDetailResponseBody) *DescribeCdnCertificateDetailResponse {
	s.Body = v
	return s
}

type DescribeCdnCertificateDetailResponseBuilder struct {
	s *DescribeCdnCertificateDetailResponse
}

func NewDescribeCdnCertificateDetailResponseBuilder() *DescribeCdnCertificateDetailResponseBuilder {
	s := &DescribeCdnCertificateDetailResponse{}
	b := &DescribeCdnCertificateDetailResponseBuilder{s: s}
	return b
}

func (b *DescribeCdnCertificateDetailResponseBuilder) RequestId(v string) *DescribeCdnCertificateDetailResponseBuilder {
	b.s.RequestId = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBuilder) ErrorMessage(v string) *DescribeCdnCertificateDetailResponseBuilder {
	b.s.ErrorMessage = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBuilder) ErrorCode(v string) *DescribeCdnCertificateDetailResponseBuilder {
	b.s.ErrorCode = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBuilder) State(v string) *DescribeCdnCertificateDetailResponseBuilder {
	b.s.State = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBuilder) Body(v *DescribeCdnCertificateDetailResponseBody) *DescribeCdnCertificateDetailResponseBuilder {
	b.s.Body = v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBuilder) Build() *DescribeCdnCertificateDetailResponse {
	return b.s
}
