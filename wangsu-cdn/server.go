package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	wangsucertmgr "github.com/certimate-go/plugins/internal/wangsucertmgr"
	wangsucdnsdk "github.com/certimate-go/plugins/internal/wangsusdk/cdn"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "wangsu-cdn"
	accessType     = "wangsu"
	displayNameKey = "plugin.wangsu-cdn.name"

	domainMatchPatternExact = "exact"
)

type wangsuCdnDeployer struct{}

func (*wangsuCdnDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*wangsuCdnDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	accessSchema, _ := plugin.LoadAccessSchema(schemaFS)
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{
		AccessSchemaJSON: accessSchema,
		DeploySchemaJSON: deploySchema,
		I18n:             i18n,
	}, nil
}

type accessConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	ApiKey          string `json:"apiKey"`
}

type extendedConfig struct {
	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`
	Domains            string `json:"domains"`
}

func (d *wangsuCdnDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}

	var extended extendedConfig
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}

	sdkClient, err := createSDKClient(access.AccessKeyId, access.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	certmgr, err := wangsucertmgr.NewCertmgr(&wangsucertmgr.CertmgrConfig{
		AccessKeyId:     access.AccessKeyId,
		AccessKeySecret: access.AccessKeySecret,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	upres, err := certmgr.Upload(ctx, req.CertificatePEM, req.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	domains, err := resolveDeployDomains(extended.DomainMatchPattern, extended.Domains)
	if err != nil {
		return nil, err
	}

	certIdAsInt, _ := strconv.ParseInt(upres.CertId, 10, 64)
	batchUpdateCertificateConfigReq := &wangsucdnsdk.BatchUpdateCertificateConfigRequest{
		CertificateId: certIdAsInt,
		DomainNames:   domains,
	}
	batchUpdateCertificateConfigResp, err := sdkClient.BatchUpdateCertificateConfigWithContext(ctx, batchUpdateCertificateConfigReq)
	logger.Debug("sdk request 'cdn.BatchUpdateCertificateConfig'", slog.Any("request", batchUpdateCertificateConfigReq), slog.Any("response", batchUpdateCertificateConfigResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cdn.BatchUpdateCertificateConfig': %w", err)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func resolveDeployDomains(domainMatchPattern string, domainsRaw string) ([]string, error) {
	domains := make([]string, 0)
	if domainsRaw != "" {
		domains = strings.Split(domainsRaw, ";")
	}

	switch domainMatchPattern {
	case "", domainMatchPatternExact:
		{
			if len(domains) == 0 {
				return nil, fmt.Errorf("config `domains` is required")
			}

			return lo.Map(domains, func(domain string, _ int) string {
				return normalizeDomain(domain)
			}), nil
		}

	default:
		return nil, fmt.Errorf("unsupported domain match pattern: '%s'", domainMatchPattern)
	}
}

func createSDKClient(accessKeyId, accessKeySecret string) (*wangsucdnsdk.Client, error) {
	client, err := wangsucdnsdk.NewClient(
		wangsucdnsdk.WithAkSk(accessKeyId, accessKeySecret),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func normalizeDomain(domain string) string {
	if strings.HasPrefix(domain, "*.") {
		return strings.TrimPrefix(domain, "*")
	}
	return domain
}
