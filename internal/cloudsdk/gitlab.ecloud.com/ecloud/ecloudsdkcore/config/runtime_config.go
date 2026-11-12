package config

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/auth/provider"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type RuntimeConfig struct {
	ReadTimeout             *int32                       `json:"readTimeout"`
	ConnectTimeout          *int32                       `json:"connectTimeout"`
	AutoRetry               *bool                        `json:"autoRetry"`
	IgnoreSSL               *bool                        `json:"ignoreSSL"`
	CertFile                *string                      `json:"certFile"`
	ClientCertFile          *string                      `json:"clientCertFile"`
	ClientKeyFile           *string                      `json:"clientKeyFile"`
	MaxRetryTimes           *int32                       `json:"maxRetryTimes"`
	RetryPeriod             *int64                       `json:"retryPeriod"`
	MaxDuringTime           *int64                       `json:"maxDuringTime"`
	HttpProxy               *string                      `json:"httpProxy"`
	HttpsProxy              *string                      `json:"httpsProxy"`
	IgnoreGateway           *bool                        `json:"ignoreGateway"`
	CentralTransportEnabled *bool                        `json:"centralTransportEnabled"`
	RuntimeHeaderParams     map[string]string            `json:"runtimeHeaderParams"`
	Provider                provider.ICredentialProvider `json:"provider"`
}

func (r *RuntimeConfig) String() string {
	return utils.Beautify(r)
}

func (r *RuntimeConfig) GoString() string {
	return r.String()
}

func (r *RuntimeConfig) ToJsonString() string {
	return utils.ToJsonString(r)
}

type RuntimeConfigBuilder struct {
	runtimeConfig *RuntimeConfig
}

func NewRuntimeConfigBuilder() *RuntimeConfigBuilder {
	runtimeConfig := &RuntimeConfig{}
	r := &RuntimeConfigBuilder{runtimeConfig: runtimeConfig}
	return r
}

func (r *RuntimeConfigBuilder) ReadTimeOut(readTimeOut int32) *RuntimeConfigBuilder {
	r.runtimeConfig.ReadTimeout = utils.Int32(readTimeOut)
	return r
}

func (r *RuntimeConfigBuilder) ConnectTimeout(connectTimeout int32) *RuntimeConfigBuilder {
	r.runtimeConfig.ConnectTimeout = utils.Int32(connectTimeout)
	return r
}

func (r *RuntimeConfigBuilder) AutoRetry(autoRetry bool) *RuntimeConfigBuilder {
	r.runtimeConfig.AutoRetry = utils.Bool(autoRetry)
	return r
}

func (r *RuntimeConfigBuilder) IgnoreSSL(ignoreSSL bool) *RuntimeConfigBuilder {
	r.runtimeConfig.IgnoreSSL = utils.Bool(ignoreSSL)
	return r
}

func (r *RuntimeConfigBuilder) CertFile(certFile string) *RuntimeConfigBuilder {
	r.runtimeConfig.CertFile = utils.String(certFile)
	return r
}

func (r *RuntimeConfigBuilder) ClientCertFile(clientCertFile string) *RuntimeConfigBuilder {
	r.runtimeConfig.ClientCertFile = utils.String(clientCertFile)
	return r
}

func (r *RuntimeConfigBuilder) ClientKeyFile(clientKeyFile string) *RuntimeConfigBuilder {
	r.runtimeConfig.ClientKeyFile = utils.String(clientKeyFile)
	return r
}

func (r *RuntimeConfigBuilder) MaxRetryTimes(maxRetryTimes int32) *RuntimeConfigBuilder {
	r.runtimeConfig.MaxRetryTimes = utils.Int32(maxRetryTimes)
	return r
}

func (r *RuntimeConfigBuilder) RetryPeriod(retryPeriod int64) *RuntimeConfigBuilder {
	r.runtimeConfig.RetryPeriod = utils.Int64(retryPeriod)
	return r
}

func (r *RuntimeConfigBuilder) MaxDuringTime(maxDuringTime int64) *RuntimeConfigBuilder {
	r.runtimeConfig.MaxDuringTime = utils.Int64(maxDuringTime)
	return r
}

func (r *RuntimeConfigBuilder) HttpProxy(httpProxy string) *RuntimeConfigBuilder {
	r.runtimeConfig.HttpProxy = utils.String(httpProxy)
	return r
}

func (r *RuntimeConfigBuilder) HttpsProxy(httpsProxy string) *RuntimeConfigBuilder {
	r.runtimeConfig.HttpsProxy = utils.String(httpsProxy)
	return r
}

func (r *RuntimeConfigBuilder) IgnoreGateway(ignoreGateway bool) *RuntimeConfigBuilder {
	r.runtimeConfig.IgnoreGateway = utils.Bool(ignoreGateway)
	return r
}

func (r *RuntimeConfigBuilder) CentralTransportEnabled(centralTransportEnabled bool) *RuntimeConfigBuilder {
	r.runtimeConfig.CentralTransportEnabled = utils.Bool(centralTransportEnabled)
	return r
}

func (r *RuntimeConfigBuilder) RuntimeHeaderParams(runtimeHeaderParams map[string]string) *RuntimeConfigBuilder {
	r.runtimeConfig.RuntimeHeaderParams = runtimeHeaderParams
	return r
}

func (r *RuntimeConfigBuilder) Provider(provider provider.ICredentialProvider) *RuntimeConfigBuilder {
	r.runtimeConfig.Provider = provider
	return r
}

func (r *RuntimeConfigBuilder) Build() *RuntimeConfig {
	return r.runtimeConfig
}
