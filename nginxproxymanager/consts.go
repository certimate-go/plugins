package main

import (
	cmgrimpl "github.com/certimate-go/plugins/internal/nginxproxymanagercertmgr"
)

const (
	AUTH_METHOD_PASSWORD = cmgrimpl.AUTH_METHOD_PASSWORD
	AUTH_METHOD_TOKEN    = cmgrimpl.AUTH_METHOD_TOKEN
)

const (
	DEPLOY_TARGET_HOST = "host"

	DEPLOY_TARGET_CERTIFICATE = "certificate"
)

const (
	HOST_MATCH_PATTERN_SPECIFIED = "specified"

	HOST_MATCH_PATTERN_CERTSAN = "certsan"
)

const (
	HOST_TYPE_PROXY       = "proxy"
	HOST_TYPE_REDIRECTION = "redirection"
	HOST_TYPE_STREAM      = "stream"
	HOST_TYPE_DEAD        = "dead"
)
