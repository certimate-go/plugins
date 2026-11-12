package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
	bunnysdk "github.com/certimate-go/plugins/internal/bunnysdk"
)

type deploymentConfig struct {
	ApiKey     string `json:"apiKey"`
	PullZoneId string `json:"pullZoneId"`
	Hostname   string `json:"hostname"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *bunnysdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ApiKey)
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
	if d.config.PullZoneId == "" {
		return nil, fmt.Errorf("config `pullZoneId` is required")
	}
	if d.config.Hostname == "" {
		return nil, fmt.Errorf("config `hostname` is required")
	}
	createCertificateReq := &bunnysdk.AddCustomCertificateRequest{
		Hostname:       d.config.Hostname,
		Certificate:    base64.StdEncoding.EncodeToString([]byte(certPEM)),
		CertificateKey: base64.StdEncoding.EncodeToString([]byte(privkeyPEM)),
	}
	err := d.sdkClient.AddCustomCertificateWithContext(ctx, d.config.PullZoneId, createCertificateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cdn.AddCustomCertificate': %w", err)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(apiKey string) (*bunnysdk.Client, error) {
	client, err := bunnysdk.NewClient(
		bunnysdk.WithApiKey(apiKey),
	)
	if err != nil {
		return nil, err
	}

	return client, err
}
