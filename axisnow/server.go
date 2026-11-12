package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/core"
	"github.com/certimate-go/certimate/pkg/plugin"
	cmgrimpl "github.com/certimate-go/plugins/internal/axisnowcertmgr"
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
	config     *deploymentConfig
	logger     *slog.Logger
	sdkCertmgr core.Certmgr
}

func newDeployment(config *deploymentConfig, logger *slog.Logger) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		ApiToken: config.ApiToken,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}

	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	pcertmgr.SetLogger(logger)
	return &deployment{
		config:     config,
		logger:     logger,
		sdkCertmgr: pcertmgr,
	}, nil
}

func (d *deployment) deploy(ctx context.Context, certPEM, privkeyPEM string) (*plugin.DeployResult, error) {
	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "axisnow"
	accessType     = "axisnow"
	displayNameKey = "plugin.axisnow.name"
)

type axisnowDeployer struct{}

func (*axisnowDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
		AccessDisplayNameKey: "plugin.axisnow.name",
	}, nil
}

func (*axisnowDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
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

func (*axisnowDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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
