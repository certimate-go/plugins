package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/samber/lo"
	ve "github.com/volcengine/volcengine-go-sdk/volcengine"
	vesession "github.com/volcengine/volcengine-go-sdk/volcengine/session"

	vecdn "github.com/certimate-go/plugins/internal/cloudsdk/github.com/volcengine/volcengine-go-sdk/service/cdn"

	"github.com/certimate-go/certimate/pkg/core"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/volcengine-cdn"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	ProjectName string `json:"projectName,omitempty"`

	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *vecdn.CDN
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey)
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

			domains = []string{d.config.Domain}
		}

	case DOMAIN_MATCH_PATTERN_WILDCARD:
		{
			if d.config.Domain == "" {
				return nil, fmt.Errorf("config `domain` is required")
			}

			if strings.HasPrefix(d.config.Domain, "*.") {
				domainCandidates, err := d.getMatchedDomainsByWildcard(ctx, d.config.Domain)
				if err != nil {
					return nil, err
				}

				domains = domainCandidates
				if len(domains) == 0 {
					return nil, fmt.Errorf("could not find any domains matched by wildcard")
				}
			} else {
				domains = []string{d.config.Domain}
			}
		}

	case DOMAIN_MATCH_PATTERN_CERTSAN:
		{
			domainCandidates, err := d.getMatchedDomainsByCertId(ctx, upres.CertId)
			if err != nil {
				return nil, err
			}

			domains = domainCandidates
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find any domains matched by certificate")
			}
		}

	default:
		return nil, fmt.Errorf("unsupported domain match pattern: '%s'", d.config.DomainMatchPattern)
	}

	if len(domains) == 0 {
		d.logger.Info("no cdn domains to deploy")
	} else {
		d.logger.Info("found cdn domains to deploy", slog.Any("domains", domains))

		if err := xloop.ForRangeAllWithContext(ctx, domains, func(ctx context.Context, domain string, _ int) error {
			return d.updateDomainCertificate(ctx, domain, upres.CertId)
		}); err != nil {
			return nil, err
		}
	}

	return &DeployResult{}, nil
}

func (d *Deployer) getMatchedDomainsByWildcard(ctx context.Context, wildcardDomain string) ([]string, error) {
	domains := make([]string, 0)

	listCdnDomainsPageNum := 1
	listCdnDomainsPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listCdnDomainsReq := &vecdn.ListCdnDomainsInput{
			Project:  lo.EmptyableToPtr(d.config.ProjectName),
			Domain:   ve.String(strings.TrimPrefix(wildcardDomain, "*.")),
			Status:   ve.String("online"),
			PageNum:  ve.Int64(int64(listCdnDomainsPageNum)),
			PageSize: ve.Int64(int64(listCdnDomainsPageSize)),
		}
		listCdnDomainsResp, err := d.sdkClient.ListCdnDomainsWithContext(ctx, listCdnDomainsReq)
		d.logger.Debug("sdk request 'cdn.ListCdnDomains'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.ListCdnDomains': %w", err)
		}

		for _, domainItem := range listCdnDomainsResp.Data {
			if xcerthostname.IsMatch(wildcardDomain, ve.StringValue(domainItem.Domain)) {
				domains = append(domains, ve.StringValue(domainItem.Domain))
			}
		}

		if len(listCdnDomainsResp.Data) < listCdnDomainsPageSize {
			break
		}

		listCdnDomainsPageSize++
	}

	return domains, nil
}

func (d *Deployer) getMatchedDomainsByCertId(ctx context.Context, cloudCertId string) ([]string, error) {
	domains := make([]string, 0)

	describeCertConfigReq := &vecdn.DescribeCertConfigInput{
		CertId: ve.String(cloudCertId),
	}
	describeCertConfigResp, err := d.sdkClient.DescribeCertConfigWithContext(ctx, describeCertConfigReq)
	d.logger.Debug("sdk request 'cdn.DescribeCertConfig'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cdn.DescribeCertConfig': %w", err)
	}

	if describeCertConfigResp.CertNotConfig != nil {
		for i := range describeCertConfigResp.CertNotConfig {
			domains = append(domains, ve.StringValue(describeCertConfigResp.CertNotConfig[i].Domain))
		}
	}

	if describeCertConfigResp.OtherCertConfig != nil {
		for i := range describeCertConfigResp.OtherCertConfig {
			domains = append(domains, ve.StringValue(describeCertConfigResp.OtherCertConfig[i].Domain))
		}
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, domain string, cloudCertId string) error {
	batchDeployCertReq := &vecdn.BatchDeployCertInput{
		Domain: ve.String(domain),
		CertId: ve.String(cloudCertId),
	}
	_, err := d.sdkClient.BatchDeployCertWithContext(ctx, batchDeployCertReq)
	d.logger.Debug("sdk request 'cdn.BatchDeployCert'")
	if err != nil {
		return err
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey string) (*vecdn.CDN, error) {
	config := ve.NewConfig().
		WithAkSk(accessKeyId, secretAccessKey).
		WithRegion("cn-north-1")

	session, err := vesession.NewSession(config)
	if err != nil {
		return nil, err
	}

	client := vecdn.New(session)
	return client, nil
}
