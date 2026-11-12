package huaweicloudscm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwscmmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/scm/v3/model"
	hwscmregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/scm/v3/region"
	"github.com/samber/lo"

	hwscm "github.com/certimate-go/plugins/internal/cloudsdk/github.com/huaweicloud/huaweicloud-sdk-go-v3/services/scm/v3"

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

	EnterpriseProjectId string `json:"enterpriseProjectId,omitempty"`

	Region string `json:"region"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *hwscm.ScmClient
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
	certX509, err := xcert.ParseCertificateFromPEM(certPEM)
	if err != nil {
		return nil, err
	}

	listCertificatesLimit := 50
	listCertificatesOffset := 0
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listCertificatesReq := &hwscmmodel.ListCertificatesRequest{
			EnterpriseProjectId: lo.EmptyableToPtr(c.config.EnterpriseProjectId),
			Limit:               lo.ToPtr(int32(listCertificatesLimit)),
			Offset:              lo.ToPtr(int32(listCertificatesOffset)),
			SortDir:             lo.ToPtr("DESC"),
			SortKey:             lo.ToPtr("certExpiredTime"),
		}
		listCertificatesResp, err := c.sdkClient.ListCertificates(listCertificatesReq)
		c.logger.Debug("sdk request 'scm.ListCertificates'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'scm.ListCertificates': %w", err)
		}

		if listCertificatesResp.Certificates == nil {
			break
		}

		for _, certItem := range *listCertificatesResp.Certificates {

			if !strings.EqualFold(certX509.Subject.CommonName, certItem.Domain) {
				continue
			}

			if certX509.NotAfter.Local().Format(time.DateTime) != strings.TrimSuffix(certItem.ExpireTime, ".0") {
				continue
			}

			exportCertificateReq := &hwscmmodel.ExportCertificateRequest{
				CertificateId: certItem.Id,
			}
			exportCertificateResp, err := c.sdkClient.ExportCertificate(exportCertificateReq)
			c.logger.Debug("sdk request 'scm.ExportCertificate'")
			if err != nil {
				if exportCertificateResp != nil && exportCertificateResp.HttpStatusCode == 404 {
					continue
				}

				return nil, fmt.Errorf("failed to execute sdk request 'scm.ExportCertificate': %w", err)
			} else {
				if !xcert.EqualCertificatesFromPEM(certPEM, lo.FromPtr(exportCertificateResp.Certificate)) {
					continue
				}
			}

			c.logger.Info("ssl certificate already exists")
			return &UploadResult{
				CertId:   certItem.Id,
				CertName: certItem.Name,
			}, nil
		}

		if len(*listCertificatesResp.Certificates) < listCertificatesLimit {
			break
		}

		listCertificatesOffset += listCertificatesLimit
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	importCertificateReq := &hwscmmodel.ImportCertificateRequest{
		Body: &hwscmmodel.ImportCertificateRequestBody{
			EnterpriseProjectId: lo.EmptyableToPtr(c.config.EnterpriseProjectId),
			Name:                certName,
			Certificate:         certPEM,
			PrivateKey:          privkeyPEM,
		},
	}
	importCertificateResp, err := c.sdkClient.ImportCertificate(importCertificateReq)
	c.logger.Debug("sdk request 'scm.ImportCertificate'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'scm.ImportCertificate': %w", err)
	}

	return &UploadResult{
		CertId:   *importCertificateResp.CertificateId,
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*hwscm.ScmClient, error) {
	if region == "" {
		region = "cn-north-4" // SCM 服务默认区域：华北北京四
	}

	auth, err := basic.NewCredentialsBuilder().
		WithAk(accessKeyId).
		WithSk(secretAccessKey).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	hcRegion, err := hwscmregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := hwscm.ScmClientBuilder().
		WithRegion(hcRegion).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	client := hwscm.NewScmClient(hcClient)
	return client, nil
}
