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

	ratpanelsdk "github.com/certimate-go/plugins/internal/ratpanelsdk"
)

type deploymentConfig struct {
	ServerUrl                string `json:"serverUrl"`
	AccessTokenId            int64  `json:"accessTokenId"`
	AccessToken              string `json:"accessToken"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *ratpanelsdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.AccessTokenId, config.AccessToken, config.AllowInsecureConnections)
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
	setSettingCertReq := &ratpanelsdk.SetSettingCertRequest{
		Certificate: certPEM,
		PrivateKey:  privkeyPEM,
	}
	_, err := d.sdkClient.SetSettingCertWithContext(ctx, setSettingCertReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'SetSettingCert': %w", err)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(serverUrl string, accessTokenId int64, accessToken string, skipTlsVerify bool) (*ratpanelsdk.Client, error) {
	client, err := ratpanelsdk.NewClient(serverUrl,
		ratpanelsdk.WithAccessToken(accessTokenId, accessToken),
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
	AccessTokenId            int64  `json:"accessTokenId"`
	AccessToken              string `json:"accessToken"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}

//go:embed schema
var schemaFS embed.FS

type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{ProviderType: "ratpanel-console", AccessProviderType: "ratpanel", ProtocolVersion: plugin.ProtocolVersion, DeployCategory: "other", DeployDisplayNameKey: "provider.ratpanel_console", AccessDisplayNameKey: "provider.ratpanel"}, nil
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
		AccessTokenId:            access.AccessTokenId,
		AccessToken:              access.AccessToken,
		AllowInsecureConnections: access.AllowInsecureConnections,
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
