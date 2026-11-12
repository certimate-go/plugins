package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type ListLoadbalanceCertificationRespResponseContentContainerTypeEnum string

const (
	ListLoadbalanceCertificationRespResponseContentContainerTypeEnumGeneric     ListLoadbalanceCertificationRespResponseContentContainerTypeEnum = "generic"
	ListLoadbalanceCertificationRespResponseContentContainerTypeEnumRsa         ListLoadbalanceCertificationRespResponseContentContainerTypeEnum = "rsa"
	ListLoadbalanceCertificationRespResponseContentContainerTypeEnumCertificate ListLoadbalanceCertificationRespResponseContentContainerTypeEnum = "certificate"
)

type ListLoadbalanceCertificationRespResponseContentTypeEnum string

const (
	ListLoadbalanceCertificationRespResponseContentTypeEnumServer ListLoadbalanceCertificationRespResponseContentTypeEnum = "SERVER"
	ListLoadbalanceCertificationRespResponseContentTypeEnumCa     ListLoadbalanceCertificationRespResponseContentTypeEnum = "CA"
	ListLoadbalanceCertificationRespResponseContentTypeEnumSni    ListLoadbalanceCertificationRespResponseContentTypeEnum = "SNI"
)

type ListLoadbalanceCertificationRespResponseContentStatusEnum string

const (
	ListLoadbalanceCertificationRespResponseContentStatusEnumPending ListLoadbalanceCertificationRespResponseContentStatusEnum = "PENDING"
	ListLoadbalanceCertificationRespResponseContentStatusEnumActive  ListLoadbalanceCertificationRespResponseContentStatusEnum = "ACTIVE"
	ListLoadbalanceCertificationRespResponseContentStatusEnumError   ListLoadbalanceCertificationRespResponseContentStatusEnum = "ERROR"
)

type ListLoadbalanceCertificationRespResponseContent struct {
	ContainerUuid *string `json:"containerUuid,omitempty"`

	BindedListeners *int32 `json:"bindedListeners,omitempty"`

	ContainerType *ListLoadbalanceCertificationRespResponseContentContainerTypeEnum `json:"containerType,omitempty"`

	Description *string `json:"description,omitempty"`

	Type *ListLoadbalanceCertificationRespResponseContentTypeEnum `json:"type,omitempty"`

	Delete *bool `json:"delete,omitempty"`

	Vaz *string `json:"vaz,omitempty"`

	ContainerRef *string `json:"containerRef,omitempty"`

	ExpirationTime *string `json:"expirationTime,omitempty"`

	Name *string `json:"name,omitempty"`

	CreatedTime *string `json:"createdTime,omitempty"`

	Id *string `json:"id,omitempty"`

	Region *string `json:"region,omitempty"`

	Status *ListLoadbalanceCertificationRespResponseContentStatusEnum `json:"status,omitempty"`
}

func (s ListLoadbalanceCertificationRespResponseContent) String() string {
	return utils.Beautify(s)
}

func (s ListLoadbalanceCertificationRespResponseContent) GoString() string {
	return s.String()
}

func (s ListLoadbalanceCertificationRespResponseContent) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetContainerUuid(v string) *ListLoadbalanceCertificationRespResponseContent {
	s.ContainerUuid = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetBindedListeners(v int32) *ListLoadbalanceCertificationRespResponseContent {
	s.BindedListeners = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetContainerType(v ListLoadbalanceCertificationRespResponseContentContainerTypeEnum) *ListLoadbalanceCertificationRespResponseContent {
	s.ContainerType = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetDescription(v string) *ListLoadbalanceCertificationRespResponseContent {
	s.Description = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetType(v ListLoadbalanceCertificationRespResponseContentTypeEnum) *ListLoadbalanceCertificationRespResponseContent {
	s.Type = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetDelete(v bool) *ListLoadbalanceCertificationRespResponseContent {
	s.Delete = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetVaz(v string) *ListLoadbalanceCertificationRespResponseContent {
	s.Vaz = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetContainerRef(v string) *ListLoadbalanceCertificationRespResponseContent {
	s.ContainerRef = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetExpirationTime(v string) *ListLoadbalanceCertificationRespResponseContent {
	s.ExpirationTime = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetName(v string) *ListLoadbalanceCertificationRespResponseContent {
	s.Name = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetCreatedTime(v string) *ListLoadbalanceCertificationRespResponseContent {
	s.CreatedTime = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetId(v string) *ListLoadbalanceCertificationRespResponseContent {
	s.Id = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetRegion(v string) *ListLoadbalanceCertificationRespResponseContent {
	s.Region = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponseContent) SetStatus(v ListLoadbalanceCertificationRespResponseContentStatusEnum) *ListLoadbalanceCertificationRespResponseContent {
	s.Status = &v
	return s
}

type ListLoadbalanceCertificationRespResponseContentBuilder struct {
	s *ListLoadbalanceCertificationRespResponseContent
}

func NewListLoadbalanceCertificationRespResponseContentBuilder() *ListLoadbalanceCertificationRespResponseContentBuilder {
	s := &ListLoadbalanceCertificationRespResponseContent{}
	b := &ListLoadbalanceCertificationRespResponseContentBuilder{s: s}
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) ContainerUuid(v string) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.ContainerUuid = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) BindedListeners(v int32) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.BindedListeners = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) ContainerType(v ListLoadbalanceCertificationRespResponseContentContainerTypeEnum) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.ContainerType = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) Description(v string) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.Description = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) Type(v ListLoadbalanceCertificationRespResponseContentTypeEnum) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.Type = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) Delete(v bool) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.Delete = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) Vaz(v string) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.Vaz = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) ContainerRef(v string) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.ContainerRef = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) ExpirationTime(v string) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.ExpirationTime = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) Name(v string) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.Name = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) CreatedTime(v string) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.CreatedTime = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) Id(v string) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.Id = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) Region(v string) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.Region = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) Status(v ListLoadbalanceCertificationRespResponseContentStatusEnum) *ListLoadbalanceCertificationRespResponseContentBuilder {
	b.s.Status = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseContentBuilder) Build() *ListLoadbalanceCertificationRespResponseContent {
	return b.s
}
