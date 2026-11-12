package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"

	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	ibmcsdk "github.com/certimate-go/plugins/internal/huaweiibmcsdk"
)

type deploymentConfig struct {
	Host                     string `json:"host"`
	Username                 string `json:"username"`
	Password                 string `json:"password"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
	AutoRestart              bool   `json:"autoRestart,omitempty"`
}

type deployment struct {
	config *deploymentConfig
	logger *slog.Logger
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the iBMC deployer is nil")
	}

	return &deployment{
		config: config,
		logger: slog.Default(),
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
	certPFXPwd := make([]byte, 24)
	if _, err := rand.Read(certPFXPwd); err != nil {
		return nil, fmt.Errorf("failed to generate PFX password: %w", err)
	}
	certPFXPwdHex := hex.EncodeToString(certPFXPwd)
	certPFX, err := xcert.TransformCertificateFromPEMToPFX(certPEM, privkeyPEM, certPFXPwdHex, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to transform certificate from PEM to PFX: %w", err)
	}
	client, err := createSDKClient(d.config.Host, d.config.Username, d.config.Password, d.config.AllowInsecureConnections)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}
	_, err = client.CreateSessionWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'SessionService.CreateSession': %w", err)
	} else {
		defer client.DeleteSessionWithContext(ctx)
	}
	listManagersResp, err := client.ListManagersWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'Managers.ListManagers': %w", err)
	}
	if len(listManagersResp.Members) == 0 {
		d.logger.Info("no ibmc managers to deploy")
	} else {
		d.logger.Info("found ibmc managers to deploy", slog.Any("managers", listManagersResp.Members))

		if err := xloop.ForRangeAllWithContext(ctx, listManagersResp.Members, func(ctx context.Context, managerInfo *ibmcsdk.Entity, _ int) error {
			importCustomCertificateReq := &ibmcsdk.ImportCustomCertificateToManagerRequest{
				ManagerID:       managerInfo.ID,
				ManagerLocation: managerInfo.ODataID,
				Certificate:     base64.StdEncoding.EncodeToString(certPFX),
				Password:        certPFXPwdHex,
			}
			_, err = client.ImportCustomCertificateToManagerWithContext(ctx, importCustomCertificateReq)
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'Managers.SecurityService.HttpsCert.ImportCustomCertificateToManager': %w", err)
			}

			if d.config.AutoRestart {
				resetManagerReq := &ibmcsdk.ResetManagerRequest{
					ManagerID:       managerInfo.ID,
					ManagerLocation: managerInfo.ODataID,
					ResetType:       "ForceRestart",
				}
				_, err = client.ResetManagerWithContext(ctx, resetManagerReq)
				if err != nil {
					return fmt.Errorf("failed to execute sdk request 'Managers.ResetManager': %w", err)
				}
			}

			return nil
		}); err != nil {
			return nil, err
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(host, username, password string, skipTlsVerify bool) (*ibmcsdk.Client, error) {
	serverUrl := ""
	if strings.Contains(host, "://") {
		serverUrl = host
	} else {
		if net.ParseIP(host) != nil && strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		serverUrl = "https://" + host
	}

	client, err := ibmcsdk.NewClient(
		serverUrl,
		ibmcsdk.WithLogins(username, password),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}

func generateCertificatePassword() (string, error) {
	password := make([]byte, 24)
	if _, err := rand.Read(password); err != nil {
		return "", err
	}
	return hex.EncodeToString(password), nil
}

type accessConfig struct {
	Host                     string `json:"host"`
	Username                 string `json:"username"`
	Password                 string `json:"password"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}

//go:embed schema
var schemaFS embed.FS

type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{ProviderType: "huaweiibmc", AccessProviderType: "huaweiibmc", ProtocolVersion: plugin.ProtocolVersion, DeployCategory: "other", DeployDisplayNameKey: "provider.huaweiibmc", AccessDisplayNameKey: "provider.huaweiibmc"}, nil
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
		Host:                     access.Host,
		Username:                 access.Username,
		Password:                 access.Password,
		AllowInsecureConnections: access.AllowInsecureConnections,
		AutoRestart:              xmaps.GetBool(extended, "autoRestart"),
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
