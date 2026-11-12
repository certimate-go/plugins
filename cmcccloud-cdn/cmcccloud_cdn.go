package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcmcdn/model"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/config"
	"github.com/samber/lo"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcmcdn"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
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
	config    *DeployerConfig
	logger    *slog.Logger
	sdkClient *ecloudsdkcmcdn.Client
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

	return &Deployer{
		config:    config,
		logger:    slog.Default(),
		sdkClient: client,
	}, nil
}

func (d *Deployer) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}
}

func (d *Deployer) Deploy(ctx context.Context, certPEM, privkeyPEM string) (*DeployResult, error) {
	var domainIds []int32
	switch d.config.DomainMatchPattern {
	case "", DOMAIN_MATCH_PATTERN_EXACT:
		{
			if d.config.Domain == "" {
				return nil, fmt.Errorf("config `domain` is required")
			}

			domainCandidates, err := d.getAllDomains(ctx)
			if err != nil {
				return nil, err
			}

			domains := lo.Filter(domainCandidates, func(domainItem *model.DescribeUserDomainsResponseList, _ int) bool {
				return d.config.Domain == lo.FromPtr(domainItem.DomainName)
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find domain")
			}

			domainIds = lo.Map(domains, func(domainItem *model.DescribeUserDomainsResponseList, _ int) int32 {
				return lo.FromPtr(domainItem.DomainId)
			})
		}

	case DOMAIN_MATCH_PATTERN_WILDCARD:
		{
			if d.config.Domain == "" {
				return nil, fmt.Errorf("config `domain` is required")
			}

			domainCandidates, err := d.getAllDomains(ctx)
			if err != nil {
				return nil, err
			}

			domains := lo.Filter(domainCandidates, func(domainItem *model.DescribeUserDomainsResponseList, _ int) bool {
				return xcerthostname.IsMatch(d.config.Domain, lo.FromPtr(domainItem.DomainName))
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find any domains matched by wildcard")
			}

			domainIds = lo.Map(domains, func(domainItem *model.DescribeUserDomainsResponseList, _ int) int32 {
				return lo.FromPtr(domainItem.DomainId)
			})
		}

	case DOMAIN_MATCH_PATTERN_CERTSAN:
		{
			domainCandidates, err := d.getAllDomains(ctx)
			if err != nil {
				return nil, err
			}

			domains := lo.Filter(domainCandidates, func(domainItem *model.DescribeUserDomainsResponseList, _ int) bool {
				return xcerthostname.IsMatchByCertificatePEM(certPEM, lo.FromPtr(domainItem.DomainName))
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find any domains matched by certificate")
			}

			domainIds = lo.Map(domains, func(domainItem *model.DescribeUserDomainsResponseList, _ int) int32 {
				return lo.FromPtr(domainItem.DomainId)
			})
		}

	default:
		return nil, fmt.Errorf("unsupported domain match pattern: '%s'", d.config.DomainMatchPattern)
	}

	if len(domainIds) == 0 {
		d.logger.Info("no cdn domains to deploy")
	} else {
		d.logger.Info("found cdn domains to deploy", slog.Any("domainIds", domainIds))

		if err := xloop.ForRangeAllWithContext(ctx, domainIds, func(ctx context.Context, domainId int32, _ int) error {
			return d.updateDomainCertificate(ctx, domainId, certPEM, privkeyPEM)
		}); err != nil {
			return nil, err
		}
	}

	return &DeployResult{}, nil
}

func (d *Deployer) getAllDomains(ctx context.Context) ([]*model.DescribeUserDomainsResponseList, error) {
	domains := make([]*model.DescribeUserDomainsResponseList, 0)

	describeUserDomainsPage := 1
	describeUserDomainsPageSize := 10
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		describeUserDomainsReq := &model.DescribeUserDomainsRequest{
			DescribeUserDomainsQuery: &model.DescribeUserDomainsQuery{
				Page:     lo.ToPtr(fmt.Sprintf("%d", describeUserDomainsPage)),
				PageSize: lo.ToPtr(fmt.Sprintf("%d", describeUserDomainsPageSize)),
			},
		}
		describeUserDomainsResp, err := d.sdkClient.DescribeUserDomains(describeUserDomainsReq)
		d.logger.Debug("sdk request 'ecdn.DescribeUserDomains'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'ecdn.DescribeUserDomains': %w", err)
		}

		if describeUserDomainsResp.Body == nil || describeUserDomainsResp.Body.List == nil {
			break
		}

		ignoredStatuses := []string{"ADD_AUDITING", "ADD_OPENING", "PAUSE_AUDITING", "PAUSE_HANDLING", "OFFLINE_AUDITING", "OFFLINE_HANDLING", "OFFLINE", "AUDIT_FAIL", "ADD_FAIL", "OFFLINE_FAIL"}
		for _, domainItem := range *describeUserDomainsResp.Body.List {
			if lo.FromPtr(domainItem.Deleted) {
				continue
			}
			if lo.Contains(ignoredStatuses, string(lo.FromPtr(domainItem.DomainStatus))) {
				continue
			}

			domains = append(domains, &domainItem)
		}

		if len(*describeUserDomainsResp.Body.List) < describeUserDomainsPageSize {
			break
		}

		describeUserDomainsPage++
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, cloudDomainId int32, certPEM, privkeyPEM string) error {
	describeCdnDomainDetailReq := &model.DescribeCdnDomainDetailRequest{
		DescribeCdnDomainDetailPath: &model.DescribeCdnDomainDetailPath{
			DomainId: lo.ToPtr(fmt.Sprintf("%d", cloudDomainId)),
		},
	}
	describeCdnDomainDetailResp, err := d.sdkClient.DescribeCdnDomainDetail(describeCdnDomainDetailReq)
	d.logger.Debug("sdk request 'ecdn.DescribeCdnDomainDetail'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'ecdn.DescribeCdnDomainDetail': %w", err)
	}

	if lo.FromPtr(describeCdnDomainDetailResp.Body.CrtUniqueId) != "" {
		describeCdnCertificateDetailReq := &model.DescribeCdnCertificateDetailRequest{
			DescribeCdnCertificateDetailPath: &model.DescribeCdnCertificateDetailPath{
				UniqueId: describeCdnDomainDetailResp.Body.CrtUniqueId,
			},
		}
		describeCdnCertificateDetailResp, err := d.sdkClient.DescribeCdnCertificateDetail(describeCdnCertificateDetailReq)
		d.logger.Debug("sdk request 'ecdn.DescribeCdnCertificateDetail'")
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'ecdn.DescribeCdnCertificateDetail': %w", err)
		} else {
			if xcert.EqualCertificatesFromPEM(certPEM, lo.FromPtr(describeCdnCertificateDetailResp.Body.Certificate)) {
				d.logger.Info("no need to deploy cdn certificate")
				return nil
			}
		}
	}

	addDomainServerCertificateReq := &model.AddDomainServerCertificateRequest{
		AddDomainServerCertificateBody: &model.AddDomainServerCertificateBody{
			DomainId:    lo.ToPtr(cloudDomainId),
			CrtName:     lo.ToPtr(fmt.Sprintf("certimate-%d", time.Now().UnixMilli())),
			Certificate: lo.ToPtr(certPEM),
			PrivateKey:  lo.ToPtr(privkeyPEM),
		},
	}
	_, err = d.sdkClient.AddDomainServerCertificate(addDomainServerCertificateReq)
	d.logger.Debug("sdk request 'ecdn.AddDomainServerCertificate'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'ecdn.AddDomainServerCertificate': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, accessKeySecret string) (*ecloudsdkcmcdn.Client, error) {
	ak := accessKeyId
	sk := accessKeySecret

	poolId := "CIDC-CORE-00"

	client := ecloudsdkcmcdn.NewClient(&config.Config{
		AccessKey: &ak,
		SecretKey: &sk,
		PoolId:    &poolId,
	})

	return client, nil
}
