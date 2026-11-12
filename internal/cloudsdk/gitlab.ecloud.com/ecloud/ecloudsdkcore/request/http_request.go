package request

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/consts"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/errs"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/param"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/position"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type HttpRequest struct {
	Method        string            `json:"method,omitempty"`
	Protocol      string            `json:"protocol,omitempty"`
	Url           string            `json:"url,omitempty"`
	Path          string            `json:"path,omitempty"`
	Action        string            `json:"action,omitempty"`
	Product       string            `json:"product,omitempty"`
	Version       string            `json:"version,omitempty"`
	SdkVersion    string            `json:"sdkVersion,omitempty"`
	Source        string            `json:"source,omitempty"`
	SecurityToken string            `json:"securityToken,omitempty"`
	QueryString   string            `json:"queryString,omitempty"`
	ContentType   string            `json:"contentType,omitempty"`
	Endpoint      string            `json:"endpoint,omitempty"`
	Body          interface{}       `json:"body,omitempty"`
	PathParams    map[string]string `json:"pathParams,omitempty"`
	QueryParams   map[string]string `json:"queryParams,omitempty"`
	HeaderParams  map[string]string `json:"headerParams,omitempty"`
}

type HttpRequestPosition string

const (
	BODY   HttpRequestPosition = "Body"
	QUERY  HttpRequestPosition = "Query"
	PATH   HttpRequestPosition = "Path"
	HEADER HttpRequestPosition = "Header"
)

func NewHttpRequest() *HttpRequest {
	return &HttpRequest{
		Method:       "POST",
		QueryParams:  make(map[string]string),
		HeaderParams: make(map[string]string),
		PathParams:   make(map[string]string),
		ContentType:  "application/json; charset=utf-8",
	}
}

func DefaultHttpRequest() *HttpRequest {
	return NewHttpRequest()
}

func (httpReq *HttpRequest) Reset() {
	httpReq.Action = ""
	httpReq.SecurityToken = ""
	httpReq.QueryParams = make(map[string]string)
	httpReq.HeaderParams = make(map[string]string)
	httpReq.PathParams = make(map[string]string)
	httpReq.Body = nil
	httpReq.Method = "POST"
}

func (httpReq *HttpRequest) BuildHeaderParams(headerParams map[string]string) {
	if httpReq.HeaderParams == nil {
		httpReq.HeaderParams = make(map[string]string)
	}
	for name, value := range headerParams {
		httpReq.HeaderParams[name] = value
	}
}

func (httpReq *HttpRequest) AddDefaultHeaders() error {
	if strings.HasSuffix(httpReq.Product, "inner") && utils.IsUnSet(httpReq.Source) {
		return errs.NewInvalidParameterError("the attribute named source in SDK config must not be null", nil)
	}
	if httpReq.HeaderParams == nil {
		httpReq.HeaderParams = make(map[string]string)
	}
	x := fmt.Sprintf("action:%s;product:%s;version:%s;sdkversion:%s;language:Golang;coreversion:1.0.5",
		httpReq.Action, httpReq.Product, httpReq.Version, httpReq.SdkVersion)
	if len(httpReq.Source) > 0 {
		x += fmt.Sprintf(";source:%s", httpReq.Source)
	}
	if len(httpReq.SecurityToken) > 0 {
		httpReq.HeaderParams["x-openapi-security-token"] = httpReq.SecurityToken
	}
	httpReq.HeaderParams["x-openapi-sdk"] = x
	httpReq.HeaderParams["User-Agent"] = "OpenAPI/2.0/Golang"
	return nil
}

func (httpReq *HttpRequest) BuildQueryParams(queryParams map[string]string) {
	if httpReq.QueryParams == nil {
		httpReq.QueryParams = make(map[string]string)
	}
	for name, value := range queryParams {
		httpReq.QueryParams[name] = value
	}
}

func (httpReq *HttpRequest) BuildPathParams(pathParams map[string]string) {
	if httpReq.PathParams == nil {
		httpReq.PathParams = make(map[string]string)
	}
	for name, value := range pathParams {
		httpReq.PathParams[name] = value
	}
}

func (httpReq *HttpRequest) BuildApiParams(params *param.Params, cm map[string]interface{}) error {
	if params == nil {
		return nil
	}

	if len(params.ContentType) > 0 {
		httpReq.ContentType = params.ContentType
	}
	if len(params.Method) > 0 {
		httpReq.Method = params.Method
	}
	if len(params.Protocol) > 0 {
		httpReq.Protocol = params.Protocol
	}
	ignoreGateway := cm["IgnoreGateway"].(*bool)
	if utils.BoolValue(ignoreGateway) {
		httpReq.Path = params.Uri
	} else {
		httpReq.Path = params.GatewayUri
	}
	if len(params.Action) > 0 {
		httpReq.Action = params.Action
	}
	if utils.IsSet(cm["Source"]) {
		httpReq.Source = *cm["Source"].(*string)
	}
	if httpReq.PathParams != nil && len(httpReq.PathParams) > 0 {
		httpReq.BuildPathParamsString()
	}
	if utils.IsSet(cm["RegionId"]) {
		httpReq.AddHeaders(map[string]string{"Region-Id": utils.StringValue(cm["RegionId"].(*string))})
	} else {
		if utils.IsSet(cm["PoolId"]) {
			httpReq.AddHeaders(map[string]string{"Pool-Id": utils.StringValue(cm["PoolId"].(*string))})
		}
	}

	if utils.IsSet(cm["SecurityToken"]) {
		httpReq.SecurityToken = utils.StringValue(cm["SecurityToken"].(*string))
	}
	err := httpReq.AddDefaultHeaders()
	if err != nil {
		return err
	}
	if utils.IsSet(cm["GlobalQueryParams"]) && len(cm["GlobalQueryParams"].(map[string]string)) > 0 {
		httpReq.BuildQueryParams(cm["GlobalQueryParams"].(map[string]string))
	}
	if utils.IsSet(cm["GlobalHeaderParams"]) && len(cm["GlobalHeaderParams"].(map[string]string)) > 0 {
		httpReq.BuildHeaderParams(cm["GlobalHeaderParams"].(map[string]string))
	}
	if utils.IsSet(cm["RuntimeHeaderParams"]) && len(cm["RuntimeHeaderParams"].(map[string]string)) > 0 {
		httpReq.BuildHeaderParams(cm["RuntimeHeaderParams"].(map[string]string))
	}
	if !utils.BoolValue(cm["CentralTransportEnabled"].(*bool)) && len(httpReq.Endpoint) > 0 {
		httpReq.Url = httpReq.Endpoint
	} else {
		httpReq.Url = utils.DefaultEndpoint
	}
	if utils.IsSet(cm["HttpProxy"]) {
		httpReq.Url = utils.StringValue(cm["HttpProxy"].(*string))
	}
	if utils.IsSet(cm["HttpsProxy"]) {
		httpReq.Url = utils.StringValue(cm["HttpsProxy"].(*string))
	}
	return nil
}

func (httpReq *HttpRequest) BuildFinalUrl() {
	url := httpReq.Url
	if len(url) == 0 {
		return
	}
	protocol := httpReq.Protocol
	if len(protocol) > 0 &&
		!strings.HasPrefix(url, "http") &&
		!strings.HasPrefix(url, "https") {
		url = protocol + "://" + url
	}
	path := httpReq.Path
	if len(path) > 0 {
		url = url + path
	}
	queryString := httpReq.QueryString
	if len(queryString) > 0 {
		url = url + queryString
	}
	httpReq.Url = url
}

func (httpReq *HttpRequest) BuildPathParamsString() {
	for name, value := range httpReq.PathParams {
		httpReq.Path = strings.ReplaceAll(httpReq.Path, fmt.Sprintf("{%s}", name), value)
	}
	if !strings.HasPrefix(httpReq.Path, "/") {
		httpReq.Path = "/" + httpReq.Path
	}
	if strings.HasSuffix(httpReq.Path, "/") {
		httpReq.Path = httpReq.Path[0 : len(httpReq.Path)-1]
	}
}

func (httpReq *HttpRequest) BuildQueryParamsString() {
	build := strings.Builder{}
	paramCount := len(httpReq.QueryParams)
	for name, value := range httpReq.QueryParams {
		build.WriteString(utils.PercentEncode(name))
		build.WriteString("=")
		build.WriteString(utils.PercentEncode(value))
		paramCount--
		if paramCount > 0 {
			build.WriteString("&")
		}
	}
	httpReq.QueryString = build.String()
}

func (httpReq *HttpRequest) ConvertRequest(request interface{}) {
	if request == nil {
		return
	}
	reqType := reflect.TypeOf(request)
	if reqType.Kind() == reflect.Ptr {
		reqType = reqType.Elem()
	}
	reqValue := reflect.ValueOf(request)
	if reqValue.Kind() == reflect.Ptr {
		reqValue = reqValue.Elem()
	}
	flag := false
	for i := 0; i < reqType.NumField(); i++ {
		fieldType := reqType.Field(i)
		value := reqValue.FieldByName(fieldType.Name)
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				continue
			}
			value = value.Elem()
		}
		propertyType := fieldType.Type
		if propertyType.Kind() == reflect.Ptr {
			propertyType = propertyType.Elem()
		}

		_, flag = propertyType.FieldByName(string(BODY))
		if flag {
			httpReq.Body = value.Interface()
			continue
		}
		_, flag = propertyType.FieldByName(string(HEADER))
		if flag {
			httpReq.BuildHeaderParams(utils.StructToMap(value.Interface(), ignorePositionField))
			continue
		}
		_, flag = propertyType.FieldByName(string(QUERY))
		if flag {
			httpReq.BuildQueryParams(utils.StructToMap(value.Interface(), ignorePositionField))
			continue
		}
		_, flag = propertyType.FieldByName(string(PATH))
		if flag {
			httpReq.BuildPathParams(utils.StructToMap(value.Interface(), ignorePositionField))
			continue
		}
	}
}

func (httpReq *HttpRequest) AddHeaders(headers map[string]string) {
	if httpReq.HeaderParams == nil {
		httpReq.HeaderParams = map[string]string{}
	}
	for name, value := range headers {
		httpReq.HeaderParams[name] = value
	}
}

var (
	_typeOfHeader = reflect.TypeOf(position.Header{})
	_typeOfBody   = reflect.TypeOf(position.Body{})
	_typeOfQuery  = reflect.TypeOf(position.Query{})
	_typeOfPath   = reflect.TypeOf(position.Path{})
)

func ignorePositionField(obj interface{}, field reflect.StructField, value reflect.Value) bool {
	if utils.ValueIsEmpty(value) {
		return true
	}
	typeOfValue := value.Type()
	return typeOfValue == _typeOfBody || typeOfValue == _typeOfQuery ||
		typeOfValue == _typeOfHeader || typeOfValue == _typeOfPath
}

func (httpReq *HttpRequest) ConvertQueryParamsFromPath() {
	if !strings.Contains(httpReq.Path, consts.QueryStartSymbol) {
		return
	}
	pathArray := strings.Split(httpReq.Path, consts.QueryStartSymbol)
	paramArray := strings.Split(pathArray[1], consts.ParameterSeparator)
	for _, param := range paramArray {
		if len(param) == 0 {
			continue
		}
		queryParamArray := strings.Split(param, consts.QuerySeparator)
		if len(queryParamArray) != 2 {
			continue
		}
		httpReq.QueryParams[queryParamArray[0]] = queryParamArray[1]
	}
	httpReq.Path = pathArray[0]
}
