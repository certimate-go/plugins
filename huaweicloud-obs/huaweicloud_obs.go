package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/core"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/huaweicloud-scm"
	obssdk "github.com/certimate-go/plugins/internal/cloudsdk/huaweicloud/obs"
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

	Bucket string `json:"bucket"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *obssdk.Client
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey, config.Region, config.Bucket)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		AccessKeyId:         config.AccessKeyId,
		SecretAccessKey:     config.SecretAccessKey,
		EnterpriseProjectId: config.EnterpriseProjectId,
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
	if d.config.Region == "" {
		return nil, fmt.Errorf("config `region` is required")
	}
	if d.config.Bucket == "" {
		return nil, fmt.Errorf("config `bucket` is required")
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

	putBucketCustomDomainReq := &obssdk.PutBucketCustomDomainRequest{
		CustomDomain:     d.config.Domain,
		Name:             upres.CertName,
		CertificateId:    upres.CertId,
		Certificate:      certPEM,
		CertificateChain: certPEM,
		PrivateKey:       privkeyPEM,
	}
	_, err = d.sdkClient.PutBucketCustomDomainWithContext(ctx, putBucketCustomDomainReq)
	d.logger.Debug("sdk request 'obs.PutBucketCustomDomain'", slog.String("params.bucket", d.config.Bucket), slog.String("params.customdomain", d.config.Domain))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'obs.PutBucketCustomDomain': %w", err)
	}

	return &DeployResult{}, nil
}

func createSDKClient(accessKeyId, secretAccessKey, region, bucket string) (*obssdk.Client, error) {
	client, err := obssdk.NewClient("",
		obssdk.WithAkSk(accessKeyId, secretAccessKey),
		obssdk.WithRegion(region),
		obssdk.WithBucket(bucket),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
