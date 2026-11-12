package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type ListLoadbalanceCertificationRespResponseStateEnum string

const (
	ListLoadbalanceCertificationRespResponseStateEnumOk        ListLoadbalanceCertificationRespResponseStateEnum = "OK"
	ListLoadbalanceCertificationRespResponseStateEnumError     ListLoadbalanceCertificationRespResponseStateEnum = "ERROR"
	ListLoadbalanceCertificationRespResponseStateEnumException ListLoadbalanceCertificationRespResponseStateEnum = "EXCEPTION"
	ListLoadbalanceCertificationRespResponseStateEnumAlarm     ListLoadbalanceCertificationRespResponseStateEnum = "ALARM"
	ListLoadbalanceCertificationRespResponseStateEnumForbidden ListLoadbalanceCertificationRespResponseStateEnum = "FORBIDDEN"
)

type ListLoadbalanceCertificationRespResponse struct {
	RequestId *string `json:"requestId,omitempty"`

	ErrorMessage *string `json:"errorMessage,omitempty"`

	ErrorCode *string `json:"errorCode,omitempty"`

	State *ListLoadbalanceCertificationRespResponseStateEnum `json:"state,omitempty"`

	Body *ListLoadbalanceCertificationRespResponseBody `json:"body,omitempty"`

	ErrorParams []string `json:"errorParams,omitempty"`
}

func (s ListLoadbalanceCertificationRespResponse) String() string {
	return utils.Beautify(s)
}

func (s ListLoadbalanceCertificationRespResponse) GoString() string {
	return s.String()
}

func (s ListLoadbalanceCertificationRespResponse) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *ListLoadbalanceCertificationRespResponse) SetRequestId(v string) *ListLoadbalanceCertificationRespResponse {
	s.RequestId = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponse) SetErrorMessage(v string) *ListLoadbalanceCertificationRespResponse {
	s.ErrorMessage = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponse) SetErrorCode(v string) *ListLoadbalanceCertificationRespResponse {
	s.ErrorCode = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponse) SetState(v ListLoadbalanceCertificationRespResponseStateEnum) *ListLoadbalanceCertificationRespResponse {
	s.State = &v
	return s
}

func (s *ListLoadbalanceCertificationRespResponse) SetBody(v *ListLoadbalanceCertificationRespResponseBody) *ListLoadbalanceCertificationRespResponse {
	s.Body = v
	return s
}

func (s *ListLoadbalanceCertificationRespResponse) SetErrorParams(v []string) *ListLoadbalanceCertificationRespResponse {
	s.ErrorParams = v
	return s
}

type ListLoadbalanceCertificationRespResponseBuilder struct {
	s *ListLoadbalanceCertificationRespResponse
}

func NewListLoadbalanceCertificationRespResponseBuilder() *ListLoadbalanceCertificationRespResponseBuilder {
	s := &ListLoadbalanceCertificationRespResponse{}
	b := &ListLoadbalanceCertificationRespResponseBuilder{s: s}
	return b
}

func (b *ListLoadbalanceCertificationRespResponseBuilder) RequestId(v string) *ListLoadbalanceCertificationRespResponseBuilder {
	b.s.RequestId = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseBuilder) ErrorMessage(v string) *ListLoadbalanceCertificationRespResponseBuilder {
	b.s.ErrorMessage = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseBuilder) ErrorCode(v string) *ListLoadbalanceCertificationRespResponseBuilder {
	b.s.ErrorCode = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseBuilder) State(v ListLoadbalanceCertificationRespResponseStateEnum) *ListLoadbalanceCertificationRespResponseBuilder {
	b.s.State = &v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseBuilder) Body(v *ListLoadbalanceCertificationRespResponseBody) *ListLoadbalanceCertificationRespResponseBuilder {
	b.s.Body = v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseBuilder) ErrorParams(v []string) *ListLoadbalanceCertificationRespResponseBuilder {
	b.s.ErrorParams = v
	return b
}

func (b *ListLoadbalanceCertificationRespResponseBuilder) Build() *ListLoadbalanceCertificationRespResponse {
	return b.s
}
