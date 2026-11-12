package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type GetLoadbalanceCertificationDetailRespResponseBodyTypeEnum string

const (
	GetLoadbalanceCertificationDetailRespResponseBodyTypeEnumServer GetLoadbalanceCertificationDetailRespResponseBodyTypeEnum = "SERVER"
	GetLoadbalanceCertificationDetailRespResponseBodyTypeEnumCa     GetLoadbalanceCertificationDetailRespResponseBodyTypeEnum = "CA"
	GetLoadbalanceCertificationDetailRespResponseBodyTypeEnumSni    GetLoadbalanceCertificationDetailRespResponseBodyTypeEnum = "SNI"
)

type GetLoadbalanceCertificationDetailRespResponseBody struct {
	PrivateKey *string `json:"privateKey,omitempty"`

	ContainerRef *string `json:"containerRef,omitempty"`

	ExpirationTime *string `json:"expirationTime,omitempty"`

	Name *string `json:"name,omitempty"`

	CreatedTime *string `json:"createdTime,omitempty"`

	Description *string `json:"description,omitempty"`

	Id *string `json:"id,omitempty"`

	PublicKey *string `json:"publicKey,omitempty"`

	Type *GetLoadbalanceCertificationDetailRespResponseBodyTypeEnum `json:"type,omitempty"`

	Vaz *string `json:"vaz,omitempty"`

	Tags *[]GetLoadbalanceCertificationDetailRespResponseTags `json:"tags,omitempty"`
}

func (s GetLoadbalanceCertificationDetailRespResponseBody) String() string {
	return utils.Beautify(s)
}

func (s GetLoadbalanceCertificationDetailRespResponseBody) GoString() string {
	return s.String()
}

func (s GetLoadbalanceCertificationDetailRespResponseBody) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetPrivateKey(v string) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.PrivateKey = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetContainerRef(v string) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.ContainerRef = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetExpirationTime(v string) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.ExpirationTime = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetName(v string) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.Name = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetCreatedTime(v string) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.CreatedTime = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetDescription(v string) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.Description = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetId(v string) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.Id = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetPublicKey(v string) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.PublicKey = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetType(v GetLoadbalanceCertificationDetailRespResponseBodyTypeEnum) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.Type = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetVaz(v string) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.Vaz = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponseBody) SetTags(v []GetLoadbalanceCertificationDetailRespResponseTags) *GetLoadbalanceCertificationDetailRespResponseBody {
	s.Tags = &v
	return s
}

type GetLoadbalanceCertificationDetailRespResponseBodyBuilder struct {
	s *GetLoadbalanceCertificationDetailRespResponseBody
}

func NewGetLoadbalanceCertificationDetailRespResponseBodyBuilder() *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	s := &GetLoadbalanceCertificationDetailRespResponseBody{}
	b := &GetLoadbalanceCertificationDetailRespResponseBodyBuilder{s: s}
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) PrivateKey(v string) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.PrivateKey = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) ContainerRef(v string) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.ContainerRef = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) ExpirationTime(v string) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.ExpirationTime = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) Name(v string) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.Name = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) CreatedTime(v string) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.CreatedTime = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) Description(v string) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.Description = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) Id(v string) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.Id = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) PublicKey(v string) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.PublicKey = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) Type(v GetLoadbalanceCertificationDetailRespResponseBodyTypeEnum) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.Type = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) Vaz(v string) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.Vaz = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) Tags(v []GetLoadbalanceCertificationDetailRespResponseTags) *GetLoadbalanceCertificationDetailRespResponseBodyBuilder {
	b.s.Tags = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBodyBuilder) Build() *GetLoadbalanceCertificationDetailRespResponseBody {
	return b.s
}
