package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type CreateLoadbalanceCertificationRequest struct {
	CreateLoadbalanceCertificationBody *CreateLoadbalanceCertificationBody `json:"createLoadbalanceCertificationBody,omitempty"`
}

func (s CreateLoadbalanceCertificationRequest) String() string {
	return utils.Beautify(s)
}

func (s CreateLoadbalanceCertificationRequest) GoString() string {
	return s.String()
}

func (s CreateLoadbalanceCertificationRequest) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *CreateLoadbalanceCertificationRequest) SetCreateLoadbalanceCertificationBody(v *CreateLoadbalanceCertificationBody) *CreateLoadbalanceCertificationRequest {
	s.CreateLoadbalanceCertificationBody = v
	return s
}

type CreateLoadbalanceCertificationRequestBuilder struct {
	s *CreateLoadbalanceCertificationRequest
}

func NewCreateLoadbalanceCertificationRequestBuilder() *CreateLoadbalanceCertificationRequestBuilder {
	s := &CreateLoadbalanceCertificationRequest{}
	b := &CreateLoadbalanceCertificationRequestBuilder{s: s}
	return b
}

func (b *CreateLoadbalanceCertificationRequestBuilder) CreateLoadbalanceCertificationBody(v *CreateLoadbalanceCertificationBody) *CreateLoadbalanceCertificationRequestBuilder {
	b.s.CreateLoadbalanceCertificationBody = v
	return b
}

func (b *CreateLoadbalanceCertificationRequestBuilder) Build() *CreateLoadbalanceCertificationRequest {
	return b.s
}
