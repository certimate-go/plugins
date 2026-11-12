package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/samber/lo"

	btpanelgosdk "github.com/certimate-go/plugins/internal/btpanelgosdk"
)

type deploymentConfig struct {
	ServerUrl string `json:"serverUrl"`

	ApiKey string `json:"apiKey"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *btpanelgosdk.Client
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
	configSetPanelSSLReq := &btpanelgosdk.ConfigSetPanelSSLRequest{
		SSLStatus: lo.ToPtr(int32(1)),
		SSLKey:    lo.ToPtr(privkeyPEM),
		SSLPem:    lo.ToPtr(certPEM),
	}
	_, err := d.sdkClient.ConfigSetPanelSSLWithContext(ctx, configSetPanelSSLReq)
	d.logger.Debug("sdk request 'config.SetPanelSSL'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'config.SetPanelSSL': %w", err)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(serverUrl, apiKey string, skipTlsVerify bool) (*btpanelgosdk.Client, error) {
	client, err := btpanelgosdk.NewClient(serverUrl,
		btpanelgosdk.WithApiKey(apiKey),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
