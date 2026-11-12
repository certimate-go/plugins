package v2

import (
	v1 "github.com/certimate-go/plugins/internal/onepanelsdk"
)

type Website v1.Website

type WebsiteDetail struct {
	Website
	Domains []*WebsiteDomainConfig `json:"domains"`
}

type WebsiteDomainConfig v1.WebsiteDomainConfig

type WebsiteHTTPSConfig struct {
	v1.WebsiteHTTPSConfig
	Http3 bool `json:"http3"`
}

type SSLCertificate v1.SSLCertificate
