package ctcccloudelb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pocketbase/pocketbase/tools/security"
	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	ctyunelb "github.com/certimate-go/plugins/internal/cloudsdk/ctyun/elb"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	RegionId string `json:"regionId"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *ctyunelb.Client
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
	listCertificatesReq := &ctyunelb.ListCertificatesRequest{
		RegionID: lo.ToPtr(c.config.RegionId),
	}
	listCertificatesResp, err := c.sdkClient.ListCertificatesWithContext(ctx, listCertificatesReq)
	c.logger.Debug("sdk request 'elb.ListCertificates'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'elb.ListCertificates': %w", err)
	} else {
		for _, certItem := range listCertificatesResp.ReturnObj {
			if xcert.EqualCertificatesFromPEM(certPEM, certItem.Certificate) {
				c.logger.Info("ssl certificate already exists")
				return &UploadResult{
					CertId:   certItem.ID,
					CertName: certItem.Name,
				}, nil
			}
		}
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	createCertificateReq := &ctyunelb.CreateCertificateRequest{
		ClientToken: lo.ToPtr(security.RandomString(32)),
		RegionID:    lo.ToPtr(c.config.RegionId),
		Name:        lo.ToPtr(certName),
		Description: lo.ToPtr("upload from Certimate"),
		Type:        lo.ToPtr("Server"),
		Certificate: lo.ToPtr(certPEM),
		PrivateKey:  lo.ToPtr(privkeyPEM),
	}
	createCertificateResp, err := c.sdkClient.CreateCertificateWithContext(ctx, createCertificateReq)
	c.logger.Debug("sdk request 'elb.CreateCertificate'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'elb.CreateCertificate': %w", err)
	}

	return &UploadResult{
		CertId:   createCertificateResp.ReturnObj.ID,
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(accessKeyId, secretAccessKey string) (*ctyunelb.Client, error) {
	client, err := ctyunelb.NewClient(
		ctyunelb.WithAkSk(accessKeyId, secretAccessKey),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
