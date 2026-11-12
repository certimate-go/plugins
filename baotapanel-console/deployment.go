package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"

	btpanelsdk "github.com/certimate-go/plugins/internal/btpanelsdk"
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
	sdkClient *btpanelsdk.Client
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
	configSavePanelSSLReq := &btpanelsdk.ConfigSavePanelSSLRequest{
		PrivateKey:  privkeyPEM,
		Certificate: certPEM,
	}
	_, err := d.sdkClient.ConfigSavePanelSSLWithContext(ctx, configSavePanelSSLReq)
	d.logger.Debug("sdk request 'config.SavePanelSSL'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'config.SavePanelSSL': %w", err)
	}

	if d.config.AutoRestart {

		systemServiceAdminReq := &btpanelsdk.SystemServiceAdminRequest{
			Name: "nginx",
			Type: "restart",
		}
		_, _ = d.sdkClient.SystemServiceAdminWithContext(ctx, systemServiceAdminReq)
		d.logger.Debug("sdk request 'system.ServiceAdmin'")
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(serverUrl, apiKey string, skipTlsVerify bool) (*btpanelsdk.Client, error) {
	client, err := btpanelsdk.NewClient(serverUrl,
		btpanelsdk.WithApiKey(apiKey),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
