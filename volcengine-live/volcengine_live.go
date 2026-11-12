package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/samber/lo"
	velive "github.com/volcengine/volc-sdk-golang/service/live/v20230101"
	ve "github.com/volcengine/volcengine-go-sdk/volcengine"

	"github.com/certimate-go/certimate/pkg/core"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/volcengine-live"
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
	sdkClient  *velive.Live
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client := velive.NewInstance()
	client.SetAccessKey(config.AccessKeyId)
	client.SetSecretKey(config.SecretAccessKey)

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
		d.logger.Info("no live domains to deploy")
	} else {
		d.logger.Info("found live domains to deploy", slog.Any("domains", domains))

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

	listDomainDetailPageNum := 1
	listDomainDetailPageSize := 1000
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listDomainDetailReq := &velive.ListDomainDetailBody{
			DomainStatusList: ve.Int32Slice([]int32{0}),
			PageNum:          int32(listDomainDetailPageNum),
			PageSize:         int32(listDomainDetailPageSize),
		}
		listDomainDetailResp, err := d.sdkClient.ListDomainDetail(ctx, listDomainDetailReq)
		d.logger.Debug("sdk request 'live.ListDomainDetail'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'live.ListDomainDetail': %w", err)
		}

		if listDomainDetailResp.Result == nil {
			break
		}

		for _, domainItem := range listDomainDetailResp.Result.DomainList {
			domains = append(domains, domainItem.Domain)
		}

		if len(listDomainDetailResp.Result.DomainList) < listDomainDetailPageSize {
			break
		}

		listDomainDetailPageNum++
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, domain string, cloudCertId string) error {
	bindCertReq := &velive.BindCertBody{
		ChainID: cloudCertId,
		Domain:  domain,
		HTTPS:   ve.Bool(true),
	}
	_, err := d.sdkClient.BindCert(ctx, bindCertReq)
	d.logger.Debug("sdk request 'live.BindCert'")
	if err != nil {
		return err
	}

	return nil
}
