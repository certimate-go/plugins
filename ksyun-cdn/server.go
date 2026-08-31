package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	ksyuncdnsdk "github.com/certimate-go/plugins/internal/ksyunsdk/cdn"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "ksyun-cdn"
	accessType     = "ksyun"
	displayNameKey = "plugin.ksyun-cdn.name"

	deployTargetDomain      = "domain"
	deployTargetCertificate = "certificate"

	domainMatchPatternExact    = "exact"
	domainMatchPatternWildcard = "wildcard"
	domainMatchPatternCertsan  = "certsan"
)

type ksyunCdnDeployer struct{}

func (*ksyunCdnDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*ksyunCdnDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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
	SecretAccessKey string `json:"secretAccessKey"`
}

type extendedConfig struct {
	ProjectId          int64  `json:"projectId,omitempty"`
	DeployTarget       string `json:"deployTarget"`
	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`
	Domain             string `json:"domain"`
	CertificateId      string `json:"certificateId,omitempty"`
}

func (d *ksyunCdnDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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

	sdkClient, err := createSDKClient(access.AccessKeyId, access.SecretAccessKey)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	runner := &cdnRunner{
		config:    &extended,
		logger:    logger,
		sdkClient: sdkClient,
	}

	switch extended.DeployTarget {
	case deployTargetDomain:
		if err := runner.deployToDomain(ctx, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
			return nil, err
		}

	case deployTargetCertificate:
		if err := runner.deployToCertificate(ctx, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", extended.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

type cdnRunner struct {
	config    *extendedConfig
	logger    *slog.Logger
	sdkClient *ksyuncdnsdk.Client
}

func (d *cdnRunner) deployToDomain(ctx context.Context, certPEM, privkeyPEM string) error {
	_, err := d.getAllDomains(ctx)
	if err != nil {
		return err
	}

	var domainIds []string
	switch d.config.DomainMatchPattern {
	case "", domainMatchPatternExact:
		{
			if d.config.Domain == "" {
				return fmt.Errorf("config `domain` is required")
			}

			domainCandidates, err := d.getAllDomains(ctx)
			if err != nil {
				return err
			}
			domains := lo.Filter(domainCandidates, func(domainItem *ksyuncdnsdk.CDNDomain, _ int) bool {
				return d.config.Domain == domainItem.DomainName
			})
			if len(domains) == 0 {
				return fmt.Errorf("could not find domain")
			}

			domainIds = lo.Map(domains, func(domainItem *ksyuncdnsdk.CDNDomain, _ int) string {
				return domainItem.DomainId
			})
		}

	case domainMatchPatternWildcard:
		{
			if d.config.Domain == "" {
				return fmt.Errorf("config `domain` is required")
			}

			domainCandidates, err := d.getAllDomains(ctx)
			if err != nil {
				return err
			}

			domains := lo.Filter(domainCandidates, func(domainItem *ksyuncdnsdk.CDNDomain, _ int) bool {
				return xcerthostname.IsMatch(d.config.Domain, domainItem.DomainName)
			})
			if len(domains) == 0 {
				return fmt.Errorf("could not find any domains matched by wildcard")
			}

			domainIds = lo.Map(domains, func(domainItem *ksyuncdnsdk.CDNDomain, _ int) string {
				return domainItem.DomainId
			})
		}

	case domainMatchPatternCertsan:
		{
			domainCandidates, err := d.getAllDomains(ctx)
			if err != nil {
				return err
			}

			domains := lo.Filter(domainCandidates, func(domainItem *ksyuncdnsdk.CDNDomain, _ int) bool {
				return xcerthostname.IsMatchByCertificatePEM(certPEM, domainItem.DomainName)
			})
			if len(domains) == 0 {
				return fmt.Errorf("could not find any domains matched by certificate")
			}

			domainIds = lo.Map(domains, func(domainItem *ksyuncdnsdk.CDNDomain, _ int) string {
				return domainItem.DomainId
			})
		}

	default:
		return fmt.Errorf("unsupported domain match pattern: '%s'", d.config.DomainMatchPattern)
	}

	if len(domainIds) == 0 {
		d.logger.Info("no cdn domains to deploy")
	} else {
		d.logger.Info("found cdn domains to deploy", slog.Any("domainIds", domainIds))

		if err := xloop.ForRangeAllWithContext(ctx, domainIds, func(ctx context.Context, domainId string, _ int) error {
			return d.updateDomainCertificate(ctx, domainId, certPEM, privkeyPEM)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (d *cdnRunner) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.CertificateId == "" {
		return fmt.Errorf("config `certificateId` is required")
	}

	setCertificateReq := &ksyuncdnsdk.SetCertificateRequest{
		CertificateId:     lo.ToPtr(d.config.CertificateId),
		CertificateName:   lo.ToPtr(fmt.Sprintf("certimate-%d", time.Now().UnixMilli())),
		ServerCertificate: lo.ToPtr(certPEM),
		PrivateKey:        lo.ToPtr(privkeyPEM),
	}
	setCertificateResp, err := d.sdkClient.SetCertificateWithContext(ctx, setCertificateReq)
	d.logger.Debug("sdk request 'cdn.SetCertificate'", slog.Any("request", setCertificateReq), slog.Any("response", setCertificateResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.SetCertificate': %w", err)
	}

	return nil
}

func (d *cdnRunner) getAllDomains(ctx context.Context) ([]*ksyuncdnsdk.CDNDomain, error) {
	domains := make([]*ksyuncdnsdk.CDNDomain, 0)

	getCdnDomainsPageNumber := 1
	getCdnDomainsPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		getCdnDomainsReq := &ksyuncdnsdk.GetCDNDomainsRequest{
			ProjectId:  lo.IfF(d.config.ProjectId != 0, func() *int64 { return lo.ToPtr(d.config.ProjectId) }).Else(nil),
			PageNumber: lo.ToPtr(int32(getCdnDomainsPageNumber)),
			PageSize:   lo.ToPtr(int32(getCdnDomainsPageSize)),
		}
		getCdnDomainsResp, err := d.sdkClient.GetCDNDomainsWithContext(ctx, getCdnDomainsReq)
		d.logger.Debug("sdk request 'cdn.GetCdnDomains'", slog.Any("request", getCdnDomainsReq), slog.Any("response", getCdnDomainsResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.GetCdnDomains': %w", err)
		}

		if getCdnDomainsResp.Domains == nil {
			break
		}

		ignoredStatuses := []string{"offline", "icp_checking", "icp_check_failed", "locking", "locked"}
		for _, domainItem := range getCdnDomainsResp.Domains {
			if lo.Contains(ignoredStatuses, domainItem.DomainStatus) {
				continue
			}

			domains = append(domains, domainItem)
		}

		if len(getCdnDomainsResp.Domains) < getCdnDomainsPageSize {
			break
		}

		getCdnDomainsPageNumber++
	}

	return domains, nil
}

func (d *cdnRunner) updateDomainCertificate(ctx context.Context, cloudDomainId string, certPEM, privkeyPEM string) error {
	configCertificateReq := &ksyuncdnsdk.ConfigCertificateRequest{
		Enable:            lo.ToPtr("on"),
		DomainIds:         lo.ToPtr(cloudDomainId),
		CertificateName:   lo.ToPtr(fmt.Sprintf("certimate-%d", time.Now().UnixMilli())),
		ServerCertificate: lo.ToPtr(certPEM),
		PrivateKey:        lo.ToPtr(privkeyPEM),
	}
	configCertificateResp, err := d.sdkClient.ConfigCertificateWithContext(ctx, configCertificateReq)
	d.logger.Debug("sdk request 'cdn.ConfigCertificate'", slog.Any("request", configCertificateReq), slog.Any("response", configCertificateResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.ConfigCertificate': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey string) (*ksyuncdnsdk.Client, error) {
	client, err := ksyuncdnsdk.NewClient(
		ksyuncdnsdk.WithAkSk(accessKeyId, secretAccessKey),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
