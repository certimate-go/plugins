package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/samber/lo"

	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	apisixsdk "github.com/certimate-go/plugins/internal/apisixsdk"
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
	sdkClient *apisixsdk.Client
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

	certX509, err := xcert.ParseCertificateFromPEM(certPEM)
	if err != nil {
		return err
	}

	updateSSLReq := &apisixsdk.UpdateSSLRequest{
		ID:          lo.ToPtr(d.config.CertificateId),
		Certificate: lo.ToPtr(certPEM),
		PrivateKey:  lo.ToPtr(privkeyPEM),
		SNIs:        lo.ToSlicePtr(certX509.DNSNames),
		Type:        lo.ToPtr("server"),
		Status:      lo.ToPtr(int32(1)),
	}
	_, err = d.sdkClient.UpdateSSLWithContext(ctx, d.config.CertificateId, updateSSLReq)
	d.logger.Debug("sdk request 'UpdateSSL'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'UpdateSSL': %w", err)
	}

	return nil
}

func createSDKClient(serverUrl, apiKey string, skipTlsVerify bool) (*apisixsdk.Client, error) {
	client, err := apisixsdk.NewClient(serverUrl,
		apisixsdk.WithApiKey(apiKey),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
