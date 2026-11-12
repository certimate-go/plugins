package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/position"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type GetLoadbalanceCertificationDetailRespPath struct {
	position.Path

	ContainerUuid *string `json:"containerUuid,omitempty"`
}

func (s GetLoadbalanceCertificationDetailRespPath) String() string {
	return utils.Beautify(s)
}

func (s GetLoadbalanceCertificationDetailRespPath) GoString() string {
	return s.String()
}

func (s GetLoadbalanceCertificationDetailRespPath) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *GetLoadbalanceCertificationDetailRespPath) SetContainerUuid(v string) *GetLoadbalanceCertificationDetailRespPath {
	s.ContainerUuid = &v
	return s
}

type GetLoadbalanceCertificationDetailRespPathBuilder struct {
	s *GetLoadbalanceCertificationDetailRespPath
}

func NewGetLoadbalanceCertificationDetailRespPathBuilder() *GetLoadbalanceCertificationDetailRespPathBuilder {
	s := &GetLoadbalanceCertificationDetailRespPath{}
	b := &GetLoadbalanceCertificationDetailRespPathBuilder{s: s}
	return b
}

func (b *GetLoadbalanceCertificationDetailRespPathBuilder) ContainerUuid(v string) *GetLoadbalanceCertificationDetailRespPathBuilder {
	b.s.ContainerUuid = &v
	return b
}

func (b *GetLoadbalanceCertificationDetailRespPathBuilder) Build() *GetLoadbalanceCertificationDetailRespPath {
	return b.s
}
