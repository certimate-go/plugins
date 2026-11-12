package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/core"
	cmgrimpl "github.com/certimate-go/plugins/internal/yandexcloudcertmgr"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	FolderId          string `json:"folderId"`
	ServiceAccountKey string `json:"serviceAccountKey"`
	CertificateId     string `json:"certificateId,omitempty"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		FolderId:          config.FolderId,
		ServiceAccountKey: config.ServiceAccountKey,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}

	return &Deployer{
		config:     config,
		logger:     slog.Default(),
		sdkCertmgr: pcertmgr,
	}, nil
}

func (d *Deployer) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}

	d.sdkCertmgr.SetLogger(logger)
}

func (d *Deployer) Deploy(ctx context.Context, certPEM, privkeyPEM string) (*DeployResult, error) {
	if d.config.CertificateId == "" {
		upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to upload certificate file: %w", err)
		} else {
			d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
		}
	} else {
		rplres, err := d.sdkCertmgr.Replace(ctx, d.config.CertificateId, certPEM, privkeyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to replace certificate file: %w", err)
		} else {
			d.logger.Info("ssl certificate replaced", slog.Any("result", rplres))
		}
	}

	return &DeployResult{}, nil
}
