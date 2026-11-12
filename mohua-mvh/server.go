package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
	mohuasdk "github.com/certimate-go/plugins/internal/mohuasdk"
)

type deploymentConfig struct {
	Username    string `json:"username"`
	ApiPassword string `json:"apiPassword"`
	HostId      string `json:"hostId"`
	DomainId    int64  `json:"domainId"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *mohuasdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.Username, config.ApiPassword)
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
	if d.config.HostId == "" {
		return nil, fmt.Errorf("config `hostId` is required")
	}
	if d.config.DomainId == 0 {
		return nil, fmt.Errorf("config `domainId` is required")
	}
	setSSLReq := &mohuasdk.SetVirtualHostSSLRequest{
		ID:      int(d.config.DomainId),
		SSLCert: certPEM,
		SSLKey:  privkeyPEM,
	}
	_, err := d.sdkClient.SetVirtualHostSSLWithContext(ctx, d.config.HostId, setSSLReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'mvh.SetSSL': %w", err)
	}

	d.logger.Debug("sdk request 'mvh.SetSSL' completed", slog.String("hostId", d.config.HostId), slog.Int64("domainId", d.config.DomainId))

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(username, apiPassword string) (*mohuasdk.Client, error) {
	client, err := mohuasdk.NewClient(
		mohuasdk.WithLogins(username, apiPassword),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

type accessConfig struct {
	Username    string `json:"username"`
	ApiPassword string `json:"apiPassword"`
}

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "mohua-mvh"
	accessType     = "mohua"
	displayNameKey = "plugin.mohua-mvh.name"
)

type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "website",
		DeployDisplayNameKey: displayNameKey,
		AccessDisplayNameKey: "plugin.mohua.name",
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
		Username:    access.Username,
		ApiPassword: access.ApiPassword,
		HostId:      xmaps.GetString(extended, "hostId"),
		DomainId:    xmaps.GetInt64(extended, "domainId"),
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
