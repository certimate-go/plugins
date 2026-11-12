package cmcccloudvlb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/config"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkvlb/model"
	"github.com/samber/lo"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkvlb"

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

	AccessKeySecret string `json:"accessKeySecret"`

	PoolId string `json:"poolId"`

	IsDefault bool `json:"isDefault,omitempty"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *ecloudsdkvlb.Client
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.AccessKeySecret, config.PoolId)
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

	listLoadbalanceCertificationResp, err := c.sdkClient.ListLoadbalanceCertificationResp()
	c.logger.Debug("sdk request 'vlb.ListLoadbalanceCertification'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'vlb.ListLoadbalanceCertification': %w", err)
	} else {
		if listLoadbalanceCertificationResp.Body != nil && listLoadbalanceCertificationResp.Body.Content != nil {
			for _, certItem := range *listLoadbalanceCertificationResp.Body.Content {

				newCertNotAfter := certX509.NotAfter
				oldCertNotAfter, _ := time.ParseInLocation(time.DateTime, lo.FromPtr(certItem.ExpirationTime), time.FixedZone("CST", 8*60*60))
				if !newCertNotAfter.Equal(oldCertNotAfter) {
					continue
				}

				getLoadbalanceCertificationDetailReq := &model.GetLoadbalanceCertificationDetailRespRequest{
					GetLoadbalanceCertificationDetailRespPath: &model.GetLoadbalanceCertificationDetailRespPath{
						ContainerUuid: certItem.Id,
					},
				}
				getLoadbalanceCertificationDetailResp, err := c.sdkClient.GetLoadbalanceCertificationDetailResp(getLoadbalanceCertificationDetailReq)
				c.logger.Debug("sdk request 'vlb.GetLoadbalanceCertificationDetail'")
				if err != nil {
					return nil, fmt.Errorf("failed to execute sdk request 'vlb.GetLoadbalanceCertificationDetail': %w", err)
				} else {
					if !xcert.EqualCertificatesFromPEM(certPEM, lo.FromPtr(getLoadbalanceCertificationDetailResp.Body.PublicKey)) {
						continue
					}
				}

				c.logger.Info("ssl certificate already exists")
				return &UploadResult{
					CertId:   lo.FromPtr(certItem.Id),
					CertName: lo.FromPtr(certItem.Name),
				}, nil
			}
		}
	}

	certName := fmt.Sprintf("certimate_%d", time.Now().UnixMilli())

	createLoadbalanceCertificationReq := &model.CreateLoadbalanceCertificationRequest{
		CreateLoadbalanceCertificationBody: &model.CreateLoadbalanceCertificationBody{
			Name:        lo.ToPtr(certName),
			Type:        lo.ToPtr(lo.Ternary(c.config.IsDefault, model.CreateLoadbalanceCertificationBodyTypeEnumServer, model.CreateLoadbalanceCertificationBodyTypeEnumSni)),
			Description: lo.ToPtr("upload from Certimate"),
			PublicKey:   lo.ToPtr(certPEM),
			PrivateKey:  lo.ToPtr(privkeyPEM),
		},
	}
	createLoadbalanceCertificationResp, err := c.sdkClient.CreateLoadbalanceCertification(createLoadbalanceCertificationReq)
	c.logger.Debug("sdk request 'vlb.CreateLoadbalanceCertification'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'vlb.CreateLoadbalanceCertification': %w", err)
	}

	return &UploadResult{
		CertId:   lo.FromPtr(createLoadbalanceCertificationResp.Body),
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(accessKeyId, accessKeySecret, poolId string) (*ecloudsdkvlb.Client, error) {
	client := ecloudsdkvlb.NewClient(&config.Config{
		AccessKey: &accessKeyId,
		SecretKey: &accessKeySecret,
		PoolId:    &poolId,
	})

	return client, nil
}
