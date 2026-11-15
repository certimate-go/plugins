package main

import (
	"context"
	"embed"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "__PLUGIN_NAME__"
	accessType     = "__ACCESS_TYPE__"
	displayNameKey = "plugin.__PLUGIN_NAME__.name"
)

type myDeployer struct {
	plugin.NoopFinalizer
}

func (*myDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:       providerType,
		AccessProviderType: accessType,
		ProtocolVersion:    plugin.ProtocolVersion,
		Capability:         plugin.CapabilityDeploy,
		Category:           "other",
		DisplayNameKey:     displayNameKey,
	}, nil
}

func (*myDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadProviderSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	accessSchema, _ := plugin.LoadAccessSchema(schemaFS)
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{
		AccessSchemaJSON:   accessSchema,
		ProviderSchemaJSON: deploySchema,
		I18n:               i18n,
	}, nil
}

func (*myDeployer) Process(ctx context.Context, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error) {
	if logger != nil {
		logger.Info("template deployer invoked", slog.String("execution", req.ExecutionID))
	}
	return &plugin.Result{Deploy: &plugin.DeployResult{}}, nil
}
