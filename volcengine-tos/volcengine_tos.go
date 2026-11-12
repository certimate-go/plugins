package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/certimate-go/certimate/pkg/core"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/volcengine-certcenter"
	tossdk "github.com/certimate-go/plugins/internal/cloudsdk/volcengine/tos"
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

	Bucket string `json:"bucket"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *tossdk.Client
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

	putBucketCustomDomainReq := &tossdk.PutBucketCustomDomainRequest{
		CustomDomainRule: &tossdk.PutBucketCustomDomainRequestCustomDomainRule{
			Domain: d.config.Domain,
			CertId: upres.CertId,
		},
	}
	_, err = d.sdkClient.PutBucketCustomDomainWithContext(ctx, putBucketCustomDomainReq)
	d.logger.Debug("sdk request 'tos.PutBucketCustomDomain'", slog.String("params.bucket", d.config.Bucket))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'tos.PutBucketCustomDomain': %w", err)
	}

	return &DeployResult{}, nil
}

func createSDKClient(accessKeyId, secretAccessKey, region, bucket string) (*tossdk.Client, error) {
	client, err := tossdk.NewClient("",
		tossdk.WithAkSk(accessKeyId, secretAccessKey),
		tossdk.WithRegion(region),
		tossdk.WithBucket(bucket),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
