package main

import (
	"context"
	"embed"
	"fmt"
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

type myDeployer struct{}

func (*myDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "other",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*myDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	accessSchema, _ := plugin.LoadAccessSchema(schemaFS)
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{
		AccessSchemaJSON: accessSchema,
		DeploySchemaJSON: deploySchema,
		I18n:             i18n,
	}, nil
}

func (*myDeployer) Deploy(_ context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	if logger != nil {
		logger.Info("template deployer invoked")
	}
	return nil, fmt.Errorf("not implemented: implement the Deploy method")
}
