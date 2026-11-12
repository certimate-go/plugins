package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"

	samwafsdk "github.com/certimate-go/plugins/internal/samwafsdk"
)

type deploymentConfig struct {
	ServerUrl string `json:"serverUrl"`

	ApiKey string `json:"apiKey"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`

	DeployTarget string `json:"deployTarget"`

	CertificateId string `json:"certificateId,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *samwafsdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiKey, config.AllowInsecureConnections)
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

	sslConfigDetailResp, err := d.sdkClient.SslConfigDetailWithContext(ctx, d.config.CertificateId)
	d.logger.Debug("sdk request 'sslconfig.Detail'", slog.Any("params.sslId", d.config.CertificateId))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'sslconfig.Detail': %w", err)
	} else if sslConfigDetailResp.Data == nil || sslConfigDetailResp.Data.Id == "" {
		return fmt.Errorf("could not find ssl config: '%s'", d.config.CertificateId)
	}

	sslConfigEditReq := &samwafsdk.SslConfigEditRequest{
		Id:          d.config.CertificateId,
		CertContent: certPEM,
		KeyContent:  privkeyPEM,
	}
	_, err = d.sdkClient.SslConfigEditWithContext(ctx, sslConfigEditReq)
	d.logger.Debug("sdk request 'sslconfig.Edit'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'sslconfig.Edit': %w", err)
	}

	return nil
}

func createSDKClient(serverUrl, apiKey string, skipTlsVerify bool) (*samwafsdk.Client, error) {
	client, err := samwafsdk.NewClient(serverUrl,
		samwafsdk.WithApiKey(apiKey),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
