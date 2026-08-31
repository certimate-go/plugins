package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
	wangsucertmgr "github.com/certimate-go/plugins/internal/wangsucertmgr"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "wangsu-certificate"
	accessType     = "wangsu"
	displayNameKey = "plugin.wangsu-certificate.name"
)

type wangsuCertificateDeployer struct{}

func (*wangsuCertificateDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "ssl",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*wangsuCertificateDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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
	AccessKeyId     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	ApiKey          string `json:"apiKey"`
}

type extendedConfig struct {
	CertificateId string `json:"certificateId,omitempty"`
}

func (d *wangsuCertificateDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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

	certmgr, err := wangsucertmgr.NewCertmgr(&wangsucertmgr.CertmgrConfig{
		AccessKeyId:     access.AccessKeyId,
		AccessKeySecret: access.AccessKeySecret,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	if extended.CertificateId == "" {
		upres, err := certmgr.Upload(ctx, req.CertificatePEM, req.PrivateKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to upload certificate file: %w", err)
		} else {
			logger.Info("ssl certificate uploaded", slog.Any("result", upres))
		}
	} else {
		rplres, err := certmgr.Replace(ctx, extended.CertificateId, req.CertificatePEM, req.PrivateKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to replace certificate file: %w", err)
		} else {
			logger.Info("ssl certificate replaced", slog.Any("result", rplres))
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}
