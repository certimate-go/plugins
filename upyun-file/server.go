package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	upyuncertmgr "github.com/certimate-go/plugins/internal/upyuncertmgr"
	upyunsdk "github.com/certimate-go/plugins/internal/upyunsdk"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "upyun-file"
	accessType     = "upyun"
	displayNameKey = "plugin.upyun-file.name"
)

type upyunFileDeployer struct{}

func (*upyunFileDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "storage",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*upyunFileDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	accessSchema, _ := plugin.LoadAccessSchema(schemaFS)
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{
		AccessSchemaJSON: accessSchema,
		DeploySchemaJSON: deploySchema,
		I18n:             i18n,
	}, nil
}

type accessConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type extendedConfig struct {
	Bucket string `json:"bucket"`
	Domain string `json:"domain"`
}

func (d *upyunFileDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}

	var extended extendedConfig
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}

	certmgr, err := upyuncertmgr.NewCertmgr(&upyuncertmgr.CertmgrConfig{
		Username: access.Username,
		Password: access.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	sdkClient, err := createSDKClient(access.Username, access.Password)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	runner := &fileRunner{
		config:     &extended,
		logger:     logger,
		sdkClient:  sdkClient,
		sdkCertmgr: certmgr,
	}

	if err := runner.deploy(ctx, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
		return nil, err
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

type fileRunner struct {
	config     *extendedConfig
	logger     *slog.Logger
	sdkClient  *upyunsdk.Client
	sdkCertmgr *upyuncertmgr.Certmgr
}

func (d *fileRunner) deploy(ctx context.Context, certPEM, privkeyPEM string) error {
	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	getHttpsServiceManagerResp, err := d.sdkClient.GetHttpsServiceManagerWithContext(ctx, d.config.Domain)
	d.logger.Debug("sdk request 'console.GetHttpsServiceManager'", slog.String("params.domain", d.config.Domain), slog.Any("response", getHttpsServiceManagerResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'console.GetHttpsServiceManager': %w", err)
	}

	_, lastCertIndex, _ := lo.FindIndexOf(getHttpsServiceManagerResp.Data.Domains, func(item upyunsdk.HttpsServiceManagerDomain) bool {
		return item.Https
	})
	if lastCertIndex == -1 {
		updateHttpsCertificateManagerReq := &upyunsdk.UpdateHttpsCertificateManagerRequest{
			CertificateId: upres.CertId,
			Domain:        d.config.Domain,
			Https:         true,
			ForceHttps:    true,
		}
		updateHttpsCertificateManagerResp, err := d.sdkClient.UpdateHttpsCertificateManagerWithContext(ctx, updateHttpsCertificateManagerReq)
		d.logger.Debug("sdk request 'console.EnableDomainHttps'", slog.Any("request", updateHttpsCertificateManagerReq), slog.Any("response", updateHttpsCertificateManagerResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'console.UpdateHttpsCertificateManager': %w", err)
		}
	} else if getHttpsServiceManagerResp.Data.Domains[lastCertIndex].CertificateId != upres.CertId {
		migrateHttpsDomainReq := &upyunsdk.MigrateHttpsDomainRequest{
			CertificateId: upres.CertId,
			Domain:        d.config.Domain,
		}
		migrateHttpsDomainResp, err := d.sdkClient.MigrateHttpsDomainWithContext(ctx, migrateHttpsDomainReq)
		d.logger.Debug("sdk request 'console.MigrateHttpsDomain'", slog.Any("request", migrateHttpsDomainReq), slog.Any("response", migrateHttpsDomainResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'console.MigrateHttpsDomain': %w", err)
		}
	}

	return nil
}

func createSDKClient(username, password string) (*upyunsdk.Client, error) {
	client, err := upyunsdk.NewClient(
		upyunsdk.WithLogins(username, password),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
