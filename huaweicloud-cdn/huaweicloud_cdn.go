package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	hwcdnmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cdn/v2/model"
	hwcdnregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cdn/v2/region"
	"github.com/samber/lo"

	hwcdn "github.com/certimate-go/plugins/internal/cloudsdk/github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cdn/v2"

	"github.com/certimate-go/certimate/pkg/core"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/huaweicloud-scm"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	EnterpriseProjectId string `json:"enterpriseProjectId,omitempty"`

	Region string `json:"region"`

	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *hwcdn.CdnClient
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(
		config.AccessKeyId,
		config.SecretAccessKey,
		config.Region,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		AccessKeyId:         config.AccessKeyId,
		SecretAccessKey:     config.SecretAccessKey,
		EnterpriseProjectId: config.EnterpriseProjectId,
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

		const MAX_DOMAINS_PER_REQUEST = 50
		domainChunks := lo.Chunk(domains, MAX_DOMAINS_PER_REQUEST)
		if err := xloop.ForRangeAllWithContext(ctx, domainChunks, func(ctx context.Context, domains []string, _ int) error {
			return d.updateDomainsCertificate(ctx, domains, upres.CertId, upres.CertName)
		}); err != nil {
			return nil, err
		}
	}

	return &DeployResult{}, nil
}

func (d *Deployer) getAllDomains(ctx context.Context) ([]string, error) {
	domains := make([]string, 0)

	listDomainsPageNumber := 1
	listDomainsPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listDomainsReq := &hwcdnmodel.ListDomainsRequest{
			EnterpriseProjectId: lo.EmptyableToPtr(d.config.EnterpriseProjectId),
			PageNumber:          lo.ToPtr(int32(listDomainsPageNumber)),
			PageSize:            lo.ToPtr(int32(listDomainsPageSize)),
		}
		listDomainsResp, err := d.sdkClient.ListDomains(listDomainsReq)
		d.logger.Debug("sdk request 'cdn.ListDomains'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.ListDomains': %w", err)
		}

		if listDomainsResp.Domains == nil {
			break
		}

		ignoredStatuses := []string{"offline", "checking", "check_failed", "deleting"}
		for _, domainItem := range *listDomainsResp.Domains {
			if lo.Contains(ignoredStatuses, lo.FromPtr(domainItem.DomainStatus)) {
				continue
			}

			domains = append(domains, lo.FromPtr(domainItem.DomainName))
		}

		if len(*listDomainsResp.Domains) < listDomainsPageSize {
			break
		}

		listDomainsPageNumber++
	}

	return domains, nil
}

func (d *Deployer) updateDomainsCertificate(ctx context.Context, domains []string, cloudCertId, cloudCertName string) error {
	updateDomainMultiCertificatesReq := &hwcdnmodel.UpdateDomainMultiCertificatesRequest{
		EnterpriseProjectId: lo.EmptyableToPtr(d.config.EnterpriseProjectId),
		Body: &hwcdnmodel.UpdateDomainMultiCertificatesRequestBody{
			Https: &hwcdnmodel.UpdateDomainMultiCertificatesRequestBodyContent{
				DomainName:       strings.Join(domains, ","),
				HttpsSwitch:      1,
				CertificateType:  lo.ToPtr(int32(2)),
				ScmCertificateId: lo.ToPtr(cloudCertId),
				CertName:         lo.ToPtr(cloudCertName),
			},
		},
	}
	_, err := d.sdkClient.UpdateDomainMultiCertificates(updateDomainMultiCertificatesReq)
	d.logger.Debug("sdk request 'cdn.UpdateDomainMultiCertificates'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.UpdateDomainMultiCertificates': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*hwcdn.CdnClient, error) {
	if region == "" {
		region = "cn-north-1"
	}

	auth, err := global.NewCredentialsBuilder().
		WithAk(accessKeyId).
		WithSk(secretAccessKey).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	hcRegion, err := hwcdnregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := hwcdn.CdnClientBuilder().
		WithRegion(hcRegion).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	client := hwcdn.NewCdnClient(hcClient)
	return client, nil
}
