package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	jdcore "github.com/jdcloud-api/jdcloud-sdk-go/core"
	jdcdnapis "github.com/jdcloud-api/jdcloud-sdk-go/services/cdn/apis"
	"github.com/samber/lo"

	jdcdn "github.com/certimate-go/plugins/internal/cloudsdk/jdcloud/cdn/client"

	"github.com/certimate-go/certimate/pkg/core"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/jdcloudsslcert"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	AccessKeySecret string `json:"accessKeySecret"`

	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *jdcdn.CdnClient
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		AccessKeyId:     config.AccessKeyId,
		AccessKeySecret: config.AccessKeySecret,
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

func (d *Deployer) getAllDomains(ctx context.Context) ([]string, error) {
	domains := make([]string, 0)

	getDomainListPageNumber := 1
	getDomainListPageSize := 50
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		getDomainListReq := jdcdnapis.NewGetDomainListRequestWithoutParam()
		getDomainListReq.SetPageNumber(getDomainListPageNumber)
		getDomainListReq.SetPageSize(getDomainListPageSize)
		getDomainListResp, err := d.sdkClient.GetDomainList(getDomainListReq)
		if err == nil && getDomainListResp.Error.Code != 0 {
			err = fmt.Errorf("JDCloud API error %d (%s): %s", getDomainListResp.Error.Code, getDomainListResp.Error.Status, getDomainListResp.Error.Message)
		}
		d.logger.Debug("sdk request 'cdn.GetDomainList'", slog.Any("request", getDomainListReq), slog.Any("response", getDomainListResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.GetDomainList': %w", err)
		}

		ignoredStatuses := []string{"offline"}
		for _, domainItem := range getDomainListResp.Result.Domains {
			if lo.Contains(ignoredStatuses, domainItem.Status) {
				continue
			}

			domains = append(domains, domainItem.Domain)
		}

		if len(getDomainListResp.Result.Domains) < getDomainListPageSize {
			break
		}

		getDomainListPageNumber++
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, domain string, cloudCertId string) error {
	queryDomainConfigReq := jdcdnapis.NewQueryDomainConfigRequestWithoutParam()
	queryDomainConfigReq.SetDomain(domain)
	queryDomainConfigResp, err := d.sdkClient.QueryDomainConfig(queryDomainConfigReq)
	if err == nil && queryDomainConfigResp.Error.Code != 0 {
		err = fmt.Errorf("JDCloud API error %d (%s): %s", queryDomainConfigResp.Error.Code, queryDomainConfigResp.Error.Status, queryDomainConfigResp.Error.Message)
	}
	d.logger.Debug("sdk request 'cdn.QueryDomainConfig'", slog.Any("request", queryDomainConfigReq))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.QueryDomainConfig': %w", err)
	}

	setHttpTypeReq := jdcdnapis.NewSetHttpTypeRequestWithoutParam()
	setHttpTypeReq.SetDomain(domain)
	setHttpTypeReq.SetHttpType("https")
	setHttpTypeReq.SetCertFrom("ssl")
	setHttpTypeReq.SetSslCertId(cloudCertId)
	setHttpTypeReq.SetJumpType(queryDomainConfigResp.Result.HttpsJumpType)
	setHttpTypeResp, err := d.sdkClient.SetHttpType(setHttpTypeReq)
	if err == nil && setHttpTypeResp.Error.Code != 0 {
		err = fmt.Errorf("JDCloud API error %d (%s): %s", setHttpTypeResp.Error.Code, setHttpTypeResp.Error.Status, setHttpTypeResp.Error.Message)
	}
	d.logger.Debug("sdk request 'cdn.QueryDomainConfig'", slog.Any("request", setHttpTypeReq), slog.Any("response", setHttpTypeResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.SetHttpType': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, accessKeySecret string) (*jdcdn.CdnClient, error) {
	clientCredentials := jdcore.NewCredentials(accessKeyId, accessKeySecret)
	client := jdcdn.NewCdnClient(clientCredentials)
	client.DisableLogger()
	return client, nil
}
