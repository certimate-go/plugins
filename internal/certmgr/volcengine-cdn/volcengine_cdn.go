package volcenginecdn

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	ve "github.com/volcengine/volcengine-go-sdk/volcengine"
	vesession "github.com/volcengine/volcengine-go-sdk/volcengine/session"

	vecdn "github.com/certimate-go/plugins/internal/cloudsdk/github.com/volcengine/volcengine-go-sdk/service/cdn"

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

	ProjectName string `json:"projectName,omitempty"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *vecdn.CDN
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey)
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

	listCertInfoPageNum := 1
	listCertInfoPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listCertInfoReq := &vecdn.ListCertInfoInput{
			Source:   ve.String("volc_cert_center"),
			PageNum:  ve.Int32(int32(listCertInfoPageNum)),
			PageSize: ve.Int32(int32(listCertInfoPageSize)),
		}
		listCertInfoResp, err := c.sdkClient.ListCertInfoWithContext(ctx, listCertInfoReq)
		c.logger.Debug("sdk request 'cdn.ListCertInfo'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.ListCertInfo': %w", err)
		}

		fingerprintSha1 := sha1.Sum(certX509.Raw)
		fingerprintSha1Hex := hex.EncodeToString(fingerprintSha1[:])
		fingerprintSha256 := sha256.Sum256(certX509.Raw)
		fingerprintSha256Hex := hex.EncodeToString(fingerprintSha256[:])
		for _, certItem := range listCertInfoResp.CertInfo {

			if !strings.EqualFold(fingerprintSha1Hex, ve.StringValue(certItem.CertFingerprint.Sha1)) {
				continue
			} else if !strings.EqualFold(fingerprintSha256Hex, ve.StringValue(certItem.CertFingerprint.Sha256)) {
				continue
			}

			c.logger.Info("ssl certificate already exists")
			return &UploadResult{
				CertId:   ve.StringValue(certItem.CertId),
				CertName: ve.StringValue(certItem.Desc),
			}, nil
		}

		if len(listCertInfoResp.CertInfo) < listCertInfoPageSize {
			break
		}

		listCertInfoPageNum++
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	addCertificateReq := &vecdn.AddCertificateInput{
		Source:      ve.String("volc_cert_center"),
		Certificate: ve.String(certPEM),
		PrivateKey:  ve.String(privkeyPEM),
		Desc:        ve.String(certName),
	}
	addCertificateResp, err := c.sdkClient.AddCertificateWithContext(ctx, addCertificateReq)
	c.logger.Debug("sdk request 'cdn.AddCertificate'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cdn.AddCertificate': %w", err)
	}

	return &UploadResult{
		CertId:   ve.StringValue(addCertificateResp.CertId),
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(accessKeyId, accessKeySecret string) (*vecdn.CDN, error) {
	config := ve.NewConfig().
		WithAkSk(accessKeyId, accessKeySecret).
		WithRegion("cn-north-1")

	session, err := vesession.NewSession(config)
	if err != nil {
		return nil, err
	}

	client := vecdn.New(session)
	return client, nil
}
