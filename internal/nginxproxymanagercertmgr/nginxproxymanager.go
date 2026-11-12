package nginxproxymanager

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	npmsdk "github.com/certimate-go/plugins/internal/nginxproxymanagersdk"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	ServerUrl string `json:"serverUrl"`

	AuthMethod string `json:"authMethod,omitempty"`

	Username string `json:"username,omitempty"`

	Password string `json:"password,omitempty"`

	ApiToken string `json:"apiToken,omitempty"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *npmsdk.Client
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.AuthMethod, config.Username, config.Password, config.ApiToken, config.AllowInsecureConnections)
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
	serverCertPEM, issuerCertPEM, err := xcert.ExtractCertificatesFromPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to extract certs: %w", err)
	}

	listCertificatesReq := &npmsdk.NginxListCertificatesRequest{}
	listCertificatesResp, err := c.sdkClient.NginxListCertificatesWithContext(ctx, listCertificatesReq)
	c.logger.Debug("sdk request 'nginx.ListCertificates'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'nginx.ListCertificates': %w", err)
	} else {
		for _, certItem := range *listCertificatesResp {
			if certItem.Meta.Certificate == serverCertPEM &&
				certItem.Meta.CertificateKey == privkeyPEM &&
				certItem.Meta.IntermediateCertificate == issuerCertPEM {

				c.logger.Info("ssl certificate already exists")
				return &UploadResult{
					CertId:   fmt.Sprintf("%d", certItem.Id),
					CertName: certItem.NiceName,
				}, nil
			}
		}
	}

	nginxCreateCertificateReq := &npmsdk.NginxCreateCertificateRequest{
		NiceName: fmt.Sprintf("certimate-%d", time.Now().UnixMilli()),
		Provider: "other",
	}
	nginxCreateCertificateResp, err := c.sdkClient.NginxCreateCertificateWithContext(ctx, nginxCreateCertificateReq)
	c.logger.Debug("sdk request 'nginx.CreateCertificate'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'nginx.CreateCertificate': %w", err)
	}

	ngincxUploadCertificateReq := &npmsdk.NginxUploadCertificateRequest{
		CertificateMeta: npmsdk.CertificateMeta{
			Certificate:             serverCertPEM,
			CertificateKey:          privkeyPEM,
			IntermediateCertificate: issuerCertPEM,
		},
	}
	_, err = c.sdkClient.NginxUploadCertificateWithContext(ctx, nginxCreateCertificateResp.Id, ngincxUploadCertificateReq)
	c.logger.Debug("sdk request 'nginx.UploadCertificate'", slog.Int64("params.certId", nginxCreateCertificateResp.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'nginx.UploadCertificate': %w", err)
	}

	return &UploadResult{
		CertId:   fmt.Sprintf("%d", nginxCreateCertificateResp.Id),
		CertName: nginxCreateCertificateResp.NiceName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	certId, err := strconv.ParseInt(certIdOrName, 10, 64)
	if err != nil {
		return nil, err
	}

	serverCertPEM, issuerCertPEM, err := xcert.ExtractCertificatesFromPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to extract certs: %w", err)
	}

	ngincxUploadCertificateReq := &npmsdk.NginxUploadCertificateRequest{
		CertificateMeta: npmsdk.CertificateMeta{
			Certificate:             serverCertPEM,
			CertificateKey:          privkeyPEM,
			IntermediateCertificate: issuerCertPEM,
		},
	}
	_, err = c.sdkClient.NginxUploadCertificateWithContext(ctx, certId, ngincxUploadCertificateReq)
	c.logger.Debug("sdk request 'nginx.UploadCertificate'", slog.Int64("params.certId", certId))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'nginx.UploadCertificate': %w", err)
	}

	return &ReplaceResult{}, nil
}

func createSDKClient(serverUrl, authMethod, username, password, apiToken string, skipTlsVerify bool) (*npmsdk.Client, error) {
	var client *npmsdk.Client
	var err error

	switch authMethod {
	case "", AUTH_METHOD_PASSWORD:
		client, err = npmsdk.NewClient(serverUrl, npmsdk.WithLogins(username, password))
	case AUTH_METHOD_TOKEN:
		client, err = npmsdk.NewClient(serverUrl, npmsdk.WithJwtToken(apiToken))
	default:
		err = fmt.Errorf("unsupported auth method '%s'", authMethod)
	}

	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
