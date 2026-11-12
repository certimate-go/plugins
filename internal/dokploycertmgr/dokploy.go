package dokploy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	dokploysdk "github.com/certimate-go/plugins/internal/dokploysdk"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	ServerUrl                string `json:"serverUrl"`
	ApiKey                   string `json:"apiKey"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *dokploysdk.Client
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiKey, config.AllowInsecureConnections)
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
	certificatesAllReq := &dokploysdk.CertificatesAllRequest{}
	certificatesAllResp, err := c.sdkClient.CertificatesAllWithContext(ctx, certificatesAllReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'certificates.all': %w", err)
	} else {
		for _, certItem := range *certificatesAllResp {
			if certItem.CertificateData == certPEM && certItem.PrivateKey == privkeyPEM {
				c.logger.Info("ssl certificate already exists")
				return &UploadResult{
					CertId:   certItem.CertificateId,
					CertName: certItem.Name,
				}, nil
			}
		}
	}
	userGetReq := &dokploysdk.UserGetRequest{}
	userGetResp, err := c.sdkClient.UserGetWithContext(ctx, userGetReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'user.get': %w", err)
	}
	certificatesCreateReq := &dokploysdk.CertificatesCreateRequest{
		Name:            lo.ToPtr(fmt.Sprintf("certimate-%d", time.Now().Unix())),
		CertificateData: lo.ToPtr(certPEM),
		PrivateKey:      lo.ToPtr(privkeyPEM),
		OrganizationId:  lo.ToPtr(userGetResp.OrganizationId),
	}
	certificatesCreateResp, err := c.sdkClient.CertificatesCreateWithContext(ctx, certificatesCreateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'certificates.create': %w", err)
	}

	return &UploadResult{
		CertId:   certificatesCreateResp.CertificateId,
		CertName: certificatesCreateResp.Name,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(serverUrl, apiKey string, skipTlsVerify bool) (*dokploysdk.Client, error) {
	client, err := dokploysdk.NewClient(serverUrl,
		dokploysdk.WithApiKey(apiKey),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
