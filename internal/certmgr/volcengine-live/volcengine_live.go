package volcenginelive

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/samber/lo"
	velive "github.com/volcengine/volc-sdk-golang/service/live/v20230101"
	ve "github.com/volcengine/volcengine-go-sdk/volcengine"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	ProjectName string `json:"projectName,omitempty"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *velive.Live
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client := velive.NewInstance()
	client.SetAccessKey(config.AccessKeyId)
	client.SetSecretKey(config.SecretAccessKey)

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
	listCertPageNum := 1
	listCertPageSize := 10
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listCertReq := &velive.ListCertV2Body{}
		listCertReq.ProjectName = lo.EmptyableToPtr(c.config.ProjectName)
		listCertReq.PageNum = ve.Int32(int32(listCertPageNum))
		listCertReq.PageSize = ve.Int32(int32(listCertPageSize))
		listCertResp, err := c.sdkClient.ListCertV2(ctx, listCertReq)
		c.logger.Debug("sdk request 'live.ListCertV2'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'live.ListCertV2': %w", err)
		}

		for _, certItem := range listCertResp.Result.CertList {

			describeCertDetailSecretReq := &velive.DescribeCertDetailSecretV2Body{
				ChainID: ve.String(certItem.ChainID),
			}
			describeCertDetailSecretResp, err := c.sdkClient.DescribeCertDetailSecretV2(ctx, describeCertDetailSecretReq)
			c.logger.Debug("sdk request 'live.DescribeCertDetailSecretV2'")
			if err != nil {
				continue
			}

			if xcert.EqualCertificatesFromPEM(certPEM, strings.Join(describeCertDetailSecretResp.Result.SSL.Chain, "\n\n")) {
				c.logger.Info("ssl certificate already exists")
				return &UploadResult{
					CertId:   certItem.ChainID,
					CertName: certItem.CertName,
				}, nil
			}
		}

		if len(listCertResp.Result.CertList) < listCertPageSize {
			break
		}

		listCertPageNum++
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	createCertReq := &velive.CreateCertBody{
		ProjectName: lo.EmptyableToPtr(c.config.ProjectName),
		CertName:    ve.String(certName),
		Rsa: velive.CreateCertBodyRsa{
			Prikey: privkeyPEM,
			Pubkey: certPEM,
		},
		UseWay: "https",
	}
	createCertResp, err := c.sdkClient.CreateCert(ctx, createCertReq)
	c.logger.Debug("sdk request 'live.CreateCert'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'live.CreateCert': %w", err)
	}

	return &UploadResult{
		CertId:   *createCertResp.Result.ChainID,
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	createCertReq := &velive.CreateCertBody{
		ProjectName: lo.EmptyableToPtr(c.config.ProjectName),
		ChainID:     ve.String(certIdOrName),
		Rsa: velive.CreateCertBodyRsa{
			Prikey: privkeyPEM,
			Pubkey: certPEM,
		},
		UseWay: "https",
	}
	_, err := c.sdkClient.CreateCert(ctx, createCertReq)
	c.logger.Debug("sdk request 'live.CreateCert'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'live.CreateCert': %w", err)
	}

	return &ReplaceResult{}, nil
}
