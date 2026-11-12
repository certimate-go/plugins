package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	hwapigmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/apig/v2/model"
	hwapigregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/apig/v2/region"
	"github.com/samber/lo"

	hwapig "github.com/certimate-go/plugins/internal/cloudsdk/github.com/huaweicloud/huaweicloud-sdk-go-v3/services/apig/v2"

	"github.com/certimate-go/certimate/pkg/core"
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

	CertificateId string `json:"certificateId,omitempty"`
}

type Deployer struct {
	config    *DeployerConfig
	logger    *slog.Logger
	sdkClient *hwapig.ApigClient
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(
		config.AccessKeyId,
		config.SecretAccessKey,
		config.Region,
	)
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
	switch d.config.DeployTarget {
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

	showDetailsOfCertificateV2Req := &hwapigmodel.ShowDetailsOfCertificateV2Request{
		CertificateId: d.config.CertificateId,
	}
	showDetailsOfCertificateV2Resp, err := d.sdkClient.ShowDetailsOfCertificateV2(showDetailsOfCertificateV2Req)
	d.logger.Debug("sdk request 'apig.ShowDetailsOfCertificateV2'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'apig.ShowDetailsOfCertificateV2': %w", err)
	}

	updateCertificateV2Req := &hwapigmodel.UpdateCertificateV2Request{
		CertificateId: d.config.CertificateId,
		Body: &hwapigmodel.CertificateForm{
			Name:        fmt.Sprintf("certimate_%d", time.Now().UnixMilli()),
			CertContent: certPEM,
			PrivateKey:  privkeyPEM,
			Type: lo.
				If(showDetailsOfCertificateV2Resp.Type.Value() == hwapigmodel.GetCertificateFormTypeEnum().INSTANCE.Value(), lo.ToPtr(hwapigmodel.GetCertificateFormTypeEnum().INSTANCE)).
				Else(lo.ToPtr(hwapigmodel.GetCertificateFormTypeEnum().GLOBAL)),
			InstanceId: showDetailsOfCertificateV2Resp.InstanceId,
		},
	}
	_, err = d.sdkClient.UpdateCertificateV2(updateCertificateV2Req)
	d.logger.Debug("sdk request 'apig.UpdateCertificateV2'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'apig.UpdateCertificateV2': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*hwapig.ApigClient, error) {
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

	hcRegion, err := hwapigregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := hwapig.ApigClientBuilder().
		WithRegion(hcRegion).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	client := hwapig.NewApigClient(hcClient)
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
