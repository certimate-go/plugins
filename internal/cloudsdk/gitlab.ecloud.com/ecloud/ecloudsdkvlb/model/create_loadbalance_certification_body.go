package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/position"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type CreateLoadbalanceCertificationBodyTypeEnum string

const (
	CreateLoadbalanceCertificationBodyTypeEnumServer CreateLoadbalanceCertificationBodyTypeEnum = "SERVER"
	CreateLoadbalanceCertificationBodyTypeEnumCa     CreateLoadbalanceCertificationBodyTypeEnum = "CA"
	CreateLoadbalanceCertificationBodyTypeEnumSni    CreateLoadbalanceCertificationBodyTypeEnum = "SNI"
)

type CreateLoadbalanceCertificationBody struct {
	position.Body

	PrivateKey *string `json:"privateKey,omitempty"`

	Name *string `json:"name,omitempty"`

	Description *string `json:"description,omitempty"`

	PublicKey *string `json:"publicKey,omitempty"`

	Type *CreateLoadbalanceCertificationBodyTypeEnum `json:"type,omitempty"`

	Tags *[]CreateLoadbalanceCertificationRequestTags `json:"tags,omitempty"`
}

func (s CreateLoadbalanceCertificationBody) String() string {
	return utils.Beautify(s)
}

func (s CreateLoadbalanceCertificationBody) GoString() string {
	return s.String()
}

func (s CreateLoadbalanceCertificationBody) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *CreateLoadbalanceCertificationBody) SetPrivateKey(v string) *CreateLoadbalanceCertificationBody {
	s.PrivateKey = &v
	return s
}

func (s *CreateLoadbalanceCertificationBody) SetName(v string) *CreateLoadbalanceCertificationBody {
	s.Name = &v
	return s
}

func (s *CreateLoadbalanceCertificationBody) SetDescription(v string) *CreateLoadbalanceCertificationBody {
	s.Description = &v
	return s
}

func (s *CreateLoadbalanceCertificationBody) SetPublicKey(v string) *CreateLoadbalanceCertificationBody {
	s.PublicKey = &v
	return s
}

func (s *CreateLoadbalanceCertificationBody) SetType(v CreateLoadbalanceCertificationBodyTypeEnum) *CreateLoadbalanceCertificationBody {
	s.Type = &v
	return s
}

func (s *CreateLoadbalanceCertificationBody) SetTags(v []CreateLoadbalanceCertificationRequestTags) *CreateLoadbalanceCertificationBody {
	s.Tags = &v
	return s
}

type CreateLoadbalanceCertificationBodyBuilder struct {
	s *CreateLoadbalanceCertificationBody
}

func NewCreateLoadbalanceCertificationBodyBuilder() *CreateLoadbalanceCertificationBodyBuilder {
	s := &CreateLoadbalanceCertificationBody{}
	b := &CreateLoadbalanceCertificationBodyBuilder{s: s}
	return b
}

func (b *CreateLoadbalanceCertificationBodyBuilder) PrivateKey(v string) *CreateLoadbalanceCertificationBodyBuilder {
	b.s.PrivateKey = &v
	return b
}

func (b *CreateLoadbalanceCertificationBodyBuilder) Name(v string) *CreateLoadbalanceCertificationBodyBuilder {
	b.s.Name = &v
	return b
}

func (b *CreateLoadbalanceCertificationBodyBuilder) Description(v string) *CreateLoadbalanceCertificationBodyBuilder {
	b.s.Description = &v
	return b
}

func (b *CreateLoadbalanceCertificationBodyBuilder) PublicKey(v string) *CreateLoadbalanceCertificationBodyBuilder {
	b.s.PublicKey = &v
	return b
}

func (b *CreateLoadbalanceCertificationBodyBuilder) Type(v CreateLoadbalanceCertificationBodyTypeEnum) *CreateLoadbalanceCertificationBodyBuilder {
	b.s.Type = &v
	return b
}

func (b *CreateLoadbalanceCertificationBodyBuilder) Tags(v []CreateLoadbalanceCertificationRequestTags) *CreateLoadbalanceCertificationBodyBuilder {
	b.s.Tags = &v
	return b
}

func (b *CreateLoadbalanceCertificationBodyBuilder) Build() *CreateLoadbalanceCertificationBody {
	return b.s
}
