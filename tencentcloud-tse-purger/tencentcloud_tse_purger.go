package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"

	tctse "github.com/certimate-go/certimate/pkg/sdk3rd-trimmed/github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tse/v20201207"

	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
)

const (
	// 服务类型：云原生网关。
	SERVICE_TYPE_CLOUDNATIVE = "cloudnative"
)

type PurgerConfig struct {
	SecretId    string `json:"secretId"`
	SecretKey   string `json:"secretKey"`
	ProjectId   int64  `json:"projectId,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	Region      string `json:"region"`
	ServiceType string `json:"serviceType"`
	GatewayId   string `json:"gatewayId,omitempty"`
}

type Purger struct {
	config    *PurgerConfig
	logger    *slog.Logger
	sdkClient *tctse.Client
}

func NewPurger(config *PurgerConfig) (*Purger, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the purger provider is nil")
	}

	client, err := createSDKClient(config.SecretId, config.SecretKey, config.Endpoint, config.Region)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return &Purger{
		config:    config,
		logger:    slog.Default(),
		sdkClient: client,
	}, nil
}

func (p *Purger) SetLogger(logger *slog.Logger) {
	if logger == nil {
		p.logger = slog.New(slog.DiscardHandler)
	} else {
		p.logger = logger
	}
}

func (p *Purger) Purge(ctx context.Context, expiry time.Duration) (int, error) {
	switch p.config.ServiceType {
	case SERVICE_TYPE_CLOUDNATIVE:
		return p.purgeCloudNative(ctx, expiry)

	default:
		return 0, fmt.Errorf("unsupported service type '%s'", p.config.ServiceType)
	}
}

func (p *Purger) purgeCloudNative(ctx context.Context, expiry time.Duration) (int, error) {
	if p.config.GatewayId == "" {
		return 0, fmt.Errorf("config `gatewayId` is required")
	}

	purgingCertIds := make([]string, 0)

	// 查询云原生网关证书列表
	// REF: https://cloud.tencent.com/document/api/1364/98588
	describeCloudNativeAPIGatewayCertificatesOffset := 0
	describeCloudNativeAPIGatewayCertificatesLimit := 100
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		describeCloudNativeAPIGatewayCertificatesReq := tctse.NewDescribeCloudNativeAPIGatewayCertificatesRequest()
		describeCloudNativeAPIGatewayCertificatesReq.GatewayId = common.StringPtr(p.config.GatewayId)
		describeCloudNativeAPIGatewayCertificatesReq.Offset = common.Int64Ptr(int64(describeCloudNativeAPIGatewayCertificatesOffset))
		describeCloudNativeAPIGatewayCertificatesReq.Limit = common.Int64Ptr(int64(describeCloudNativeAPIGatewayCertificatesLimit))
		describeCloudNativeAPIGatewayCertificatesResp, err := p.sdkClient.DescribeCloudNativeAPIGatewayCertificatesWithContext(ctx, describeCloudNativeAPIGatewayCertificatesReq)
		p.logger.Debug("sdk request 'tse.DescribeCloudNativeAPIGatewayCertificates'", slog.Any("request", describeCloudNativeAPIGatewayCertificatesReq), slog.Any("response", describeCloudNativeAPIGatewayCertificatesResp))
		if err != nil {
			return 0, fmt.Errorf("failed to execute sdk request 'tse.DescribeCloudNativeAPIGatewayCertificates': %w", err)
		}

		for _, certItem := range describeCloudNativeAPIGatewayCertificatesResp.Response.Result.CertificatesList {
			certNotAfter, err := time.ParseInLocation(time.DateTime, lo.FromPtr(certItem.ExpireTime), time.FixedZone("CST", 8*60*60))
			if err != nil {
				continue
			}

			if time.Since(certNotAfter) >= expiry {
				purgingCertIds = append(purgingCertIds, lo.FromPtr(certItem.CertId))
			}
		}

		if len(describeCloudNativeAPIGatewayCertificatesResp.Response.Result.CertificatesList) < describeCloudNativeAPIGatewayCertificatesLimit {
			break
		}

		describeCloudNativeAPIGatewayCertificatesOffset += describeCloudNativeAPIGatewayCertificatesLimit
	}

	// 删除证书
	// REF: https://cloud.tencent.com/document/api/1364/98587
	purgedCount := 0
	if len(purgingCertIds) > 0 {
		p.logger.Info("found certificates to purge", slog.Any("certIds", purgingCertIds))

		if err := xloop.ForRangeAllWithContext(ctx, purgingCertIds, func(ctx context.Context, certId string, i int) error {
			if i > 0 {
				if err := xwait.DelayWithContext(ctx, 1*time.Second); err != nil {
					return err
				}
			}

			deleteCloudNativeAPIGatewayCertificateReq := tctse.NewDeleteCloudNativeAPIGatewayCertificateRequest()
			deleteCloudNativeAPIGatewayCertificateReq.GatewayId = common.StringPtr(p.config.GatewayId)
			deleteCloudNativeAPIGatewayCertificateReq.Id = common.StringPtr(certId)
			deleteCloudNativeAPIGatewayCertificateResp, err := p.sdkClient.DeleteCloudNativeAPIGatewayCertificateWithContext(ctx, deleteCloudNativeAPIGatewayCertificateReq)
			p.logger.Debug("sdk request 'tse.DeleteCloudNativeAPIGatewayCertificate'", slog.Any("request", deleteCloudNativeAPIGatewayCertificateReq), slog.Any("response", deleteCloudNativeAPIGatewayCertificateResp))
			if err != nil {
				if sdkErr, ok := err.(*tcerrors.TencentCloudSDKError); ok {
					if sdkErrCode := sdkErr.Code; sdkErrCode == "ResourceNotFound.ResourceNotFound" {
						return nil
					}
				}

				return fmt.Errorf("failed to execute sdk request 'tse.DeleteCloudNativeAPIGatewayCertificate': %w", err)
			}

			purgedCount++
			return nil
		}); err != nil {
			return 0, err
		}
	}

	return purgedCount, nil
}

func createSDKClient(secretId, secretKey, endpoint, region string) (*tctse.Client, error) {
	credential := common.NewCredential(secretId, secretKey)

	cpf := profile.NewClientProfile()
	if endpoint != "" {
		cpf.HttpProfile.Endpoint = endpoint
	}

	client, err := tctse.NewClient(credential, region, cpf)
	if err != nil {
		return nil, err
	}

	return client, nil
}
