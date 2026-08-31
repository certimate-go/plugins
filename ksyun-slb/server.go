package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/plugin"
	slbcertmgr "github.com/certimate-go/plugins/internal/ksyuncertmgr/slb"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "ksyun-slb"
	accessType     = "ksyun"
	displayNameKey = "plugin.ksyun-slb.name"

	deployTargetCertificate = "certificate"
)

type ksyunSlbDeployer struct{}

func (*ksyunSlbDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "loadbalance",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*ksyunSlbDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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

type extendedConfig struct {
	ProjectId     int64  `json:"projectId,omitempty"`
	Region        string `json:"region"`
	DeployTarget  string `json:"deployTarget"`
	CertificateId string `json:"certificateId,omitempty"`
}

func (d *ksyunSlbDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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

	certmgr, err := slbcertmgr.NewCertmgr(&slbcertmgr.CertmgrConfig{
		AccessKeyId:     access.AccessKeyId,
		SecretAccessKey: access.SecretAccessKey,
		ProjectId:       extended.ProjectId,
		Region:          extended.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	runner := &slbRunner{
		config:     &extended,
		logger:     logger,
		sdkCertmgr: certmgr,
	}

	switch extended.DeployTarget {
	case deployTargetCertificate:
		if err := runner.deployToCertificate(ctx, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", extended.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

type slbRunner struct {
	config     *extendedConfig
	logger     *slog.Logger
	sdkCertmgr *slbcertmgr.Certmgr
}

func (d *slbRunner) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.CertificateId == "" {
		return fmt.Errorf("config `certificateId` is required")
	}

	rplres, err := d.sdkCertmgr.Replace(ctx, d.config.CertificateId, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to replace certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate replaced", slog.Any("result", rplres))
	}

	return nil
}
