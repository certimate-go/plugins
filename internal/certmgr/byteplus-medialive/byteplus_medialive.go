package byteplusmedialive

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	bp "github.com/byteplus-sdk/byteplus-go-sdk-v2/byteplus"
	bplive "github.com/byteplus-sdk/byteplus-sdk-golang/service/live/v20230101"
	"github.com/samber/lo"

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
	sdkClient *bplive.Live
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client := bplive.NewInstance()
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

		listCertReq := &bplive.ListCertV2Body{}
		listCertReq.ProjectName = lo.EmptyableToPtr(c.config.ProjectName)
		listCertReq.PageNum = bp.Int32(int32(listCertPageNum))
		listCertReq.PageSize = bp.Int32(int32(listCertPageSize))
		listCertResp, err := c.sdkClient.ListCertV2(listCertReq)
		c.logger.Debug("sdk request 'medialive.ListCertV2'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'medialive.ListCertV2': %w", err)
		}

		for _, certItem := range listCertResp.Result.CertList {

			describeCertDetailSecretReq := &bplive.DescribeCertDetailSecretV2Body{
				ChainID: bp.String(certItem.ChainID),
			}
			describeCertDetailSecretResp, err := c.sdkClient.DescribeCertDetailSecretV2(describeCertDetailSecretReq)
			c.logger.Debug("sdk request 'medialive.DescribeCertDetailSecretV2'")
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

	createCertReq := &bplive.CreateCertBody{
		ProjectName: lo.EmptyableToPtr(c.config.ProjectName),
		CertName:    bp.String(certName),
		Rsa: bplive.CreateCertBodyRsa{
			Prikey: privkeyPEM,
			Pubkey: certPEM,
		},
		UseWay: "https",
	}
	createCertResp, err := c.sdkClient.CreateCert(createCertReq)
	c.logger.Debug("sdk request 'medialive.CreateCert'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'medialive.CreateCert': %w", err)
	}

	return &UploadResult{
		CertId:   *createCertResp.Result.ChainID,
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	createCertReq := &bplive.CreateCertBody{
		ProjectName: lo.EmptyableToPtr(c.config.ProjectName),
		ChainID:     bp.String(certIdOrName),
		Rsa: bplive.CreateCertBodyRsa{
			Prikey: privkeyPEM,
			Pubkey: certPEM,
		},
		UseWay: "https",
	}
	_, err := c.sdkClient.CreateCert(createCertReq)
	c.logger.Debug("sdk request 'medialive.CreateCert'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'medialive.CreateCert': %w", err)
	}

	return &ReplaceResult{}, nil
}
