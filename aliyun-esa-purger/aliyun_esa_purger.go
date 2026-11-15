package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	aliopen "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/alibabacloud-go/tea/tea"

	aliesa "github.com/certimate-go/certimate/pkg/sdk3rd-trimmed/github.com/alibabacloud-go/esa-20240910/v3/client"

	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
)

type PurgerConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	ResourceGroupId string `json:"resourceGroupId,omitempty"`
	Region          string `json:"region"`
	SiteId          int64  `json:"siteId"`
}

type Purger struct {
	config    *PurgerConfig
	logger    *slog.Logger
	sdkClient *aliesa.Client
}

func NewPurger(config *PurgerConfig) (*Purger, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the purger provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.AccessKeySecret, config.Region)
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
	if p.config.SiteId == 0 {
		return 0, fmt.Errorf("config `siteId` is required")
	}

	purgingCertIds := make([]string, 0)

	// 查询站点证书列表
	// REF: https://help.aliyun.com/zh/edge-security-acceleration/esa/api-esa-2024-09-10-listcertificates
	listCertificatesPageNumber := 1
	listCertificatesPageSize := 10
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		listCertificatesReq := &aliesa.ListCertificatesRequest{
			SiteId:     tea.Int64(p.config.SiteId),
			PageNumber: tea.Int64(int64(listCertificatesPageNumber)),
			PageSize:   tea.Int64(int64(listCertificatesPageSize)),
		}
		listCertificatesResp, err := p.sdkClient.ListCertificatesWithContext(ctx, listCertificatesReq, &dara.RuntimeOptions{})
		p.logger.Debug("sdk request 'esa.ListCertificates'", slog.Any("request", listCertificatesReq), slog.Any("response", listCertificatesResp))
		if err != nil {
			return 0, fmt.Errorf("failed to execute sdk request 'esa.ListCertificates': %w", err)
		}

		for _, certItem := range listCertificatesResp.Body.Result {
			certNotAfter, err := time.ParseInLocation(time.DateTime, tea.StringValue(certItem.NotAfter), time.FixedZone("CST", 8*60*60))
			if err != nil {
				continue
			}

			if time.Since(certNotAfter) >= expiry {
				purgingCertIds = append(purgingCertIds, tea.StringValue(certItem.Id))
			}
		}

		if len(listCertificatesResp.Body.Result) < listCertificatesPageSize {
			break
		}

		listCertificatesPageNumber++
	}

	// 删除站点证书
	// REF: https://help.aliyun.com/zh/edge-security-acceleration/esa/api-esa-2024-09-10-deletecertificate
	purgedCount := 0
	if len(purgingCertIds) > 0 {
		p.logger.Info("found certificates to purge", slog.Any("certIds", purgingCertIds))

		if err := xloop.ForRangeAllWithContext(ctx, purgingCertIds, func(ctx context.Context, certId string, i int) error {
			if i > 0 {
				if err := xwait.DelayWithContext(ctx, 1*time.Second); err != nil {
					return err
				}
			}

			deleteCertificateReq := &aliesa.DeleteCertificateRequest{
				SiteId: tea.Int64(p.config.SiteId),
				Id:     tea.String(certId),
			}
			deleteCertificateResp, err := p.sdkClient.DeleteCertificateWithContext(ctx, deleteCertificateReq, &dara.RuntimeOptions{})
			p.logger.Debug("sdk request 'esa.DeleteCertificate'", slog.Any("request", deleteCertificateReq), slog.Any("response", deleteCertificateResp))
			if err != nil {
				if sdkErr, ok := err.(*tea.SDKError); ok {
					if sdkErrCode := tea.StringValue(sdkErr.Code); sdkErrCode == "Certificate.NotFound" {
						return nil
					}
				}

				return fmt.Errorf("failed to execute sdk request 'esa.DeleteCertificate': %w", err)
			}

			purgedCount++
			return nil
		}); err != nil {
			return 0, err
		}
	}

	return purgedCount, nil
}

func createSDKClient(accessKeyId, accessKeySecret, region string) (*aliesa.Client, error) {
	// 接入点一览 https://api.aliyun.com/product/ESA
	var endpoint string
	switch region {
	case "":
		endpoint = "esa.cn-hangzhou.aliyuncs.com"
	default:
		endpoint = fmt.Sprintf("esa.%s.aliyuncs.com", region)
	}

	config := &aliopen.Config{
		AccessKeyId:     tea.String(accessKeyId),
		AccessKeySecret: tea.String(accessKeySecret),
		Endpoint:        tea.String(endpoint),
	}

	client, err := aliesa.NewClient(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}
