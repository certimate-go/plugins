package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeUserDomainsResponseBody struct {
	PageSize *int32 `json:"pageSize,omitempty"`

	Page *int32 `json:"page,omitempty"`

	List *[]DescribeUserDomainsResponseList `json:"list,omitempty"`
}

func (s DescribeUserDomainsResponseBody) String() string {
	return utils.Beautify(s)
}

func (s DescribeUserDomainsResponseBody) GoString() string {
	return s.String()
}

func (s DescribeUserDomainsResponseBody) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeUserDomainsResponseBody) SetPageSize(v int32) *DescribeUserDomainsResponseBody {
	s.PageSize = &v
	return s
}

func (s *DescribeUserDomainsResponseBody) SetPage(v int32) *DescribeUserDomainsResponseBody {
	s.Page = &v
	return s
}

func (s *DescribeUserDomainsResponseBody) SetList(v []DescribeUserDomainsResponseList) *DescribeUserDomainsResponseBody {
	s.List = &v
	return s
}

type DescribeUserDomainsResponseBodyBuilder struct {
	s *DescribeUserDomainsResponseBody
}

func NewDescribeUserDomainsResponseBodyBuilder() *DescribeUserDomainsResponseBodyBuilder {
	s := &DescribeUserDomainsResponseBody{}
	b := &DescribeUserDomainsResponseBodyBuilder{s: s}
	return b
}

func (b *DescribeUserDomainsResponseBodyBuilder) PageSize(v int32) *DescribeUserDomainsResponseBodyBuilder {
	b.s.PageSize = &v
	return b
}

func (b *DescribeUserDomainsResponseBodyBuilder) Page(v int32) *DescribeUserDomainsResponseBodyBuilder {
	b.s.Page = &v
	return b
}

func (b *DescribeUserDomainsResponseBodyBuilder) List(v []DescribeUserDomainsResponseList) *DescribeUserDomainsResponseBodyBuilder {
	b.s.List = &v
	return b
}

func (b *DescribeUserDomainsResponseBodyBuilder) Build() *DescribeUserDomainsResponseBody {
	return b.s
}
