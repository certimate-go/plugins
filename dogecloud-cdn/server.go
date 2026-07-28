package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	dogecloudcertmgr "github.com/certimate-go/plugins/internal/dogecloudcertmgr"
	dogecloudsdk "github.com/certimate-go/plugins/internal/dogecloudsdk"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "dogecloud-cdn"
	accessType     = "dogecloud"
	displayNameKey = "plugin.dogecloud-cdn.name"

	domainMatchPatternExact   = "exact"
	domainMatchPatternCertsan = "certsan"
)

type dogeCloudCdnDeployer struct{}

func (*dogeCloudCdnDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*dogeCloudCdnDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

type extendedConfig struct {
	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`
	Domain             string `json:"domain"`
}

func (d *dogeCloudCdnDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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

	certmgr, err := dogecloudcertmgr.NewCertmgr(&dogecloudcertmgr.CertmgrConfig{
		AccessKey: access.AccessKey,
		SecretKey: access.SecretKey,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	sdkClient, err := createSDKClient(access.AccessKey, access.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	upres, err := certmgr.Upload(ctx, req.CertificatePEM, req.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	}
	logger.Info("ssl certificate uploaded", slog.Any("result", upres))

	certId, _ := strconv.ParseInt(upres.CertId, 10, 64)

	var domains []string
	switch extended.DomainMatchPattern {
	case "", domainMatchPatternExact:
		if extended.Domain == "" {
			return nil, fmt.Errorf("config `domain` is required")
		}
		domains = []string{extended.Domain}

	case domainMatchPatternCertsan:
		domainCandidates, err := getAllCdnDomains(ctx, sdkClient, logger)
		if err != nil {
			return nil, err
		}
		domains = lo.Filter(domainCandidates, func(domain string, _ int) bool {
			return xcerthostname.IsMatchByCertificatePEM(req.CertificatePEM, domain)
		})
		if len(domains) == 0 {
			return nil, fmt.Errorf("could not find any domains matched by certificate")
		}

	default:
		return nil, fmt.Errorf("unsupported domain match pattern: '%s'", extended.DomainMatchPattern)
	}

	if len(domains) == 0 {
		logger.Info("no cdn domains to deploy")
	} else {
		logger.Info("found cdn domains to deploy", slog.Any("domains", domains))
		if err := xloop.ForRangeAllWithContext(ctx, domains, func(ctx context.Context, domain string, _ int) error {
			return bindCdnCert(ctx, sdkClient, logger, domain, certId)
		}); err != nil {
			return nil, err
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(accessKey, secretKey string) (*dogecloudsdk.Client, error) {
	return dogecloudsdk.NewClient(dogecloudsdk.WithAkSk(accessKey, secretKey))
}

func getAllCdnDomains(ctx context.Context, c *dogecloudsdk.Client, logger *slog.Logger) ([]string, error) {
	domains := make([]string, 0)

	resp, err := c.ListCdnDomainWithContext(ctx)
	logger.Debug("sdk request 'cdn.ListCdnDomain'", slog.Any("response", resp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cdn.ListCdnDomain': %w", err)
	}

	ignoredStatuses := []string{"offline"}
	if resp.Data != nil {
		for _, item := range resp.Data.Domains {
			if lo.Contains(ignoredStatuses, item.Status) {
				continue
			}
			domains = append(domains, item.Name)
		}
	}

	return domains, nil
}

func bindCdnCert(ctx context.Context, c *dogecloudsdk.Client, logger *slog.Logger, domain string, cloudCertId int64) error {
	req := &dogecloudsdk.BindCdnCertRequest{
		CertId: cloudCertId,
		Domain: domain,
	}
	resp, err := c.BindCdnCertWithContext(ctx, req)
	logger.Debug("sdk request 'cdn.BindCdnCert'", slog.Any("request", req), slog.Any("response", resp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.BindCdnCert': %w", err)
	}
	return nil
}
