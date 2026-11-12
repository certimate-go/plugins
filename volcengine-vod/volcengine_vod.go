package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/samber/lo"
	ve "github.com/volcengine/volcengine-go-sdk/volcengine"
	vesession "github.com/volcengine/volcengine-go-sdk/volcengine/session"

	vevod "github.com/certimate-go/plugins/internal/cloudsdk/github.com/volcengine/volcengine-go-sdk/service/vod20260101"

	"github.com/certimate-go/certimate/pkg/core"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/volcengine-certcenter"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	ProjectName string `json:"projectName,omitempty"`

	Region string `json:"region"`

	SpaceName string `json:"spaceName"`

	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`

	DomainType string `json:"domainType"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *vevod.VOD20260101
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey, config.Region)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		AccessKeyId:     config.AccessKeyId,
		SecretAccessKey: config.SecretAccessKey,
		ProjectName:     config.ProjectName,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}

	return &Deployer{
		config:     config,
		logger:     slog.Default(),
		sdkClient:  client,
		sdkCertmgr: pcertmgr,
	}, nil
}

func (d *Deployer) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}

	d.sdkCertmgr.SetLogger(logger)
}

func (d *Deployer) Deploy(ctx context.Context, certPEM, privkeyPEM string) (*DeployResult, error) {
	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	domains := make([]string, 0)
	switch d.config.DomainMatchPattern {
	case "", DOMAIN_MATCH_PATTERN_EXACT:
		{
			if d.config.Domain == "" {
				return nil, fmt.Errorf("config `domain` is required")
			}

			domains = append(domains, d.config.Domain)
		}

	case DOMAIN_MATCH_PATTERN_WILDCARD:
		{
			if d.config.Domain == "" {
				return nil, fmt.Errorf("config `domain` is required")
			}

			if strings.HasPrefix(d.config.Domain, "*.") {
				domainCandidates, err := d.getAllDomains(ctx)
				if err != nil {
					return nil, err
				}

				domains = lo.Filter(domainCandidates, func(domain string, _ int) bool {
					return xcerthostname.IsMatch(d.config.Domain, domain)
				})
				if len(domains) == 0 {
					return nil, fmt.Errorf("could not find any domains matched by wildcard")
				}
			} else {
				domains = append(domains, d.config.Domain)
			}
		}

	case DOMAIN_MATCH_PATTERN_CERTSAN:
		{
			domainCandidates, err := d.getAllDomains(ctx)
			if err != nil {
				return nil, err
			}

			domains = lo.Filter(domainCandidates, func(domain string, _ int) bool {
				return xcerthostname.IsMatchByCertificatePEM(certPEM, domain)
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find any domains matched by certificate")
			}
		}

	default:
		return nil, fmt.Errorf("unsupported domain match pattern: '%s'", d.config.DomainMatchPattern)
	}

	if len(domains) == 0 {
		d.logger.Info("no vod domains to deploy")
	} else {
		d.logger.Info("found vod domains to deploy", slog.Any("domains", domains))

		if err := xloop.ForRangeAllWithContext(ctx, domains, func(ctx context.Context, domain string, _ int) error {
			return d.updateDomainCertificate(ctx, domain, upres.CertId)
		}); err != nil {
			return nil, err
		}
	}

	return &DeployResult{}, nil
}

func (d *Deployer) getAllDomains(ctx context.Context) ([]string, error) {
	domains := make([]string, 0)

	listVodDomainPageNum := 1
	listVodDomainPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listVodDomainReq := &vevod.ListVodDomainsInput{
			SpaceName:  ve.String(d.config.SpaceName),
			DomainType: ve.String(convertDomainType2CloudDomainType(d.config.DomainType)),
			ListCdnDomainsParam: &vevod.ListCdnDomainsParamForListVodDomainsInput{
				PageNum:  ve.Int64(int64(listVodDomainPageNum)),
				PageSize: ve.Int64(int64(listVodDomainPageSize)),
			},
		}
		listVodDomainResp, err := d.sdkClient.ListVodDomainsWithContext(ctx, listVodDomainReq)
		d.logger.Debug("sdk request 'vod.ListVodDomain'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'vod.ListVodDomain': %w", err)
		}

		if listVodDomainResp.VodInfo == nil {
			break
		}

		for _, domainItem := range listVodDomainResp.VodInfo.Domains {
			domains = append(domains, ve.StringValue(domainItem.Domain))
		}

		if len(listVodDomainResp.VodInfo.Domains) < listVodDomainPageSize {
			break
		}

		listVodDomainPageNum++
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, domain string, cloudCertId string) error {
	describeVodDomainConfigReq := &vevod.DescribeVodDomainConfigInput{
		SpaceName:  ve.String(d.config.SpaceName),
		DomainType: ve.String(convertDomainType2CloudDomainType(d.config.DomainType)),
		DescribeCdnDomainParam: &vevod.DescribeCdnDomainParamForDescribeVodDomainConfigInput{
			Domain: ve.String(domain),
		},
	}
	describeVodDomainConfigResp, err := d.sdkClient.DescribeVodDomainConfigWithContext(ctx, describeVodDomainConfigReq)
	d.logger.Debug("sdk request 'vod.DescribeVodDomainConfig'")
	if err != nil {
		return err
	} else {
		if describeVodDomainConfigResp.DomainInfo != nil &&
			describeVodDomainConfigResp.DomainInfo.DomainConfig != nil &&
			describeVodDomainConfigResp.DomainInfo.DomainConfig.HTTPS != nil &&
			describeVodDomainConfigResp.DomainInfo.DomainConfig.HTTPS.CertInfo != nil &&
			ve.BoolValue(describeVodDomainConfigResp.DomainInfo.DomainConfig.HTTPS.Switch) &&
			ve.StringValue(describeVodDomainConfigResp.DomainInfo.DomainConfig.HTTPS.CertInfo.CertId) == cloudCertId {
			return nil
		}
	}

	updateVodDomainConfigReq := &vevod.UpdateVodDomainConfigInput{
		SpaceName:  ve.String(d.config.SpaceName),
		DomainType: ve.String(convertDomainType2CloudDomainType(d.config.DomainType)),
		UpdateCdnConfigParam: &vevod.UpdateCdnConfigParamForUpdateVodDomainConfigInput{
			Domain: ve.String(domain),
			HTTPS: &vevod.HTTPSForUpdateVodDomainConfigInput{
				Switch: ve.Bool(true),
				CertInfo: &vevod.CertInfoForUpdateVodDomainConfigInput{
					CertId: ve.String(cloudCertId),
				},
			},
		},
	}
	_, err = d.sdkClient.UpdateVodDomainConfigWithContext(ctx, updateVodDomainConfigReq)
	d.logger.Debug("sdk request 'vod.UpdateVodDomainConfig'")
	if err != nil {
		return err
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*vevod.VOD20260101, error) {
	if region == "" {
		region = "cn-north-1"
	}

	config := ve.NewConfig().
		WithAkSk(accessKeyId, secretAccessKey).
		WithRegion(region)

	session, err := vesession.NewSession(config)
	if err != nil {
		return nil, err
	}

	client := vevod.New(session)
	return client, nil
}

func convertDomainType2CloudDomainType(domainType string) string {
	switch domainType {
	case DOMAIN_TYPE_PLAY:
		return "vod_play"
	case DOMAIN_TYPE_IMAGE:
		return "vod_image"
	case DOMAIN_TYPE_THIRD:
		return "third"
	default:
		return domainType
	}
}
