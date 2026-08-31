package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
	kcmcertmgr "github.com/certimate-go/plugins/internal/ksyuncertmgr/kcm"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "ksyun-kcm"
	accessType     = "ksyun"
	displayNameKey = "plugin.ksyun-kcm.name"
)

type ksyunKcmDeployer struct{}

func (*ksyunKcmDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "ssl",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*ksyunKcmDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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
	SecretAccessKey string `json:"secretAccessKey"`
}

type extendedConfig struct{}

func (d *ksyunKcmDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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

	certmgr, err := kcmcertmgr.NewCertmgr(&kcmcertmgr.CertmgrConfig{
		AccessKeyId:     access.AccessKeyId,
		SecretAccessKey: access.SecretAccessKey,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	upres, err := certmgr.Upload(ctx, req.CertificatePEM, req.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}
