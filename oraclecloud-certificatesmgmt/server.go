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
	providerType = "oraclecloud-certificatesmgmt"
	accessType   = "oraclecloud"
)

type pluginDeployer struct {
	plugin.NoopFinalizer
}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType: providerType, AccessProviderType: accessType, ProtocolVersion: plugin.ProtocolVersion,
		Capability: plugin.CapabilityDeploy, Category: "ssl", DisplayNameKey: "plugin.oraclecloud-certificatesmgmt.name",
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
	AuthMethod           string `json:"authMethod"`
	PrivateKey           string `json:"privateKey,omitempty"`
	PrivateKeyPassphrase string `json:"privateKeyPassphrase,omitempty"`
	PublicKeyFingerprint string `json:"publicKeyFingerprint,omitempty"`
	TenancyOcid          string `json:"tenancyOcid,omitempty"`
	UserOcid             string `json:"userOcid,omitempty"`
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
		AuthMethod:           access.AuthMethod,
		PrivateKey:           access.PrivateKey,
		PrivateKeyPassphrase: access.PrivateKeyPassphrase,
		PublicKeyFingerprint: access.PublicKeyFingerprint,
		TenancyOcid:          access.TenancyOcid,
		UserOcid:             access.UserOcid,
		CompartmentOcid:      xmaps.GetString(extended, "compartmentOcid"),
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
