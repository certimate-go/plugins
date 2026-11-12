package asiaispcdn

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	asiaispcdnsdk "github.com/certimate-go/plugins/internal/asiaispcdnsdk"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *asiaispcdnsdk.Client
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.AccessKeySecret)
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
	if upres, upok, err := c.tryGetResultIfCertExists(ctx, certPEM, privkeyPEM); err != nil {
		return nil, err
	} else if upok {
		c.logger.Info("ssl certificate already exists")
		return upres, nil
	}
	certName := fmt.Sprintf("certimate_%d", time.Now().UnixMilli())
	certificateUploadReq := &asiaispcdnsdk.CertificateUploadRequest{
		PublicKey:  lo.ToPtr(certPEM),
		PrivateKey: lo.ToPtr(privkeyPEM),
		Name:       lo.ToPtr(certName),
	}
	certificateUploadResp, err := c.sdkClient.CertificateUploadWithContext(ctx, certificateUploadReq)
	c.logger.Debug("sdk request 'certificateUpload'")
	if err != nil {
		if certificateUploadResp != nil && certificateUploadResp.GetCode() == 80003 {
			if upres, upok, err := c.tryGetResultIfCertExists(ctx, certPEM, privkeyPEM); err != nil {
				return nil, err
			} else if !upok {
				return nil, fmt.Errorf("could not find ssl certificate, may be upload failed")
			} else {
				c.logger.Info("ssl certificate already exists")
				return upres, nil
			}
		}

		return nil, fmt.Errorf("failed to execute sdk request 'certificateUpload': %w", err)
	}

	certId, err := certificateUploadResp.Data.Int64()
	if err != nil {
		return nil, fmt.Errorf("received invalid certificate id: %w", err)
	}

	return &UploadResult{
		CertId:   fmt.Sprintf("%d", certId),
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func (c *Certmgr) tryGetResultIfCertExists(ctx context.Context, certPEM, privkeyPEM string) (*UploadResult, bool, error) {
	certificateQueryListResp, err := c.sdkClient.CertificateQueryListWithContext(ctx)
	c.logger.Debug("sdk request 'certificateQueryList'")
	if err != nil {
		return nil, false, fmt.Errorf("failed to execute sdk request 'certificateQueryList': %w", err)
	} else {
		for _, certItem := range certificateQueryListResp.Data {
			certificateQueryResp, err := c.sdkClient.CertificateQueryWithContext(ctx, certItem.CertId)
			c.logger.Debug("sdk request 'certificateQuery'")
			if err != nil {
				if certificateQueryResp != nil && certificateQueryResp.GetCode() == 80012 {
					continue
				}

				return nil, false, fmt.Errorf("failed to execute sdk request 'certificateQuery': %w", err)
			}

			if certificateQueryResp.Data.PublicKey == certPEM && certificateQueryResp.Data.PrivateKey == privkeyPEM {
				return &UploadResult{
					CertId:   certItem.CertId,
					CertName: certItem.Name,
				}, true, nil
			}
		}
	}

	return nil, false, nil
}

func createSDKClient(accessKeyId, accessKeySecret string) (*asiaispcdnsdk.Client, error) {
	client, err := asiaispcdnsdk.NewClient(
		asiaispcdnsdk.WithAkSk(accessKeyId, accessKeySecret),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
