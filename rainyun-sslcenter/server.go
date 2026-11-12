package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
	"github.com/certimate-go/plugins/internal/rainyun"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "rainyun-sslcenter"
	accessType     = "rainyun"
	displayNameKey = "plugin.rainyun-sslcenter.name"
)

type accessConfig struct {
	ApiKey string `json:"apiKey"`
}
type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{ProviderType: providerType, AccessProviderType: accessType, ProtocolVersion: plugin.ProtocolVersion, DeployCategory: "ssl", DeployDisplayNameKey: displayNameKey}, nil
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

func (*pluginDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &raw); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var access accessConfig
	if err := xmaps.Populate(raw, &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var extended map[string]any
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}
	client, err := rainyun.NewClient(access.ApiKey)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}
	if id := xmaps.GetInt64(extended, "certificateId"); id != 0 {
		if err := client.Replace(ctx, id, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
			return nil, fmt.Errorf("failed to replace certificate file: %w", err)
		}
		logger.Info("ssl certificate replaced", slog.Int64("certificateId", id))
	} else {
		id, err := client.Upload(ctx, req.CertificatePEM, req.PrivateKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to upload certificate file: %w", err)
		}
		logger.Info("ssl certificate uploaded", slog.Int64("certificateId", id))
	}
	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}
