package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/qiniu/go-sdk/v7/auth"

	"github.com/certimate-go/certimate/pkg/plugin"
	qiniusslcert "github.com/certimate-go/plugins/internal/qiniusslcert"
	qiniusdk "github.com/certimate-go/plugins/internal/qiniusdk"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "qiniu-kodo"
	accessType     = "qiniu"
	displayNameKey = "plugin.qiniu-kodo.name"
)

type qiniuKodoDeployer struct{}

func (*qiniuKodoDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "storage",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*qiniuKodoDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

type extendedConfig struct {
	Bucket string `json:"bucket"`
	Domain string `json:"domain"`
}

func (d *qiniuKodoDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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
	if extended.Domain == "" {
		return nil, fmt.Errorf("config `domain` is required")
	}

	certmgr, err := qiniusslcert.NewCertmgr(&qiniusslcert.CertmgrConfig{
		AccessKey: access.AccessKey,
		SecretKey: access.SecretKey,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	upres, err := certmgr.Upload(ctx, req.CertificatePEM, req.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	}
	logger.Info("ssl certificate uploaded", slog.Any("result", upres))

	kodoManager := qiniusdk.NewKodoManager(auth.New(access.AccessKey, access.SecretKey))

	bindResp, err := kodoManager.BindBucketCert(ctx, extended.Domain, upres.CertId)
	logger.Debug("sdk request 'kodo.BindCert'", slog.String("params.domain", extended.Domain), slog.String("params.certId", upres.CertId), slog.Any("response", bindResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'kodo.BindCert': %w", err)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}
