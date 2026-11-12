package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
	unicloudsdk "github.com/certimate-go/plugins/internal/unicloudsdk"
)

type deploymentConfig struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	SpaceProvider string `json:"spaceProvider"`
	SpaceId       string `json:"spaceId"`
	Domain        string `json:"domain"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *unicloudsdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.Username, config.Password)
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
	if d.config.SpaceProvider == "" {
		return nil, fmt.Errorf("config `spaceProvider` is required")
	}
	if d.config.SpaceId == "" {
		return nil, fmt.Errorf("config `spaceId` is required")
	}
	if d.config.Domain == "" {
		return nil, fmt.Errorf("config `domain` is required")
	}
	createDomainWithCertReq := &unicloudsdk.CreateDomainWithCertRequest{
		Provider: d.config.SpaceProvider,
		SpaceId:  d.config.SpaceId,
		Domain:   d.config.Domain,
		Cert:     url.QueryEscape(certPEM),
		Key:      url.QueryEscape(privkeyPEM),
	}
	_, err := d.sdkClient.CreateDomainWithCertWithContext(ctx, createDomainWithCertReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'unicloud.host.CreateDomainWithCert': %w", err)
	}

	d.logger.Debug("sdk request 'unicloud.host.CreateDomainWithCert' completed", slog.String("spaceId", d.config.SpaceId), slog.String("domain", d.config.Domain))

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(username, password string) (*unicloudsdk.Client, error) {
	client, err := unicloudsdk.NewClient(
		unicloudsdk.WithLogins(username, password),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

type accessConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "unicloud-webhost"
	accessType     = "unicloud"
	displayNameKey = "plugin.unicloud-webhost.name"
)

type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "website",
		DeployDisplayNameKey: displayNameKey,
		AccessDisplayNameKey: "plugin.unicloud.name",
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
		Username:      access.Username,
		Password:      access.Password,
		SpaceProvider: xmaps.GetString(extended, "spaceProvider"),
		SpaceId:       xmaps.GetString(extended, "spaceId"),
		Domain:        xmaps.GetString(extended, "domain"),
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
