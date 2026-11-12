package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	cpanelsdk "github.com/certimate-go/plugins/internal/cpanelsdk"
)

type deploymentConfig struct {
	ServerUrl                string `json:"serverUrl"`
	Username                 string `json:"username"`
	ApiToken                 string `json:"apiToken"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
	DeployTarget             string `json:"deployTarget"`
	Domain                   string `json:"domain,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *cpanelsdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.Username, config.ApiToken, config.AllowInsecureConnections)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return &deployment{
		config:    config,
		logger:    slog.Default(),
		sdkClient: client,
	}, nil
}

func (d *deployment) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}
}

func (d *deployment) deploy(ctx context.Context, certPEM, privkeyPEM string) (*plugin.DeployResult, error) {
	switch d.config.DeployTarget {
	case DEPLOY_TARGET_WEBSITE:
		if err := d.deployToWebsite(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) deployToWebsite(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.Domain == "" {
		return fmt.Errorf("config `domain` is required")
	}
	serverCertPEM, issuerCertPEM, err := xcert.ExtractCertificatesFromPEM(certPEM)
	if err != nil {
		return fmt.Errorf("failed to extract certs: %w", err)
	}
	sslInstallSSLReq := &cpanelsdk.SSLInstallSSLRequest{
		Domain:   lo.ToPtr(d.config.Domain),
		Cert:     lo.ToPtr(serverCertPEM),
		Key:      lo.ToPtr(privkeyPEM),
		CABundle: lo.ToPtr(issuerCertPEM),
	}
	_, err = d.sdkClient.SSLInstallSSLWithContext(ctx, sslInstallSSLReq)
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'SSL.install_ssl': %w", err)
	}

	return nil
}

func createSDKClient(serverUrl, username, apiToken string, skipTlsVerify bool) (*cpanelsdk.Client, error) {
	client, err := cpanelsdk.NewClient(serverUrl,
		cpanelsdk.WithUsername(username),
		cpanelsdk.WithApiToken(apiToken),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
