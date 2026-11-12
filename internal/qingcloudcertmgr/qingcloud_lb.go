package qingcloudlb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"
	qcconfig "github.com/yunify/qingcloud-sdk-go/config"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	qclbsdk "github.com/certimate-go/plugins/internal/qingcloudsdk/lb"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	ZoneId          string `json:"zoneId"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *qclbsdk.LoadBalancerService
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey, config.ZoneId)
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
	describeServerCertificatesOffset := 0
	describeServerCertificatesLimit := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		describeServerCertificatesReq := &qclbsdk.DescribeServerCertificatesInput{
			Offset: lo.ToPtr(describeServerCertificatesOffset),
			Limit:  lo.ToPtr(describeServerCertificatesLimit),
		}
		describeServerCertificatesResp, err := c.sdkClient.DescribeServerCertificates(describeServerCertificatesReq)
		c.logger.Debug("sdk request 'lb.DescribeServerCertificates'", slog.Any("response", describeServerCertificatesResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'lb.DescribeServerCertificates': %w", err)
		}

		for _, certItem := range describeServerCertificatesResp.ServerCertificateSet {
			if xcert.EqualCertificatesFromPEM(certPEM, lo.FromPtr(certItem.CertificateContent)) {
				return &UploadResult{
					CertId:   lo.FromPtr(certItem.ServerCertificateID),
					CertName: lo.FromPtr(certItem.ServerCertificateName),
				}, nil
			}
		}

		if len(describeServerCertificatesResp.ServerCertificateSet) < describeServerCertificatesLimit {
			break
		}

		describeServerCertificatesOffset += describeServerCertificatesLimit
	}
	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())
	createServerCertificateReq := &qclbsdk.CreateServerCertificateInput{
		ServerCertificateName: lo.ToPtr(certName),
		CertificateContent:    lo.ToPtr(certPEM),
		PrivateKey:            lo.ToPtr(privkeyPEM),
	}
	createServerCertificateResp, err := c.sdkClient.CreateServerCertificate(createServerCertificateReq)
	c.logger.Debug("sdk request 'lb.CreateServerCertificate'", slog.Any("response", createServerCertificateResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'lb.CreateServerCertificate': %w", err)
	}

	return &UploadResult{
		CertId:   lo.FromPtr(createServerCertificateResp.ServerCertificateID),
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(accessKeyId, secretAccessKey, zoneId string) (*qclbsdk.LoadBalancerService, error) {
	config, err := qcconfig.New(accessKeyId, secretAccessKey)
	if err != nil {
		return nil, err
	} else {
		config.Zone = zoneId
	}

	service, err := qclbsdk.NewService(config)
	if err != nil {
		return nil, err
	}

	return service, nil
}
