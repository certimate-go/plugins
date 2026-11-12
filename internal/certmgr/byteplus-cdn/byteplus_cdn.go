package bytepluscdn

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	bpcdn "github.com/byteplus-sdk/byteplus-sdk-golang/service/cdn"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type Certmgr struct {
	logger    *slog.Logger
	sdkClient *bpcdn.CDN
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(client *bpcdn.CDN) *Certmgr {
	return &Certmgr{
		logger:    slog.Default(),
		sdkClient: client,
	}
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

		listCertInfoReq := &bpcdn.ListCertInfoRequest{
			PageNum:  bpcdn.GetInt64Ptr(int64(listCertInfoPageNum)),
			PageSize: bpcdn.GetInt64Ptr(int64(listCertInfoPageSize)),
			Source:   bpcdn.GetStrPtr("cert_center"),
		}
		listCertInfoResp, err := c.sdkClient.ListCertInfo(listCertInfoReq)
		c.logger.Debug("sdk request 'cdn.ListCertInfo'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.ListCertInfo': %w", err)
		}

		fingerprintSha1 := sha1.Sum(certX509.Raw)
		fingerprintSha1Hex := hex.EncodeToString(fingerprintSha1[:])
		fingerprintSha256 := sha256.Sum256(certX509.Raw)
		fingerprintSha256Hex := hex.EncodeToString(fingerprintSha256[:])
		for _, certItem := range listCertInfoResp.Result.CertInfo {

			if !strings.EqualFold(fingerprintSha1Hex, certItem.CertFingerprint.Sha1) {
				continue
			} else if !strings.EqualFold(fingerprintSha256Hex, certItem.CertFingerprint.Sha256) {
				continue
			}

			c.logger.Info("ssl certificate already exists")
			return &UploadResult{
				CertId:   certItem.CertId,
				CertName: certItem.Desc,
			}, nil
		}

		if len(listCertInfoResp.Result.CertInfo) < listCertInfoPageSize {
			break
		}

		listCertInfoPageNum++
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	addCertificateReq := &bpcdn.AddCertificateRequest{
		Certificate: certPEM,
		PrivateKey:  privkeyPEM,
		Source:      bpcdn.GetStrPtr("cert_center"),
		Desc:        bpcdn.GetStrPtr(certName),
	}
	addCertificateResp, err := c.sdkClient.AddCertificate(addCertificateReq)
	c.logger.Debug("sdk request 'cdn.AddCertificate'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cdn.AddCertificate': %w", err)
	}

	return &UploadResult{
		CertId:   addCertificateResp.Result.CertId,
		CertName: certName,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}
