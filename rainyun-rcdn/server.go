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
	providerType   = "rainyun-rcdn"
	accessType     = "rainyun"
	displayNameKey = "plugin.rainyun-rcdn.name"
)

type accessConfig struct {
	ApiKey string `json:"apiKey"`
}
type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{ProviderType: providerType, AccessProviderType: accessType, ProtocolVersion: plugin.ProtocolVersion, DeployCategory: "cdn", DeployDisplayNameKey: displayNameKey}, nil
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
	instanceID := xmaps.GetInt64(extended, "instanceId")
	if instanceID == 0 {
		return nil, fmt.Errorf("config `instanceId` is required")
	}
	domain := xmaps.GetString(extended, "domain")
	if domain == "" {
		return nil, fmt.Errorf("config `domain` is required")
	}
	id, err := client.Upload(ctx, req.CertificatePEM, req.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	}
	logger.Info("ssl certificate uploaded", slog.Int64("certificateId", id))
	if err := client.Bind(ctx, instanceID, id, domain); err != nil {
		return nil, err
	}
	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}
