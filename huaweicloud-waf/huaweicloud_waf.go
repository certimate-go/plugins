package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	hwwafmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/waf/v1/model"
	hwwafregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/waf/v1/region"
	"github.com/samber/lo"

	hwwaf "github.com/certimate-go/plugins/internal/cloudsdk/github.com/huaweicloud/huaweicloud-sdk-go-v3/services/waf/v1"

	"github.com/certimate-go/certimate/pkg/core"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/huaweicloud-waf"
	xhuaweicloud "github.com/certimate-go/plugins/internal/cloudsdk/huaweicloud/auth"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	EnterpriseProjectId string `json:"enterpriseProjectId,omitempty"`

	Region string `json:"region"`

	DeployTarget string `json:"deployTarget"`

	Domain string `json:"domain,omitempty"`

	CertificateId string `json:"certificateId,omitempty"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *hwwaf.WafClient
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
		AccessKeyId:         config.AccessKeyId,
		SecretAccessKey:     config.SecretAccessKey,
		EnterpriseProjectId: config.EnterpriseProjectId,
		Region:              config.Region,
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
	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	switch d.config.DeployTarget {
	case DEPLOY_TARGET_CLOUDSERVER:
		if err := d.deployToCloudServer(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	case DEPLOY_TARGET_PREMIUMHOST:
		if err := d.deployToPremiumHost(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	case DEPLOY_TARGET_CERTIFICATE:
		if err := d.deployToCertificate(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &DeployResult{}, nil
}

func (d *Deployer) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.CertificateId == "" {
		return fmt.Errorf("config `certificateId` is required")
	}

	showCertificateReq := &hwwafmodel.ShowCertificateRequest{
		EnterpriseProjectId: lo.EmptyableToPtr(d.config.EnterpriseProjectId),
		CertificateId:       d.config.CertificateId,
	}
	showCertificateResp, err := d.sdkClient.ShowCertificate(showCertificateReq)
	d.logger.Debug("sdk request 'waf.ShowCertificate'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'waf.ShowCertificate': %w", err)
	}

	updateCertificateReq := &hwwafmodel.UpdateCertificateRequest{
		EnterpriseProjectId: lo.EmptyableToPtr(d.config.EnterpriseProjectId),
		CertificateId:       d.config.CertificateId,
		Body: &hwwafmodel.UpdateCertificateRequestBody{
			Name:    *showCertificateResp.Name,
			Content: lo.ToPtr(certPEM),
			Key:     lo.ToPtr(privkeyPEM),
		},
	}
	_, err = d.sdkClient.UpdateCertificate(updateCertificateReq)
	d.logger.Debug("sdk request 'waf.UpdateCertificate'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'waf.UpdateCertificate': %w", err)
	}

	return nil
}

func (d *Deployer) deployToCloudServer(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.Domain == "" {
		return fmt.Errorf("config `domain` is required")
	}

	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	hostId := ""
	listHostPage := 1
	listHostPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		listHostReq := &hwwafmodel.ListHostRequest{
			EnterpriseProjectId: lo.EmptyableToPtr(d.config.EnterpriseProjectId),
			Hostname:            lo.ToPtr(strings.TrimPrefix(d.config.Domain, "*")),
			Page:                lo.ToPtr(int32(listHostPage)),
			Pagesize:            lo.ToPtr(int32(listHostPageSize)),
		}
		listHostResp, err := d.sdkClient.ListHost(listHostReq)
		d.logger.Debug("sdk request 'waf.ListHost'")
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'waf.ListHost': %w", err)
		}

		if listHostResp.Items == nil {
			break
		}

		for _, hostItem := range *listHostResp.Items {
			if strings.TrimPrefix(d.config.Domain, "*") == *hostItem.Hostname {
				hostId = *hostItem.Id
				break
			}
		}

		if len(*listHostResp.Items) < listHostPageSize {
			break
		}

		listHostPage++
	}
	if hostId == "" {
		return fmt.Errorf("could not find cloudserver host '%s'", d.config.Domain)
	}

	updateHostReq := &hwwafmodel.UpdateHostRequest{
		EnterpriseProjectId: lo.EmptyableToPtr(d.config.EnterpriseProjectId),
		InstanceId:          hostId,
		Body: &hwwafmodel.UpdateHostRequestBody{
			Certificateid:   lo.ToPtr(upres.CertId),
			Certificatename: lo.ToPtr(upres.CertName),
		},
	}
	_, err = d.sdkClient.UpdateHost(updateHostReq)
	d.logger.Debug("sdk request 'waf.UpdateHost'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'waf.UpdateHost': %w", err)
	}

	return nil
}

func (d *Deployer) deployToPremiumHost(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.Domain == "" {
		return fmt.Errorf("config `domain` is required")
	}

	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	var hostId string
	listPremiumHostPage := 1
	listPremiumHostPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		listPremiumHostReq := &hwwafmodel.ListPremiumHostRequest{
			EnterpriseProjectId: lo.EmptyableToPtr(d.config.EnterpriseProjectId),
			Hostname:            lo.ToPtr(strings.TrimPrefix(d.config.Domain, "*")),
			Page:                lo.ToPtr(fmt.Sprintf("%d", listPremiumHostPage)),
			Pagesize:            lo.ToPtr(fmt.Sprintf("%d", listPremiumHostPageSize)),
		}
		listPremiumHostResp, err := d.sdkClient.ListPremiumHost(listPremiumHostReq)
		d.logger.Debug("sdk request 'waf.ListPremiumHost'")
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'waf.ListPremiumHost': %w", err)
		}

		if listPremiumHostResp.Items == nil {
			break
		}

		for _, hostItem := range *listPremiumHostResp.Items {
			if strings.TrimPrefix(d.config.Domain, "*") == *hostItem.Hostname {
				hostId = *hostItem.Id
				break
			}
		}

		if len(*listPremiumHostResp.Items) < listPremiumHostPageSize {
			break
		}

		listPremiumHostPage++
	}
	if hostId == "" {
		return fmt.Errorf("could not find premium host '%s'", d.config.Domain)
	}

	updatePremiumHostReq := &hwwafmodel.UpdatePremiumHostRequest{
		EnterpriseProjectId: lo.EmptyableToPtr(d.config.EnterpriseProjectId),
		HostId:              hostId,
		Body: &hwwafmodel.UpdatePremiumHostRequestBody{
			Certificateid:   lo.ToPtr(upres.CertId),
			Certificatename: lo.ToPtr(upres.CertName),
		},
	}
	_, err = d.sdkClient.UpdatePremiumHost(updatePremiumHostReq)
	d.logger.Debug("sdk request 'waf.UpdatePremiumHost'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'waf.UpdatePremiumHost': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*hwwaf.WafClient, error) {
	projectId, err := getSDKProjectId(accessKeyId, secretAccessKey, region)
	if err != nil {
		return nil, err
	}

	auth, err := basic.NewCredentialsBuilder().
		WithAk(accessKeyId).
		WithSk(secretAccessKey).
		WithProjectId(projectId).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	hcRegion, err := hwwafregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := hwwaf.WafClientBuilder().
		WithRegion(hcRegion).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	client := hwwaf.NewWafClient(hcClient)
	return client, nil
}

func getSDKProjectId(accessKeyId, secretAccessKey, region string) (string, error) {
	auth, err := global.NewCredentialsBuilder().
		WithAk(accessKeyId).
		WithSk(secretAccessKey).
		SafeBuild()
	if err != nil {
		return "", err
	}

	return xhuaweicloud.GetKeystoneProjectIDWithRegion(auth, region)
}
