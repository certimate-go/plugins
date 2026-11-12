package jdcloudsslcert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	jdcore "github.com/jdcloud-api/jdcloud-sdk-go/core"
	jdsslapis "github.com/jdcloud-api/jdcloud-sdk-go/services/ssl/apis"

	jdssl "github.com/certimate-go/certimate/pkg/sdk3rd-trimmed/github.com/jdcloud-api/jdcloud-sdk-go/services/ssl/client"

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
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *jdssl.SslClient
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.AccessKeySecret)
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

	privkeyPEM = strings.TrimSpace(privkeyPEM)
	privkeyPEM = strings.ReplaceAll(privkeyPEM, "\r", "")
	privkeyPEM = strings.ReplaceAll(privkeyPEM, "\n", "\r\n")
	privkeyPEM = privkeyPEM + "\r\n"

	describeCertsPageNumber := 1
	describeCertsPageSize := 10
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		describeCertsReq := jdsslapis.NewDescribeCertsRequestWithoutParam()
		describeCertsReq.SetDomainName(certX509.Subject.CommonName)
		describeCertsReq.SetPageNumber(describeCertsPageNumber)
		describeCertsReq.SetPageSize(describeCertsPageSize)
		describeCertsResp, err := c.sdkClient.DescribeCerts(describeCertsReq)
		if err == nil && describeCertsResp.Error.Code != 0 {
			err = fmt.Errorf("JDCloud API error %d (%s): %s", describeCertsResp.Error.Code, describeCertsResp.Error.Status, describeCertsResp.Error.Message)
		}
		c.logger.Debug("sdk request 'ssl.DescribeCerts'", slog.Any("request", describeCertsReq), slog.Any("response", describeCertsResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'ssl.DescribeCerts': %w", err)
		}

		keyDigest := sha256.Sum256([]byte(privkeyPEM))
		keyDigestHex := hex.EncodeToString(keyDigest[:])
		for _, certItem := range describeCertsResp.Result.CertListDetails {

			if !strings.EqualFold(certX509.Subject.CommonName, certItem.CommonName) {
				continue
			}

			if !strings.EqualFold(strings.Join(certX509.DNSNames, ","), strings.Join(certItem.DnsNames, ",")) {
				continue
			}

			newCertNotBefore := certX509.NotBefore
			newCertNotAfter := certX509.NotAfter
			oldCertNotBefore, _ := time.Parse(time.RFC3339, certItem.StartTime)
			oldCertNotAfter, _ := time.Parse(time.RFC3339, certItem.EndTime)
			if !newCertNotBefore.Equal(oldCertNotBefore) || !newCertNotAfter.Equal(oldCertNotAfter) {
				continue
			}

			if !strings.EqualFold(keyDigestHex, certItem.Digest) {
				continue
			}

			c.logger.Info("ssl certificate already exists")
			return &UploadResult{
				CertId:   certItem.CertId,
				CertName: certItem.CertName,
			}, nil
		}

		if len(describeCertsResp.Result.CertListDetails) < int(describeCertsPageSize) {
			break
		} else {
			describeCertsPageNumber++
		}
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	uploadCertReq := jdsslapis.NewUploadCertRequestWithoutParam()
	uploadCertReq.SetCertName(certName)
	uploadCertReq.SetCertFile(certPEM)
	uploadCertReq.SetKeyFile(privkeyPEM)
	uploadCertResp, err := c.sdkClient.UploadCert(uploadCertReq)
	if err == nil && uploadCertResp.Error.Code != 0 {
		err = fmt.Errorf("JDCloud API error %d (%s): %s", uploadCertResp.Error.Code, uploadCertResp.Error.Status, uploadCertResp.Error.Message)
	}
	c.logger.Debug("sdk request 'ssl.UploadCertificate'", slog.Any("response", uploadCertResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'ssl.UploadCertificate': %w", err)
	}

	return &UploadResult{
		CertId:   uploadCertResp.Result.CertId,
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(accessKeyId, accessKeySecret string) (*jdssl.SslClient, error) {
	clientCredentials := jdcore.NewCredentials(accessKeyId, accessKeySecret)
	client := jdssl.NewSslClient(clientCredentials)
	client.DisableLogger()
	return client, nil
}
