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

	AutoRestart bool `json:"autoRestart"`
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
	vipConfigUploadSslCertReq := &samwafsdk.VipConfigUploadSslCertRequest{
		CertContent: certPEM,
		KeyContent:  privkeyPEM,
	}
	_, err := d.sdkClient.VipConfigUploadSslCertWithContext(ctx, vipConfigUploadSslCertReq)
	d.logger.Debug("sdk request 'vipconfig.UploadSslCert'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'vipconfig.UploadSslCert': %w", err)
	}

	vipConfigUpdateSslEnableReq := &samwafsdk.VipConfigUpdateSslEnableRequest{
		SslEnable: true,
	}
	_, err = d.sdkClient.VipConfigUpdateSslEnableWithContext(ctx, vipConfigUpdateSslEnableReq)
	d.logger.Debug("sdk request 'vipconfig.UpdateSslEnable'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'vipconfig.UpdateSslEnable': %w", err)
	}

	if d.config.AutoRestart {

		_, err = d.sdkClient.VipConfigRestartManagerWithContext(ctx)
		d.logger.Debug("sdk request 'vipconfig.RestartManager'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'vipconfig.RestartManager': %w", err)
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
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
