package main

import (
	"context"
	"fmt"
	"log/slog"

	vebase "github.com/volcengine/volc-sdk-golang/base"
	veimagex "github.com/volcengine/volc-sdk-golang/service/imagex/v2"

	"github.com/certimate-go/certimate/pkg/core"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/volcengine-certcenter"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	ProjectName string `json:"projectName,omitempty"`

	Region string `json:"region"`

	ServiceId string `json:"serviceId"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *veimagex.Imagex
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey, config.Region)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		AccessKeyId:     config.AccessKeyId,
		SecretAccessKey: config.SecretAccessKey,
		ProjectName:     config.ProjectName,
		Region:          config.Region,
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
	if d.config.ServiceId == "" {
		return nil, fmt.Errorf("config `serviceId` is required")
	}
	if d.config.Domain == "" {
		return nil, fmt.Errorf("config `domain` is required")
	}

	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	getDomainConfigReq := &veimagex.GetDomainConfigQuery{
		ServiceID:  d.config.ServiceId,
		DomainName: d.config.Domain,
	}
	getDomainConfigResp, err := d.sdkClient.GetDomainConfig(ctx, getDomainConfigReq)
	d.logger.Debug("sdk request 'imagex.GetDomainConfig'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'imagex.GetDomainConfig': %w", err)
	}

	updateHttpsReq := &veimagex.UpdateHTTPSReq{
		UpdateHTTPSQuery: &veimagex.UpdateHTTPSQuery{
			ServiceID: d.config.ServiceId,
		},
		UpdateHTTPSBody: &veimagex.UpdateHTTPSBody{
			Domain: d.config.Domain,
			HTTPS: &veimagex.UpdateHTTPSBodyHTTPS{
				CertID:      upres.CertId,
				EnableHTTPS: true,
			},
		},
	}
	if getDomainConfigResp.Result != nil && getDomainConfigResp.Result.HTTPSConfig != nil {
		updateHttpsReq.UpdateHTTPSBody.HTTPS.EnableHTTPS = getDomainConfigResp.Result.HTTPSConfig.EnableHTTPS
		updateHttpsReq.UpdateHTTPSBody.HTTPS.EnableHTTP2 = getDomainConfigResp.Result.HTTPSConfig.EnableHTTP2
		updateHttpsReq.UpdateHTTPSBody.HTTPS.EnableOcsp = getDomainConfigResp.Result.HTTPSConfig.EnableOcsp
		updateHttpsReq.UpdateHTTPSBody.HTTPS.TLSVersions = getDomainConfigResp.Result.HTTPSConfig.TLSVersions
		updateHttpsReq.UpdateHTTPSBody.HTTPS.EnableForceRedirect = getDomainConfigResp.Result.HTTPSConfig.EnableForceRedirect
		updateHttpsReq.UpdateHTTPSBody.HTTPS.ForceRedirectType = getDomainConfigResp.Result.HTTPSConfig.ForceRedirectType
		updateHttpsReq.UpdateHTTPSBody.HTTPS.ForceRedirectCode = getDomainConfigResp.Result.HTTPSConfig.ForceRedirectCode
	}
	_, err = d.sdkClient.UpdateHTTPS(ctx, updateHttpsReq)
	d.logger.Debug("sdk request 'imagex.UpdateHttps'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'imagex.UpdateHttps': %w", err)
	}

	return &DeployResult{}, nil
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*veimagex.Imagex, error) {
	var instance *veimagex.Imagex
	if region == "" {
		instance = veimagex.NewInstance()
	} else {
		instance = veimagex.NewInstanceWithRegion(region)
	}

	instance.SetCredential(vebase.Credentials{
		AccessKeyID:     accessKeyId,
		SecretAccessKey: secretAccessKey,
	})

	return instance, nil
}
