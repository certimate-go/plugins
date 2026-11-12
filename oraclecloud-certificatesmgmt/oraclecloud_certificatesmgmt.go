package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/core"
	cmgrimpl "github.com/certimate-go/plugins/internal/oraclecloudcertmgr"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AuthMethod           string `json:"authMethod"`
	PrivateKey           string `json:"privateKey,omitempty"`
	PrivateKeyPassphrase string `json:"privateKeyPassphrase,omitempty"`
	PublicKeyFingerprint string `json:"publicKeyFingerprint,omitempty"`
	TenancyOcid          string `json:"tenancyOcid,omitempty"`
	UserOcid             string `json:"userOcid,omitempty"`
	CompartmentOcid      string `json:"compartmentOcid"`
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
		AuthMethod:           config.AuthMethod,
		PrivateKey:           config.PrivateKey,
		PrivateKeyPassphrase: config.PrivateKeyPassphrase,
		PublicKeyFingerprint: config.PublicKeyFingerprint,
		TenancyOcid:          config.TenancyOcid,
		UserOcid:             config.UserOcid,
		CompartmentOcid:      config.CompartmentOcid,
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
	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	return &DeployResult{}, nil
}
