package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	upyuncertmgr "github.com/certimate-go/plugins/internal/upyuncertmgr"
	upyunsdk "github.com/certimate-go/plugins/internal/upyunsdk"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "upyun-cdn"
	accessType     = "upyun"
	displayNameKey = "plugin.upyun-cdn.name"

	domainMatchPatternExact    = "exact"
	domainMatchPatternWildcard = "wildcard"
	domainMatchPatternCertsan  = "certsan"
)

type upyunCdnDeployer struct{}

func (*upyunCdnDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*upyunCdnDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	accessSchema, _ := plugin.LoadAccessSchema(schemaFS)
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{
		AccessSchemaJSON: accessSchema,
		DeploySchemaJSON: deploySchema,
		I18n:             i18n,
	}, nil
}

type accessConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type extendedConfig struct {
	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`
	Domain             string `json:"domain"`
}

func (d *upyunCdnDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}

	var extended extendedConfig
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}

	certmgr, err := upyuncertmgr.NewCertmgr(&upyuncertmgr.CertmgrConfig{
		Username: access.Username,
		Password: access.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	sdkClient, err := createSDKClient(access.Username, access.Password)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	runner := &cdnRunner{
		config:     &extended,
		logger:     logger,
		sdkClient:  sdkClient,
		sdkCertmgr: certmgr,
	}

	if err := runner.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
		return nil, err
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

type cdnRunner struct {
	config     *extendedConfig
	logger     *slog.Logger
	sdkClient  *upyunsdk.Client
	sdkCertmgr *upyuncertmgr.Certmgr
}

func (d *cdnRunner) deploy(ctx context.Context, certPEM, privkeyPEM string) error {
	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	var domains []string
	switch d.config.DomainMatchPattern {
	case "", domainMatchPatternExact:
		{
			if d.config.Domain == "" {
				return fmt.Errorf("config `domain` is required")
			}

			domains = []string{d.config.Domain}
		}

	case domainMatchPatternWildcard:
		{
			if d.config.Domain == "" {
				return fmt.Errorf("config `domain` is required")
			}

			if strings.HasPrefix(d.config.Domain, "*.") {
				domainCandidates, err := d.getAllDomains(ctx)
				if err != nil {
					return err
				}

				domains = lo.Filter(domainCandidates, func(domain string, _ int) bool {
					return xcerthostname.IsMatch(d.config.Domain, domain)
				})
				if len(domains) == 0 {
					return fmt.Errorf("could not find any domains matched by wildcard")
				}
			} else {
				domains = []string{d.config.Domain}
			}
		}

	case domainMatchPatternCertsan:
		{
			domainCandidates, err := d.getAllDomains(ctx)
			if err != nil {
				return err
			}

			domains = lo.Filter(domainCandidates, func(domain string, _ int) bool {
				return xcerthostname.IsMatchByCertificatePEM(certPEM, domain)
			})
			if len(domains) == 0 {
				return fmt.Errorf("could not find any domains matched by certificate")
			}
		}

	default:
		return fmt.Errorf("unsupported domain match pattern: '%s'", d.config.DomainMatchPattern)
	}

	if len(domains) == 0 {
		d.logger.Info("no cdn domains to deploy")
	} else {
		d.logger.Info("found cdn domains to deploy", slog.Any("domains", domains))

		if err := xloop.ForRangeAllWithContext(ctx, domains, func(ctx context.Context, domain string, _ int) error {
			return d.updateDomainCertificate(ctx, domain, upres.CertId)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (d *cdnRunner) getAllDomains(ctx context.Context) ([]string, error) {
	domains := make([]string, 0)

	getBucketsPage := 1
	getBucketsPerPage := 10
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		getBucketsReq := &upyunsdk.GetBucketsRequest{
			Type:          "ucdn",
			Tag:           "all",
			Status:        "all",
			IsSecurityCDN: false,
			WithDomains:   true,
			Page:          int32(getBucketsPage),
			PerPage:       int32(getBucketsPerPage),
		}
		getBucketsResp, err := d.sdkClient.GetBucketsWithContext(ctx, getBucketsReq)
		d.logger.Debug("sdk request 'console.GetBuckets'", slog.Any("request", getBucketsReq), slog.Any("response", getBucketsResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'console.GetBuckets': %w", err)
		}

		if getBucketsResp.Data == nil {
			break
		}

		for _, bucketItem := range getBucketsResp.Data.Buckets {
			if !bucketItem.Visible {
				continue
			}

			for _, domainItem := range bucketItem.Domains {
				if strings.EqualFold(domainItem.Status, "NORMAL") && !strings.HasSuffix(domainItem.Domain, ".test.upcdn.net") {
					domains = append(domains, domainItem.Domain)
				}
			}
		}

		if len(getBucketsResp.Data.Buckets) < getBucketsPerPage {
			break
		}

		getBucketsPage++
	}

	return domains, nil
}

func (d *cdnRunner) updateDomainCertificate(ctx context.Context, domain string, cloudCertId string) error {
	getHttpsServiceManagerResp, err := d.sdkClient.GetHttpsServiceManagerWithContext(ctx, domain)
	d.logger.Debug("sdk request 'console.GetHttpsServiceManager'", slog.String("params.domain", domain), slog.Any("response", getHttpsServiceManagerResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'console.GetHttpsServiceManager': %w", err)
	}

	_, lastCertIndex, _ := lo.FindIndexOf(getHttpsServiceManagerResp.Data.Domains, func(item upyunsdk.HttpsServiceManagerDomain) bool {
		return item.Https
	})
	if lastCertIndex == -1 {
		updateHttpsCertificateManagerReq := &upyunsdk.UpdateHttpsCertificateManagerRequest{
			CertificateId: cloudCertId,
			Domain:        domain,
			Https:         true,
			ForceHttps:    true,
		}
		updateHttpsCertificateManagerResp, err := d.sdkClient.UpdateHttpsCertificateManagerWithContext(ctx, updateHttpsCertificateManagerReq)
		d.logger.Debug("sdk request 'console.EnableDomainHttps'", slog.Any("request", updateHttpsCertificateManagerReq), slog.Any("response", updateHttpsCertificateManagerResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'console.UpdateHttpsCertificateManager': %w", err)
		}
	} else if getHttpsServiceManagerResp.Data.Domains[lastCertIndex].CertificateId != cloudCertId {
		migrateHttpsDomainReq := &upyunsdk.MigrateHttpsDomainRequest{
			CertificateId: cloudCertId,
			Domain:        domain,
		}
		migrateHttpsDomainResp, err := d.sdkClient.MigrateHttpsDomainWithContext(ctx, migrateHttpsDomainReq)
		d.logger.Debug("sdk request 'console.MigrateHttpsDomain'", slog.Any("request", migrateHttpsDomainReq), slog.Any("response", migrateHttpsDomainResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'console.MigrateHttpsDomain': %w", err)
		}
	}

	return nil
}

func createSDKClient(username, password string) (*upyunsdk.Client, error) {
	client, err := upyunsdk.NewClient(
		upyunsdk.WithLogins(username, password),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
