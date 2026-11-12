package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type CreateLoadbalanceCertificationRequestTags struct {
	TagValue *string `json:"tagValue,omitempty"`

	TagKey *string `json:"tagKey,omitempty"`
}

func (s CreateLoadbalanceCertificationRequestTags) String() string {
	return utils.Beautify(s)
}

func (s CreateLoadbalanceCertificationRequestTags) GoString() string {
	return s.String()
}

func (s CreateLoadbalanceCertificationRequestTags) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *CreateLoadbalanceCertificationRequestTags) SetTagValue(v string) *CreateLoadbalanceCertificationRequestTags {
	s.TagValue = &v
	return s
}

func (s *CreateLoadbalanceCertificationRequestTags) SetTagKey(v string) *CreateLoadbalanceCertificationRequestTags {
	s.TagKey = &v
	return s
}

type CreateLoadbalanceCertificationRequestTagsBuilder struct {
	s *CreateLoadbalanceCertificationRequestTags
}

func NewCreateLoadbalanceCertificationRequestTagsBuilder() *CreateLoadbalanceCertificationRequestTagsBuilder {
	s := &CreateLoadbalanceCertificationRequestTags{}
	b := &CreateLoadbalanceCertificationRequestTagsBuilder{s: s}
	return b
}

func (b *CreateLoadbalanceCertificationRequestTagsBuilder) TagValue(v string) *CreateLoadbalanceCertificationRequestTagsBuilder {
	b.s.TagValue = &v
	return b
}

func (b *CreateLoadbalanceCertificationRequestTagsBuilder) TagKey(v string) *CreateLoadbalanceCertificationRequestTagsBuilder {
	b.s.TagKey = &v
	return b
}

func (b *CreateLoadbalanceCertificationRequestTagsBuilder) Build() *CreateLoadbalanceCertificationRequestTags {
	return b.s
}
