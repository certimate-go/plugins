package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type UpdateListenerRequest struct {
	UpdateListenerBody *UpdateListenerBody `json:"updateListenerBody,omitempty"`
}

func (s UpdateListenerRequest) String() string {
	return utils.Beautify(s)
}

func (s UpdateListenerRequest) GoString() string {
	return s.String()
}

func (s UpdateListenerRequest) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *UpdateListenerRequest) SetUpdateListenerBody(v *UpdateListenerBody) *UpdateListenerRequest {
	s.UpdateListenerBody = v
	return s
}

type UpdateListenerRequestBuilder struct {
	s *UpdateListenerRequest
}

func NewUpdateListenerRequestBuilder() *UpdateListenerRequestBuilder {
	s := &UpdateListenerRequest{}
	b := &UpdateListenerRequestBuilder{s: s}
	return b
}

func (b *UpdateListenerRequestBuilder) UpdateListenerBody(v *UpdateListenerBody) *UpdateListenerRequestBuilder {
	b.s.UpdateListenerBody = v
	return b
}

func (b *UpdateListenerRequestBuilder) Build() *UpdateListenerRequest {
	return b.s
}
