package ecloudsdkcore

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/auth"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/auth/provider"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/config"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/errs"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/http"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/param"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/request"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/response"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/retry"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type APIClient struct {
	Config      *config.Config
	HttpClient  *http.NetHttpClient
	HttpRequest *request.HttpRequest
}

func NewAPIClient() *APIClient {
	return &APIClient{HttpClient: http.NewHttpClient()}
}

func NewCustomizedAPIClient(config *config.Config, httpRequest *request.HttpRequest) *APIClient {
	return &APIClient{
		Config:      config,
		HttpClient:  http.NewHttpClient(),
		HttpRequest: httpRequest,
	}
}

func DefaultApiClient(config *config.Config, httpRequest *request.HttpRequest) *APIClient {
	return NewCustomizedAPIClient(config, httpRequest)
}

func InitConfig(c *config.Config) {
	if utils.IsUnSet(c.AutoRetry) {
		autoRetry := false
		c.AutoRetry = &autoRetry
	}
	if utils.IsUnSet(c.IgnoreSSL) {
		ignoreSSL := true
		c.IgnoreSSL = &ignoreSSL
	}
	if utils.IsUnSet(c.IgnoreGateway) {
		ignoreGateway := false
		c.IgnoreGateway = &ignoreGateway
	}
	if utils.IsUnSet(c.CentralTransportEnabled) {
		centralTransportEnabled := true
		c.CentralTransportEnabled = &centralTransportEnabled
	}
}

func (c *APIClient) Excute(params *param.Params, rc *config.RuntimeConfig,
	returnType interface{},
) (*response.HttpResponse, error) {
	httpReq := c.HttpRequest
	httpReq = new(request.HttpRequest)
	if err := utils.DeepCopy(httpReq, c.HttpRequest); err != nil {
		errs.NewServerRequestError("copy object error", nil)
	}
	if rc == nil {
		rc = &config.RuntimeConfig{}
	}
	c.resetHttpRequest(httpReq)
	if rc == nil {
		rc = &config.RuntimeConfig{}
	}
	cm := utils.Merge(c.Config, rc)
	if err := c.buildHttpRequest(params, cm, httpReq); err != nil {
		return nil, err
	}
	httpReq.BuildFinalUrl()
	if utils.BoolValue(cm["AutoRetry"].(*bool)) {
		retryTemplate := c.buildRetryTemplate(cm)
		res, err := retryTemplate.Call(func() (interface{}, error) {
			return c.HttpClient.Execute(httpReq, cm, returnType)
		})
		if err != nil {
			return nil, err
		}
		httpResponse, _ := res.(*response.HttpResponse)
		return httpResponse, nil
	}
	return c.HttpClient.Execute(httpReq, cm, returnType)
}

func (c *APIClient) buildRetryTemplate(cm map[string]interface{}) *retry.Template {
	templateBuilder := retry.NewBuilder()
	if utils.IsSet(cm["MaxRetryTimes"]) {
		templateBuilder.SetRetryTimes(utils.Int32Value(cm["MaxRetryTimes"].(*int32)))
	}
	if utils.IsSet(cm["RetryPeriod"]) {
		templateBuilder.SetRetryPolicy(retry.WaitRetryPolicy(utils.Int64Value(cm["RetryPeriod"].(*int64))))
	} else {
		templateBuilder.SetRetryPolicy(retry.NoWait())
	}
	if utils.IsSet(cm["MaxDuringTime"]) {
		templateBuilder.SetMaxDuringTime(utils.Int64Value(cm["MaxDuringTime"].(*int64)))
	}
	return templateBuilder.Build()
}

func (c *APIClient) buildHttpRequest(params *param.Params, cm map[string]interface{}, httpReq *request.HttpRequest) error {
	httpReq.ConvertRequest(params.Request)
	err := httpReq.BuildApiParams(params, cm)
	if err != nil {
		return err
	}
	if utils.IsUnSet(cm["Provider"]) {
		if utils.IsUnSet(c.Config.AccessKey) || utils.IsUnSet(c.Config.SecretKey) {
			return errs.NewServerRequestError("accessKey or secretKey can not be null", nil)
		}
		credential := auth.NewCredentialBuilder().AccessKey(*c.Config.AccessKey).SecretKey(*c.Config.SecretKey).Build()
		err := auth.GetCredentialManager(*credential.CredentialType).Sign(httpReq, credential)
		if err != nil {
			return err
		}
		return nil

	}
	p := cm["Provider"].(provider.ICredentialProvider)
	credential, err := p.GetCredential()
	if err != nil {
		return err
	}
	if utils.IsSet(c.Config.Provider) && p != c.Config.Provider {
		otherCredential, err := c.Config.Provider.GetCredential()
		if err != nil {
			return err
		}
		credential.CopyValues(otherCredential)
	}
	if utils.IsUnSet(credential.AccessKey) || utils.IsUnSet(credential.SecretKey) {
		credential.AccessKey = c.Config.AccessKey
		credential.SecretKey = c.Config.SecretKey
	}
	err = auth.GetCredentialManager(*credential.CredentialType).Sign(httpReq, credential)
	if err != nil {
		return err
	}
	return nil
}

func (c *APIClient) resetHttpRequest(req *request.HttpRequest) {
	req.Reset()
}
