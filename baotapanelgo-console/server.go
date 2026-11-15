package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
)

type accessConfig struct {
	ServerUrl                string `json:"serverUrl"`
	ApiKey                   string `json:"apiKey"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}

//go:embed schema
var schemaFS embed.FS

const (
	providerType = "baotapanelgo-console"
	accessType   = "baotapanelgo"
)

type pluginDeployer struct {
	plugin.NoopFinalizer
}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType: providerType, AccessProviderType: accessType, ProtocolVersion: plugin.ProtocolVersion,
		Capability: plugin.CapabilityDeploy, Category: "other", DisplayNameKey: "plugin.baotapanelgo-console.name", AccessDisplayNameKey: "plugin.baotapanelgo.access",
	}, nil
}

func (*pluginDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
	deploy, err := plugin.LoadProviderSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	access, err := plugin.LoadAccessSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{ProviderSchemaJSON: deploy, AccessSchemaJSON: access, I18n: i18n}, nil
}

func (*pluginDeployer) Process(ctx context.Context, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error) {
	var rawAccess map[string]any
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &rawAccess); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var access accessConfig
	if err := xmaps.Populate(rawAccess, &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var extended map[string]any
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}
	d, err := newDeployment(&deploymentConfig{
		ServerUrl:                access.ServerUrl,
		ApiKey:                   access.ApiKey,
		AllowInsecureConnections: access.AllowInsecureConnections,
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	return d.deploy(ctx, req.Deploy.CertificatePEM, req.Deploy.PrivateKeyPEM)
}
