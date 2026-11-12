package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	bpcdn "github.com/byteplus-sdk/byteplus-sdk-golang/service/cdn"
	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/byteplus-cdn"
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
	sdkClient  *bpcdn.CDN
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client := bpcdn.NewInstance()
	client.SetSchema("https")
	client.Client.Client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return fmt.Errorf("BytePlus CDN API redirects are not allowed")
	}
	client.Client.SetAccessKey(config.AccessKeyId)
	client.Client.SetSecretKey(config.SecretAccessKey)

	return &Deployer{
		config:     config,
		logger:     slog.Default(),
		sdkClient:  client,
		sdkCertmgr: cmgrimpl.NewCertmgr(client),
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

		listCdnDomainsReq := &bpcdn.ListCdnDomainsRequest{
			Project:  lo.EmptyableToPtr(d.config.ProjectName),
			Domain:   bpcdn.GetStrPtr(strings.TrimPrefix(wildcardDomain, "*.")),
			Status:   bpcdn.GetStrPtr("online"),
			PageNum:  bpcdn.GetInt64Ptr(int64(listCdnDomainsPageNum)),
			PageSize: bpcdn.GetInt64Ptr(int64(listCdnDomainsPageSize)),
		}
		listCdnDomainsResp, err := d.sdkClient.ListCdnDomains(listCdnDomainsReq)
		d.logger.Debug("sdk request 'cdn.ListCdnDomains'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.ListCdnDomains': %w", err)
		}

		for _, domainItem := range listCdnDomainsResp.Result.Data {
			if xcerthostname.IsMatch(wildcardDomain, domainItem.Domain) {
				domains = append(domains, domainItem.Domain)
			}
		}

		if len(listCdnDomainsResp.Result.Data) < listCdnDomainsPageSize {
			break
		}

		listCdnDomainsPageSize++
	}

	return domains, nil
}

func (d *Deployer) getMatchedDomainsByCertId(ctx context.Context, cloudCertId string) ([]string, error) {
	domains := make([]string, 0)

	describeCertConfigReq := &bpcdn.DescribeCertConfigRequest{
		CertId: cloudCertId,
	}
	describeCertConfigResp, err := d.sdkClient.DescribeCertConfig(describeCertConfigReq)
	d.logger.Debug("sdk request 'cdn.DescribeCertConfig'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cdn.DescribeCertConfig': %w", err)
	}

	if describeCertConfigResp.Result.CertNotConfig != nil {
		for i := range describeCertConfigResp.Result.CertNotConfig {
			domains = append(domains, describeCertConfigResp.Result.CertNotConfig[i].Domain)
		}
	}

	if describeCertConfigResp.Result.OtherCertConfig != nil {
		for i := range describeCertConfigResp.Result.OtherCertConfig {
			domains = append(domains, describeCertConfigResp.Result.OtherCertConfig[i].Domain)
		}
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, domain string, cloudCertId string) error {
	batchDeployCertReq := &bpcdn.BatchDeployCertRequest{
		CertId: cloudCertId,
		Domain: domain,
	}
	_, err := d.sdkClient.BatchDeployCert(batchDeployCertReq)
	d.logger.Debug("sdk request 'cdn.BatchDeployCert'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.BatchDeployCert': %w", err)
	}

	return nil
}
