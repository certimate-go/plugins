package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/ctcccloud-icdn"
	ctyunicdn "github.com/certimate-go/plugins/internal/cloudsdk/ctyun/icdn"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *ctyunicdn.Client
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

	var domains []string
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
				domains = []string{d.config.Domain}
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
		d.logger.Info("no icdn domains to deploy")
	} else {
		d.logger.Info("found icdn domains to deploy", slog.Any("domains", domains))

		if err := xloop.ForRangeAllWithContext(ctx, domains, func(ctx context.Context, domain string, _ int) error {
			return d.updateDomainCertificate(ctx, domain, upres.CertName)
		}); err != nil {
			return nil, err
		}
	}

	return &DeployResult{}, nil
}

func (d *Deployer) getAllDomains(ctx context.Context) ([]string, error) {
	domains := make([]string, 0)

	queryDomainsPage := 1
	queryDomainsPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		queryDomainListReq := &ctyunicdn.QueryDomainListRequest{
			Page:        lo.ToPtr(int32(queryDomainsPage)),
			PageSize:    lo.ToPtr(int32(queryDomainsPageSize)),
			ProductCode: lo.ToPtr("006"),
		}
		queryDomainListResp, err := d.sdkClient.QueryDomainListWithContext(ctx, queryDomainListReq)
		d.logger.Debug("sdk request 'cdn.QueryDomainList'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.QueryDomainList': %w", err)
		}

		if queryDomainListResp.ReturnObj == nil {
			break
		}

		ignoredStatuses := []int32{1, 5, 6, 7, 8, 9, 11, 12}
		for _, domainItem := range queryDomainListResp.ReturnObj.Results {
			if lo.Contains(ignoredStatuses, domainItem.Status) {
				continue
			}

			domains = append(domains, domainItem.Domain)
		}

		if len(queryDomainListResp.ReturnObj.Results) < queryDomainsPageSize {
			break
		}

		queryDomainsPage++
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, domain string, cloudCertName string) error {
	queryDomainDetailReq := &ctyunicdn.QueryDomainDetailRequest{
		Domain: lo.ToPtr(domain),
	}
	queryDomainDetailResp, err := d.sdkClient.QueryDomainDetailWithContext(ctx, queryDomainDetailReq)
	d.logger.Debug("sdk request 'icdn.QueryDomainDetail'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'icdn.QueryDomainDetail': %w", err)
	} else {
		if queryDomainDetailResp.ReturnObj != nil &&
			queryDomainDetailResp.ReturnObj.HttpsStatus == "on" &&
			queryDomainDetailResp.ReturnObj.CertName == cloudCertName {
			return nil
		}
	}

	updateDomainReq := &ctyunicdn.UpdateDomainRequest{
		Domain:      lo.ToPtr(domain),
		HttpsStatus: lo.ToPtr("on"),
		CertName:    lo.ToPtr(cloudCertName),
	}
	_, err = d.sdkClient.UpdateDomainWithContext(ctx, updateDomainReq)
	d.logger.Debug("sdk request 'icdn.UpdateDomain'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'icdn.UpdateDomain': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey string) (*ctyunicdn.Client, error) {
	client, err := ctyunicdn.NewClient(
		ctyunicdn.WithAkSk(accessKeyId, secretAccessKey),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
