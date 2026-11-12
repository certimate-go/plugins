package baiducloudcertmgr

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/baidubce/bce-sdk-go/bce"

	bcehttp "github.com/baidubce/bce-sdk-go/http"
	baiducert "github.com/baidubce/bce-sdk-go/services/cert"
	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

type UploadResult = core.CertmgrUploadResult

type CertmgrConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`
}

type Certmgr struct {
	logger    *slog.Logger
	sdkClient *baiducert.Client
}

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return &Certmgr{
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

	listCertDetail, err := c.sdkClient.ListCertDetail()
	c.logger.Debug("sdk request 'cert.ListCertDetail'", slog.Any("response", listCertDetail))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cert.ListCertDetail': %w", err)
	} else {
		for _, certItem := range listCertDetail.Certs {

			if !strings.EqualFold(certX509.Subject.CommonName, certItem.CertCommonName) {
				continue
			}

			if certItem.CertDNSNames != strings.Join(certX509.DNSNames, ",") {
				continue
			}

			newCertNotBefore := certX509.NotBefore
			newCertNotAfter := certX509.NotAfter
			oldCertNotBefore, _ := time.Parse(time.RFC3339, certItem.CertStartTime)
			oldCertNotAfter, _ := time.Parse(time.RFC3339, certItem.CertStopTime)
			if !newCertNotBefore.Equal(oldCertNotBefore) || !newCertNotAfter.Equal(oldCertNotAfter) {
				continue
			}

			existingPEM, err := c.getCertRawData(certItem.CertId)
			c.logger.Debug("sdk request 'cert.GetCertRawData'", slog.String("params.certId", certItem.CertId))
			if err != nil {
				if sdkErr, ok := err.(*bce.BceServiceError); ok {
					if sdkErrCode := sdkErr.Code; sdkErrCode == "ResourceNotFoundException" {
						continue
					}
				}

				return nil, fmt.Errorf("failed to execute sdk request 'cert.GetCertRawData': %w", err)
			} else {
				if !xcert.EqualCertificatesFromPEM(certPEM, existingPEM) {
					continue
				}
			}

			c.logger.Info("ssl certificate already exists")
			return &UploadResult{
				CertId:   certItem.CertId,
				CertName: certItem.CertName,
			}, nil
		}
	}

	createCertReq := &baiducert.CreateCertArgs{}
	createCertReq.CertName = fmt.Sprintf("certimate-%d", time.Now().UnixMilli())
	createCertReq.CertServerData = certPEM
	createCertReq.CertPrivateData = privkeyPEM
	createCertResp, err := c.sdkClient.CreateCert(createCertReq)
	c.logger.Debug("sdk request 'cert.CreateCert'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cert.CreateCert': %w", err)
	}

	return &UploadResult{
		CertId:   createCertResp.CertId,
		CertName: createCertResp.CertName,
	}, nil
}

func (c *Certmgr) getCertRawData(id string) (string, error) {
	result := struct {
		CertServerData string `json:"certServerData"`
	}{}
	err := bce.NewRequestBuilder(c.sdkClient).
		WithMethod(bcehttp.GET).
		WithURL(baiducert.URI_PREFIX + baiducert.REQUEST_CERT_URL + "/" + id + "/rawData").
		WithResult(&result).
		Do()
	return result.CertServerData, err
}

func createSDKClient(accessKeyId, secretAccessKey string) (*baiducert.Client, error) {
	client, err := baiducert.NewClient(accessKeyId, secretAccessKey, "")
	if err != nil {
		return nil, err
	}

	return client, nil
}
