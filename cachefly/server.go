package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	cacheflysdk "github.com/certimate-go/plugins/internal/cacheflysdk"
)

type accessConfig struct {
	ApiToken string `json:"apiToken"`
}
type (
	extendedConfig   struct{}
	deploymentConfig struct {
		accessConfig
		extendedConfig
	}
)

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *cacheflysdk.Client
}

func newDeployment(config *deploymentConfig, logger *slog.Logger) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ApiToken)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &deployment{
		config:    config,
		logger:    logger,
		sdkClient: client,
	}, nil
}

func (d *deployment) deploy(ctx context.Context, certPEM, privkeyPEM string) (*plugin.DeployResult, error) {
	createCertificateReq := &cacheflysdk.CreateCertificateRequest{
		Certificate:    lo.ToPtr(certPEM),
		CertificateKey: lo.ToPtr(privkeyPEM),
	}
	_, err := d.sdkClient.CreateCertificateWithContext(ctx, createCertificateReq)
	d.logger.Debug("sdk request 'CreateCertificate'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'CreateCertificate': %w", err)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(apiToken string) (*cacheflysdk.Client, error) {
	client, err := cacheflysdk.NewClient(
		cacheflysdk.WithApiToken(apiToken),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "cachefly"
	accessType     = "cachefly"
	displayNameKey = "plugin.cachefly.name"
)

type cacheflyDeployer struct{}

func (*cacheflyDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
		AccessDisplayNameKey: "plugin.cachefly.name",
	}, nil
}

func (*cacheflyDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	accessSchema, err := plugin.LoadAccessSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{AccessSchemaJSON: accessSchema, DeploySchemaJSON: deploySchema, I18n: i18n}, nil
}

func (*cacheflyDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var extended extendedConfig
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}
	d, err := newDeployment(&deploymentConfig{accessConfig: access, extendedConfig: extended}, logger)
	if err != nil {
		return nil, err
	}
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
