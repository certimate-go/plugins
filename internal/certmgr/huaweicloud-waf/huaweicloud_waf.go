package huaweicloudwaf

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	hwwafmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/waf/v1/model"
	hwwafregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/waf/v1/region"
	"github.com/samber/lo"

	hwwaf "github.com/certimate-go/plugins/internal/cloudsdk/github.com/huaweicloud/huaweicloud-sdk-go-v3/services/waf/v1"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	xhuaweicloud "github.com/certimate-go/plugins/internal/cloudsdk/huaweicloud/auth"
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
	sdkClient *hwwaf.WafClient
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
	listCertificatesPage := 1
	listCertificatesPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listCertificatesReq := &hwwafmodel.ListCertificatesRequest{
			EnterpriseProjectId: lo.EmptyableToPtr(c.config.EnterpriseProjectId),
			Page:                lo.ToPtr(int32(listCertificatesPage)),
			Pagesize:            lo.ToPtr(int32(listCertificatesPageSize)),
		}
		listCertificatesResp, err := c.sdkClient.ListCertificates(listCertificatesReq)
		c.logger.Debug("sdk request 'waf.ListCertificates'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'waf.ListCertificates': %w", err)
		}

		if listCertificatesResp.Items == nil {
			break
		}

		for _, certItem := range *listCertificatesResp.Items {
			showCertificateReq := &hwwafmodel.ShowCertificateRequest{
				EnterpriseProjectId: lo.EmptyableToPtr(c.config.EnterpriseProjectId),
				CertificateId:       certItem.Id,
			}
			showCertificateResp, err := c.sdkClient.ShowCertificate(showCertificateReq)
			c.logger.Debug("sdk request 'waf.ShowCertificate'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'waf.ShowCertificate': %w", err)
			}

			if xcert.EqualCertificatesFromPEM(certPEM, lo.FromPtr(showCertificateResp.Content)) {
				c.logger.Info("ssl certificate already exists")
				return &UploadResult{
					CertId:   certItem.Id,
					CertName: certItem.Name,
				}, nil
			}
		}

		if len(*listCertificatesResp.Items) < listCertificatesPageSize {
			break
		}

		listCertificatesPage++
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	createCertificateReq := &hwwafmodel.CreateCertificateRequest{
		EnterpriseProjectId: lo.EmptyableToPtr(c.config.EnterpriseProjectId),
		Body: &hwwafmodel.CreateCertificateRequestBody{
			Name:    certName,
			Content: certPEM,
			Key:     privkeyPEM,
		},
	}
	createCertificateResp, err := c.sdkClient.CreateCertificate(createCertificateReq)
	c.logger.Debug("sdk request 'waf.CreateCertificate'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'waf.CreateCertificate': %w", err)
	}

	return &UploadResult{
		CertId:   *createCertificateResp.Id,
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*hwwaf.WafClient, error) {
	projectId, err := getSDKProjectId(accessKeyId, secretAccessKey, region)
	if err != nil {
		return nil, err
	}

	auth, err := basic.NewCredentialsBuilder().
		WithAk(accessKeyId).
		WithSk(secretAccessKey).
		WithProjectId(projectId).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	hcRegion, err := hwwafregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := hwwaf.WafClientBuilder().
		WithRegion(hcRegion).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	client := hwwaf.NewWafClient(hcClient)
	return client, nil
}

func getSDKProjectId(accessKeyId, secretAccessKey, region string) (string, error) {
	auth, err := global.NewCredentialsBuilder().
		WithAk(accessKeyId).
		WithSk(secretAccessKey).
		SafeBuild()
	if err != nil {
		return "", err
	}

	return xhuaweicloud.GetKeystoneProjectIDWithRegion(auth, region)
}
