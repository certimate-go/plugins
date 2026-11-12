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

//go:embed schema
var schemaFS embed.FS

const (
	providerType = "ucloud-pathx"
	accessType   = "ucloud"
)

type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{ProviderType: providerType, AccessProviderType: accessType, ProtocolVersion: plugin.ProtocolVersion, DeployCategory: "accelerator", DeployDisplayNameKey: "plugin.ucloud-pathx.name"}, nil
}

func (*pluginDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
	deploy, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{DeploySchemaJSON: deploy, I18n: i18n}, nil
}

type accessConfig struct {
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey"`
	ProjectId  string `json:"projectId"`
}

func (*pluginDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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
	d, err := NewDeployer(&DeployerConfig{
		PublicKey:     access.PublicKey,
		PrivateKey:    access.PrivateKey,
		ProjectId:     access.ProjectId,
		Endpoint:      xmaps.GetString(extended, "endpoint"),
		AcceleratorId: xmaps.GetString(extended, "acceleratorId"),
		ListenerPort:  xmaps.GetInt32(extended, "listenerPort"),
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	if _, err := d.Deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
		return nil, err
	}
	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}
