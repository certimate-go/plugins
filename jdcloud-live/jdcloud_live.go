package main

import (
	"context"
	"fmt"
	"log/slog"

	jdcore "github.com/jdcloud-api/jdcloud-sdk-go/core"
	jdliveapis "github.com/jdcloud-api/jdcloud-sdk-go/services/live/apis"
	"github.com/samber/lo"

	jdlive "github.com/certimate-go/certimate/pkg/sdk3rd-trimmed/github.com/jdcloud-api/jdcloud-sdk-go/services/live/client"

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
	sdkClient *jdlive.LiveClient
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
		d.logger.Info("no live domains to deploy")
	} else {
		d.logger.Info("found live domains to deploy", slog.Any("domains", domains))

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

	describeLiveDomainsPageNumber := 1
	describeLiveDomainsPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		describeLiveDomainsReq := jdliveapis.NewDescribeLiveDomainsRequestWithoutParam()
		describeLiveDomainsReq.SetPageNum(describeLiveDomainsPageNumber)
		describeLiveDomainsReq.SetPageSize(describeLiveDomainsPageSize)
		describeLiveDomainsResp, err := d.sdkClient.DescribeLiveDomains(describeLiveDomainsReq)
		if err == nil && describeLiveDomainsResp.Error.Code != 0 {
			err = fmt.Errorf("JDCloud API error %d (%s): %s", describeLiveDomainsResp.Error.Code, describeLiveDomainsResp.Error.Status, describeLiveDomainsResp.Error.Message)
		}
		d.logger.Debug("sdk request 'live.DescribeLiveDomainsRequest'", slog.Any("request", describeLiveDomainsReq), slog.Any("response", describeLiveDomainsResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'live.DescribeLiveDomainsRequest': %w", err)
		}

		ignoredStatuses := []string{"offline", "checking", "check_failed"}
		for _, domainItem := range describeLiveDomainsResp.Result.DomainDetails {
			for _, playDomainItem := range domainItem.PlayDomains {
				if lo.Contains(ignoredStatuses, playDomainItem.DomainStatus) {
					continue
				}

				domains = append(domains, playDomainItem.PlayDomain)
			}
		}

		if len(describeLiveDomainsResp.Result.DomainDetails) < describeLiveDomainsPageSize {
			break
		}

		describeLiveDomainsPageNumber++
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, domain string, certPEM, privkeyPEM string) error {
	setLiveDomainCertificateReq := jdliveapis.NewSetLiveDomainCertificateRequestWithoutParam()
	setLiveDomainCertificateReq.SetPlayDomain(domain)
	setLiveDomainCertificateReq.SetCertStatus("on")
	setLiveDomainCertificateReq.SetCert(certPEM)
	setLiveDomainCertificateReq.SetKey(privkeyPEM)
	setLiveDomainCertificateResp, err := d.sdkClient.SetLiveDomainCertificate(setLiveDomainCertificateReq)
	if err == nil && setLiveDomainCertificateResp.Error.Code != 0 {
		err = fmt.Errorf("JDCloud API error %d (%s): %s", setLiveDomainCertificateResp.Error.Code, setLiveDomainCertificateResp.Error.Status, setLiveDomainCertificateResp.Error.Message)
	}
	d.logger.Debug("sdk request 'live.SetLiveDomainCertificate'", slog.Any("response", setLiveDomainCertificateResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'live.SetLiveDomainCertificate': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, accessKeySecret string) (*jdlive.LiveClient, error) {
	clientCredentials := jdcore.NewCredentials(accessKeyId, accessKeySecret)
	client := jdlive.NewLiveClient(clientCredentials)
	client.DisableLogger()
	return client, nil
}
