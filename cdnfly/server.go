package main

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
	cdnflysdk "github.com/certimate-go/plugins/internal/cdnflysdk"
)

type accessConfig struct {
	ServerUrl                string `json:"serverUrl"`
	ApiKey                   string `json:"apiKey"`
	ApiSecret                string `json:"apiSecret"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}
type extendedConfig struct {
	DeployTarget  string `json:"deployTarget"`
	SiteId        string `json:"siteId,omitempty"`
	CertificateId string `json:"certificateId,omitempty"`
}
type deploymentConfig struct {
	accessConfig
	extendedConfig
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *cdnflysdk.Client
}

func newDeployment(config *deploymentConfig, logger *slog.Logger) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiKey, config.ApiSecret, config.AllowInsecureConnections)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &deployment{
		config:    config,
		logger:    logger,
		sdkClient: client,
	}, nil
}

func (d *deployment) deploy(ctx context.Context, certPEM, privkeyPEM string) (*plugin.DeployResult, error) {
	switch d.config.DeployTarget {
	case DEPLOY_TARGET_WEBSITE:
		if err := d.deployToSite(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	case DEPLOY_TARGET_CERTIFICATE:
		if err := d.deployToCertificate(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) deployToSite(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.SiteId == "" {
		return fmt.Errorf("config `siteId` is required")
	}
	getSiteResp, err := d.sdkClient.GetSiteWithContext(ctx, d.config.SiteId)
	d.logger.Debug("sdk request 'GetSite'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'GetSite': %w", err)
	}
	createCertificateReq := &cdnflysdk.CreateCertRequest{
		Name: lo.ToPtr(fmt.Sprintf("certimate-%d", time.Now().UnixMilli())),
		Type: lo.ToPtr("custom"),
		Cert: lo.ToPtr(certPEM),
		Key:  lo.ToPtr(privkeyPEM),
	}
	createCertificateResp, err := d.sdkClient.CreateCertWithContext(ctx, createCertificateReq)
	d.logger.Debug("sdk request 'CreateCert'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'CreateCert': %w", err)
	}
	updateSiteReqHttpsListen := make(map[string]any)
	_ = json.Unmarshal([]byte(getSiteResp.Data.HttpsListen), &updateSiteReqHttpsListen)
	updateSiteReqHttpsListen["cert"] = createCertificateResp.Data
	updateSiteReq := &cdnflysdk.UpdateSiteRequest{
		HttpsListen: lo.ToPtr(updateSiteReqHttpsListen),
	}
	_, err = d.sdkClient.UpdateSiteWithContext(ctx, d.config.SiteId, updateSiteReq)
	d.logger.Debug("sdk request 'UpdateSite'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'UpdateSite': %w", err)
	}

	return nil
}

func (d *deployment) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.CertificateId == "" {
		return fmt.Errorf("config `certificateId` is required")
	}
	updateCertReq := &cdnflysdk.UpdateCertRequest{
		Type: lo.ToPtr("custom"),
		Cert: lo.ToPtr(certPEM),
		Key:  lo.ToPtr(privkeyPEM),
	}
	_, err := d.sdkClient.UpdateCertWithContext(ctx, d.config.CertificateId, updateCertReq)
	d.logger.Debug("sdk request 'UpdateCert'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'UpdateCert': %w", err)
	}

	return nil
}

func createSDKClient(serverUrl, apiKey, apiSecret string, skipTlsVerify bool) (*cdnflysdk.Client, error) {
	client, err := cdnflysdk.NewClient(serverUrl,
		cdnflysdk.WithApiKey(apiKey, apiSecret),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}

const (
	DEPLOY_TARGET_WEBSITE     = "website"
	DEPLOY_TARGET_CERTIFICATE = "certificate"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "cdnfly"
	accessType     = "cdnfly"
	displayNameKey = "plugin.cdnfly.name"
)

type cdnflyDeployer struct{}

func (*cdnflyDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
		AccessDisplayNameKey: "plugin.cdnfly.name",
	}, nil
}

func (*cdnflyDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
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
	return &plugin.ConfigSchema{AccessSchemaJSON: accessSchema, DeploySchemaJSON: deploySchema, I18n: i18n}, nil
}

func (*cdnflyDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &input); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}
	extended := extendedConfig{DeployTarget: xmaps.GetString(input, "deployTarget"), SiteId: xmaps.GetString(input, "siteId"), CertificateId: xmaps.GetString(input, "certificateId")}
	d, err := newDeployment(&deploymentConfig{accessConfig: access, extendedConfig: extended}, logger)
	if err != nil {
		return nil, err
	}
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
