package ctcccloudcms

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	ctyuncms "github.com/certimate-go/plugins/internal/cloudsdk/ctyun/cms"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *ctyuncms.Client
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
	if upres, upok, err := c.tryGetResultIfCertExists(ctx, certPEM); err != nil {
		return nil, err
	} else if upok {
		c.logger.Info("ssl certificate already exists")
		return upres, nil
	}

	serverCertPEM, issuerCertPEM, err := xcert.ExtractCertificatesFromPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to extract certs: %w", err)
	}

	certName := fmt.Sprintf("cm%d", time.Now().Unix())

	uploadCertificateReq := &ctyuncms.UploadCertificateRequest{
		Name:               lo.ToPtr(certName),
		Certificate:        lo.ToPtr(serverCertPEM),
		CertificateChain:   lo.ToPtr(issuerCertPEM),
		PrivateKey:         lo.ToPtr(privkeyPEM),
		EncryptionStandard: lo.ToPtr("INTERNATIONAL"),
	}
	uploadCertificateResp, err := c.sdkClient.UploadCertificateWithContext(ctx, uploadCertificateReq)
	c.logger.Debug("sdk request 'cms.UploadCertificate'")
	if err != nil {
		if uploadCertificateResp != nil && uploadCertificateResp.GetError() == "CCMS_100000067" {
			if upres, upok, err := c.tryGetResultIfCertExists(ctx, certPEM); err != nil {
				return nil, err
			} else if !upok {
				return nil, fmt.Errorf("ctyun cms: no certificate found")
			} else {
				c.logger.Info("ssl certificate already exists")
				return upres, nil
			}
		}

		return nil, fmt.Errorf("failed to execute sdk request 'cms.UploadCertificate': %w", err)
	}

	if upres, upok, err := c.tryGetResultIfCertExists(ctx, certPEM); err != nil {
		return nil, err
	} else if !upok {
		return nil, fmt.Errorf("could not find ssl certificate, may be upload failed")
	} else {
		return upres, nil
	}
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func (c *Certmgr) tryGetResultIfCertExists(ctx context.Context, certPEM string) (*UploadResult, bool, error) {
	certX509, err := xcert.ParseCertificateFromPEM(certPEM)
	if err != nil {
		return nil, false, err
	}

	getCertificateListPageNum := 1
	getCertificateListPageSize := 10
	for {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		default:
		}

		getCertificateListReq := &ctyuncms.GetCertificateListRequest{
			PageNum:  lo.ToPtr(int32(getCertificateListPageNum)),
			PageSize: lo.ToPtr(int32(getCertificateListPageSize)),
			Keyword:  lo.ToPtr(certX509.Subject.CommonName),
			Origin:   lo.ToPtr("UPLOAD"),
		}
		getCertificateListResp, err := c.sdkClient.GetCertificateListWithContext(ctx, getCertificateListReq)
		c.logger.Debug("sdk request 'cms.GetCertificateList'")
		if err != nil {
			return nil, false, fmt.Errorf("failed to execute sdk request 'cms.GetCertificateList': %w", err)
		}

		if getCertificateListResp.ReturnObj == nil {
			break
		}

		fingerprintSha1 := sha1.Sum(certX509.Raw)
		fingerprintSha1Hex := hex.EncodeToString(fingerprintSha1[:])
		for _, certItem := range getCertificateListResp.ReturnObj.List {

			if !strings.EqualFold(strings.Join(certX509.DNSNames, ","), certItem.DomainName) {
				continue
			}

			newCertNotBefore := certX509.NotBefore
			newCertNotAfter := certX509.NotAfter
			oldCertNotBefore, _ := time.Parse(time.RFC3339, certItem.IssueTime)
			oldCertNotAfter, _ := time.Parse(time.RFC3339, certItem.ExpireTime)
			if !newCertNotBefore.Equal(oldCertNotBefore) || !newCertNotAfter.Equal(oldCertNotAfter) {
				continue
			}

			if !strings.EqualFold(fingerprintSha1Hex, certItem.Fingerprint) {
				continue
			}

			c.logger.Info("ssl certificate already exists")
			return &UploadResult{
				CertId:   certItem.Id,
				CertName: certItem.Name,
			}, true, nil
		}

		if len(getCertificateListResp.ReturnObj.List) < getCertificateListPageSize {
			break
		}

		getCertificateListPageNum++
	}

	return nil, false, nil
}

func createSDKClient(accessKeyId, secretAccessKey string) (*ctyuncms.Client, error) {
	client, err := ctyuncms.NewClient(
		ctyuncms.WithAkSk(accessKeyId, secretAccessKey),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}
