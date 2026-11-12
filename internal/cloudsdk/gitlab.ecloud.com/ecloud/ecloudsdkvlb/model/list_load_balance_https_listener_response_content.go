package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type ListLoadBalanceHTTPSListenerResponseContentProtocolEnum string

const (
	ListLoadBalanceHTTPSListenerResponseContentProtocolEnumHttp            ListLoadBalanceHTTPSListenerResponseContentProtocolEnum = "HTTP"
	ListLoadBalanceHTTPSListenerResponseContentProtocolEnumHttps           ListLoadBalanceHTTPSListenerResponseContentProtocolEnum = "HTTPS"
	ListLoadBalanceHTTPSListenerResponseContentProtocolEnumTcp             ListLoadBalanceHTTPSListenerResponseContentProtocolEnum = "TCP"
	ListLoadBalanceHTTPSListenerResponseContentProtocolEnumUdp             ListLoadBalanceHTTPSListenerResponseContentProtocolEnum = "UDP"
	ListLoadBalanceHTTPSListenerResponseContentProtocolEnumTerminatedHttps ListLoadBalanceHTTPSListenerResponseContentProtocolEnum = "TERMINATED_HTTPS"
	ListLoadBalanceHTTPSListenerResponseContentProtocolEnumSip             ListLoadBalanceHTTPSListenerResponseContentProtocolEnum = "SIP"
)

type ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum string

const (
	ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnumHttp   ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum = "HTTP"
	ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnumHttps  ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum = "HTTPS"
	ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnumTcp    ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum = "TCP"
	ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnumPing   ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum = "PING"
	ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnumUdp    ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum = "UDP"
	ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnumHttp10 ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum = "HTTP10"
	ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnumHttp11 ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum = "HTTP11"
)

type ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnum string

const (
	ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnumNone         ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnum = "NONE"
	ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnumAppCookie    ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnum = "APP_COOKIE"
	ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnumHttpCookie   ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnum = "HTTP_COOKIE"
	ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnumSourceIp     ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnum = "SOURCE_IP"
	ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnumSourceIpPort ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnum = "SOURCE_IP_PORT"
	ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnumCallId       ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnum = "CALL_ID"
)

type ListLoadBalanceHTTPSListenerResponseContentOpStatusEnum string

const (
	ListLoadBalanceHTTPSListenerResponseContentOpStatusEnumInactive ListLoadBalanceHTTPSListenerResponseContentOpStatusEnum = "INACTIVE"
	ListLoadBalanceHTTPSListenerResponseContentOpStatusEnumActive   ListLoadBalanceHTTPSListenerResponseContentOpStatusEnum = "ACTIVE"
	ListLoadBalanceHTTPSListenerResponseContentOpStatusEnumCreating ListLoadBalanceHTTPSListenerResponseContentOpStatusEnum = "CREATING"
	ListLoadBalanceHTTPSListenerResponseContentOpStatusEnumDeleting ListLoadBalanceHTTPSListenerResponseContentOpStatusEnum = "DELETING"
	ListLoadBalanceHTTPSListenerResponseContentOpStatusEnumUpdating ListLoadBalanceHTTPSListenerResponseContentOpStatusEnum = "UPDATING"
	ListLoadBalanceHTTPSListenerResponseContentOpStatusEnumError    ListLoadBalanceHTTPSListenerResponseContentOpStatusEnum = "ERROR"
)

type ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum string

const (
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumActive        ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "ACTIVE"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumBuild         ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "BUILD"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumDown          ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "DOWN"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumError         ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "ERROR"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumPendingCreate ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "PENDING_CREATE"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumPendingUpdate ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "PENDING_UPDATE"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumPendingDelete ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "PENDING_DELETE"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumMigrating     ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "MIGRATING"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumDeleted       ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "DELETED"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumUpgrading     ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "UPGRADING"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumPartDeleted   ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "PART_DELETED"
	ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnumUnrecognized  ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum = "UNRECOGNIZED"
)

type ListLoadBalanceHTTPSListenerResponseContentLbAlgorithmEnum string

const (
	ListLoadBalanceHTTPSListenerResponseContentLbAlgorithmEnumRoundRobin       ListLoadBalanceHTTPSListenerResponseContentLbAlgorithmEnum = "ROUND_ROBIN"
	ListLoadBalanceHTTPSListenerResponseContentLbAlgorithmEnumLeastConnections ListLoadBalanceHTTPSListenerResponseContentLbAlgorithmEnum = "LEAST_CONNECTIONS"
	ListLoadBalanceHTTPSListenerResponseContentLbAlgorithmEnumSourceIp         ListLoadBalanceHTTPSListenerResponseContentLbAlgorithmEnum = "SOURCE_IP"
)

type ListLoadBalanceHTTPSListenerResponseContent struct {
	HealthDelay *int32 `json:"healthDelay,omitempty"`

	ModifiedTime *string `json:"modifiedTime,omitempty"`

	Notes *string `json:"notes,omitempty"`

	SniContainerIds *string `json:"sniContainerIds,omitempty"`

	IsMultiAz *bool `json:"isMultiAz,omitempty"`

	RedirectToListenerName *string `json:"redirectToListenerName,omitempty"`

	Protocol *ListLoadBalanceHTTPSListenerResponseContentProtocolEnum `json:"protocol,omitempty"`

	SniContainerIdList []string `json:"sniContainerIdList,omitempty"`

	CreatedTime *string `json:"createdTime,omitempty"`

	Id *string `json:"id,omitempty"`

	MutualAuthenticationUp *bool `json:"mutualAuthenticationUp,omitempty"`

	ResponseTimeout *int32 `json:"responseTimeout,omitempty"`

	PoolName *string `json:"poolName,omitempty"`

	SniUp *bool `json:"sniUp,omitempty"`

	KeepAliveTimeout *int32 `json:"keepAliveTimeout,omitempty"`

	HealthHttpMethod *string `json:"healthHttpMethod,omitempty"`

	HealthType *ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum `json:"healthType,omitempty"`

	LoadBalanceFlavor *int32 `json:"loadBalanceFlavor,omitempty"`

	LoadBalanceId *string `json:"loadBalanceId,omitempty"`

	ProtocolPort *int32 `json:"protocolPort,omitempty"`

	HealthExpectedCode *string `json:"healthExpectedCode,omitempty"`

	GroupName *string `json:"groupName,omitempty"`

	TlsCustomizeProtocolName *string `json:"tlsCustomizeProtocolName,omitempty"`

	Name *string `json:"name,omitempty"`

	PoolId *string `json:"poolId,omitempty"`

	SessionPersistence *ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnum `json:"sessionPersistence,omitempty"`

	HealthUrlPath *string `json:"healthUrlPath,omitempty"`

	HealthAdminStateUp *bool `json:"healthAdminStateUp,omitempty"`

	OpStatus *ListLoadBalanceHTTPSListenerResponseContentOpStatusEnum `json:"opStatus,omitempty"`

	HealthTimeout *int32 `json:"healthTimeout,omitempty"`

	MultiAzUuid *string `json:"multiAzUuid,omitempty"`

	HostHeadMode *string `json:"hostHeadMode,omitempty"`

	GroupType *string `json:"groupType,omitempty"`

	RedirectToListenerId *string `json:"redirectToListenerId,omitempty"`

	Description *string `json:"description,omitempty"`

	Transparent *bool `json:"transparent,omitempty"`

	MaxConcurrency *int32 `json:"maxConcurrency,omitempty"`

	NewConnection *int32 `json:"newConnection,omitempty"`

	ProvisioningStatus *ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum `json:"provisioningStatus,omitempty"`

	Http2 *bool `json:"http2,omitempty"`

	AdminStateUp *bool `json:"adminStateUp,omitempty"`

	DefaultTlsContainerId *string `json:"defaultTlsContainerId,omitempty"`

	CookieName *string `json:"cookieName,omitempty"`

	RequestTimeout *int32 `json:"requestTimeout,omitempty"`

	TlsCustomizeProtocolId *int64 `json:"tlsCustomizeProtocolId,omitempty"`

	LbAlgorithm *ListLoadBalanceHTTPSListenerResponseContentLbAlgorithmEnum `json:"lbAlgorithm,omitempty"`

	TlsUp *bool `json:"tlsUp,omitempty"`

	ProxyProtocol *bool `json:"proxyProtocol,omitempty"`

	HealthId *string `json:"healthId,omitempty"`

	DataCompression *bool `json:"dataCompression,omitempty"`

	ConnectionLimit *int32 `json:"connectionLimit,omitempty"`

	Deleted *bool `json:"deleted,omitempty"`

	HealthMaxRetries *int32 `json:"healthMaxRetries,omitempty"`

	GroupEnabled *bool `json:"groupEnabled,omitempty"`

	CaContainerId *string `json:"caContainerId,omitempty"`

	ControlGroupId *string `json:"controlGroupId,omitempty"`
}

func (s ListLoadBalanceHTTPSListenerResponseContent) String() string {
	return utils.Beautify(s)
}

func (s ListLoadBalanceHTTPSListenerResponseContent) GoString() string {
	return s.String()
}

func (s ListLoadBalanceHTTPSListenerResponseContent) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHealthDelay(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.HealthDelay = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetModifiedTime(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.ModifiedTime = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetNotes(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.Notes = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetSniContainerIds(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.SniContainerIds = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetIsMultiAz(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.IsMultiAz = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetRedirectToListenerName(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.RedirectToListenerName = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetProtocol(v ListLoadBalanceHTTPSListenerResponseContentProtocolEnum) *ListLoadBalanceHTTPSListenerResponseContent {
	s.Protocol = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetSniContainerIdList(v []string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.SniContainerIdList = v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetCreatedTime(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.CreatedTime = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetId(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.Id = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetMutualAuthenticationUp(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.MutualAuthenticationUp = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetResponseTimeout(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.ResponseTimeout = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetPoolName(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.PoolName = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetSniUp(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.SniUp = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetKeepAliveTimeout(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.KeepAliveTimeout = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHealthHttpMethod(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.HealthHttpMethod = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHealthType(v ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum) *ListLoadBalanceHTTPSListenerResponseContent {
	s.HealthType = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetLoadBalanceFlavor(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.LoadBalanceFlavor = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetLoadBalanceId(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.LoadBalanceId = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetProtocolPort(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.ProtocolPort = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHealthExpectedCode(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.HealthExpectedCode = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetGroupName(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.GroupName = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetTlsCustomizeProtocolName(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.TlsCustomizeProtocolName = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetName(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.Name = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetPoolId(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.PoolId = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetSessionPersistence(v ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnum) *ListLoadBalanceHTTPSListenerResponseContent {
	s.SessionPersistence = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHealthUrlPath(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.HealthUrlPath = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHealthAdminStateUp(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.HealthAdminStateUp = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetOpStatus(v ListLoadBalanceHTTPSListenerResponseContentOpStatusEnum) *ListLoadBalanceHTTPSListenerResponseContent {
	s.OpStatus = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHealthTimeout(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.HealthTimeout = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetMultiAzUuid(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.MultiAzUuid = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHostHeadMode(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.HostHeadMode = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetGroupType(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.GroupType = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetRedirectToListenerId(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.RedirectToListenerId = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetDescription(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.Description = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetTransparent(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.Transparent = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetMaxConcurrency(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.MaxConcurrency = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetNewConnection(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.NewConnection = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetProvisioningStatus(v ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum) *ListLoadBalanceHTTPSListenerResponseContent {
	s.ProvisioningStatus = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHttp2(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.Http2 = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetAdminStateUp(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.AdminStateUp = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetDefaultTlsContainerId(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.DefaultTlsContainerId = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetCookieName(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.CookieName = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetRequestTimeout(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.RequestTimeout = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetTlsCustomizeProtocolId(v int64) *ListLoadBalanceHTTPSListenerResponseContent {
	s.TlsCustomizeProtocolId = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetLbAlgorithm(v ListLoadBalanceHTTPSListenerResponseContentLbAlgorithmEnum) *ListLoadBalanceHTTPSListenerResponseContent {
	s.LbAlgorithm = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetTlsUp(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.TlsUp = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetProxyProtocol(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.ProxyProtocol = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHealthId(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.HealthId = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetDataCompression(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.DataCompression = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetConnectionLimit(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.ConnectionLimit = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetDeleted(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.Deleted = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetHealthMaxRetries(v int32) *ListLoadBalanceHTTPSListenerResponseContent {
	s.HealthMaxRetries = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetGroupEnabled(v bool) *ListLoadBalanceHTTPSListenerResponseContent {
	s.GroupEnabled = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetCaContainerId(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.CaContainerId = &v
	return s
}

func (s *ListLoadBalanceHTTPSListenerResponseContent) SetControlGroupId(v string) *ListLoadBalanceHTTPSListenerResponseContent {
	s.ControlGroupId = &v
	return s
}

type ListLoadBalanceHTTPSListenerResponseContentBuilder struct {
	s *ListLoadBalanceHTTPSListenerResponseContent
}

func NewListLoadBalanceHTTPSListenerResponseContentBuilder() *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	s := &ListLoadBalanceHTTPSListenerResponseContent{}
	b := &ListLoadBalanceHTTPSListenerResponseContentBuilder{s: s}
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) HealthDelay(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.HealthDelay = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) ModifiedTime(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.ModifiedTime = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) Notes(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.Notes = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) SniContainerIds(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.SniContainerIds = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) IsMultiAz(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.IsMultiAz = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) RedirectToListenerName(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.RedirectToListenerName = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) Protocol(v ListLoadBalanceHTTPSListenerResponseContentProtocolEnum) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.Protocol = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) SniContainerIdList(v []string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.SniContainerIdList = v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) CreatedTime(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.CreatedTime = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) Id(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.Id = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) MutualAuthenticationUp(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.MutualAuthenticationUp = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) ResponseTimeout(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.ResponseTimeout = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) PoolName(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.PoolName = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) SniUp(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.SniUp = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) KeepAliveTimeout(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.KeepAliveTimeout = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) HealthHttpMethod(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.HealthHttpMethod = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) HealthType(v ListLoadBalanceHTTPSListenerResponseContentHealthTypeEnum) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.HealthType = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) LoadBalanceFlavor(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.LoadBalanceFlavor = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) LoadBalanceId(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.LoadBalanceId = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) ProtocolPort(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.ProtocolPort = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) HealthExpectedCode(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.HealthExpectedCode = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) GroupName(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.GroupName = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) TlsCustomizeProtocolName(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.TlsCustomizeProtocolName = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) Name(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.Name = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) PoolId(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.PoolId = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) SessionPersistence(v ListLoadBalanceHTTPSListenerResponseContentSessionPersistenceEnum) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.SessionPersistence = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) HealthUrlPath(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.HealthUrlPath = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) HealthAdminStateUp(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.HealthAdminStateUp = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) OpStatus(v ListLoadBalanceHTTPSListenerResponseContentOpStatusEnum) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.OpStatus = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) HealthTimeout(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.HealthTimeout = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) MultiAzUuid(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.MultiAzUuid = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) HostHeadMode(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.HostHeadMode = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) GroupType(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.GroupType = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) RedirectToListenerId(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.RedirectToListenerId = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) Description(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.Description = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) Transparent(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.Transparent = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) MaxConcurrency(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.MaxConcurrency = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) NewConnection(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.NewConnection = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) ProvisioningStatus(v ListLoadBalanceHTTPSListenerResponseContentProvisioningStatusEnum) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.ProvisioningStatus = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) Http2(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.Http2 = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) AdminStateUp(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.AdminStateUp = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) DefaultTlsContainerId(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.DefaultTlsContainerId = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) CookieName(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.CookieName = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) RequestTimeout(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.RequestTimeout = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) TlsCustomizeProtocolId(v int64) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.TlsCustomizeProtocolId = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) LbAlgorithm(v ListLoadBalanceHTTPSListenerResponseContentLbAlgorithmEnum) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.LbAlgorithm = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) TlsUp(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.TlsUp = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) ProxyProtocol(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.ProxyProtocol = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) HealthId(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.HealthId = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) DataCompression(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.DataCompression = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) ConnectionLimit(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.ConnectionLimit = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) Deleted(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.Deleted = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) HealthMaxRetries(v int32) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.HealthMaxRetries = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) GroupEnabled(v bool) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.GroupEnabled = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) CaContainerId(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.CaContainerId = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) ControlGroupId(v string) *ListLoadBalanceHTTPSListenerResponseContentBuilder {
	b.s.ControlGroupId = &v
	return b
}

func (b *ListLoadBalanceHTTPSListenerResponseContentBuilder) Build() *ListLoadBalanceHTTPSListenerResponseContent {
	return b.s
}
