package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	"github.com/certimate-go/certimate/pkg/plugin"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
	cmgrimpl "github.com/certimate-go/plugins/internal/asiaispcdncertmgr"
	asiaispcdnsdk "github.com/certimate-go/plugins/internal/asiaispcdnsdk"
)

type accessConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
}
type extendedConfig struct {
	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`
	Domain             string `json:"domain"`
}
type deploymentConfig struct {
	accessConfig
	extendedConfig
}

type deployment struct {
	config     *deploymentConfig
	logger     *slog.Logger
	sdkClient  *asiaispcdnsdk.Client
	sdkCertmgr core.Certmgr
}

func newDeployment(config *deploymentConfig, logger *slog.Logger) (*deployment, error) {
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

	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	pcertmgr.SetLogger(logger)
	return &deployment{
		config:     config,
		logger:     logger,
		sdkClient:  client,
		sdkCertmgr: pcertmgr,
	}, nil
}

func (d *deployment) deploy(ctx context.Context, certPEM, privkeyPEM string) (*plugin.DeployResult, error) {
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
			certId, _ := strconv.ParseInt(upres.CertId, 10, 64)
			return d.updateDomainCertificate(ctx, domain, certId)
		}); err != nil {
			return nil, err
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) getAllDomains(ctx context.Context) ([]string, error) {
	domains := make([]string, 0)
	domainQueryListReq := &asiaispcdnsdk.DomainQueryListRequest{}
	domainQueryListResp, err := d.sdkClient.DomainQueryListWithContext(ctx, domainQueryListReq)
	d.logger.Debug("sdk request 'domainQueryList'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'domainQueryList': %w", err)
	}

	for _, domainItem := range domainQueryListResp.Data {
		if domainItem.DomainStatus == 0 {
			continue
		}

		domains = append(domains, domainItem.Domain)
	}

	return domains, nil
}

func (d *deployment) updateDomainCertificate(ctx context.Context, domain string, cloudCertId int64) error {
	domainQueryListReq := &asiaispcdnsdk.DomainQueryListRequest{
		Domain: lo.ToPtr(domain),
	}
	domainQueryListResp, err := d.sdkClient.DomainQueryListWithContext(ctx, domainQueryListReq)
	d.logger.Debug("sdk request 'domainQueryList'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'domainQueryList': %w", err)
	} else {

		domainEntry, _ := lo.Find(domainQueryListResp.Data, func(domainInfo *asiaispcdnsdk.Domain) bool {
			return domainInfo.Domain == domain
		})
		if domainEntry == nil {
			return fmt.Errorf("could not find domain '%s'", domain)
		}

		if domainEntry.CertId == cloudCertId {
			return nil
		}

		if domainEntry.DomainStatus == 2 {
			if err := d.waitForDomainReady(ctx, domain); err != nil {
				return err
			}
		}
	}
	domainModifyReq := &asiaispcdnsdk.DomainModifyRequest{
		Domain:   lo.ToPtr(domain),
		Protocol: lo.ToPtr("https"),
		CertId:   lo.ToPtr(cloudCertId),
	}
	_, err = d.sdkClient.DomainModifyWithContext(ctx, domainModifyReq)
	d.logger.Debug("sdk request 'domainModify'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'domainModify': %w", err)
	}

	return nil
}

func (d *deployment) waitForDomainReady(ctx context.Context, domain string) error {
	if _, err := xwait.UntilWithContext(ctx, func(_ context.Context, _ int) (bool, error) {
		domainQueryListReq := &asiaispcdnsdk.DomainQueryListRequest{
			Domain: lo.ToPtr(domain),
		}
		domainQueryListResp, err := d.sdkClient.DomainQueryListWithContext(ctx, domainQueryListReq)
		d.logger.Debug("sdk request 'domainQueryList'")
		if err != nil {
			return false, fmt.Errorf("failed to execute sdk request 'domainQueryList': %w", err)
		}

		domainEntry, _ := lo.Find(domainQueryListResp.Data, func(domainInfo *asiaispcdnsdk.Domain) bool {
			return domainInfo.Domain == domain
		})
		if domainEntry == nil {
			return false, fmt.Errorf("could not find domain '%s'", domain)
		}

		if domainEntry.DomainStatus != 2 {
			return true, nil
		}

		d.logger.Info("waiting for domain's status to not be 'Operating' ...")
		return false, nil
	}, 10*time.Second); err != nil {
		return err
	}

	return nil
}

func createSDKClient(accessKeyId, accessKeySecret string) (*asiaispcdnsdk.Client, error) {
	client, err := asiaispcdnsdk.NewClient(
		asiaispcdnsdk.WithAkSk(accessKeyId, accessKeySecret),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

const (
	DOMAIN_MATCH_PATTERN_EXACT    = "exact"
	DOMAIN_MATCH_PATTERN_WILDCARD = "wildcard"
	DOMAIN_MATCH_PATTERN_CERTSAN  = "certsan"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "asiaispcdn"
	accessType     = "asiaispcdn"
	displayNameKey = "plugin.asiaispcdn.name"
)

type asiaispcdnDeployer struct{}

func (*asiaispcdnDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
		AccessDisplayNameKey: "plugin.asiaispcdn.name",
	}, nil
}

func (*asiaispcdnDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	accessSchema, err := plugin.LoadAccessSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{AccessSchemaJSON: accessSchema, DeploySchemaJSON: deploySchema, I18n: i18n}, nil
}

func (*asiaispcdnDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var extended extendedConfig
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}
	d, err := newDeployment(&deploymentConfig{accessConfig: access, extendedConfig: extended}, logger)
	if err != nil {
		return nil, err
	}
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
