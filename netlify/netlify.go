package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	netlifysdk "github.com/certimate-go/plugins/internal/netlifysdk"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	ApiToken     string `json:"apiToken"`
	DeployTarget string `json:"deployTarget"`
	SiteId       string `json:"siteId,omitempty"`
}

type Deployer struct {
	config    *DeployerConfig
	logger    *slog.Logger
	sdkClient *netlifysdk.Client
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ApiToken)
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
	switch d.config.DeployTarget {
	case DEPLOY_TARGET_WEBSITE:
		if err := d.deployToWebsite(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &DeployResult{}, nil
}

func (d *Deployer) deployToWebsite(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.SiteId == "" {
		return fmt.Errorf("config `siteId` is required")
	}
	serverCertPEM, issuerCertPEM, err := xcert.ExtractCertificatesFromPEM(certPEM)
	if err != nil {
		return fmt.Errorf("failed to extract certs: %w", err)
	}
	provisionSiteTLSCertificateReq := &netlifysdk.ProvisionSiteTLSCertificateRequest{
		Certificate:    serverCertPEM,
		CACertificates: issuerCertPEM,
		Key:            privkeyPEM,
	}
	_, err = d.sdkClient.ProvisionSiteTLSCertificateWithContext(ctx, d.config.SiteId, provisionSiteTLSCertificateReq)
	d.logger.Debug("sdk request 'ProvisionSiteTLSCertificate'", slog.String("params.siteId", d.config.SiteId))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'ProvisionSiteTLSCertificate': %w", err)
	}

	return nil
}

func createSDKClient(apiToken string) (*netlifysdk.Client, error) {
	client, err := netlifysdk.NewClient(
		netlifysdk.WithApiToken(apiToken),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
