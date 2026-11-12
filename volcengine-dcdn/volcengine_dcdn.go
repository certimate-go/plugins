package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/samber/lo"
	ve "github.com/volcengine/volcengine-go-sdk/volcengine"
	vesession "github.com/volcengine/volcengine-go-sdk/volcengine/session"

	vedcdn "github.com/certimate-go/plugins/internal/cloudsdk/github.com/volcengine/volcengine-go-sdk/service/dcdn"

	"github.com/certimate-go/certimate/pkg/core"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
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
	sdkClient  *vedcdn.DCDN
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
			domain := normalizeDomain(d.config.Domain)
			if domain == "" {
				return nil, fmt.Errorf("config `domain` is required")
			}

			domains = []string{domain}
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

	createCertBindReq := &vedcdn.CreateCertBindInput{
		CertSource:  ve.String("volc"),
		CertId:      ve.String(upres.CertId),
		DomainNames: ve.StringSlice(domains),
	}
	_, err = d.sdkClient.CreateCertBindWithContext(ctx, createCertBindReq)
	d.logger.Debug("sdk request 'dcdn.CreateCertBind'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'dcdn.CreateCertBind': %w", err)
	}

	return &DeployResult{}, nil
}

func (d *Deployer) getAllDomains(ctx context.Context) ([]string, error) {
	domains := make([]string, 0)

	listDomainConfigPageNumber := 1
	listDomainConfigPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listDomainConfigReq := &vedcdn.ListDomainConfigInput{
			ProjectName: lo.IfF(d.config.ProjectName != "", func() []*string { return ve.StringSlice([]string{d.config.ProjectName}) }).Else(nil),
			PageNumber:  ve.Int32(int32(listDomainConfigPageNumber)),
			PageSize:    ve.Int32(int32(listDomainConfigPageSize)),
		}
		listDomainConfigResp, err := d.sdkClient.ListDomainConfigWithContext(ctx, listDomainConfigReq)
		d.logger.Debug("sdk request 'dcdn.ListDomainConfig'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'dcdn.ListDomainConfig': %w", err)
		}

		ignoredStatuses := []string{"Stop"}
		for _, domainItem := range listDomainConfigResp.DomainList {
			if lo.Contains(ignoredStatuses, *domainItem.Status) {
				continue
			}

			domains = append(domains, *domainItem.Domain)
		}

		if len(listDomainConfigResp.DomainList) < listDomainConfigPageSize {
			break
		}

		listDomainConfigPageNumber++
	}

	return domains, nil
}

func createSDKClient(accessKeyId, secretAccessKey string) (*vedcdn.DCDN, error) {
	config := ve.NewConfig().
		WithAkSk(accessKeyId, secretAccessKey).
		WithRegion("cn-beijing")

	session, err := vesession.NewSession(config)
	if err != nil {
		return nil, err
	}

	client := vedcdn.New(session)
	return client, nil
}

func normalizeDomain(domain string) string {
	if strings.HasPrefix(domain, "*.") {
		return strings.TrimPrefix(domain, "*")
	}
	return domain
}
