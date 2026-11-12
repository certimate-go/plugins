package ucloudupathxcertmgr

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ucloud/ucloud-sdk-go/services/uaccount"
	"github.com/ucloud/ucloud-sdk-go/ucloud"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	ucloudsdk "github.com/certimate-go/plugins/internal/ucloudsdk/upathx"
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
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *ucloudsdk.UPathXClient
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.PublicKey, config.PrivateKey, config.ProjectId, config.Endpoint)
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

	certName := fmt.Sprintf("certimate_%d", time.Now().UnixMilli())

	createPathXSSLReq := c.sdkClient.NewCreatePathXSSLRequest()
	createPathXSSLReq.SSLName = ucloud.String(certName)
	createPathXSSLReq.SSLType = ucloud.String("Pem")
	createPathXSSLReq.UserCert = ucloud.String(serverCertPEM)
	createPathXSSLReq.CACert = ucloud.String(issuerCertPEM)
	createPathXSSLReq.PrivateKey = ucloud.String(privkeyPEM)
	createPathXSSLResp, err := c.sdkClient.CreatePathXSSL(createPathXSSLReq)
	c.logger.Debug("sdk request 'pathx.CreatePathXSSL'", slog.Any("response", createPathXSSLResp))

	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'pathx.CreatePathXSSL': %w", err)
	}

	return &UploadResult{
		CertId:   createPathXSSLResp.SSLId,
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

	describePathXSSLOffset := 0
	describePathXSSLLimit := 100
	for {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		default:
		}

		describePathXSSLReq := c.sdkClient.NewDescribePathXSSLRequest()
		describePathXSSLReq.Offset = ucloud.Int(describePathXSSLOffset)
		describePathXSSLReq.Limit = ucloud.Int(describePathXSSLLimit)
		describePathXSSLResp, err := c.sdkClient.DescribePathXSSL(describePathXSSLReq)
		c.logger.Debug("sdk request 'pathx.DescribePathXSSL'", slog.Any("request", describePathXSSLReq))
		if err != nil {
			return nil, false, fmt.Errorf("failed to execute sdk request 'pathx.DescribePathXSSL': %w", err)
		}

		for _, sslItem := range describePathXSSLResp.DataSet {

			if certX509.NotAfter.Unix() != int64(sslItem.ExpireTime) {
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

				newSSLContent := privkeyPEM + serverCertPEM + issuerCertPEM
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

		if len(describePathXSSLResp.DataSet) < describePathXSSLLimit {
			break
		}

		describePathXSSLOffset += describePathXSSLLimit
	}

	return nil, false, nil
}

func createSDKClient(publicKey, privateKey, projectId, endpoint string) (*ucloudsdk.UPathXClient, error) {
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

	if cfg.ProjectId == "" {
		defaultProjectId, err := getSDKDefaultProjectId(publicKey, privateKey, endpoint)
		if err != nil {
			return nil, err
		}

		cfg.ProjectId = defaultProjectId
	}

	credential := auth.NewCredential()
	credential.PublicKey = publicKey
	credential.PrivateKey = privateKey

	client := ucloudsdk.NewClient(&cfg, &credential)
	return client, nil
}

func getSDKDefaultProjectId(publicKey, privateKey, endpoint string) (string, error) {
	cfg := ucloud.NewConfig()
	if endpoint != "" {
		if strings.Contains(endpoint, "://") {
			cfg.BaseUrl = endpoint
		} else {
			cfg.BaseUrl = "https://" + endpoint
		}
	}

	credential := auth.NewCredential()
	credential.PrivateKey = privateKey
	credential.PublicKey = publicKey

	client := uaccount.NewClient(&cfg, &credential)

	request := client.NewGetProjectListRequest()
	response, err := client.GetProjectList(request)
	if err != nil {
		return "", err
	}

	for _, projectItem := range response.ProjectSet {
		if projectItem.IsDefault {
			return projectItem.ProjectId, nil
		}
	}

	return "", fmt.Errorf("ucloud: no default project found")
}
