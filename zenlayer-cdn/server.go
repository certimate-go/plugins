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
	zcommon "github.com/zenlayer/zenlayercloud-sdk-go/zenlayercloud/common"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
	zenlayercertmgr "github.com/certimate-go/plugins/internal/zenlayercertmgr/cdn"
	zcdnsdk "github.com/certimate-go/plugins/internal/zenlayersdk/cdn"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "zenlayer-cdn"
	accessType     = "zenlayer"
	displayNameKey = "plugin.zenlayer-cdn.name"

	deployTargetDomain      = "domain"
	deployTargetCertificate = "certificate"

	domainMatchPatternExact    = "exact"
	domainMatchPatternWildcard = "wildcard"
	domainMatchPatternCertsan  = "certsan"
)

type zenlayerCdnDeployer struct{}

func (*zenlayerCdnDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*zenlayerCdnDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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
	AccessKeyId       string `json:"accessKeyId"`
	AccessKeyPassword string `json:"accessKeyPassword"`
	ResourceGroupId   string `json:"resourceGroupId,omitempty"`
}

type extendedConfig struct {
	DeployTarget       string `json:"deployTarget"`
	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`
	Domain             string `json:"domain"`
	CertificateId      string `json:"certificateId,omitempty"`
}

func (d *zenlayerCdnDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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

	certmgr, err := zenlayercertmgr.NewCertmgr(&zenlayercertmgr.CertmgrConfig{
		AccessKeyId:       access.AccessKeyId,
		AccessKeyPassword: access.AccessKeyPassword,
		ResourceGroupId:   access.ResourceGroupId,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	sdkClient, err := createSDKClient(access.AccessKeyId, access.AccessKeyPassword)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	runner := &cdnRunner{
		config:     &extended,
		logger:     logger,
		sdkClient:  sdkClient,
		sdkCertmgr: certmgr,
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
	config     *extendedConfig
	logger     *slog.Logger
	sdkClient  *zcdnsdk.Client
	sdkCertmgr *zenlayercertmgr.Certmgr
}

func (d *cdnRunner) deployToDomain(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.Domain == "" {
		return fmt.Errorf("config `domain` is required")
	}

	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	domainIds := make([]string, 0)
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

			domains := lo.Filter(domainCandidates, func(domainItem *zcdnsdk.DomainInfo, _ int) bool {
				return d.config.Domain == domainItem.DomainName
			})
			if len(domains) == 0 {
				return fmt.Errorf("could not find domain")
			}

			domainIds = lo.Map(domains, func(domainItem *zcdnsdk.DomainInfo, _ int) string {
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

			domains := lo.Filter(domainCandidates, func(domainItem *zcdnsdk.DomainInfo, _ int) bool {
				return xcerthostname.IsMatch(d.config.Domain, domainItem.DomainName)
			})
			if len(domains) == 0 {
				return fmt.Errorf("could not find any domains matched by wildcard")
			}

			domainIds = lo.Map(domains, func(domainItem *zcdnsdk.DomainInfo, _ int) string {
				return domainItem.DomainId
			})
		}

	case domainMatchPatternCertsan:
		{
			domainCandidates, err := d.getAllDomains(ctx)
			if err != nil {
				return err
			}

			domains := lo.Filter(domainCandidates, func(domainItem *zcdnsdk.DomainInfo, _ int) bool {
				return xcerthostname.IsMatchByCertificatePEM(certPEM, domainItem.DomainName)
			})
			if len(domains) == 0 {
				return fmt.Errorf("could not find any domains matched by certificate")
			}

			domainIds = lo.Map(domains, func(domainItem *zcdnsdk.DomainInfo, _ int) string {
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
			return d.updateDomainCertificate(ctx, domainId, upres.CertId)
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

	rplres, err := d.sdkCertmgr.Replace(ctx, d.config.CertificateId, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to replace certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate replaced", slog.Any("result", rplres))
	}

	return nil
}

func (d *cdnRunner) getAllDomains(ctx context.Context) ([]*zcdnsdk.DomainInfo, error) {
	domains := make([]*zcdnsdk.DomainInfo, 0)

	describeDomainsPageNum := 1
	describeDomainsPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		describeDomainsReq := zcdnsdk.NewDescribeDomainsRequest()
		describeDomainsReq.DomainStatus = "ENABLED"
		describeDomainsReq.PageNum = describeDomainsPageNum
		describeDomainsReq.PageSize = describeDomainsPageSize
		describeDomainsResp, err := d.sdkClient.DescribeDomains(describeDomainsReq)
		d.logger.Debug("sdk request 'cdn.DescribeDomains'", slog.Any("request", describeDomainsReq), slog.Any("response", describeDomainsResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.DescribeDomains': %w", err)
		}

		for _, domainItem := range describeDomainsResp.Response.DataSet {
			domains = append(domains, domainItem)
		}

		if len(describeDomainsResp.Response.DataSet) < describeDomainsPageSize {
			break
		}

		describeDomainsPageNum++
	}

	return domains, nil
}

func (d *cdnRunner) updateDomainCertificate(ctx context.Context, cloudDomainId string, cloudCertId string) error {
	describeDomainCertificateReq := zcdnsdk.NewDescribeDomainCertificateRequest()
	describeDomainCertificateReq.DomainId = cloudDomainId
	describeDomainCertificateResp, err := d.sdkClient.DescribeDomainCertificate(describeDomainCertificateReq)
	d.logger.Debug("sdk request 'cdn.DescribeDomainCertificate'", slog.Any("request", describeDomainCertificateReq), slog.Any("response", describeDomainCertificateResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.DescribeDomainCertificate': %w", err)
	} else if describeDomainCertificateResp.Response.Certificate != nil && describeDomainCertificateResp.Response.Certificate.CertificateId == cloudCertId {
		return nil
	}

	modifyDomainCertificateReq := zcdnsdk.NewModifyDomainCertificateRequest()
	modifyDomainCertificateReq.DomainId = cloudDomainId
	modifyDomainCertificateReq.CertificateId = cloudCertId
	modifyDomainCertificateResp, err := d.sdkClient.ModifyDomainCertificate(modifyDomainCertificateReq)
	d.logger.Debug("sdk request 'cdn.ModifyDomainCertificate'", slog.Any("request", modifyDomainCertificateReq), slog.Any("response", modifyDomainCertificateResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.ModifyDomainCertificate': %w", err)
	}

	if _, err := xwait.UntilWithContext(ctx, func(_ context.Context, _ int) (bool, error) {
		describeDomainsReq := zcdnsdk.NewDescribeDomainsRequest()
		describeDomainsReq.DomainIds = []string{cloudDomainId}
		describeDomainsReq.PageNum = 1
		describeDomainsReq.PageSize = 1
		describeDomainsResp, err := d.sdkClient.DescribeDomains(describeDomainsReq)
		d.logger.Debug("sdk request 'cdn.DescribeDomains'", slog.Any("request", describeDomainsReq), slog.Any("response", describeDomainsResp))
		if err != nil {
			return false, fmt.Errorf("failed to execute sdk request 'cdn.DescribeDomains': %w", err)
		} else if len(describeDomainsResp.Response.DataSet) == 0 {
			return false, fmt.Errorf("could not found domain '%s'", cloudDomainId)
		}

		switch describeDomainsResp.Response.DataSet[0].ConfigStatus {
		case "DEPLOYED":
			return true, nil
		case "FAILED":
			return false, fmt.Errorf("unexpected domain status")
		}

		d.logger.Info("waiting for domain deploying completion ...")
		return false, nil
	}, 10*time.Second); err != nil {
		return err
	}

	return nil
}

func createSDKClient(accessKeyId, accessKeyPassword string) (*zcdnsdk.Client, error) {
	config := zcommon.NewConfig()

	client, err := zcdnsdk.NewClient(config, accessKeyId, accessKeyPassword)
	if err != nil {
		return nil, err
	}

	return client, nil
}
