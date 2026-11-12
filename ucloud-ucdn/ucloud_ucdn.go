package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/ucloud/ucloud-sdk-go/ucloud"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"

	"github.com/certimate-go/certimate/pkg/core"
	ucloudsdk "github.com/certimate-go/plugins/internal/ucloudsdk/ucdn"
	cmgrimpl "github.com/certimate-go/plugins/internal/ucloudusslcertmgr"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	PublicKey string `json:"publicKey"`

	PrivateKey string `json:"privateKey"`

	ProjectId string `json:"projectId,omitempty"`

	Endpoint string `json:"endpoint,omitempty"`

	DomainId string `json:"domainId"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *ucloudsdk.UCDNClient
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.PublicKey, config.PrivateKey, config.ProjectId, config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		PublicKey:  config.PublicKey,
		PrivateKey: config.PrivateKey,
		ProjectId:  config.ProjectId,
		Endpoint:   config.Endpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}

	return &Deployer{
		config:     config,
		logger:     slog.Default(),
		sdkClient:  client,
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
	if d.config.DomainId == "" {
		return nil, fmt.Errorf("config `domainId` is required")
	}

	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	var domainConfigInfo ucloudsdk.DomainConfigInfo
	getUcdnDomainConfigReq := d.sdkClient.NewGetUcdnDomainConfigRequest()
	getUcdnDomainConfigReq.DomainId = []string{d.config.DomainId}
	getUcdnDomainConfigResp, err := d.sdkClient.GetUcdnDomainConfig(getUcdnDomainConfigReq)
	d.logger.Debug("sdk request 'ucdn.GetUcdnDomainConfig'", slog.Any("request", getUcdnDomainConfigReq), slog.Any("response", getUcdnDomainConfigResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'ucdn.GetUcdnDomainConfig': %w", err)
	} else {
		if len(getUcdnDomainConfigResp.DomainList) == 0 {
			return nil, fmt.Errorf("could not find domain '%s'", d.config.DomainId)
		}

		domainConfigInfo = getUcdnDomainConfigResp.DomainList[0]
	}

	certId, _ := strconv.Atoi(upres.CertId)
	updateUcdnDomainHttpsConfigV2Req := d.sdkClient.NewUpdateUcdnDomainHttpsConfigV2Request()
	updateUcdnDomainHttpsConfigV2Req.DomainId = ucloud.String(d.config.DomainId)
	updateUcdnDomainHttpsConfigV2Req.HttpsStatusCn = lo.
		IfF(domainConfigInfo.AreaCode == "all" || domainConfigInfo.AreaCode == "cn", func() *string { return ucloud.String(domainConfigInfo.HttpsStatusCn) }).
		Else(nil)
	updateUcdnDomainHttpsConfigV2Req.HttpsStatusAbroad = lo.
		IfF(domainConfigInfo.AreaCode == "all" || domainConfigInfo.AreaCode == "abroad", func() *string { return ucloud.String(domainConfigInfo.HttpsStatusAbroad) }).
		Else(nil)
	updateUcdnDomainHttpsConfigV2Req.CertId = ucloud.Int(certId)
	updateUcdnDomainHttpsConfigV2Req.CertName = ucloud.String(upres.CertName)
	updateUcdnDomainHttpsConfigV2Req.CertType = ucloud.String("ussl")
	updateUcdnDomainHttpsConfigV2Resp, err := d.sdkClient.UpdateUcdnDomainHttpsConfigV2(updateUcdnDomainHttpsConfigV2Req)
	d.logger.Debug("sdk request 'ucdn.UpdateUcdnDomainHttpsConfigV2'", slog.Any("request", updateUcdnDomainHttpsConfigV2Req), slog.Any("response", updateUcdnDomainHttpsConfigV2Resp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'ucdn.UpdateUcdnDomainHttpsConfigV2': %w", err)
	}

	return &DeployResult{}, nil
}

func createSDKClient(publicKey, privateKey, projectId, endpoint string) (*ucloudsdk.UCDNClient, error) {
	if publicKey == "" {
		return nil, fmt.Errorf("ucloud: invalid public key")
	}
	if privateKey == "" {
		return nil, fmt.Errorf("ucloud: invalid private key")
	}

	cfg := ucloud.NewConfig()
	if projectId != "" {
		cfg.ProjectId = projectId
	}
	if endpoint != "" {
		if strings.Contains(endpoint, "://") {
			cfg.BaseUrl = endpoint
		} else {
			cfg.BaseUrl = "https://" + endpoint
		}
	}

	credential := auth.NewCredential()
	credential.PublicKey = publicKey
	credential.PrivateKey = privateKey

	client := ucloudsdk.NewClient(&cfg, &credential)
	return client, nil
}
