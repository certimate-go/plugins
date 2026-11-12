package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type GetLoadbalanceCertificationDetailRespResponseStateEnum string

const (
	GetLoadbalanceCertificationDetailRespResponseStateEnumOk        GetLoadbalanceCertificationDetailRespResponseStateEnum = "OK"
	GetLoadbalanceCertificationDetailRespResponseStateEnumError     GetLoadbalanceCertificationDetailRespResponseStateEnum = "ERROR"
	GetLoadbalanceCertificationDetailRespResponseStateEnumException GetLoadbalanceCertificationDetailRespResponseStateEnum = "EXCEPTION"
	GetLoadbalanceCertificationDetailRespResponseStateEnumAlarm     GetLoadbalanceCertificationDetailRespResponseStateEnum = "ALARM"
	GetLoadbalanceCertificationDetailRespResponseStateEnumForbidden GetLoadbalanceCertificationDetailRespResponseStateEnum = "FORBIDDEN"
)

type GetLoadbalanceCertificationDetailRespResponse struct {
	RequestId *string `json:"requestId,omitempty"`

	ErrorMessage *string `json:"errorMessage,omitempty"`

	ErrorCode *string `json:"errorCode,omitempty"`

	State *GetLoadbalanceCertificationDetailRespResponseStateEnum `json:"state,omitempty"`

	Body *GetLoadbalanceCertificationDetailRespResponseBody `json:"body,omitempty"`

	ErrorParams []string `json:"errorParams,omitempty"`
}

func (s GetLoadbalanceCertificationDetailRespResponse) String() string {
	return utils.Beautify(s)
}

func (s GetLoadbalanceCertificationDetailRespResponse) GoString() string {
	return s.String()
}

func (s GetLoadbalanceCertificationDetailRespResponse) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *GetLoadbalanceCertificationDetailRespResponse) SetRequestId(v string) *GetLoadbalanceCertificationDetailRespResponse {
	s.RequestId = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponse) SetErrorMessage(v string) *GetLoadbalanceCertificationDetailRespResponse {
	s.ErrorMessage = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponse) SetErrorCode(v string) *GetLoadbalanceCertificationDetailRespResponse {
	s.ErrorCode = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponse) SetState(v GetLoadbalanceCertificationDetailRespResponseStateEnum) *GetLoadbalanceCertificationDetailRespResponse {
	s.State = &v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponse) SetBody(v *GetLoadbalanceCertificationDetailRespResponseBody) *GetLoadbalanceCertificationDetailRespResponse {
	s.Body = v
	return s
}

func (s *GetLoadbalanceCertificationDetailRespResponse) SetErrorParams(v []string) *GetLoadbalanceCertificationDetailRespResponse {
	s.ErrorParams = v
	return s
}

type GetLoadbalanceCertificationDetailRespResponseBuilder struct {
	s *GetLoadbalanceCertificationDetailRespResponse
}

func NewGetLoadbalanceCertificationDetailRespResponseBuilder() *GetLoadbalanceCertificationDetailRespResponseBuilder {
	s := &GetLoadbalanceCertificationDetailRespResponse{}
	b := &GetLoadbalanceCertificationDetailRespResponseBuilder{s: s}
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBuilder) RequestId(v string) *GetLoadbalanceCertificationDetailRespResponseBuilder {
	b.s.RequestId = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBuilder) ErrorMessage(v string) *GetLoadbalanceCertificationDetailRespResponseBuilder {
	b.s.ErrorMessage = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBuilder) ErrorCode(v string) *GetLoadbalanceCertificationDetailRespResponseBuilder {
	b.s.ErrorCode = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBuilder) State(v GetLoadbalanceCertificationDetailRespResponseStateEnum) *GetLoadbalanceCertificationDetailRespResponseBuilder {
	b.s.State = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBuilder) Body(v *GetLoadbalanceCertificationDetailRespResponseBody) *GetLoadbalanceCertificationDetailRespResponseBuilder {
	b.s.Body = v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBuilder) ErrorParams(v []string) *GetLoadbalanceCertificationDetailRespResponseBuilder {
	b.s.ErrorParams = v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespResponseBuilder) Build() *GetLoadbalanceCertificationDetailRespResponse {
	return b.s
}
