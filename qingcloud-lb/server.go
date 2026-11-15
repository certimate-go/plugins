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
	providerType = "qingcloud-lb"
	accessType   = "qingcloud"
)

type pluginDeployer struct {
	plugin.NoopFinalizer
}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType: providerType, AccessProviderType: accessType, ProtocolVersion: plugin.ProtocolVersion,
		Capability: plugin.CapabilityDeploy, Category: "loadbalance", DisplayNameKey: "plugin.qingcloud-lb.name",
	}, nil
}

func (*pluginDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
	deploy, err := plugin.LoadProviderSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{ProviderSchemaJSON: deploy, I18n: i18n}, nil
}

type accessConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
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
	d, err := NewDeployer(&DeployerConfig{
		AccessKeyId:     access.AccessKeyId,
		SecretAccessKey: access.SecretAccessKey,
		ZoneId:          xmaps.GetString(extended, "zoneId"),
		DeployTarget:    xmaps.GetString(extended, "deployTarget"),
		LoadbalancerId:  xmaps.GetString(extended, "loadbalancerId"),
		ListenerId:      xmaps.GetString(extended, "listenerId"),
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	if _, err := d.Deploy(ctx, req.Deploy.CertificatePEM, req.Deploy.PrivateKeyPEM); err != nil {
		return nil, err
	}
	return &plugin.Result{Deploy: &plugin.DeployResult{ExtendedDataJSON: "{}"}}, nil
}
