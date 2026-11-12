package byteplusquery

import (
	"github.com/byteplus-sdk/byteplus-go-sdk-v2/byteplus/byteplusquery"
	"github.com/byteplus-sdk/byteplus-go-sdk-v2/byteplus/request"
)

var BuildHandler = request.NamedHandler{Name: byteplusquery.BuildHandler.Name, Fn: Build}

func Build(r *request.Request) {
	byteplusquery.Build(r)
}
