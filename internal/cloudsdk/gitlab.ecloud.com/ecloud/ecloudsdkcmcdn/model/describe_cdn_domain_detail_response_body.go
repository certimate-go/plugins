package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeCdnDomainDetailResponseBodyCdnProtocolEnum string

const (
	DescribeCdnDomainDetailResponseBodyCdnProtocolEnumHttp      DescribeCdnDomainDetailResponseBodyCdnProtocolEnum = "HTTP"
	DescribeCdnDomainDetailResponseBodyCdnProtocolEnumHttps     DescribeCdnDomainDetailResponseBodyCdnProtocolEnum = "HTTPS"
	DescribeCdnDomainDetailResponseBodyCdnProtocolEnumHttpHttps DescribeCdnDomainDetailResponseBodyCdnProtocolEnum = "HTTP_HTTPS"
)

type DescribeCdnDomainDetailResponseBodyConfigStatusEnum string

const (
	DescribeCdnDomainDetailResponseBodyConfigStatusEnumUpdateFail DescribeCdnDomainDetailResponseBodyConfigStatusEnum = "UPDATE_FAIL"
	DescribeCdnDomainDetailResponseBodyConfigStatusEnumUpdating   DescribeCdnDomainDetailResponseBodyConfigStatusEnum = "UPDATING"
	DescribeCdnDomainDetailResponseBodyConfigStatusEnumUpdated    DescribeCdnDomainDetailResponseBodyConfigStatusEnum = "UPDATED"
)

type DescribeCdnDomainDetailResponseBodyCdnTypeEnum string

const (
	DescribeCdnDomainDetailResponseBodyCdnTypeEnumWeb        DescribeCdnDomainDetailResponseBodyCdnTypeEnum = "web"
	DescribeCdnDomainDetailResponseBodyCdnTypeEnumDownload   DescribeCdnDomainDetailResponseBodyCdnTypeEnum = "download"
	DescribeCdnDomainDetailResponseBodyCdnTypeEnumVideo      DescribeCdnDomainDetailResponseBodyCdnTypeEnum = "video"
	DescribeCdnDomainDetailResponseBodyCdnTypeEnumLivestream DescribeCdnDomainDetailResponseBodyCdnTypeEnum = "liveStream"
)

type DescribeCdnDomainDetailResponseBodyDomainStatusEnum string

const (
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumAddAuditing          DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "ADD_AUDITING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumAddOpening           DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "ADD_OPENING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumUpdateAuditing       DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "UPDATE_AUDITING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumUpdateHandling       DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "UPDATE_HANDLING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumPauseAuditing        DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "PAUSE_AUDITING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumPauseHandling        DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "PAUSE_HANDLING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumRecoverAuditing      DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "RECOVER_AUDITING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumRecoverHandling      DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "RECOVER_HANDLING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumOfflineAuditing      DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "OFFLINE_AUDITING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumOfflineHandling      DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "OFFLINE_HANDLING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumRunning              DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "RUNNING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumPaused               DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "PAUSED"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumOffline              DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "OFFLINE"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumAuditFail            DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "AUDIT_FAIL"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumUpdateFail           DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "UPDATE_FAIL"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumAddFail              DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "ADD_FAIL"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumOfflineFail          DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "OFFLINE_FAIL"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumPauseOfflineAuditing DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "PAUSE_OFFLINE_AUDITING"
	DescribeCdnDomainDetailResponseBodyDomainStatusEnumPauseOfflineFail     DescribeCdnDomainDetailResponseBodyDomainStatusEnum = "PAUSE_OFFLINE_FAIL"
)

type DescribeCdnDomainDetailResponseBody struct {
	ErrorPageDetail *DescribeCdnDomainDetailResponseErrorPageDetail `json:"errorPageDetail,omitempty"`

	FileName *string `json:"fileName,omitempty"`

	EnableSetReqHeader *bool `json:"enableSetReqHeader,omitempty"`

	Sources *[]DescribeCdnDomainDetailResponseSources `json:"sources,omitempty"`

	ConfigCommons *string `json:"configCommons,omitempty"`

	CdnProtocol *DescribeCdnDomainDetailResponseBodyCdnProtocolEnum `json:"cdnProtocol,omitempty"`

	Cname *string `json:"cname,omitempty"`

	CrtName *string `json:"crtName,omitempty"`

	BackSourceHost *DescribeCdnDomainDetailResponseBackSourceHost `json:"backSourceHost,omitempty"`

	IcpLicensing *string `json:"icpLicensing,omitempty"`

	ConfigStatus *DescribeCdnDomainDetailResponseBodyConfigStatusEnum `json:"configStatus,omitempty"`

	UrlParameterDetail *DescribeCdnDomainDetailResponseUrlParameterDetail `json:"urlParameterDetail,omitempty"`

	Commons *string `json:"commons,omitempty"`

	EnableHttp2Parameter *bool `json:"enableHttp2Parameter,omitempty"`

	EnableUrlParameter *bool `json:"enableUrlParameter,omitempty"`

	Ipv6OnOff *bool `json:"ipv6OnOff,omitempty"`

	IpBlackList *DescribeCdnDomainDetailResponseIpBlackList `json:"ipBlackList,omitempty"`

	EnableReferrerAntiStealingLink *bool `json:"enableReferrerAntiStealingLink,omitempty"`

	CacheDetails *[]DescribeCdnDomainDetailResponseCacheDetails `json:"cacheDetails,omitempty"`

	EnableHttp2Status *bool `json:"enableHttp2Status,omitempty"`

	EnableIpBlackList *bool `json:"enableIpBlackList,omitempty"`

	EnableSetRespHeader *bool `json:"enableSetRespHeader,omitempty"`

	FileList []string `json:"fileList,omitempty"`

	RasLinkDetail *DescribeCdnDomainDetailResponseRasLinkDetail `json:"rasLinkDetail,omitempty"`

	CdnType *DescribeCdnDomainDetailResponseBodyCdnTypeEnum `json:"cdnType,omitempty"`

	CrtUniqueId *string `json:"crtUniqueId,omitempty"`

	DomainStatus *DescribeCdnDomainDetailResponseBodyDomainStatusEnum `json:"domainStatus,omitempty"`

	UpdateTime *string `json:"updateTime,omitempty"`

	EnableCustomErrorPage *bool `json:"enableCustomErrorPage,omitempty"`

	RespHeaderDetails *[]DescribeCdnDomainDetailResponseRespHeaderDetails `json:"respHeaderDetails,omitempty"`

	DomainId *int32 `json:"domainId,omitempty"`

	EnableBackSourceHost *bool `json:"enableBackSourceHost,omitempty"`

	AddType *int32 `json:"addType,omitempty"`

	Deleted *bool `json:"deleted,omitempty"`

	CreateTime *string `json:"createTime,omitempty"`

	DomainName *string `json:"domainName,omitempty"`

	EnableGzipStatus *bool `json:"enableGzipStatus,omitempty"`

	ReqHeaderDetails *[]DescribeCdnDomainDetailResponseReqHeaderDetails `json:"reqHeaderDetails,omitempty"`

	EnableCache *bool `json:"enableCache,omitempty"`
}

func (s DescribeCdnDomainDetailResponseBody) String() string {
	return utils.Beautify(s)
}

func (s DescribeCdnDomainDetailResponseBody) GoString() string {
	return s.String()
}

func (s DescribeCdnDomainDetailResponseBody) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeCdnDomainDetailResponseBody) SetErrorPageDetail(v *DescribeCdnDomainDetailResponseErrorPageDetail) *DescribeCdnDomainDetailResponseBody {
	s.ErrorPageDetail = v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetFileName(v string) *DescribeCdnDomainDetailResponseBody {
	s.FileName = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableSetReqHeader(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableSetReqHeader = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetSources(v []DescribeCdnDomainDetailResponseSources) *DescribeCdnDomainDetailResponseBody {
	s.Sources = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetConfigCommons(v string) *DescribeCdnDomainDetailResponseBody {
	s.ConfigCommons = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetCdnProtocol(v DescribeCdnDomainDetailResponseBodyCdnProtocolEnum) *DescribeCdnDomainDetailResponseBody {
	s.CdnProtocol = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetCname(v string) *DescribeCdnDomainDetailResponseBody {
	s.Cname = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetCrtName(v string) *DescribeCdnDomainDetailResponseBody {
	s.CrtName = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetBackSourceHost(v *DescribeCdnDomainDetailResponseBackSourceHost) *DescribeCdnDomainDetailResponseBody {
	s.BackSourceHost = v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetIcpLicensing(v string) *DescribeCdnDomainDetailResponseBody {
	s.IcpLicensing = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetConfigStatus(v DescribeCdnDomainDetailResponseBodyConfigStatusEnum) *DescribeCdnDomainDetailResponseBody {
	s.ConfigStatus = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetUrlParameterDetail(v *DescribeCdnDomainDetailResponseUrlParameterDetail) *DescribeCdnDomainDetailResponseBody {
	s.UrlParameterDetail = v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetCommons(v string) *DescribeCdnDomainDetailResponseBody {
	s.Commons = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableHttp2Parameter(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableHttp2Parameter = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableUrlParameter(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableUrlParameter = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetIpv6OnOff(v bool) *DescribeCdnDomainDetailResponseBody {
	s.Ipv6OnOff = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetIpBlackList(v *DescribeCdnDomainDetailResponseIpBlackList) *DescribeCdnDomainDetailResponseBody {
	s.IpBlackList = v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableReferrerAntiStealingLink(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableReferrerAntiStealingLink = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetCacheDetails(v []DescribeCdnDomainDetailResponseCacheDetails) *DescribeCdnDomainDetailResponseBody {
	s.CacheDetails = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableHttp2Status(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableHttp2Status = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableIpBlackList(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableIpBlackList = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableSetRespHeader(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableSetRespHeader = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetFileList(v []string) *DescribeCdnDomainDetailResponseBody {
	s.FileList = v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetRasLinkDetail(v *DescribeCdnDomainDetailResponseRasLinkDetail) *DescribeCdnDomainDetailResponseBody {
	s.RasLinkDetail = v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetCdnType(v DescribeCdnDomainDetailResponseBodyCdnTypeEnum) *DescribeCdnDomainDetailResponseBody {
	s.CdnType = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetCrtUniqueId(v string) *DescribeCdnDomainDetailResponseBody {
	s.CrtUniqueId = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetDomainStatus(v DescribeCdnDomainDetailResponseBodyDomainStatusEnum) *DescribeCdnDomainDetailResponseBody {
	s.DomainStatus = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetUpdateTime(v string) *DescribeCdnDomainDetailResponseBody {
	s.UpdateTime = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableCustomErrorPage(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableCustomErrorPage = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetRespHeaderDetails(v []DescribeCdnDomainDetailResponseRespHeaderDetails) *DescribeCdnDomainDetailResponseBody {
	s.RespHeaderDetails = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetDomainId(v int32) *DescribeCdnDomainDetailResponseBody {
	s.DomainId = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableBackSourceHost(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableBackSourceHost = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetAddType(v int32) *DescribeCdnDomainDetailResponseBody {
	s.AddType = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetDeleted(v bool) *DescribeCdnDomainDetailResponseBody {
	s.Deleted = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetCreateTime(v string) *DescribeCdnDomainDetailResponseBody {
	s.CreateTime = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetDomainName(v string) *DescribeCdnDomainDetailResponseBody {
	s.DomainName = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableGzipStatus(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableGzipStatus = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetReqHeaderDetails(v []DescribeCdnDomainDetailResponseReqHeaderDetails) *DescribeCdnDomainDetailResponseBody {
	s.ReqHeaderDetails = &v
	return s
}

func (s *DescribeCdnDomainDetailResponseBody) SetEnableCache(v bool) *DescribeCdnDomainDetailResponseBody {
	s.EnableCache = &v
	return s
}

type DescribeCdnDomainDetailResponseBodyBuilder struct {
	s *DescribeCdnDomainDetailResponseBody
}

func NewDescribeCdnDomainDetailResponseBodyBuilder() *DescribeCdnDomainDetailResponseBodyBuilder {
	s := &DescribeCdnDomainDetailResponseBody{}
	b := &DescribeCdnDomainDetailResponseBodyBuilder{s: s}
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) ErrorPageDetail(v *DescribeCdnDomainDetailResponseErrorPageDetail) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.ErrorPageDetail = v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) FileName(v string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.FileName = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableSetReqHeader(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableSetReqHeader = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) Sources(v []DescribeCdnDomainDetailResponseSources) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.Sources = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) ConfigCommons(v string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.ConfigCommons = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) CdnProtocol(v DescribeCdnDomainDetailResponseBodyCdnProtocolEnum) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.CdnProtocol = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) Cname(v string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.Cname = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) CrtName(v string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.CrtName = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) BackSourceHost(v *DescribeCdnDomainDetailResponseBackSourceHost) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.BackSourceHost = v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) IcpLicensing(v string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.IcpLicensing = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) ConfigStatus(v DescribeCdnDomainDetailResponseBodyConfigStatusEnum) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.ConfigStatus = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) UrlParameterDetail(v *DescribeCdnDomainDetailResponseUrlParameterDetail) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.UrlParameterDetail = v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) Commons(v string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.Commons = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableHttp2Parameter(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableHttp2Parameter = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableUrlParameter(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableUrlParameter = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) Ipv6OnOff(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.Ipv6OnOff = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) IpBlackList(v *DescribeCdnDomainDetailResponseIpBlackList) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.IpBlackList = v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableReferrerAntiStealingLink(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableReferrerAntiStealingLink = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) CacheDetails(v []DescribeCdnDomainDetailResponseCacheDetails) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.CacheDetails = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableHttp2Status(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableHttp2Status = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableIpBlackList(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableIpBlackList = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableSetRespHeader(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableSetRespHeader = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) FileList(v []string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.FileList = v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) RasLinkDetail(v *DescribeCdnDomainDetailResponseRasLinkDetail) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.RasLinkDetail = v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) CdnType(v DescribeCdnDomainDetailResponseBodyCdnTypeEnum) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.CdnType = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) CrtUniqueId(v string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.CrtUniqueId = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) DomainStatus(v DescribeCdnDomainDetailResponseBodyDomainStatusEnum) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.DomainStatus = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) UpdateTime(v string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.UpdateTime = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableCustomErrorPage(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableCustomErrorPage = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) RespHeaderDetails(v []DescribeCdnDomainDetailResponseRespHeaderDetails) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.RespHeaderDetails = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) DomainId(v int32) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.DomainId = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableBackSourceHost(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableBackSourceHost = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) AddType(v int32) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.AddType = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) Deleted(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.Deleted = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) CreateTime(v string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.CreateTime = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) DomainName(v string) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.DomainName = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableGzipStatus(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableGzipStatus = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) ReqHeaderDetails(v []DescribeCdnDomainDetailResponseReqHeaderDetails) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.ReqHeaderDetails = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) EnableCache(v bool) *DescribeCdnDomainDetailResponseBodyBuilder {
	b.s.EnableCache = &v
	return b
}

func (b *DescribeCdnDomainDetailResponseBodyBuilder) Build() *DescribeCdnDomainDetailResponseBody {
	return b.s
}
