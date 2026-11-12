package volcengineapig

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/samber/lo"
	ve "github.com/volcengine/volcengine-go-sdk/volcengine"
	vesession "github.com/volcengine/volcengine-go-sdk/volcengine/session"

	veapig "github.com/certimate-go/plugins/internal/cloudsdk/github.com/volcengine/volcengine-go-sdk/service/apig"

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

	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *veapig.APIG
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
		Region:          config.Region,
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

	domainIds := make([]string, 0)
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

			domains := lo.Filter(domainCandidates, func(domainItem *veapig.ItemForListCustomDomainsOutput, _ int) bool {
				return d.config.Domain == lo.FromPtr(domainItem.Domain)
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find domain")
			}

			domainIds = lo.Map(domains, func(domainItem *veapig.ItemForListCustomDomainsOutput, _ int) string {
				return lo.FromPtr(domainItem.Id)
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

			domains := lo.Filter(domainCandidates, func(domainItem *veapig.ItemForListCustomDomainsOutput, _ int) bool {
				return xcerthostname.IsMatch(d.config.Domain, lo.FromPtr(domainItem.Domain))
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find any domains matched by wildcard")
			}

			domainIds = lo.Map(domains, func(domainItem *veapig.ItemForListCustomDomainsOutput, _ int) string {
				return lo.FromPtr(domainItem.Id)
			})
		}

	case DOMAIN_MATCH_PATTERN_CERTSAN:
		{
			domainCandidates, err := d.getAllDomains(ctx)
			if err != nil {
				return nil, err
			}

			domains := lo.Filter(domainCandidates, func(domainItem *veapig.ItemForListCustomDomainsOutput, _ int) bool {
				return xcerthostname.IsMatchByCertificatePEM(certPEM, lo.FromPtr(domainItem.Domain))
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find any domains matched by certificate")
			}

			domainIds = lo.Map(domains, func(domainItem *veapig.ItemForListCustomDomainsOutput, _ int) string {
				return lo.FromPtr(domainItem.Id)
			})
		}

	default:
		return nil, fmt.Errorf("unsupported domain match pattern: '%s'", d.config.DomainMatchPattern)
	}

	if len(domainIds) == 0 {
		d.logger.Info("no apig domains to deploy")
	} else {
		d.logger.Info("found apig domains to deploy", slog.Any("domainIds", domainIds))

		if err := xloop.ForRangeAllWithContext(ctx, domainIds, func(ctx context.Context, domainId string, _ int) error {
			return d.updateDomainCertificate(ctx, domainId, upres.CertId)
		}); err != nil {
			return nil, err
		}
	}

	return &DeployResult{}, nil
}

func (d *Deployer) getAllDomains(ctx context.Context) ([]*veapig.ItemForListCustomDomainsOutput, error) {
	domains := make([]*veapig.ItemForListCustomDomainsOutput, 0)

	listCustomDomainsPageNumber := 1
	listCustomDomainsPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listCustomDomainsReq := &veapig.ListCustomDomainsInput{
			PageNumber: ve.Int64(int64(listCustomDomainsPageNumber)),
			PageSize:   ve.Int64(int64(listCustomDomainsPageSize)),
		}
		listCustomDomainsResp, err := d.sdkClient.ListCustomDomainsWithContext(ctx, listCustomDomainsReq)
		d.logger.Debug("sdk request 'apig.ListCustomDomains'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'apig.ListCustomDomains': %w", err)
		}

		ignoredStatuses := []string{"Creating", "CreationFailed", "Deleting", "DeletionFailed"}
		for _, domainItem := range listCustomDomainsResp.Items {
			if lo.Contains(ignoredStatuses, *domainItem.Status) {
				continue
			}

			domains = append(domains, domainItem)
		}

		if len(listCustomDomainsResp.Items) < listCustomDomainsPageSize {
			break
		}

		listCustomDomainsPageNumber++
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, cloudDomainId string, cloudCertId string) error {
	getCustomDomainReq := &veapig.GetCustomDomainInput{
		Id: ve.String(cloudDomainId),
	}
	getCustomDomainResp, err := d.sdkClient.GetCustomDomainWithContext(ctx, getCustomDomainReq)
	d.logger.Debug("sdk request 'apig.GetCustomDomain'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'apig.GetCustomDomain': %w", err)
	}

	updateCustomDomainReq := &veapig.UpdateCustomDomainInput{
		Id:            ve.String(cloudDomainId),
		Protocol:      getCustomDomainResp.CustomDomain.Protocol,
		CertificateId: ve.String(cloudCertId),
	}
	if !lo.Contains(ve.StringValueSlice(updateCustomDomainReq.Protocol), "HTTPS") {
		updateCustomDomainReq.Protocol = append(updateCustomDomainReq.Protocol, ve.String("HTTPS"))
	}
	_, err = d.sdkClient.UpdateCustomDomainWithContext(ctx, updateCustomDomainReq)
	d.logger.Debug("sdk request 'apig.UpdateCustomDomain'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'apig.UpdateCustomDomain': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*veapig.APIG, error) {
	config := ve.NewConfig().
		WithAkSk(accessKeyId, secretAccessKey).
		WithRegion(region)

	session, err := vesession.NewSession(config)
	if err != nil {
		return nil, err
	}

	client := veapig.New(session)
	return client, nil
}
