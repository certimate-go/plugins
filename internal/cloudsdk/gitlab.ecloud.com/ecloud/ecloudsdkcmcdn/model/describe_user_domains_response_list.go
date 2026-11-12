package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeUserDomainsResponseListCdnTypeEnum string

const (
	DescribeUserDomainsResponseListCdnTypeEnumWeb        DescribeUserDomainsResponseListCdnTypeEnum = "web"
	DescribeUserDomainsResponseListCdnTypeEnumDownload   DescribeUserDomainsResponseListCdnTypeEnum = "download"
	DescribeUserDomainsResponseListCdnTypeEnumVideo      DescribeUserDomainsResponseListCdnTypeEnum = "video"
	DescribeUserDomainsResponseListCdnTypeEnumLivestream DescribeUserDomainsResponseListCdnTypeEnum = "liveStream"
)

type DescribeUserDomainsResponseListDomainStatusEnum string

const (
	DescribeUserDomainsResponseListDomainStatusEnumAddAuditing          DescribeUserDomainsResponseListDomainStatusEnum = "ADD_AUDITING"
	DescribeUserDomainsResponseListDomainStatusEnumAddOpening           DescribeUserDomainsResponseListDomainStatusEnum = "ADD_OPENING"
	DescribeUserDomainsResponseListDomainStatusEnumUpdateAuditing       DescribeUserDomainsResponseListDomainStatusEnum = "UPDATE_AUDITING"
	DescribeUserDomainsResponseListDomainStatusEnumUpdateHandling       DescribeUserDomainsResponseListDomainStatusEnum = "UPDATE_HANDLING"
	DescribeUserDomainsResponseListDomainStatusEnumPauseAuditing        DescribeUserDomainsResponseListDomainStatusEnum = "PAUSE_AUDITING"
	DescribeUserDomainsResponseListDomainStatusEnumPauseHandling        DescribeUserDomainsResponseListDomainStatusEnum = "PAUSE_HANDLING"
	DescribeUserDomainsResponseListDomainStatusEnumRecoverAuditing      DescribeUserDomainsResponseListDomainStatusEnum = "RECOVER_AUDITING"
	DescribeUserDomainsResponseListDomainStatusEnumRecoverHandling      DescribeUserDomainsResponseListDomainStatusEnum = "RECOVER_HANDLING"
	DescribeUserDomainsResponseListDomainStatusEnumOfflineAuditing      DescribeUserDomainsResponseListDomainStatusEnum = "OFFLINE_AUDITING"
	DescribeUserDomainsResponseListDomainStatusEnumOfflineHandling      DescribeUserDomainsResponseListDomainStatusEnum = "OFFLINE_HANDLING"
	DescribeUserDomainsResponseListDomainStatusEnumRunning              DescribeUserDomainsResponseListDomainStatusEnum = "RUNNING"
	DescribeUserDomainsResponseListDomainStatusEnumPaused               DescribeUserDomainsResponseListDomainStatusEnum = "PAUSED"
	DescribeUserDomainsResponseListDomainStatusEnumOffline              DescribeUserDomainsResponseListDomainStatusEnum = "OFFLINE"
	DescribeUserDomainsResponseListDomainStatusEnumAuditFail            DescribeUserDomainsResponseListDomainStatusEnum = "AUDIT_FAIL"
	DescribeUserDomainsResponseListDomainStatusEnumUpdateFail           DescribeUserDomainsResponseListDomainStatusEnum = "UPDATE_FAIL"
	DescribeUserDomainsResponseListDomainStatusEnumAddFail              DescribeUserDomainsResponseListDomainStatusEnum = "ADD_FAIL"
	DescribeUserDomainsResponseListDomainStatusEnumOfflineFail          DescribeUserDomainsResponseListDomainStatusEnum = "OFFLINE_FAIL"
	DescribeUserDomainsResponseListDomainStatusEnumPauseOfflineAuditing DescribeUserDomainsResponseListDomainStatusEnum = "PAUSE_OFFLINE_AUDITING"
	DescribeUserDomainsResponseListDomainStatusEnumPauseOfflineFail     DescribeUserDomainsResponseListDomainStatusEnum = "PAUSE_OFFLINE_FAIL"
)

type DescribeUserDomainsResponseListConfigStatusEnum string

const (
	DescribeUserDomainsResponseListConfigStatusEnumUpdateFail DescribeUserDomainsResponseListConfigStatusEnum = "UPDATE_FAIL"
	DescribeUserDomainsResponseListConfigStatusEnumUpdating   DescribeUserDomainsResponseListConfigStatusEnum = "UPDATING"
	DescribeUserDomainsResponseListConfigStatusEnumUpdated    DescribeUserDomainsResponseListConfigStatusEnum = "UPDATED"
)

type DescribeUserDomainsResponseList struct {
	Deleted *bool `json:"deleted,omitempty"`

	CdnType *DescribeUserDomainsResponseListCdnTypeEnum `json:"cdnType,omitempty"`

	ConfigCommons *string `json:"configCommons,omitempty"`

	CreateTime *string `json:"createTime,omitempty"`

	DomainName *string `json:"domainName,omitempty"`

	Cname *string `json:"cname,omitempty"`

	DomainStatus *DescribeUserDomainsResponseListDomainStatusEnum `json:"domainStatus,omitempty"`

	UpdateTime *string `json:"updateTime,omitempty"`

	ConfigStatus *DescribeUserDomainsResponseListConfigStatusEnum `json:"configStatus,omitempty"`

	UserId *string `json:"userId,omitempty"`

	Commons *string `json:"commons,omitempty"`

	DomainId *int32 `json:"domainId,omitempty"`
}

func (s DescribeUserDomainsResponseList) String() string {
	return utils.Beautify(s)
}

func (s DescribeUserDomainsResponseList) GoString() string {
	return s.String()
}

func (s DescribeUserDomainsResponseList) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeUserDomainsResponseList) SetDeleted(v bool) *DescribeUserDomainsResponseList {
	s.Deleted = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetCdnType(v DescribeUserDomainsResponseListCdnTypeEnum) *DescribeUserDomainsResponseList {
	s.CdnType = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetConfigCommons(v string) *DescribeUserDomainsResponseList {
	s.ConfigCommons = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetCreateTime(v string) *DescribeUserDomainsResponseList {
	s.CreateTime = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetDomainName(v string) *DescribeUserDomainsResponseList {
	s.DomainName = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetCname(v string) *DescribeUserDomainsResponseList {
	s.Cname = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetDomainStatus(v DescribeUserDomainsResponseListDomainStatusEnum) *DescribeUserDomainsResponseList {
	s.DomainStatus = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetUpdateTime(v string) *DescribeUserDomainsResponseList {
	s.UpdateTime = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetConfigStatus(v DescribeUserDomainsResponseListConfigStatusEnum) *DescribeUserDomainsResponseList {
	s.ConfigStatus = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetUserId(v string) *DescribeUserDomainsResponseList {
	s.UserId = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetCommons(v string) *DescribeUserDomainsResponseList {
	s.Commons = &v
	return s
}

func (s *DescribeUserDomainsResponseList) SetDomainId(v int32) *DescribeUserDomainsResponseList {
	s.DomainId = &v
	return s
}

type DescribeUserDomainsResponseListBuilder struct {
	s *DescribeUserDomainsResponseList
}

func NewDescribeUserDomainsResponseListBuilder() *DescribeUserDomainsResponseListBuilder {
	s := &DescribeUserDomainsResponseList{}
	b := &DescribeUserDomainsResponseListBuilder{s: s}
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) Deleted(v bool) *DescribeUserDomainsResponseListBuilder {
	b.s.Deleted = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) CdnType(v DescribeUserDomainsResponseListCdnTypeEnum) *DescribeUserDomainsResponseListBuilder {
	b.s.CdnType = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) ConfigCommons(v string) *DescribeUserDomainsResponseListBuilder {
	b.s.ConfigCommons = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) CreateTime(v string) *DescribeUserDomainsResponseListBuilder {
	b.s.CreateTime = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) DomainName(v string) *DescribeUserDomainsResponseListBuilder {
	b.s.DomainName = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) Cname(v string) *DescribeUserDomainsResponseListBuilder {
	b.s.Cname = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) DomainStatus(v DescribeUserDomainsResponseListDomainStatusEnum) *DescribeUserDomainsResponseListBuilder {
	b.s.DomainStatus = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) UpdateTime(v string) *DescribeUserDomainsResponseListBuilder {
	b.s.UpdateTime = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) ConfigStatus(v DescribeUserDomainsResponseListConfigStatusEnum) *DescribeUserDomainsResponseListBuilder {
	b.s.ConfigStatus = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) UserId(v string) *DescribeUserDomainsResponseListBuilder {
	b.s.UserId = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) Commons(v string) *DescribeUserDomainsResponseListBuilder {
	b.s.Commons = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) DomainId(v int32) *DescribeUserDomainsResponseListBuilder {
	b.s.DomainId = &v
	return b
}

func (b *DescribeUserDomainsResponseListBuilder) Build() *DescribeUserDomainsResponseList {
	return b.s
}
