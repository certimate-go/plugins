package main

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"

	pvesdk "github.com/certimate-go/plugins/internal/proxmoxvesdk"
)

type deploymentConfig struct {
	ServerUrl                string `json:"serverUrl"`
	ApiToken                 string `json:"apiToken"`
	ApiTokenSecret           string `json:"apiTokenSecret"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
	NodeName                 string `json:"nodeName"`
	AutoRestart              bool   `json:"autoRestart"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *pvesdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiToken, config.ApiTokenSecret, config.AllowInsecureConnections)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return &deployment{
		config:    config,
		logger:    slog.Default(),
		sdkClient: client,
	}, nil
}

func (d *deployment) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}
}

func (d *deployment) deploy(ctx context.Context, certPEM, privkeyPEM string) (*plugin.DeployResult, error) {
	if d.config.NodeName == "" {
		return nil, fmt.Errorf("config `nodeName` is required")
	}
	nodeUploadCustomCertificateReq := &pvesdk.NodeUploadCustomCertificateRequest{
		Certificates: certPEM,
		Key:          privkeyPEM,
		Force:        true,
		Restart:      d.config.AutoRestart,
	}
	_, err := d.sdkClient.NodeUploadCustomCertificateWithContext(ctx, d.config.NodeName, nodeUploadCustomCertificateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'node.UploadCustomCertificate': %w", err)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(serverUrl, apiToken, apiTokenSecret string, skipTlsVerify bool) (*pvesdk.Client, error) {
	client, err := pvesdk.NewClient(
		serverUrl,
		pvesdk.WithApiToken(apiToken, apiTokenSecret),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}

type accessConfig struct {
	ServerUrl                string `json:"serverUrl"`
	ApiToken                 string `json:"apiToken"`
	ApiTokenSecret           string `json:"apiTokenSecret"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}

//go:embed schema
var schemaFS embed.FS

type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{ProviderType: "proxmoxve", AccessProviderType: "proxmoxve", ProtocolVersion: plugin.ProtocolVersion, DeployCategory: "other", DeployDisplayNameKey: "provider.proxmoxve", AccessDisplayNameKey: "provider.proxmoxve"}, nil
}

func (*pluginDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
	access, err := plugin.LoadAccessSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	deploy, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{AccessSchemaJSON: access, DeploySchemaJSON: deploy, I18n: i18n}, nil
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
	d, err := newDeployment(&deploymentConfig{
		ServerUrl:                access.ServerUrl,
		ApiToken:                 access.ApiToken,
		ApiTokenSecret:           access.ApiTokenSecret,
		AllowInsecureConnections: access.AllowInsecureConnections,
		NodeName:                 xmaps.GetString(extended, "nodeName"),
		AutoRestart:              xmaps.GetBool(extended, "autoRestart"),
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
