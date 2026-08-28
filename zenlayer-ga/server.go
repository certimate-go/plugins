package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	zcommon "github.com/zenlayer/zenlayercloud-sdk-go/zenlayercloud/common"

	"github.com/certimate-go/certimate/pkg/plugin"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
	zenlayercertmgr "github.com/certimate-go/plugins/internal/zenlayercertmgr/zga"
	zgasdk "github.com/certimate-go/plugins/internal/zenlayersdk/zga"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "zenlayer-ga"
	accessType     = "zenlayer"
	displayNameKey = "plugin.zenlayer-ga.name"

	deployTargetAccelerator = "accelerator"
	deployTargetCertificate = "certificate"
)

type zenlayerGaDeployer struct{}

func (*zenlayerGaDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "accelerator",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*zenlayerGaDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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
	AccessKeyId       string `json:"accessKeyId"`
	AccessKeyPassword string `json:"accessKeyPassword"`
	ResourceGroupId   string `json:"resourceGroupId,omitempty"`
}

type extendedConfig struct {
	DeployTarget  string `json:"deployTarget"`
	AcceleratorId string `json:"acceleratorId,omitempty"`
	CertificateId string `json:"certificateId,omitempty"`
}

func (d *zenlayerGaDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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

	certmgr, err := zenlayercertmgr.NewCertmgr(&zenlayercertmgr.CertmgrConfig{
		AccessKeyId:       access.AccessKeyId,
		AccessKeyPassword: access.AccessKeyPassword,
		ResourceGroupId:   access.ResourceGroupId,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	sdkClient, err := createSDKClient(access.AccessKeyId, access.AccessKeyPassword)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	runner := &gaRunner{
		config:     &extended,
		logger:     logger,
		sdkClient:  sdkClient,
		sdkCertmgr: certmgr,
	}

	switch extended.DeployTarget {
	case deployTargetAccelerator:
		if err := runner.deployToAccelerator(ctx, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
			return nil, err
		}

	case deployTargetCertificate:
		if err := runner.deployToCertificate(ctx, req.CertificatePEM, req.PrivateKeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", extended.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

type gaRunner struct {
	config     *extendedConfig
	logger     *slog.Logger
	sdkClient  *zgasdk.Client
	sdkCertmgr *zenlayercertmgr.Certmgr
}

func (d *gaRunner) deployToAccelerator(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.AcceleratorId == "" {
		return fmt.Errorf("config `acceleratorId` is required")
	}

	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	describeAcceleratorsReq := zgasdk.NewDescribeAcceleratorsRequest()
	describeAcceleratorsReq.AcceleratorIds = []string{d.config.AcceleratorId}
	describeAcceleratorsReq.PageNum = 1
	describeAcceleratorsReq.PageSize = 1
	describeAcceleratorsResp, err := d.sdkClient.DescribeAccelerators(describeAcceleratorsReq)
	d.logger.Debug("sdk request 'zga.DescribeAccelerators'", slog.Any("request", describeAcceleratorsReq), slog.Any("response", describeAcceleratorsResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'zga.DescribeAccelerators': %w", err)
	} else if len(describeAcceleratorsResp.Response.DataSet) == 0 {
		return fmt.Errorf("could not found accelerator '%s'", d.config.AcceleratorId)
	}

	acceleratorInfo := describeAcceleratorsResp.Response.DataSet[0]
	if acceleratorInfo.Certificate == nil || acceleratorInfo.Certificate.CertificateId != upres.CertId {
		modifyAcceleratorCertificateReq := zgasdk.NewModifyAcceleratorCertificateRequest()
		modifyAcceleratorCertificateReq.AcceleratorId = acceleratorInfo.AcceleratorId
		modifyAcceleratorCertificateReq.CertificateId = upres.CertId
		modifyAcceleratorCertificateResp, err := d.sdkClient.ModifyAcceleratorCertificate(modifyAcceleratorCertificateReq)
		d.logger.Debug("sdk request 'zga.ModifyAcceleratorCertificate'", slog.Any("request", modifyAcceleratorCertificateReq), slog.Any("response", modifyAcceleratorCertificateResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'zga.ModifyAcceleratorCertificate': %w", err)
		}
	}

	if _, err := xwait.UntilWithContext(ctx, func(_ context.Context, _ int) (bool, error) {
		describeAcceleratorsReq := zgasdk.NewDescribeAcceleratorsRequest()
		describeAcceleratorsReq.AcceleratorIds = []string{acceleratorInfo.AcceleratorId}
		describeAcceleratorsReq.PageNum = 1
		describeAcceleratorsReq.PageSize = 1
		describeAcceleratorsResp, err := d.sdkClient.DescribeAccelerators(describeAcceleratorsReq)
		d.logger.Debug("sdk request 'zga.DescribeAccelerators'", slog.Any("request", describeAcceleratorsReq), slog.Any("response", describeAcceleratorsResp))
		if err != nil {
			return false, fmt.Errorf("failed to execute sdk request 'zga.DescribeAccelerators': %w", err)
		} else if len(describeAcceleratorsResp.Response.DataSet) == 0 {
			return false, fmt.Errorf("could not found accelerator '%s'", d.config.AcceleratorId)
		}

		switch describeAcceleratorsResp.Response.DataSet[0].AcceleratorStatus {
		case "Accelerating":
			return true, nil
		case "NotAccelerate", "StopAccelerate", "AccelerateFailure":
			return false, fmt.Errorf("unexpected accelerator status")
		}

		d.logger.Info("waiting for accelerator deploying completion ...")
		return false, nil
	}, 10*time.Second); err != nil {
		return err
	}

	return nil
}

func (d *gaRunner) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
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

func createSDKClient(accessKeyId, accessKeyPassword string) (*zgasdk.Client, error) {
	config := zcommon.NewConfig()

	client, err := zgasdk.NewClient(config, accessKeyId, accessKeyPassword)
	if err != nil {
		return nil, err
	}

	return client, nil
}
