package main

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"

	"github.com/pquerna/otp/totp"
	"github.com/samber/lo"

	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
	dsmsdk "github.com/certimate-go/plugins/internal/synologydsmsdk"
)

type deploymentConfig struct {
	ServerUrl                  string `json:"serverUrl"`
	Username                   string `json:"username"`
	Password                   string `json:"password"`
	TotpSecret                 string `json:"totpSecret,omitempty"`
	AllowInsecureConnections   bool   `json:"allowInsecureConnections,omitempty"`
	CertificateIdOrDescription string `json:"certificateIdOrDesc,omitempty"`
	IsDefault                  bool   `json:"isDefault,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *dsmsdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.AllowInsecureConnections)
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
		d.logger = slog.Default()
	} else {
		d.logger = logger
	}
}

func (d *deployment) deploy(ctx context.Context, certPEM string, privkeyPEM string) (*plugin.DeployResult, error) {
	serverCertPEM, intermediateCertPEM, err := xcert.ExtractCertificatesFromPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to extract certs: %w", err)
	}
	var otpCode string
	if d.config.TotpSecret != "" {
		now := time.Now()
		wait := time.Duration(30-now.Unix()%30) * time.Second
		if wait > 0 {
			wait = wait + 1*time.Second
			d.logger.Info("waiting for the next TOTP time step ...", slog.Int("wait", int(wait.Seconds())))
			if err := xwait.DelayWithContext(ctx, wait); err != nil {
				return nil, err
			}
		}

		now = time.Now()
		otpCodeStr, err := totp.GenerateCode(d.config.TotpSecret, now)
		if err != nil {
			return nil, fmt.Errorf("failed to generate TOTP code: %w", err)
		}

		otpCode = otpCodeStr
	}
	loginReq := &dsmsdk.LoginRequest{
		Account:  d.config.Username,
		Password: d.config.Password,
		OtpCode:  otpCode,
	}
	_, err = d.sdkClient.LoginWithContext(ctx, loginReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'SYNO.API.Auth:login': %w", err)
	}
	defer func() {
		_, _ = d.sdkClient.LogoutWithContext(ctx)
	}()
	if d.config.CertificateIdOrDescription == "" {
		importCertificateReq := &dsmsdk.ImportCertificateRequest{
			ID:          "",
			Description: fmt.Sprintf("certimate-%d", time.Now().UnixMilli()),
			Key:         privkeyPEM,
			Cert:        serverCertPEM,
			InterCert:   intermediateCertPEM,
			AsDefault:   d.config.IsDefault,
		}
		_, err = d.sdkClient.ImportCertificateWithContext(ctx, importCertificateReq)
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'SYNO.Core.Certificate:import': %w", err)
		}
	} else {
		var certInfo *dsmsdk.CertificateInfo
		listCertificatesResp, err := d.sdkClient.ListCertificatesWithContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'SYNO.Core.Certificate.CRT:list': %w", err)
		} else {
			matchedCerts := lo.Filter(listCertificatesResp.Data.Certificates, func(certItem *dsmsdk.CertificateInfo, _ int) bool {
				return certItem.ID == d.config.CertificateIdOrDescription
			})
			if len(matchedCerts) == 0 {
				matchedCerts = lo.Filter(listCertificatesResp.Data.Certificates, func(certItem *dsmsdk.CertificateInfo, _ int) bool {
					return certItem.Description == d.config.CertificateIdOrDescription
				})
			}
			if len(matchedCerts) == 0 {
				return nil, fmt.Errorf("could not find certificate '%s'", d.config.CertificateIdOrDescription)
			} else {
				if len(matchedCerts) > 1 {
					d.logger.Warn("found several certificates matched '%s', using the first one")
				}
				certInfo = matchedCerts[0]
			}
		}
		importCertificateReq := &dsmsdk.ImportCertificateRequest{
			ID:          certInfo.ID,
			Description: certInfo.Description,
			Key:         privkeyPEM,
			Cert:        serverCertPEM,
			InterCert:   intermediateCertPEM,
			AsDefault:   d.config.IsDefault || certInfo.IsDefault,
		}
		_, err = d.sdkClient.ImportCertificateWithContext(ctx, importCertificateReq)
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'SYNO.Core.Certificate:import': %w", err)
		}
	}

	if d.config.IsDefault {
		listCertificatesResp, err := d.sdkClient.ListCertificatesWithContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'SYNO.Core.Certificate.CRT:list': %w", err)
		} else {
			var defaultCertId string
			for _, certItem := range listCertificatesResp.Data.Certificates {
				if certItem.IsDefault {
					defaultCertId = certItem.ID
					break
				}
			}

			if defaultCertId != "" {
				settings := make([]*dsmsdk.ServiceCertificateSetting, 0)
				for _, certItem := range listCertificatesResp.Data.Certificates {
					if certItem.ID == defaultCertId {
						continue
					}

					for _, service := range certItem.Services {
						settings = append(settings, &dsmsdk.ServiceCertificateSetting{
							Service:   service,
							CertID:    defaultCertId,
							OldCertID: certItem.ID,
						})
					}
				}
				if len(settings) > 0 {
					setServiceCertificateReq := &dsmsdk.SetServiceCertificateRequest{
						Settings: settings,
					}
					_, err = d.sdkClient.SetServiceCertificateWithContext(ctx, setServiceCertificateReq)
					if err != nil {
						return nil, fmt.Errorf("failed to execute sdk request 'SYNO.Core.Certificate.Service:set': %w", err)
					}
				}
			}
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(serverUrl string, skipTlsVerify bool) (*dsmsdk.Client, error) {
	client, err := dsmsdk.NewClient(serverUrl)
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
	Username                 string `json:"username"`
	Password                 string `json:"password"`
	TotpSecret               string `json:"totpSecret,omitempty"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}

//go:embed schema
var schemaFS embed.FS

type pluginDeployer struct{}

func (*pluginDeployer) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{ProviderType: "synologydsm", AccessProviderType: "synologydsm", ProtocolVersion: plugin.ProtocolVersion, DeployCategory: "other", DeployDisplayNameKey: "provider.synologydsm", AccessDisplayNameKey: "provider.synologydsm"}, nil
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
		ServerUrl:                  access.ServerUrl,
		Username:                   access.Username,
		Password:                   access.Password,
		TotpSecret:                 access.TotpSecret,
		AllowInsecureConnections:   access.AllowInsecureConnections,
		CertificateIdOrDescription: xmaps.GetString(extended, "certificateIdOrDesc"),
		IsDefault:                  xmaps.GetBool(extended, "isDefault"),
	})
	if err != nil {
		return nil, err
	}
	d.SetLogger(logger)
	return d.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM)
}
