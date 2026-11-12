package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type ListLoadBalanceHTTPSListenerResponseStateEnum string

const (
	ListLoadBalanceHTTPSListenerResponseStateEnumOk        ListLoadBalanceHTTPSListenerResponseStateEnum = "OK"
	ListLoadBalanceHTTPSListenerResponseStateEnumError     ListLoadBalanceHTTPSListenerResponseStateEnum = "ERROR"
	ListLoadBalanceHTTPSListenerResponseStateEnumException ListLoadBalanceHTTPSListenerResponseStateEnum = "EXCEPTION"
	ListLoadBalanceHTTPSListenerResponseStateEnumAlarm     ListLoadBalanceHTTPSListenerResponseStateEnum = "ALARM"
	ListLoadBalanceHTTPSListenerResponseStateEnumForbidden ListLoadBalanceHTTPSListenerResponseStateEnum = "FORBIDDEN"
)

type ListLoadBalanceHTTPSListenerResponse struct {
	RequestId *string `json:"requestId,omitempty"`

	ErrorMessage *string `json:"errorMessage,omitempty"`

	ErrorCode *string `json:"errorCode,omitempty"`

	State *ListLoadBalanceHTTPSListenerResponseStateEnum `json:"state,omitempty"`

	Body *ListLoadBalanceHTTPSListenerResponseBody `json:"body,omitempty"`

	ErrorParams []string `json:"errorParams,omitempty"`
}

func (s ListLoadBalanceHTTPSListenerResponse) String() string {
	return utils.Beautify(s)
}

func (s ListLoadBalanceHTTPSListenerResponse) GoString() string {
	return s.String()
}

func (s ListLoadBalanceHTTPSListenerResponse) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *ListLoadBalanceHTTPSListenerResponse) SetRequestId(v string) *ListLoadBalanceHTTPSListenerResponse {
	s.RequestId = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponse) SetErrorMessage(v string) *ListLoadBalanceHTTPSListenerResponse {
	s.ErrorMessage = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponse) SetErrorCode(v string) *ListLoadBalanceHTTPSListenerResponse {
	s.ErrorCode = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponse) SetState(v ListLoadBalanceHTTPSListenerResponseStateEnum) *ListLoadBalanceHTTPSListenerResponse {
	s.State = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponse) SetBody(v *ListLoadBalanceHTTPSListenerResponseBody) *ListLoadBalanceHTTPSListenerResponse {
	s.Body = v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponse) SetErrorParams(v []string) *ListLoadBalanceHTTPSListenerResponse {
	s.ErrorParams = v
	return s
}

type ListLoadBalanceHTTPSListenerResponseBuilder struct {
	s *ListLoadBalanceHTTPSListenerResponse
}

func NewListLoadBalanceHTTPSListenerResponseBuilder() *ListLoadBalanceHTTPSListenerResponseBuilder {
	s := &ListLoadBalanceHTTPSListenerResponse{}
	b := &ListLoadBalanceHTTPSListenerResponseBuilder{s: s}
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseBuilder) RequestId(v string) *ListLoadBalanceHTTPSListenerResponseBuilder {
	b.s.RequestId = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseBuilder) ErrorMessage(v string) *ListLoadBalanceHTTPSListenerResponseBuilder {
	b.s.ErrorMessage = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseBuilder) ErrorCode(v string) *ListLoadBalanceHTTPSListenerResponseBuilder {
	b.s.ErrorCode = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseBuilder) State(v ListLoadBalanceHTTPSListenerResponseStateEnum) *ListLoadBalanceHTTPSListenerResponseBuilder {
	b.s.State = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseBuilder) Body(v *ListLoadBalanceHTTPSListenerResponseBody) *ListLoadBalanceHTTPSListenerResponseBuilder {
	b.s.Body = v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseBuilder) ErrorParams(v []string) *ListLoadBalanceHTTPSListenerResponseBuilder {
	b.s.ErrorParams = v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseBuilder) Build() *ListLoadBalanceHTTPSListenerResponse {
	return b.s
}
