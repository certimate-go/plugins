package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
	flyiosdk "github.com/certimate-go/plugins/internal/flyiosdk"
)

type deploymentConfig struct {
	ApiToken string `json:"apiToken"`
	AppName  string `json:"appName"`
	Domain   string `json:"domain"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *flyiosdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ApiToken)
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
	if d.config.AppName == "" {
		return nil, fmt.Errorf("config `appName` is required")
	}
	if d.config.Domain == "" {
		return nil, fmt.Errorf("config `domain` is required")
	}
	importCustomCertificateReq := &flyiosdk.ImportCustomCertificateRequest{
		AppName:    d.config.AppName,
		Hostname:   d.config.Domain,
		Fullchain:  certPEM,
		PrivateKey: privkeyPEM,
	}
	_, err := d.sdkClient.ImportCustomCertificateWithContext(ctx, importCustomCertificateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'ImportCustomCertificate': %w", err)
	}

	d.logger.Debug("sdk request 'ImportCustomCertificate' completed", slog.String("appName", d.config.AppName), slog.String("domain", d.config.Domain))

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(apiToken string) (*flyiosdk.Client, error) {
	client, err := flyiosdk.NewClient(
		flyiosdk.WithApiToken(apiToken),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

type accessConfig struct {
	ApiToken string `json:"apiToken"`
}

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "flyio"
	accessType     = "flyio"
	displayNameKey = "plugin.flyio.name"
)

type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "website",
		DeployDisplayNameKey: displayNameKey,
		AccessDisplayNameKey: "plugin.flyio.name",
	}, nil
}

func (*pluginDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	accessSchema, err := plugin.LoadAccessSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{DeploySchemaJSON: deploySchema, AccessSchemaJSON: accessSchema, I18n: i18n}, nil
}

func (*pluginDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var extended map[string]any
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}
	d, err := newDeployment(&deploymentConfig{
		ApiToken: access.ApiToken,
		AppName:  xmaps.GetString(extended, "appName"),
		Domain:   xmaps.GetString(extended, "domain"),
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
