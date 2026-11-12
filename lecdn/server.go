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
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
	lecdnclientsdkv3 "github.com/certimate-go/plugins/internal/lecdnsdk/client/v3"
	lecdnclientsdkv4 "github.com/certimate-go/plugins/internal/lecdnsdk/client/v4"
	lecdnmastersdkv3 "github.com/certimate-go/plugins/internal/lecdnsdk/master/v3"
	lecdnmastersdkv4 "github.com/certimate-go/plugins/internal/lecdnsdk/master/v4"
)

type accessConfig struct {
	ServerUrl                string `json:"serverUrl"`
	ApiVersion               string `json:"apiVersion"`
	ApiRole                  string `json:"apiRole"`
	AuthMethod               string `json:"authMethod,omitempty"`
	Username                 string `json:"username,omitempty"`
	Password                 string `json:"password,omitempty"`
	ApiKey                   string `json:"apiKey,omitempty"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}
type extendedConfig struct {
	DeployTarget  string `json:"deployTarget"`
	CertificateId int64  `json:"certificateId,omitempty"`
	ClientId      int64  `json:"clientId,omitempty"`
}
type deploymentConfig struct {
	accessConfig
	extendedConfig
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient any
}

func newDeployment(config *deploymentConfig, logger *slog.Logger) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiVersion, config.ApiRole, config.AuthMethod, config.Username, config.Password, config.ApiKey, config.AllowInsecureConnections)
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
	case DEPLOY_TARGET_CERTIFICATE:
		if err := d.deployToCertificate(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.CertificateId == 0 {
		return fmt.Errorf("config `certificateId` is required")
	}
	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())
	certDesc := "upload from Certimate"
	switch sdkClient := d.sdkClient.(type) {
	case *lecdnclientsdkv3.Client:
		{
			updateSSLCertReq := &lecdnclientsdkv3.UpdateCertificateRequest{
				Name:        certName,
				Description: certDesc,
				Type:        "upload",
				SSLPEM:      base64.StdEncoding.EncodeToString([]byte(certPEM)),
				SSLKey:      base64.StdEncoding.EncodeToString([]byte(privkeyPEM)),
				AutoRenewal: false,
			}
			_, err := sdkClient.UpdateCertificateWithContext(ctx, d.config.CertificateId, updateSSLCertReq)
			d.logger.Debug("sdk request 'UpdateCertificate'")
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'UpdateCertificate': %w", err)
			}
		}

	case *lecdnmastersdkv3.Client:
		{
			updateSSLCertReq := &lecdnmastersdkv3.UpdateCertificateRequest{
				ClientId:    d.config.ClientId,
				Name:        certName,
				Description: certDesc,
				Type:        "upload",
				SSLPEM:      base64.StdEncoding.EncodeToString([]byte(certPEM)),
				SSLKey:      base64.StdEncoding.EncodeToString([]byte(privkeyPEM)),
				AutoRenewal: false,
			}
			_, err := sdkClient.UpdateCertificateWithContext(ctx, d.config.CertificateId, updateSSLCertReq)
			d.logger.Debug("sdk request 'UpdateCertificate'")
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'UpdateCertificate': %w", err)
			}
		}

	case *lecdnclientsdkv4.Client:
		{
			updateSSLCertReq := &lecdnclientsdkv4.UpdateCertificateRequest{
				Name:        certName,
				Description: certDesc,
				SSLPEM:      base64.StdEncoding.EncodeToString([]byte(certPEM)),
				SSLKey:      base64.StdEncoding.EncodeToString([]byte(privkeyPEM)),
				AutoRenewal: false,
			}
			_, err := sdkClient.UpdateCertificateWithContext(ctx, d.config.CertificateId, updateSSLCertReq)
			d.logger.Debug("sdk request 'UpdateCertificate'")
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'UpdateCertificate': %w", err)
			}
		}

	case *lecdnmastersdkv4.Client:
		{
			updateSSLCertReq := &lecdnmastersdkv4.UpdateCertificateRequest{
				Name:        certName,
				Description: certDesc,
				SSLPEM:      base64.StdEncoding.EncodeToString([]byte(certPEM)),
				SSLKey:      base64.StdEncoding.EncodeToString([]byte(privkeyPEM)),
				AutoRenewal: false,
			}
			_, err := sdkClient.UpdateCertificateWithContext(ctx, d.config.CertificateId, updateSSLCertReq)
			d.logger.Debug("sdk request 'UpdateCertificate'")
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'UpdateCertificate': %w", err)
			}
		}

	default:
		panic("unreachable")
	}

	return nil
}

const (
	sdkVersionV3 = "v3"
	sdkVersionV4 = "v4"

	sdkRoleClient = "client"
	sdkRoleMaster = "master"
)

func createSDKClient(serverUrl, apiVersion, apiRole, authMethod, username, password, apiKey string, skipTlsVerify bool) (any, error) {
	switch apiVersion {
	case sdkVersionV3:
		{
			switch apiRole {
			case sdkRoleClient:
				{
					var client *lecdnclientsdkv3.Client
					var err error

					switch authMethod {
					case "", AUTH_METHOD_PASSWORD:
						client, err = lecdnclientsdkv3.NewClient(serverUrl, lecdnclientsdkv3.WithLogins(username, password))
					case AUTH_METHOD_APIKEY:
						client, err = lecdnclientsdkv3.NewClient(serverUrl, lecdnclientsdkv3.WithApiKey(apiKey))
					default:
						err = fmt.Errorf("unsupported auth method '%s'", authMethod)
					}

					if err != nil {
						return nil, err
					}

					if skipTlsVerify {
						client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
					}

					return client, nil
				}

			case sdkRoleMaster:
				{
					var client *lecdnmastersdkv3.Client
					var err error

					switch authMethod {
					case "", AUTH_METHOD_PASSWORD:
						client, err = lecdnmastersdkv3.NewClient(serverUrl, lecdnmastersdkv3.WithLogins(username, password))
					case AUTH_METHOD_APIKEY:
						client, err = lecdnmastersdkv3.NewClient(serverUrl, lecdnmastersdkv3.WithApiKey(apiKey))
					default:
						err = fmt.Errorf("unsupported auth method '%s'", authMethod)
					}

					if err != nil {
						return nil, err
					}

					if skipTlsVerify {
						client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
					}

					return client, nil
				}
			}
		}

	case sdkVersionV4:
		{
			switch apiRole {
			case sdkRoleClient:
				{
					var client *lecdnclientsdkv4.Client
					var err error

					switch authMethod {
					case "", AUTH_METHOD_PASSWORD:
						client, err = lecdnclientsdkv4.NewClient(serverUrl, lecdnclientsdkv4.WithLogins(username, password))
					case AUTH_METHOD_APIKEY:
						client, err = lecdnclientsdkv4.NewClient(serverUrl, lecdnclientsdkv4.WithApiKey(apiKey))
					default:
						err = fmt.Errorf("unsupported auth method '%s'", authMethod)
					}

					if err != nil {
						return nil, err
					}

					if skipTlsVerify {
						client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
					}

					return client, nil
				}

			case sdkRoleMaster:
				{
					var client *lecdnmastersdkv4.Client
					var err error

					switch authMethod {
					case "", AUTH_METHOD_PASSWORD:
						client, err = lecdnmastersdkv4.NewClient(serverUrl, lecdnmastersdkv4.WithLogins(username, password))
					case AUTH_METHOD_APIKEY:
						client, err = lecdnmastersdkv4.NewClient(serverUrl, lecdnmastersdkv4.WithApiKey(apiKey))
					default:
						err = fmt.Errorf("unsupported auth method '%s'", authMethod)
					}

					if err != nil {
						return nil, err
					}

					if skipTlsVerify {
						client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
					}

					return client, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("lecdn: invalid api version or user role")
}

const (
	AUTH_METHOD_PASSWORD = "password"
	AUTH_METHOD_APIKEY   = "apikey"
)

const (
	DEPLOY_TARGET_CERTIFICATE = "certificate"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "lecdn"
	accessType     = "lecdn"
	displayNameKey = "plugin.lecdn.name"
)

type lecdnDeployer struct{}

func (*lecdnDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
		AccessDisplayNameKey: "plugin.lecdn.name",
	}, nil
}

func (*lecdnDeployer) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
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

func (*lecdnDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &input); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}
	extended := extendedConfig{DeployTarget: xmaps.GetString(input, "deployTarget"), CertificateId: xmaps.GetInt64(input, "certificateId"), ClientId: xmaps.GetInt64(input, "clientId")}

	d, err := newDeployment(&deploymentConfig{accessConfig: access, extendedConfig: extended}, logger)
	if err != nil {
		return nil, err
	}
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
