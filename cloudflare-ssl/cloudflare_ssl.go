package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	cloudflaresdk "github.com/certimate-go/plugins/internal/cloudflaresdk"
)

type deploymentConfig struct {
	ApiToken      string `json:"apiToken"`
	Environment   string `json:"environment,omitempty"`
	ZoneId        string `json:"zoneId"`
	CertificateId string `json:"certificateId,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *cloudflaresdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ApiToken)
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
	if d.config.ZoneId == "" {
		return nil, fmt.Errorf("config `zoneId` is required")
	}

	if d.config.CertificateId == "" {
		customCertificateCreateReq := &cloudflaresdk.CustomCertificateCreateRequest{
			ZoneId:       d.config.ZoneId,
			Certificate:  lo.ToPtr(certPEM),
			PrivateKey:   lo.ToPtr(privkeyPEM),
			BundleMethod: lo.ToPtr("ubiquitous"),
			Deploy:       lo.EmptyableToPtr(d.config.Environment),
		}
		_, err := d.sdkClient.CustomCertificateCreateWithContext(ctx, customCertificateCreateReq)
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'CustomCertificates.Create': %w", err)
		}
	} else {
		customCertificateEditReq := &cloudflaresdk.CustomCertificateEditRequest{
			ZoneId:        d.config.ZoneId,
			CertificateId: d.config.CertificateId,
			Certificate:   lo.ToPtr(certPEM),
			PrivateKey:    lo.ToPtr(privkeyPEM),
			BundleMethod:  lo.ToPtr("ubiquitous"),
			Deploy:        lo.EmptyableToPtr(d.config.Environment),
		}
		_, err := d.sdkClient.CustomCertificateEditWithContext(ctx, customCertificateEditReq)
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'CustomCertificates.Edit': %w", err)
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(apiToken string) (*cloudflaresdk.Client, error) {
	client, err := cloudflaresdk.NewClient(
		cloudflaresdk.WithApiToken(apiToken),
	)
	if err != nil {
		return nil, err
	}

	return client, err
}
