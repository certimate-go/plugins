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
	providerType = "volcengine-certcenter"
	accessType   = "volcengine"
)

type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{ProviderType: providerType, AccessProviderType: accessType, ProtocolVersion: plugin.ProtocolVersion, DeployCategory: "ssl", DeployDisplayNameKey: "plugin.volcengine-certcenter.name"}, nil
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
	AccessKeyId     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	ProjectName     string `json:"projectName,omitempty"`
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
		AccessKeyId:     access.AccessKeyId,
		SecretAccessKey: access.SecretAccessKey,
		ProjectName:     access.ProjectName,
		Region:          xmaps.GetString(extended, "region"),
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
