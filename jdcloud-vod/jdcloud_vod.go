package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	jdcore "github.com/jdcloud-api/jdcloud-sdk-go/core"
	jdvodapis "github.com/jdcloud-api/jdcloud-sdk-go/services/vod/apis"
	"github.com/samber/lo"

	jdvod "github.com/certimate-go/plugins/internal/cloudsdk/jdcloud/vod/client"

	"github.com/certimate-go/certimate/pkg/core"
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
	sdkClient *jdvod.VodClient
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
	var domains []string
	switch d.config.DomainMatchPattern {
	case "", DOMAIN_MATCH_PATTERN_EXACT:
		{
			if d.config.Domain == "" {
				return nil, fmt.Errorf("config `domain` is required")
			}

			domains = []string{d.config.Domain}
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
			return d.updateDomainCertificate(ctx, domain, certPEM, privkeyPEM)
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

		listDomainsReq := jdvodapis.NewListDomainsRequestWithoutParam()
		listDomainsReq.SetPageNumber(listDomainsPageNumber)
		listDomainsReq.SetPageSize(listDomainsPageSize)
		listDomainsResp, err := d.sdkClient.ListDomains(listDomainsReq)
		if err == nil && listDomainsResp.Error.Code != 0 {
			err = fmt.Errorf("JDCloud API error %d (%s): %s", listDomainsResp.Error.Code, listDomainsResp.Error.Status, listDomainsResp.Error.Message)
		}
		d.logger.Debug("sdk request 'vod.ListDomains'", slog.Any("request", listDomainsReq), slog.Any("response", listDomainsResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'vod.ListDomains': %w", err)
		}

		ignoredStatuses := []string{"init", "stopped"}
		for _, domainItem := range listDomainsResp.Result.Content {
			if lo.Contains(ignoredStatuses, domainItem.Status) {
				continue
			}

			domains = append(domains, domainItem.Name)
		}

		if len(listDomainsResp.Result.Content) < listDomainsPageSize {
			break
		}

		listDomainsPageNumber++
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, domain string, certPEM, privkeyPEM string) error {
	domainId, err := d.findDomainIdByDomain(ctx, domain)
	if err != nil {
		return err
	}

	getHttpSslReq := jdvodapis.NewGetHttpSslRequestWithoutParam()
	getHttpSslReq.SetDomainId(domainId)
	getHttpSslResp, err := d.sdkClient.GetHttpSsl(getHttpSslReq)
	if err == nil && getHttpSslResp.Error.Code != 0 {
		err = fmt.Errorf("JDCloud API error %d (%s): %s", getHttpSslResp.Error.Code, getHttpSslResp.Error.Status, getHttpSslResp.Error.Message)
	}
	d.logger.Debug("sdk request 'vod.GetHttpSsl'", slog.Any("request", getHttpSslReq))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'vod.GetHttpSsl': %w", err)
	}

	setHttpSslReq := jdvodapis.NewSetHttpSslRequestWithoutParam()
	setHttpSslReq.SetDomainId(domainId)
	setHttpSslReq.SetTitle(fmt.Sprintf("certimate-%d", time.Now().UnixMilli()))
	setHttpSslReq.SetSslCert(certPEM)
	setHttpSslReq.SetSslKey(privkeyPEM)
	setHttpSslReq.SetSource("default")
	setHttpSslReq.SetJumpType(getHttpSslResp.Result.JumpType)
	setHttpSslReq.SetEnabled(true)
	setHttpSslResp, err := d.sdkClient.SetHttpSsl(setHttpSslReq)
	if err == nil && setHttpSslResp.Error.Code != 0 {
		err = fmt.Errorf("JDCloud API error %d (%s): %s", setHttpSslResp.Error.Code, setHttpSslResp.Error.Status, setHttpSslResp.Error.Message)
	}
	d.logger.Debug("sdk request 'vod.SetHttpSsl'", slog.Any("response", setHttpSslResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'vod.SetHttpSsl': %w", err)
	}

	return nil
}

func (d *Deployer) findDomainIdByDomain(ctx context.Context, domain string) (int, error) {
	listDomainsPageNumber := 1
	listDomainsPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		listDomainsReq := jdvodapis.NewListDomainsRequestWithoutParam()
		listDomainsReq.SetPageNumber(listDomainsPageNumber)
		listDomainsReq.SetPageSize(listDomainsPageSize)
		listDomainsResp, err := d.sdkClient.ListDomains(listDomainsReq)
		if err == nil && listDomainsResp.Error.Code != 0 {
			err = fmt.Errorf("JDCloud API error %d (%s): %s", listDomainsResp.Error.Code, listDomainsResp.Error.Status, listDomainsResp.Error.Message)
		}
		d.logger.Debug("sdk request 'vod.ListDomains'", slog.Any("request", listDomainsReq), slog.Any("response", listDomainsResp))
		if err != nil {
			return 0, fmt.Errorf("failed to execute sdk request 'vod.ListDomains': %w", err)
		}

		for _, domainItem := range listDomainsResp.Result.Content {
			if domainItem.Name == domain {
				domainId, _ := strconv.Atoi(domainItem.Id)
				return domainId, nil
			}
		}

		if len(listDomainsResp.Result.Content) < listDomainsPageSize {
			break
		}

		listDomainsPageNumber++
	}

	return 0, fmt.Errorf("could not find domain '%s'", domain)
}

func createSDKClient(accessKeyId, accessKeySecret string) (*jdvod.VodClient, error) {
	clientCredentials := jdcore.NewCredentials(accessKeyId, accessKeySecret)
	client := jdvod.NewVodClient(clientCredentials)
	client.DisableLogger()
	return client, nil
}
