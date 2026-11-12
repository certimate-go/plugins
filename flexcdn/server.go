package main

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	flexcdnsdk "github.com/certimate-go/plugins/internal/flexcdnsdk"
)

type accessConfig struct {
	ServerUrl                string `json:"serverUrl"`
	ApiRole                  string `json:"apiRole"`
	AccessKeyId              string `json:"accessKeyId"`
	AccessKey                string `json:"accessKey"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}
type extendedConfig struct {
	DeployTarget  string `json:"deployTarget"`
	CertificateId int64  `json:"certificateId,omitempty"`
}
type deploymentConfig struct {
	accessConfig
	extendedConfig
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *flexcdnsdk.Client
}

func newDeployment(config *deploymentConfig, logger *slog.Logger) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiRole, config.AccessKeyId, config.AccessKey, config.AllowInsecureConnections)
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
	case "certificate":
		if err := d.deployToCertificate(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	certX509, err := xcert.ParseCertificateFromPEM(certPEM)
	if err != nil {
		return err
	}

	updateSSLCertReq := &flexcdnsdk.UpdateSSLCertRequest{
		SSLCertId:   d.config.CertificateId,
		IsOn:        true,
		Name:        fmt.Sprintf("certimate-%d", time.Now().UnixMilli()),
		Description: "upload from Certimate",
		ServerName:  certX509.Subject.CommonName,
		IsCA:        false,
		CertData:    base64.StdEncoding.EncodeToString([]byte(certPEM)),
		KeyData:     base64.StdEncoding.EncodeToString([]byte(privkeyPEM)),
		TimeBeginAt: certX509.NotBefore.Unix(),
		TimeEndAt:   certX509.NotAfter.Unix(),
		DNSNames:    certX509.DNSNames,
		CommonNames: []string{certX509.Subject.CommonName},
	}
	_, err = d.sdkClient.UpdateSSLCertWithContext(ctx, updateSSLCertReq)
	d.logger.Debug("sdk request 'UpdateSSLCert'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'UpdateSSLCert': %w", err)
	}

	return nil
}

func createSDKClient(serverUrl, apiRole, accessKeyId, accessKey string, skipTlsVerify bool) (*flexcdnsdk.Client, error) {
	client, err := flexcdnsdk.NewClient(serverUrl,
		flexcdnsdk.WithRole(apiRole),
		flexcdnsdk.WithAccessKey(accessKeyId, accessKey),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "flexcdn"
	accessType     = "flexcdn"
	displayNameKey = "plugin.flexcdn.name"
)

type flexcdnDeployer struct{}

func (*flexcdnDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
		AccessDisplayNameKey: "plugin.flexcdn.name",
	}, nil
}

func (*flexcdnDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
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

func (*flexcdnDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var input struct {
		DeployTarget  string      `json:"deployTarget"`
		CertificateId json.Number `json:"certificateId,omitempty"`
	}
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &input); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}
	certificateID, err := input.CertificateId.Int64()
	if err != nil || certificateID <= 0 {
		return nil, fmt.Errorf("config `certificateId` must be a positive integer")
	}
	extended := extendedConfig{DeployTarget: input.DeployTarget, CertificateId: certificateID}
	d, err := newDeployment(&deploymentConfig{accessConfig: access, extendedConfig: extended}, logger)
	if err != nil {
		return nil, err
	}
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
