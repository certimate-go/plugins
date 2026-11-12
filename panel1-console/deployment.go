package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/certimate-go/certimate/pkg/plugin"

	onepanelsdk "github.com/certimate-go/plugins/internal/onepanelsdk"
	onepanelsdk2 "github.com/certimate-go/plugins/internal/onepanelsdk/v2"
)

type deploymentConfig struct {
	ServerUrl string `json:"serverUrl"`

	ApiVersion string `json:"apiVersion"`

	ApiKey string `json:"apiKey"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`

	AutoRestart bool `json:"autoRestart"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient any
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiVersion, config.ApiKey, config.AllowInsecureConnections)
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
	switch sdkClient := d.sdkClient.(type) {
	case *onepanelsdk.Client:
		{
			settingsSSLUpdateReq := &onepanelsdk.SettingsSSLUpdateRequest{
				Cert:        certPEM,
				Key:         privkeyPEM,
				SSL:         "enable",
				SSLType:     "import-paste",
				AutoRestart: strconv.FormatBool(d.config.AutoRestart),
			}
			_, err := sdkClient.SettingsSSLUpdateWithContext(ctx, settingsSSLUpdateReq)
			d.logger.Debug("sdk request 'SettingsSSLUpdate'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'SettingsSSLUpdate': %w", err)
			}
		}

	case *onepanelsdk2.Client:
		{
			coreSettingsSSLUpdateReq := &onepanelsdk2.CoreSettingsSSLUpdateRequest{
				Cert:        certPEM,
				Key:         privkeyPEM,
				SSL:         "Enable",
				SSLType:     "import-paste",
				AutoRestart: strconv.FormatBool(d.config.AutoRestart),
			}
			_, err := sdkClient.CoreSettingsSSLUpdateWithContext(ctx, coreSettingsSSLUpdateReq)
			d.logger.Debug("sdk request 'CoreSettingsSSLUpdate'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'CoreSettingsSSLUpdate': %w", err)
			}
		}

	default:
		panic("unreachable")
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

const (
	sdkVersionV1 = "v1"
	sdkVersionV2 = "v2"
)

func createSDKClient(serverUrl, apiVersion, apiKey string, skipTlsVerify bool) (any, error) {
	switch apiVersion {
	case sdkVersionV1:
		{
			client, err := onepanelsdk.NewClient(
				serverUrl,
				onepanelsdk.WithApiKey(apiKey),
			)
			if err != nil {
				return nil, err
			}

			if skipTlsVerify {
				client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
			}

			return client, nil
		}

	case sdkVersionV2:
		{
			client, err := onepanelsdk2.NewClient(
				serverUrl,
				onepanelsdk2.WithApiKey(apiKey),
			)
			if err != nil {
				return nil, err
			}

			if skipTlsVerify {
				client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
			}

			return client, nil
		}
	}

	return nil, fmt.Errorf("1panel: invalid api version")
}
