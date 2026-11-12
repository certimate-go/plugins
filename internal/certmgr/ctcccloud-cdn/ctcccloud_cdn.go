package ctcccloudcdn

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	ctyuncdn "github.com/certimate-go/plugins/internal/cloudsdk/ctyun/cdn"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *ctyuncdn.Client
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return &Certmgr{
		config:    config,
		logger:    slog.Default(),
		sdkClient: client,
	}, nil
}

func (c *Certmgr) SetLogger(logger *slog.Logger) {
	if logger == nil {
		c.logger = slog.New(slog.DiscardHandler)
	} else {
		c.logger = logger
	}
}

func (c *Certmgr) Upload(ctx context.Context, certPEM, privkeyPEM string) (*UploadResult, error) {
	certX509, err := xcert.ParseCertificateFromPEM(certPEM)
	if err != nil {
		return nil, err
	}

	queryCertListPage := 1
	queryCertListPerPage := 1000
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		queryCertListReq := &ctyuncdn.QueryCertListRequest{
			Page:      lo.ToPtr(int32(queryCertListPage)),
			PerPage:   lo.ToPtr(int32(queryCertListPerPage)),
			UsageMode: lo.ToPtr(int32(0)),
		}
		queryCertListResp, err := c.sdkClient.QueryCertListWithContext(ctx, queryCertListReq)
		c.logger.Debug("sdk request 'cdn.QueryCertList'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.QueryCertList': %w", err)
		}

		if queryCertListResp.ReturnObj == nil {
			break
		}

		for _, certItem := range queryCertListResp.ReturnObj.Results {

			if !strings.EqualFold(certX509.Subject.CommonName, certItem.CN) {
				continue
			}

			if !slices.Equal(certX509.DNSNames, certItem.SANs) {
				continue
			}

			if certX509.NotBefore.Unix() != certItem.IssueTime {
				continue
			} else if certX509.NotAfter.Unix() != certItem.ExpiresTime {
				continue
			}

			queryCertDetailReq := &ctyuncdn.QueryCertDetailRequest{
				Id: lo.ToPtr(certItem.Id),
			}
			queryCertDetailResp, err := c.sdkClient.QueryCertDetailWithContext(ctx, queryCertDetailReq)
			c.logger.Debug("sdk request 'cdn.QueryCertDetail'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'cdn.QueryCertDetail': %w", err)
			} else if queryCertDetailResp.ReturnObj != nil && queryCertDetailResp.ReturnObj.Result != nil {
				if !xcert.EqualCertificatesFromPEM(certPEM, queryCertDetailResp.ReturnObj.Result.Certs) {
					continue
				}
			}

			c.logger.Info("ssl certificate already exists")
			return &UploadResult{
				CertId:   fmt.Sprintf("%d", queryCertDetailResp.ReturnObj.Result.Id),
				CertName: queryCertDetailResp.ReturnObj.Result.Name,
			}, nil
		}

		if len(queryCertListResp.ReturnObj.Results) < queryCertListPerPage {
			break
		}

		queryCertListPage++
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	createCertReq := &ctyuncdn.CreateCertRequest{
		Name:  lo.ToPtr(certName),
		Certs: lo.ToPtr(certPEM),
		Key:   lo.ToPtr(privkeyPEM),
	}
	createCertResp, err := c.sdkClient.CreateCertWithContext(ctx, createCertReq)
	c.logger.Debug("sdk request 'cdn.CreateCert'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cdn.CreateCert': %w", err)
	}

	return &UploadResult{
		CertId:   fmt.Sprintf("%d", createCertResp.ReturnObj.Id),
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(accessKeyId, secretAccessKey string) (*ctyuncdn.Client, error) {
	client, err := ctyuncdn.NewClient(
		ctyuncdn.WithAkSk(accessKeyId, secretAccessKey),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
