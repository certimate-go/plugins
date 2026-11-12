package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"

	safelinesdk "github.com/certimate-go/plugins/internal/safelinesdk"
)

type deploymentConfig struct {
	ServerUrl string `json:"serverUrl"`

	ApiToken string `json:"apiToken"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`

	DeployTarget string `json:"deployTarget"`

	CertificateId int64 `json:"certificateId,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *safelinesdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiToken, config.AllowInsecureConnections)
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
	if d.config.CertificateId == 0 {
		return fmt.Errorf("config `certificateId` is required")
	}

	updateCertificateReq := &safelinesdk.UpdateCertificateRequest{
		Id:   d.config.CertificateId,
		Type: 2,
		Manual: &safelinesdk.CertificateManul{
			Crt: certPEM,
			Key: privkeyPEM,
		},
	}
	_, err := d.sdkClient.UpdateCertificateWithContext(ctx, updateCertificateReq)
	d.logger.Debug("sdk request 'open.UpdateCertificate'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'open.UpdateCertificate': %w", err)
	}

	return nil
}

func createSDKClient(serverUrl, apiToken string, skipTlsVerify bool) (*safelinesdk.Client, error) {
	client, err := safelinesdk.NewClient(serverUrl,
		safelinesdk.WithApiToken(apiToken),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
