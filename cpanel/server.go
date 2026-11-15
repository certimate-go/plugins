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
	Username                 string `json:"username"`
	ApiToken                 string `json:"apiToken"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "cpanel"
	accessType     = "cpanel"
	displayNameKey = "plugin.cpanel.name"
)

type pluginDeployer struct {
	plugin.NoopFinalizer
}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		Capability:           plugin.CapabilityDeploy,
		Category:             "website",
		DisplayNameKey:       displayNameKey,
		AccessDisplayNameKey: "provider.cpanel",
	}, nil
}

func (*pluginDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadProviderSchema(schemaFS)
	if err != nil {
		return nil, err
	}

	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{ProviderSchemaJSON: deploySchema, I18n: i18n}, nil
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
		Username:                 access.Username,
		ApiToken:                 access.ApiToken,
		AllowInsecureConnections: access.AllowInsecureConnections,
		DeployTarget:             xmaps.GetString(extended, "deployTarget"),
		Domain:                   xmaps.GetString(extended, "domain"),
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	return d.deploy(ctx, req.Deploy.CertificatePEM, req.Deploy.PrivateKeyPEM)
}
