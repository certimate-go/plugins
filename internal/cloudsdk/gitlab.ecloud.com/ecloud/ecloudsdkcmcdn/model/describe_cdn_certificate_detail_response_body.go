package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnCertificateDetailResponseBody struct {
	ContactMobile *string `json:"contactMobile,omitempty"`

	DomainIds *string `json:"domainIds,omitempty"`

	PrivateKey *string `json:"privateKey,omitempty"`

	ContactEmail *string `json:"contactEmail,omitempty"`

	CreateTime *string `json:"createTime,omitempty"`

	ContactName *string `json:"contactName,omitempty"`

	ExpirationTime *string `json:"expirationTime,omitempty"`

	Certificate *string `json:"certificate,omitempty"`

	CrtName *string `json:"crtName,omitempty"`

	DomainNames *string `json:"domainNames,omitempty"`

	UniqueId *string `json:"uniqueId,omitempty"`
}

func (s DescribeCdnCertificateDetailResponseBody) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnCertificateDetailResponseBody) GoString() string {
	return s.String()
}

func (s DescribeCdnCertificateDetailResponseBody) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnCertificateDetailResponseBody) SetContactMobile(v string) *DescribeCdnCertificateDetailResponseBody {
	s.ContactMobile = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponseBody) SetDomainIds(v string) *DescribeCdnCertificateDetailResponseBody {
	s.DomainIds = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponseBody) SetPrivateKey(v string) *DescribeCdnCertificateDetailResponseBody {
	s.PrivateKey = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponseBody) SetContactEmail(v string) *DescribeCdnCertificateDetailResponseBody {
	s.ContactEmail = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponseBody) SetCreateTime(v string) *DescribeCdnCertificateDetailResponseBody {
	s.CreateTime = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponseBody) SetContactName(v string) *DescribeCdnCertificateDetailResponseBody {
	s.ContactName = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponseBody) SetExpirationTime(v string) *DescribeCdnCertificateDetailResponseBody {
	s.ExpirationTime = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponseBody) SetCertificate(v string) *DescribeCdnCertificateDetailResponseBody {
	s.Certificate = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponseBody) SetCrtName(v string) *DescribeCdnCertificateDetailResponseBody {
	s.CrtName = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponseBody) SetDomainNames(v string) *DescribeCdnCertificateDetailResponseBody {
	s.DomainNames = &v
	return s
}

func (s *DescribeCdnCertificateDetailResponseBody) SetUniqueId(v string) *DescribeCdnCertificateDetailResponseBody {
	s.UniqueId = &v
	return s
}

type DescribeCdnCertificateDetailResponseBodyBuilder struct {
	s *DescribeCdnCertificateDetailResponseBody
}

func NewDescribeCdnCertificateDetailResponseBodyBuilder() *DescribeCdnCertificateDetailResponseBodyBuilder {
	s := &DescribeCdnCertificateDetailResponseBody{}
	b := &DescribeCdnCertificateDetailResponseBodyBuilder{s: s}
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) ContactMobile(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.ContactMobile = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) DomainIds(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.DomainIds = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) PrivateKey(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.PrivateKey = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) ContactEmail(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.ContactEmail = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) CreateTime(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.CreateTime = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) ContactName(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.ContactName = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) ExpirationTime(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.ExpirationTime = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) Certificate(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.Certificate = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) CrtName(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.CrtName = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) DomainNames(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.DomainNames = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) UniqueId(v string) *DescribeCdnCertificateDetailResponseBodyBuilder {
	b.s.UniqueId = &v
	return b
}

func (b *DescribeCdnCertificateDetailResponseBodyBuilder) Build() *DescribeCdnCertificateDetailResponseBody {
	return b.s
}
