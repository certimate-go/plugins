package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	ctyunfaas "github.com/certimate-go/plugins/internal/cloudsdk/ctyun/faas"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	RegionId string `json:"regionId"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *ctyunfaas.Client
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return &Deployer{
		config:    config,
		logger:    slog.Default(),
		sdkClient: client,
	}, nil
}

func (d *Deployer) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}
}

func (d *Deployer) Deploy(ctx context.Context, certPEM, privkeyPEM string) (*DeployResult, error) {
	if d.config.RegionId == "" {
		return nil, fmt.Errorf("config `regionId` is required")
	}
	if d.config.Domain == "" {
		return nil, fmt.Errorf("config `domain` is required")
	}

	var faasCustomDomain *ctyunfaas.CustomDomain
	getCustomDomainReq := &ctyunfaas.GetCustomDomainRequest{
		RegionId:   lo.ToPtr(d.config.RegionId),
		DomainName: lo.ToPtr(d.config.Domain),
		CnameCheck: lo.ToPtr(false),
	}
	getCustomDomainResp, err := d.sdkClient.GetCustomDomainWithContext(ctx, getCustomDomainReq)
	d.logger.Debug("sdk request 'faas.GetCustomDomain'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'faas.GetCustomDomain': %w", err)
	} else {
		faasCustomDomain = getCustomDomainResp.ReturnObj

		if faasCustomDomain.CertConfig != nil &&
			faasCustomDomain.CertConfig.Certificate == certPEM &&
			faasCustomDomain.CertConfig.PrivateKey == privkeyPEM {
			d.logger.Info("no need to deploy faas custom domain certificate")
			return &DeployResult{}, nil
		}
	}

	updateCustomDomainReq := &ctyunfaas.UpdateCustomDomainRequest{
		RegionId:   lo.ToPtr(d.config.RegionId),
		DomainName: lo.ToPtr(d.config.Domain),
		Protocol:   lo.ToPtr(faasCustomDomain.Protocol),
		AuthConfig: faasCustomDomain.AuthConfig,
		CertConfig: &ctyunfaas.CustomDomainCertConfig{
			CertName:    fmt.Sprintf("certimate-%d", time.Now().UnixMilli()),
			Certificate: certPEM,
			PrivateKey:  privkeyPEM,
		},
	}
	if !strings.Contains(*updateCustomDomainReq.Protocol, "HTTPS") {
		if *updateCustomDomainReq.Protocol == "" {
			updateCustomDomainReq.Protocol = lo.ToPtr("HTTPS")
		} else {
			updateCustomDomainReq.Protocol = lo.ToPtr(*updateCustomDomainReq.Protocol + ",HTTPS")
		}
	}
	_, err = d.sdkClient.UpdateCustomDomainWithContext(ctx, updateCustomDomainReq)
	d.logger.Debug("sdk request 'faas.UpdateCustomDomain'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'faas.UpdateCustomDomain': %w", err)
	}

	return &DeployResult{}, nil
}

func createSDKClient(accessKeyId, secretAccessKey string) (*ctyunfaas.Client, error) {
	client, err := ctyunfaas.NewClient(
		ctyunfaas.WithAkSk(accessKeyId, secretAccessKey),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
