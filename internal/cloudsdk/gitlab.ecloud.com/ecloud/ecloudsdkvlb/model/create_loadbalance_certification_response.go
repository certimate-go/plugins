package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type CreateLoadbalanceCertificationResponseStateEnum string

const (
	CreateLoadbalanceCertificationResponseStateEnumOk        CreateLoadbalanceCertificationResponseStateEnum = "OK"
	CreateLoadbalanceCertificationResponseStateEnumError     CreateLoadbalanceCertificationResponseStateEnum = "ERROR"
	CreateLoadbalanceCertificationResponseStateEnumException CreateLoadbalanceCertificationResponseStateEnum = "EXCEPTION"
	CreateLoadbalanceCertificationResponseStateEnumAlarm     CreateLoadbalanceCertificationResponseStateEnum = "ALARM"
	CreateLoadbalanceCertificationResponseStateEnumForbidden CreateLoadbalanceCertificationResponseStateEnum = "FORBIDDEN"
)

type CreateLoadbalanceCertificationResponse struct {
	RequestId *string `json:"requestId,omitempty"`

	ErrorMessage *string `json:"errorMessage,omitempty"`

	ErrorCode *string `json:"errorCode,omitempty"`

	State *CreateLoadbalanceCertificationResponseStateEnum `json:"state,omitempty"`

	Body *string `json:"body,omitempty"`

	ErrorParams []string `json:"errorParams,omitempty"`
}

func (s CreateLoadbalanceCertificationResponse) String() string {
	return utils.Beautify(s)
}

func (s CreateLoadbalanceCertificationResponse) GoString() string {
	return s.String()
}

func (s CreateLoadbalanceCertificationResponse) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *CreateLoadbalanceCertificationResponse) SetRequestId(v string) *CreateLoadbalanceCertificationResponse {
	s.RequestId = &v
	return s
}

func (s *CreateLoadbalanceCertificationResponse) SetErrorMessage(v string) *CreateLoadbalanceCertificationResponse {
	s.ErrorMessage = &v
	return s
}

func (s *CreateLoadbalanceCertificationResponse) SetErrorCode(v string) *CreateLoadbalanceCertificationResponse {
	s.ErrorCode = &v
	return s
}

func (s *CreateLoadbalanceCertificationResponse) SetState(v CreateLoadbalanceCertificationResponseStateEnum) *CreateLoadbalanceCertificationResponse {
	s.State = &v
	return s
}

func (s *CreateLoadbalanceCertificationResponse) SetBody(v string) *CreateLoadbalanceCertificationResponse {
	s.Body = &v
	return s
}

func (s *CreateLoadbalanceCertificationResponse) SetErrorParams(v []string) *CreateLoadbalanceCertificationResponse {
	s.ErrorParams = v
	return s
}

type CreateLoadbalanceCertificationResponseBuilder struct {
	s *CreateLoadbalanceCertificationResponse
}

func NewCreateLoadbalanceCertificationResponseBuilder() *CreateLoadbalanceCertificationResponseBuilder {
	s := &CreateLoadbalanceCertificationResponse{}
	b := &CreateLoadbalanceCertificationResponseBuilder{s: s}
	return b
}

func (b *CreateLoadbalanceCertificationResponseBuilder) RequestId(v string) *CreateLoadbalanceCertificationResponseBuilder {
	b.s.RequestId = &v
	return b
}

func (b *CreateLoadbalanceCertificationResponseBuilder) ErrorMessage(v string) *CreateLoadbalanceCertificationResponseBuilder {
	b.s.ErrorMessage = &v
	return b
}

func (b *CreateLoadbalanceCertificationResponseBuilder) ErrorCode(v string) *CreateLoadbalanceCertificationResponseBuilder {
	b.s.ErrorCode = &v
	return b
}

func (b *CreateLoadbalanceCertificationResponseBuilder) State(v CreateLoadbalanceCertificationResponseStateEnum) *CreateLoadbalanceCertificationResponseBuilder {
	b.s.State = &v
	return b
}

func (b *CreateLoadbalanceCertificationResponseBuilder) Body(v string) *CreateLoadbalanceCertificationResponseBuilder {
	b.s.Body = &v
	return b
}

func (b *CreateLoadbalanceCertificationResponseBuilder) ErrorParams(v []string) *CreateLoadbalanceCertificationResponseBuilder {
	b.s.ErrorParams = v
	return b
}

func (b *CreateLoadbalanceCertificationResponseBuilder) Build() *CreateLoadbalanceCertificationResponse {
	return b.s
}
