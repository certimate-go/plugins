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
	baishancertmgr "github.com/certimate-go/plugins/internal/baishancertmgr"
	baishansdk "github.com/certimate-go/plugins/internal/baishansdk"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "baishan-cdn"
	accessType     = "baishan"
	displayNameKey = "plugin.baishan-cdn.name"

	deployTargetDomain      = "domain"
	deployTargetCertificate = "certificate"

	domainMatchPatternExact = "exact"
)

type baishanCdnDeployer struct{}

func (*baishanCdnDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*baishanCdnDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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
	ApiToken string `json:"apiToken"`
}

type extendedConfig struct {
	DeployTarget       string `json:"deployTarget"`
	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`
	Domain             string `json:"domain"`
	CertificateId      string `json:"certificateId,omitempty"`
}

func (d *baishanCdnDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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

	sdkClient, err := createSDKClient(access.ApiToken)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	certmgr, err := baishancertmgr.NewCertmgr(&baishancertmgr.CertmgrConfig{
		ApiToken: access.ApiToken,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	runner := &cdnRunner{
		config:    &extended,
		logger:    logger,
		sdkClient: sdkClient,
		certmgr:   certmgr,
	}

	switch extended.DeployTarget {
	case deployTargetDomain:
		if err := runner.deployToDomain(ctx, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
			return nil, err
		}

	case deployTargetCertificate:
		if err := runner.deployToCertificate(ctx, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", extended.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

type cdnRunner struct {
	config    *extendedConfig
	logger    *slog.Logger
	sdkClient *baishansdk.Client
	certmgr   *baishancertmgr.Certmgr
}

func (d *cdnRunner) deployToDomain(ctx context.Context, certPEM, privkeyPEM string) error {
	domain := normalizeDomain(d.config.Domain)
	if domain == "" {
		return fmt.Errorf("config `domain` is required")
	}

	upres, err := d.certmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	getDomainConfigReq := &baishansdk.GetDomainConfigRequest{
		Domains: lo.ToPtr(domain),
		Config:  []*string{lo.ToPtr("https")},
	}
	getDomainConfigResp, err := d.sdkClient.GetDomainConfigWithContext(ctx, getDomainConfigReq)
	d.logger.Debug("sdk request 'cdn.GetDomainConfig'", slog.Any("request", getDomainConfigReq), slog.Any("response", getDomainConfigResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.GetDomainConfig': %w", err)
	} else if len(getDomainConfigResp.Data) == 0 {
		return fmt.Errorf("could not find domain '%s'", domain)
	}

	setDomainConfigReq := &baishansdk.SetDomainConfigRequest{
		Domains: lo.ToPtr(domain),
		Config: &baishansdk.DomainConfig{
			Https: &baishansdk.DomainConfigHttps{
				CertId:      json.Number(upres.CertId),
				ForceHttps:  getDomainConfigResp.Data[0].Config.Https.ForceHttps,
				EnableHttp2: getDomainConfigResp.Data[0].Config.Https.EnableHttp2,
				EnableOcsp:  getDomainConfigResp.Data[0].Config.Https.EnableOcsp,
			},
		},
	}
	setDomainConfigResp, err := d.sdkClient.SetDomainConfigWithContext(ctx, setDomainConfigReq)
	d.logger.Debug("sdk request 'cdn.SetDomainConfig'", slog.Any("request", setDomainConfigReq), slog.Any("response", setDomainConfigResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.SetDomainConfig': %w", err)
	}

	return nil
}

func (d *cdnRunner) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.CertificateId == "" {
		return fmt.Errorf("config `certificateId` is required")
	}

	rplres, err := d.certmgr.Replace(ctx, d.config.CertificateId, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to replace certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate replaced", slog.Any("result", rplres))
	}

	return nil
}

func createSDKClient(apiToken string) (*baishansdk.Client, error) {
	client, err := baishansdk.NewClient(
		baishansdk.WithApiToken(apiToken),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func normalizeDomain(domain string) string {
	if strings.HasPrefix(domain, "*.") {
		return strings.TrimPrefix(domain, "*")
	}
	return domain
}
