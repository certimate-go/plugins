package ucloudulbcertmgr

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ucloud/ucloud-sdk-go/ucloud"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	ucloudsdk "github.com/certimate-go/plugins/internal/ucloudsdk/ulb"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	PublicKey string `json:"publicKey"`

	PrivateKey string `json:"privateKey"`

	ProjectId string `json:"projectId,omitempty"`

	Endpoint string `json:"endpoint,omitempty"`

	Region string `json:"region"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *ucloudsdk.ULBClient
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.PublicKey, config.PrivateKey, config.ProjectId, config.Endpoint, config.Region)
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

	serverCertPEM, issuerCertPEM, err := xcert.ExtractCertificatesFromPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to extract certs: %w", err)
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	createSSLReq := c.sdkClient.NewCreateSSLRequest()
	createSSLReq.SSLName = ucloud.String(certName)
	createSSLReq.SSLType = ucloud.String("Pem")
	createSSLReq.UserCert = ucloud.String(serverCertPEM)
	createSSLReq.CaCert = ucloud.String(issuerCertPEM)
	createSSLReq.PrivateKey = ucloud.String(privkeyPEM)
	createSSLResp, err := c.sdkClient.CreateSSL(createSSLReq)
	c.logger.Debug("sdk request 'ulb.CreateSSL'", slog.Any("response", createSSLResp))

	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'ulb.CreateSSL': %w", err)
	}

	return &UploadResult{
		CertId:   createSSLResp.SSLId,
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func (c *Certmgr) tryGetResultIfCertExists(ctx context.Context, certPEM, privkeyPEM string) (*UploadResult, bool, error) {
	certX509, err := xcert.ParseCertificateFromPEM(certPEM)
	if err != nil {
		return nil, false, err
	}

	describeSSLOffset := 0
	describeSSLLimit := 100
	for {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		default:
		}

		describeSSLReq := c.sdkClient.NewDescribeSSLRequest()
		describeSSLReq.Offset = ucloud.Int(describeSSLOffset)
		describeSSLReq.Limit = ucloud.Int(describeSSLLimit)
		describeSSLResp, err := c.sdkClient.DescribeSSL(describeSSLReq)
		c.logger.Debug("sdk request 'ulb.DescribeSSL'", slog.Any("request", describeSSLReq))
		if err != nil {
			return nil, false, fmt.Errorf("failed to execute sdk request 'ulb.DescribeSSL': %w", err)
		}

		for _, sslItem := range describeSSLResp.DataSet {

			if certX509.NotBefore.Unix() != int64(sslItem.NotBefore) {
				continue
			} else if certX509.NotAfter.Unix() != int64(sslItem.NotAfter) {
				continue
			}

			serverCertPEM, issuerCertPEM, err := xcert.ExtractCertificatesFromPEM(certPEM)
			if err != nil {
				continue
			} else {
				oldSSLContent := sslItem.SSLContent
				oldSSLContent = strings.ReplaceAll(oldSSLContent, "\r", "")
				oldSSLContent = strings.ReplaceAll(oldSSLContent, "\n", "")
				oldSSLContent = strings.ReplaceAll(oldSSLContent, "\t", "")
				oldSSLContent = strings.ReplaceAll(oldSSLContent, " ", "")

				newSSLContent := serverCertPEM + privkeyPEM + issuerCertPEM
				newSSLContent = strings.ReplaceAll(newSSLContent, "\r", "")
				newSSLContent = strings.ReplaceAll(newSSLContent, "\n", "")
				newSSLContent = strings.ReplaceAll(newSSLContent, "\t", "")
				newSSLContent = strings.ReplaceAll(newSSLContent, " ", "")

				if oldSSLContent != newSSLContent {
					continue
				}
			}

			return &UploadResult{
				CertId:   sslItem.SSLId,
				CertName: sslItem.SSLName,
			}, true, nil
		}

		if len(describeSSLResp.DataSet) < describeSSLLimit {
			break
		}

		describeSSLOffset += describeSSLLimit
	}

	return nil, false, nil
}

func createSDKClient(publicKey, privateKey, projectId, endpoint, region string) (*ucloudsdk.ULBClient, error) {
	if publicKey == "" {
		return nil, fmt.Errorf("ucloud: invalid public key")
	}
	if privateKey == "" {
		return nil, fmt.Errorf("ucloud: invalid private key")
	}

	cfg := ucloud.NewConfig()
	if projectId != "" {
		cfg.ProjectId = projectId
	}
	if endpoint != "" {
		if strings.Contains(endpoint, "://") {
			cfg.BaseUrl = endpoint
		} else {
			cfg.BaseUrl = "https://" + endpoint
		}
	}
	if region != "" {
		cfg.Region = region
	}

	credential := auth.NewCredential()
	credential.PublicKey = publicKey
	credential.PrivateKey = privateKey

	client := ucloudsdk.NewClient(&cfg, &credential)
	return client, nil
}
