package yandexcloudcertificatemanager

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	yccertificatemanagerproto "github.com/yandex-cloud/go-genproto/yandex/cloud/certificatemanager/v1"
	yccertificatemanager "github.com/yandex-cloud/go-sdk/services/certificatemanager/v1"
	ycsdk "github.com/yandex-cloud/go-sdk/v2"
	yccreds "github.com/yandex-cloud/go-sdk/v2/credentials"
	yciamkey "github.com/yandex-cloud/go-sdk/v2/pkg/iamkey"
	ycoptions "github.com/yandex-cloud/go-sdk/v2/pkg/options"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	FolderId          string `json:"folderId"`
	ServiceAccountKey string `json:"serviceAccountKey"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient yccertificatemanager.CertificateClient
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.ServiceAccountKey)
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
	serverCertPEM, issuerCertPEM, err := xcert.ExtractCertificatesFromPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to extract certs: %w", err)
	}
	listCertificatesPageToken := ""
	listCertificatesPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listCertificatesReq := &yccertificatemanagerproto.ListCertificatesRequest{
			FolderId:  c.config.FolderId,
			PageSize:  int64(listCertificatesPageSize),
			PageToken: listCertificatesPageToken,
		}
		listCertificatesResp, err := c.sdkClient.List(ctx, listCertificatesReq)
		c.logger.Debug("sdk request 'certificatemanager.certificate.list'", slog.Any("response", listCertificatesResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'certificatemanager.certificate.list': %w", err)
		}

		for _, certItem := range listCertificatesResp.Certificates {
			if !strings.EqualFold(strings.Join(certX509.DNSNames, ","), strings.Join(certItem.Domains, ",")) {
				continue
			}
			if certX509.Subject.String() != certItem.Subject {
				continue
			}
			if certX509.Issuer.String() != certItem.Issuer {
				continue
			}
			if !strings.EqualFold(certX509.SerialNumber.Text(16), certItem.Serial) {
				continue
			}
			if certItem.NotBefore != nil && certX509.NotBefore.Unix() != certItem.NotBefore.AsTime().Unix() {
				continue
			} else if certItem.NotAfter != nil && certX509.NotAfter.Unix() != certItem.NotAfter.AsTime().Unix() {
				continue
			}
			c.logger.Info("ssl certificate already exists")
			return &UploadResult{
				CertId:   certItem.Id,
				CertName: certItem.Name,
			}, nil
		}

		if len(listCertificatesResp.Certificates) == 0 || listCertificatesResp.NextPageToken == "" {
			break
		}

		listCertificatesPageToken = listCertificatesResp.NextPageToken
	}
	createCertificateReq := &yccertificatemanagerproto.CreateCertificateRequest{
		FolderId:    c.config.FolderId,
		Name:        fmt.Sprintf("certimate-%d", time.Now().UnixMilli()),
		Description: "upload from Certimate",
		Certificate: serverCertPEM,
		Chain:       issuerCertPEM,
		PrivateKey:  privkeyPEM,
	}
	createCertificateResp, err := c.sdkClient.Create(ctx, createCertificateReq)
	c.logger.Debug("sdk request 'certificatemanager.certificate.create'", slog.Any("response", createCertificateResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'certificatemanager.certificate.create': %w", err)
	}

	return &UploadResult{
		CertId:   createCertificateResp.Response().Id,
		CertName: createCertificateResp.Response().Name,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	serverCertPEM, issuerCertPEM, err := xcert.ExtractCertificatesFromPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to extract certs: %w", err)
	}
	getCertificateReq := &yccertificatemanagerproto.GetCertificateRequest{
		CertificateId: certIdOrName,
		View:          yccertificatemanagerproto.CertificateView_BASIC,
	}
	getCertificateResp, err := c.sdkClient.Get(ctx, getCertificateReq)
	c.logger.Debug("sdk request 'certificatemanager.certificate.get'", slog.Any("response", getCertificateResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'certificatemanager.certificate.get': %w", err)
	}
	updateCertificateReq := &yccertificatemanagerproto.UpdateCertificateRequest{
		CertificateId:      certIdOrName,
		Name:               getCertificateResp.Name,
		Description:        getCertificateResp.Description,
		Labels:             getCertificateResp.Labels,
		Certificate:        serverCertPEM,
		Chain:              issuerCertPEM,
		PrivateKey:         privkeyPEM,
		DeletionProtection: getCertificateResp.DeletionProtection,
	}
	updateCertificateResp, err := c.sdkClient.Update(ctx, updateCertificateReq)
	c.logger.Debug("sdk request 'certificatemanager.certificate.update'", slog.Any("response", updateCertificateResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'certificatemanager.certificate.update': %w", err)
	}

	return &ReplaceResult{}, nil
}

func createSDKClient(serviceAccountKey string) (yccertificatemanager.CertificateClient, error) {
	saKey := []byte(serviceAccountKey)
	saConf := &yciamkey.Key{}
	if err := json.Unmarshal(saKey, saConf); err != nil {
		return nil, fmt.Errorf("unable to acquire service account config: %w", err)
	}

	creds, err := yccreds.ServiceAccountKey(saConf)
	if err != nil {
		return nil, err
	}

	sdk, err := ycsdk.Build(context.Background(), ycoptions.WithCredentials(creds))
	if err != nil {
		return nil, err
	}

	client := yccertificatemanager.NewCertificateClient(sdk)
	return client, nil
}
