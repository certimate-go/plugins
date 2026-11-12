package ctcccloudao

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
	ctyunao "github.com/certimate-go/plugins/internal/cloudsdk/ctyun/ao"
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
	sdkClient *ctyunao.Client
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

	listCertPage := 1
	listCertPerPage := 1000
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listCertsReq := &ctyunao.ListCertsRequest{
			Page:      lo.ToPtr(int32(listCertPage)),
			PerPage:   lo.ToPtr(int32(listCertPerPage)),
			UsageMode: lo.ToPtr(int32(0)),
		}
		listCertsResp, err := c.sdkClient.ListCertsWithContext(ctx, listCertsReq)
		c.logger.Debug("sdk request 'ao.ListCerts'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'ao.ListCerts': %w", err)
		}

		if listCertsResp.ReturnObj == nil {
			break
		}

		for _, certItem := range listCertsResp.ReturnObj.Results {

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

			queryCertReq := &ctyunao.QueryCertRequest{
				Id: lo.ToPtr(certItem.Id),
			}
			queryCertResp, err := c.sdkClient.QueryCertWithContext(ctx, queryCertReq)
			c.logger.Debug("sdk request 'ao.QueryCert'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'ao.QueryCert': %w", err)
			} else if queryCertResp.ReturnObj != nil && queryCertResp.ReturnObj.Result != nil {
				if !xcert.EqualCertificatesFromPEM(certPEM, queryCertResp.ReturnObj.Result.Certs) {
					continue
				}
			}

			c.logger.Info("ssl certificate already exists")
			return &UploadResult{
				CertId:   fmt.Sprintf("%d", queryCertResp.ReturnObj.Result.Id),
				CertName: queryCertResp.ReturnObj.Result.Name,
			}, nil
		}

		if len(listCertsResp.ReturnObj.Results) < listCertPerPage {
			break
		}

		listCertPage++
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	createCertReq := &ctyunao.CreateCertRequest{
		Name:  lo.ToPtr(certName),
		Certs: lo.ToPtr(certPEM),
		Key:   lo.ToPtr(privkeyPEM),
	}
	createCertResp, err := c.sdkClient.CreateCertWithContext(ctx, createCertReq)
	c.logger.Debug("sdk request 'ao.CreateCert'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'ao.CreateCert': %w", err)
	}

	return &UploadResult{
		CertId:   fmt.Sprintf("%d", createCertResp.ReturnObj.Id),
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(accessKeyId, secretAccessKey string) (*ctyunao.Client, error) {
	client, err := ctyunao.NewClient(
		ctyunao.WithAkSk(accessKeyId, secretAccessKey),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
