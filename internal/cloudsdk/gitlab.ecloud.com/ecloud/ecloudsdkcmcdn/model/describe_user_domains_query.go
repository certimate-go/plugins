package model

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/position"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type DescribeUserDomainsQueryCdnTypeEnum string

const (
	DescribeUserDomainsQueryCdnTypeEnumWeb        DescribeUserDomainsQueryCdnTypeEnum = "web"
	DescribeUserDomainsQueryCdnTypeEnumDownload   DescribeUserDomainsQueryCdnTypeEnum = "download"
	DescribeUserDomainsQueryCdnTypeEnumVideo      DescribeUserDomainsQueryCdnTypeEnum = "video"
	DescribeUserDomainsQueryCdnTypeEnumLivestream DescribeUserDomainsQueryCdnTypeEnum = "liveStream"
)

type DescribeUserDomainsQuery struct {
	position.Query

	CdnType *DescribeUserDomainsQueryCdnTypeEnum `json:"cdnType,omitempty"`

	PageSize *string `json:"pageSize,omitempty"`

	Page *string `json:"page,omitempty"`

	QueryWord *string `json:"queryWord,omitempty"`
}

func (s DescribeUserDomainsQuery) String() string {
	return utils.Beautify(s)
}

func (s DescribeUserDomainsQuery) GoString() string {
	return s.String()
}

func (s DescribeUserDomainsQuery) ToJsonString() string {
	return utils.ToJsonString(s)
}

func (s *DescribeUserDomainsQuery) SetCdnType(v DescribeUserDomainsQueryCdnTypeEnum) *DescribeUserDomainsQuery {
	s.CdnType = &v
	return s
}

func (s *DescribeUserDomainsQuery) SetPageSize(v string) *DescribeUserDomainsQuery {
	s.PageSize = &v
	return s
}

func (s *DescribeUserDomainsQuery) SetPage(v string) *DescribeUserDomainsQuery {
	s.Page = &v
	return s
}

func (s *DescribeUserDomainsQuery) SetQueryWord(v string) *DescribeUserDomainsQuery {
	s.QueryWord = &v
	return s
}

type DescribeUserDomainsQueryBuilder struct {
	s *DescribeUserDomainsQuery
}

func NewDescribeUserDomainsQueryBuilder() *DescribeUserDomainsQueryBuilder {
	s := &DescribeUserDomainsQuery{}
	b := &DescribeUserDomainsQueryBuilder{s: s}
	return b
}

func (b *DescribeUserDomainsQueryBuilder) CdnType(v DescribeUserDomainsQueryCdnTypeEnum) *DescribeUserDomainsQueryBuilder {
	b.s.CdnType = &v
	return b
}

func (b *DescribeUserDomainsQueryBuilder) PageSize(v string) *DescribeUserDomainsQueryBuilder {
	b.s.PageSize = &v
	return b
}

func (b *DescribeUserDomainsQueryBuilder) Page(v string) *DescribeUserDomainsQueryBuilder {
	b.s.Page = &v
	return b
}

func (b *DescribeUserDomainsQueryBuilder) QueryWord(v string) *DescribeUserDomainsQueryBuilder {
	b.s.QueryWord = &v
	return b
}

func (b *DescribeUserDomainsQueryBuilder) Build() *DescribeUserDomainsQuery {
	return b.s
}
