package huaweicloudelb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	hwelbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
	hwelbregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/region"
	"github.com/samber/lo"

	hwelb "github.com/certimate-go/plugins/internal/cloudsdk/github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3"

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
	sdkClient *hwelb.ElbClient
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
	listCertificatesMarker := (*string)(nil)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listCertificatesReq := &hwelbmodel.ListCertificatesRequest{
			Marker: listCertificatesMarker,
			Limit:  lo.ToPtr(int32(2000)),
			Type:   lo.ToPtr([]string{"server"}),
		}
		listCertificatesResp, err := c.sdkClient.ListCertificates(listCertificatesReq)
		c.logger.Debug("sdk request 'elb.ListCertificates'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'elb.ListCertificates': %w", err)
		}

		if listCertificatesResp.Certificates == nil {
			break
		}

		for _, certItem := range *listCertificatesResp.Certificates {
			if xcert.EqualCertificatesFromPEM(certPEM, certItem.Certificate) {
				c.logger.Info("ssl certificate already exists")
				return &UploadResult{
					CertId:   certItem.Id,
					CertName: certItem.Name,
				}, nil
			}
		}

		if len(*listCertificatesResp.Certificates) == 0 || listCertificatesResp.PageInfo.NextMarker == nil {
			break
		}

		listCertificatesMarker = listCertificatesResp.PageInfo.NextMarker
	}

	projectId, err := getSDKProjectId(c.config.AccessKeyId, c.config.SecretAccessKey, c.config.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get SDK project id: %w", err)
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	createCertificateReq := &hwelbmodel.CreateCertificateRequest{
		Body: &hwelbmodel.CreateCertificateRequestBody{
			Certificate: &hwelbmodel.CreateCertificateOption{
				EnterpriseProjectId: lo.EmptyableToPtr(c.config.EnterpriseProjectId),
				ProjectId:           lo.ToPtr(projectId),
				Name:                lo.ToPtr(certName),
				Certificate:         lo.ToPtr(certPEM),
				PrivateKey:          lo.ToPtr(privkeyPEM),
			},
		},
	}
	createCertificateResp, err := c.sdkClient.CreateCertificate(createCertificateReq)
	c.logger.Debug("sdk request 'elb.CreateCertificate'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'elb.CreateCertificate': %w", err)
	}

	return &UploadResult{
		CertId:   createCertificateResp.Certificate.Id,
		CertName: createCertificateResp.Certificate.Name,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	updateCertificateReq := &hwelbmodel.UpdateCertificateRequest{
		CertificateId: certIdOrName,
		Body: &hwelbmodel.UpdateCertificateRequestBody{
			Certificate: &hwelbmodel.UpdateCertificateOption{
				Certificate: lo.ToPtr(certPEM),
				PrivateKey:  lo.ToPtr(privkeyPEM),
			},
		},
	}
	_, err := c.sdkClient.UpdateCertificate(updateCertificateReq)
	c.logger.Debug("sdk request 'elb.UpdateCertificate'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'elb.UpdateCertificate': %w", err)
	}

	return &ReplaceResult{}, nil
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*hwelb.ElbClient, error) {
	if region == "" {
		region = "cn-north-4"
	}

	auth, err := basic.NewCredentialsBuilder().
		WithAk(accessKeyId).
		WithSk(secretAccessKey).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	hcRegion, err := hwelbregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := hwelb.ElbClientBuilder().
		WithRegion(hcRegion).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	client := hwelb.NewElbClient(hcClient)
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
