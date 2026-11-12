package http

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/auth"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/auth/provider"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/errs"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/request"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/response"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type NetHttpClient struct{}

type RetryFunc func(request *request.HttpRequest,
	returnType *interface{}) (*response.HttpResponse, error)

func NewHttpClient() *NetHttpClient {
	return &NetHttpClient{}
}

var clientPool = &sync.Map{}

func (hc *NetHttpClient) getClient(request *request.HttpRequest, cm map[string]interface{}) (*http.Client, error) {
	var host string

	if cm["ClientProxyHost"] != nil && cm["ClientProxyPort"] != nil {
		host = fmt.Sprintf("%s:%s", *cm["ClientProxyHost"].(*string), *cm["ClientProxyPort"].(*string))
	} else {
		url, err := url.Parse(request.Url)
		if err != nil {
			return nil, errs.NewInvalidParameterError(fmt.Sprintf("request url invalid, url=: %s", request.Url), err)
		}
		host = url.Host
	}
	client, ok := clientPool.Load(host)
	if client == nil && !ok {
		var err error
		client, err = hc.buildHttpClient(cm)
		if err != nil {
			return nil, err
		}
		clientPool.Store(host, client)
	}
	return client.(*http.Client), nil
}

func (hc *NetHttpClient) Execute(hr *request.HttpRequest, cm map[string]interface{}, rt interface{}) (*response.HttpResponse, error) {
	req, err := hc.buildRequest(hr, cm)
	if err != nil {
		return nil, err
	}
	client, err := hc.getClient(hr, cm)
	if err != nil {
		return nil, errs.NewServerRequestError("get http error: ", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errs.NewServerRequestError(err.Error(), err)
	}
	if resp == nil {
		return nil, errs.NewServerResponseError("response is nil", nil, -1, nil, "")
	}
	if err = handleResponse(resp, cm, rt); err != nil {
		return nil, err
	}
	return &response.HttpResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Data:       resp,
	}, nil
}

func (hc *NetHttpClient) buildHttpClient(cm map[string]interface{}) (*http.Client, error) {
	hcb := NewHttpClientBuilder()
	if utils.IsSet(cm["ReadTimeout"]) {
		hcb.SetReadTimeout(utils.Int32Value(cm["ReadTimeout"].(*int32)))
	}

	if utils.IsSet(cm["ConnectTimeout"]) {
		hcb.SetConnectTimeout(utils.Int32Value(cm["ConnectTimeout"].(*int32)))
	}

	if utils.IsSet(cm["ClientProxyHost"]) {
		proxy := Proxy{
			Protocol: cm["ClientProxyProtocol"].(*string),
			Host:     cm["ClientProxyHost"].(*string),
			Port:     cm["ClientProxyPort"].(*int32),
			Username: cm["ClientProxyUsername"].(*string),
			Password: cm["ClientProxyPassword"].(*string),
		}
		hcb.SetClientProxy(proxy)
	}
	ignore := true
	if utils.IsSet(cm["IgnoreSSL"]) {
		ignore = utils.BoolValue(cm["IgnoreSSL"].(*bool))
	}
	certFile := ""
	if utils.IsSet(cm["CertFile"]) {
		certFile = utils.StringValue(cm["CertFile"].(*string))
	}
	clientCertFile := ""
	if utils.IsSet(cm["ClientCertFile"]) {
		clientCertFile = utils.StringValue(cm["ClientCertFile"].(*string))
	}
	clientKeyFile := ""
	if utils.IsSet(cm["ClientKeyFile"]) {
		clientKeyFile = utils.StringValue(cm["ClientKeyFile"].(*string))
	}
	err := hcb.ApplySSLSettings(ignore, certFile, clientCertFile, clientKeyFile)
	if err != nil {
		return nil, err
	}
	return hcb.Build(), nil
}

func handleResponse(resp *http.Response, cm map[string]interface{}, returnType interface{}) error {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		return errs.NewServerResponseError(fmt.Sprintf("response status is: %s,status code is:%d",
			resp.Status, resp.StatusCode), err, resp.StatusCode, resp.Header, "can not read response body")
	}

	if isSuccessful(resp.StatusCode) {
		if returnType == nil || resp.StatusCode == 204 {
			if resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil
		}

		err = deserialize(returnType, body, cm, resp.Header.Get("Content-Type"))
		if err != nil {
			return errs.NewServerResponseError(fmt.Sprintf("can't deserialize response body with content-type: %s,response body:%s",
				resp.Header.Get("Content-Type"), string(body)), err, resp.StatusCode, resp.Header, string(body))
		}
	} else {
		if body == nil {
			return errs.NewServerResponseError(fmt.Sprintf("response status is: %s,status code is:%d",
				resp.Status, resp.StatusCode), nil, resp.StatusCode, resp.Header, "response body is nil")
		}
		respBody := string(body)
		return errs.NewServerResponseError(fmt.Sprintf("response status: %s,http status code:%d,response body:%s",
			resp.Status, resp.StatusCode, respBody), nil, resp.StatusCode, resp.Header, respBody)
	}
	return nil
}

func isSuccessful(code int) bool {
	return code >= 200 && code < 300
}

func (hc *NetHttpClient) buildRequest(hr *request.HttpRequest, cm map[string]interface{}) (request *http.Request, err error) {
	req, err := prepareRequest(hr, cm)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func prepareRequest(hr *request.HttpRequest, cm map[string]interface{}) (req *http.Request, err error) {
	var body *bytes.Buffer
	rawBody := hr.Body
	contentType := hr.ContentType

	if rawBody != nil {
		if utils.IsSet(cm["Provider"]) {
			provider := cm["Provider"].(provider.ICredentialProvider)
			credential, err := provider.GetCredential()
			if err != nil {
				return nil, err
			}
			if *credential.CredentialType == auth.CredentialMop && *credential.EncryptionType == auth.EncrytMopRsa {
				bodyBuf := &bytes.Buffer{}
				err := json.NewEncoder(bodyBuf).Encode(rawBody)
				if err != nil {
					return nil, err
				}
				encryptContent, err := auth.GetCredentialManager(*credential.CredentialType).Encrypt(bodyBuf.Bytes(), *credential.PublicKey)
				if err != nil {
					return nil, err
				}
				rawBody = encryptContent
			}
		}
		contentType = utils.DetectContentType(rawBody)
		body, err = utils.SetBody(rawBody, contentType)
		if err != nil {
			return nil, errs.NewServerRequestError(fmt.Sprintf("set request body error: %s", err.Error()), err)
		}
	}

	rawUrl := hr.Url
	if len(rawUrl) == 0 {
		return nil, errs.NewServerRequestError("request url is empty", nil)
	}
	realUrl, err := url.Parse(rawUrl)
	if err != nil {
		return nil, errs.NewServerRequestError(fmt.Sprintf("can't parse request url: %s, "+
			"error is: %s", rawUrl, err.Error()), err)
	}

	method := hr.Method
	if body != nil {
		req, err = http.NewRequest(method, realUrl.String(), body)
	} else {
		req, err = http.NewRequest(method, realUrl.String(), nil)
	}
	if err != nil {
		return nil, errs.NewServerRequestError(fmt.Sprintf("can't create http request, "+
			"method=%s, url=%s, error: %s", method, realUrl.String(), err.Error()), err)
	}

	headers := hr.HeaderParams
	for name, value := range headers {
		if name == "Host" {
			req.Host = value
			continue
		}
		req.Header.Add(name, value)
	}
	if len(contentType) == 0 {
		contentType = "application/json; charset=utf-8"
	}
	req.Header.Add("Content-Type", contentType)
	return req, nil
}

func deserialize(respType interface{}, respBody []byte, configMap map[string]interface{}, contentType string) (err error) {
	if respBody == nil {
		return errs.NewServerResponseError("response body is nil", nil, -1, nil, "")
	}
	if utils.IsSet(configMap["Provider"]) {
		provider := configMap["Provider"].(provider.ICredentialProvider)
		credential, err := provider.GetCredential()
		if err != nil {
			return err
		}
		if *credential.CredentialType == auth.CredentialMop && *credential.EncryptionType == auth.EncrytMopRsa {
			decryptContent, err := auth.GetCredentialManager(*credential.CredentialType).Decrypt(string(respBody), *credential.PrivateKey)
			if err != nil {
				return err
			}
			respBody = utils.StringToBytes(decryptContent)
		}
	}
	if strings.Contains(contentType, "application/xml") {
		if err = xml.Unmarshal(respBody, respType); err != nil {
			return errs.NewServerResponseError(fmt.Sprintf("can't deserialize response body with: %s",
				string(respBody)), err, -1, nil, string(respBody))
		}
		return nil
	} else if strings.Contains(contentType, "json") {
		if err = json.Unmarshal(respBody, respType); err != nil {
			return errs.NewServerResponseError(fmt.Sprintf("can't deserialize response body with: %s",
				string(respBody)), err, -1, nil, string(respBody))
		}
		return nil
	}
	return errors.New("undefined response type")
}
