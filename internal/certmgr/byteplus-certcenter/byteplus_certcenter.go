package bytepluscertcenter

import (
	"context"
	"fmt"
	"log/slog"

	bp "github.com/byteplus-sdk/byteplus-go-sdk-v2/byteplus"
	bpsesion "github.com/byteplus-sdk/byteplus-go-sdk-v2/byteplus/session"
	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	bpcertificateservice "github.com/certimate-go/plugins/internal/cloudsdk/byteplus/certificateservice"
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

	Region string `json:"region"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *bpcertificateservice.CERTIFICATESERVICE
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey, config.Region)
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
	importCertificateReq := &bpcertificateservice.ImportCertificateInput{
		ProjectName: lo.EmptyableToPtr(c.config.ProjectName),
		CertificateInfo: &bpcertificateservice.CertificateInfoForImportCertificateInput{
			CertificateChain: bp.String(certPEM),
			PrivateKey:       bp.String(privkeyPEM),
		},
		Repeatable: bp.Bool(false),
	}
	importCertificateResp, err := c.sdkClient.ImportCertificateWithContext(ctx, importCertificateReq)
	c.logger.Debug("sdk request 'certificateservice.ImportCertificate'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'certificateservice.ImportCertificate': %w", err)
	}

	var sslId string
	if importCertificateResp.InstanceId != nil && *importCertificateResp.InstanceId != "" {
		sslId = *importCertificateResp.InstanceId
	}
	if importCertificateResp.RepeatId != nil && *importCertificateResp.RepeatId != "" {
		sslId = *importCertificateResp.RepeatId
	}

	if sslId == "" {
		return nil, fmt.Errorf("received empty certificate id, both `InstanceId` and `RepeatId` are empty")
	}

	return &UploadResult{
		CertId: sslId,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*bpcertificateservice.CERTIFICATESERVICE, error) {
	if region == "" {
		region = "ap-singapore-1"
	}

	config := bp.NewConfig().
		WithAkSk(accessKeyId, secretAccessKey).
		WithRegion(region).
		WithEndpoint("certificate-service.byteplusapi.com")

	session, err := bpsesion.NewSession(config)
	if err != nil {
		return nil, err
	}

	client := bpcertificateservice.New(session, config)
	return client, nil
}
