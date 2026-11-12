package param

import "github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"

type Params struct {
	Action      string      `json:"action,omitempty"`
	Protocol    string      `json:"protocol,omitempty"`
	Uri         string      `json:"uri,omitempty"`
	GatewayUri  string      `json:"gatewayUri,omitempty"`
	Request     interface{} `json:"request,omitempty"`
	Method      string      `json:"method,omitempty"`
	AuthType    int32       `json:"authType,omitempty"`
	ContentType string      `json:"contentType,omitempty"`
}

func (p *Params) String() string {
	return utils.Beautify(p)
}

func (p *Params) GoString() string {
	return p.String()
}

func (p *Params) ToJsonString() string {
	return utils.ToJsonString(p)
}

type Builder struct {
	params *Params
}

func NewParamsBuilder() *Builder {
	params := &Params{}
	b := &Builder{params: params}
	return b
}

func (b *Builder) Action(action string) *Builder {
	b.params.Action = action
	return b
}

func (b *Builder) Protocol(protocol string) *Builder {
	b.params.Protocol = protocol
	return b
}

func (b *Builder) Uri(uri string) *Builder {
	b.params.Uri = uri
	return b
}

func (b *Builder) GatewayUri(gatewayUri string) *Builder {
	b.params.GatewayUri = gatewayUri
	return b
}

func (b *Builder) Request(request interface{}) *Builder {
	b.params.Request = request
	return b
}

func (b *Builder) Method(method string) *Builder {
	b.params.Method = method
	return b
}

func (b *Builder) AuthType(authType int32) *Builder {
	b.params.AuthType = authType
	return b
}

func (b *Builder) ContentType(contentType string) *Builder {
	b.params.ContentType = contentType
	return b
}

func (b *Builder) Build() *Params {
	return b.params
}
