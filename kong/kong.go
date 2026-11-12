package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/samber/lo"

	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	kongsdk "github.com/certimate-go/plugins/internal/kongsdk"
)

type deploymentConfig struct {
	ServerUrl string `json:"serverUrl"`

	ApiToken string `json:"apiToken,omitempty"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`

	DeployTarget string `json:"deployTarget"`

	Workspace string `json:"workspace,omitempty"`

	CertificateId string `json:"certificateId,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *kongsdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.Workspace, config.ApiToken, config.AllowInsecureConnections)
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
	case DEPLOY_TARGET_CERTIFICATE:
		if err := d.deployToCertificate(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.CertificateId == "" {
		return fmt.Errorf("config `certificateId` is required")
	}

	certX509, err := xcert.ParseCertificateFromPEM(certPEM)
	if err != nil {
		return err
	}

	upsertCertificateReq := &kongsdk.UpsertCertificateRequest{
		Id:   lo.ToPtr(d.config.CertificateId),
		Cert: lo.ToPtr(certPEM),
		Key:  lo.ToPtr(privkeyPEM),
		SNIs: lo.ToSlicePtr(certX509.DNSNames),
	}
	_, err = d.sdkClient.UpsertCertificateWithContext(ctx, d.config.CertificateId, upsertCertificateReq)
	d.logger.Debug("sdk request 'UpsertCertificate'", slog.String("params.certificateId", d.config.CertificateId))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'UpsertCertificate': %w", err)
	}

	return nil
}

func createSDKClient(serverUrl, workspace, apiToken string, skipTlsVerify bool) (*kongsdk.Client, error) {
	client, err := kongsdk.NewClient(serverUrl,
		kongsdk.WithWorkspace(workspace),
		kongsdk.WithApiToken(apiToken),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, err
}
