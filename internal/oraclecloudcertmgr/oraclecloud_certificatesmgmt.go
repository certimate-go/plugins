package oraclecloudcertificatesmgmt

import (
	"context"
	"crypto/x509"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/nrdcg/oci-go-sdk/certificatesmanagement/v1065"
	"github.com/nrdcg/oci-go-sdk/common/v1065"
	"github.com/nrdcg/oci-go-sdk/common/v1065/auth"
	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	AuthMethod           string `json:"authMethod"`
	PrivateKey           string `json:"privateKey,omitempty"`
	PrivateKeyPassphrase string `json:"privateKeyPassphrase,omitempty"`
	PublicKeyFingerprint string `json:"publicKeyFingerprint,omitempty"`
	TenancyOcid          string `json:"tenancyOcid,omitempty"`
	UserOcid             string `json:"userOcid,omitempty"`
	CompartmentOcid      string `json:"compartmentOcid"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *certificatesmanagement.CertificatesManagementClient
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.AuthMethod, config.PrivateKey, config.PrivateKeyPassphrase, config.PublicKeyFingerprint, config.TenancyOcid, config.UserOcid)
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
	listCertificatesPage := 1
	listCertificatesLimit := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listCertificatesReq := certificatesmanagement.ListCertificatesRequest{
			CompartmentId: common.String(c.config.CompartmentOcid),
			Page:          common.String(strconv.Itoa(listCertificatesPage)),
			Limit:         common.Int(listCertificatesLimit),
		}
		listCertificatesResp, err := c.sdkClient.ListCertificates(ctx, listCertificatesReq)
		c.logger.Debug("sdk request 'certificatesmgmt.ListCertificates'", slog.Any("response", listCertificatesResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'certificatesmgmt.ListCertificates': %w", err)
		}

		for _, certItem := range listCertificatesResp.Items {
			if certItem.Subject == nil || !strings.EqualFold(certX509.Subject.CommonName, lo.FromPtr(certItem.Subject.CommonName)) {
				continue
			}
			if certItem.CurrentVersionSummary == nil || !strings.EqualFold(certX509.SerialNumber.String(), lo.FromPtr(certItem.CurrentVersionSummary.SerialNumber)) {
				continue
			}
			if certItem.CurrentVersionSummary.Validity == nil || !certX509.NotBefore.Equal(lo.FromPtr(certItem.CurrentVersionSummary.Validity.TimeOfValidityNotBefore).Time) {
				continue
			} else if certItem.CurrentVersionSummary.Validity == nil || !certX509.NotAfter.Equal(lo.FromPtr(certItem.CurrentVersionSummary.Validity.TimeOfValidityNotAfter).Time) {
				continue
			}
			switch certX509.PublicKeyAlgorithm {
			case x509.RSA:
				if !strings.HasPrefix(string(certItem.KeyAlgorithm), "RSA") {
					continue
				}
			case x509.ECDSA:
				if !strings.HasPrefix(string(certItem.KeyAlgorithm), "EC") {
					continue
				}
			default:
				continue
			}
			c.logger.Info("ssl certificate already exists")
			return &UploadResult{
				CertId:   lo.FromPtr(certItem.Id),
				CertName: lo.FromPtr(certItem.Name),
			}, nil
		}

		if len(listCertificatesResp.Items) < listCertificatesLimit || listCertificatesResp.OpcNextPage == nil {
			break
		}

		listCertificatesPage++
	}
	createCertificateReq := certificatesmanagement.CreateCertificateRequest{
		CreateCertificateDetails: certificatesmanagement.CreateCertificateDetails{
			CompartmentId: common.String(c.config.CompartmentOcid),
			Name:          common.String(fmt.Sprintf("certimate-%s-%d", strings.ToLower(certX509.SerialNumber.String()), time.Now().Unix())),
			CertificateConfig: certificatesmanagement.CreateCertificateByImportingConfigDetails{
				CertificatePem: common.String(serverCertPEM),
				CertChainPem:   common.String(issuerCertPEM),
				PrivateKeyPem:  common.String(privkeyPEM),
			},
			Description: common.String("upload from Certimate"),
		},
	}
	createCertificateResp, err := c.sdkClient.CreateCertificate(ctx, createCertificateReq)
	c.logger.Debug("sdk request 'certificatesmgmt.CreateCertificate'", slog.Any("response", createCertificateResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'certificatesmgmt.CreateCertificate': %w", err)
	}

	return &UploadResult{
		CertId:   lo.FromPtr(createCertificateResp.Certificate.Id),
		CertName: lo.FromPtr(createCertificateResp.Certificate.Name),
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	return nil, core.ErrUnsupported
}

func createSDKClient(authMethod string, privateKey, privateKeyPassphrase, publicKeyFingerprint, tenancyOcid, userOcid string) (*certificatesmanagement.CertificatesManagementClient, error) {
	var cfgProvider common.ConfigurationProvider
	switch authMethod {
	case "", AUTH_METHOD_APIKEY:
		pkpwd := (*string)(nil)
		if privateKeyPassphrase != "" {
			pkpwd = &privateKeyPassphrase
		}
		cfgProvider = common.NewRawConfigurationProvider(
			tenancyOcid,
			userOcid,
			"",
			publicKeyFingerprint,
			privateKey,
			pkpwd,
		)
	case AUTH_METHOD_INSTANCEPRINCIPAL:
		configurationProvider, err := auth.InstancePrincipalConfigurationProvider()
		if err != nil {
			return nil, err
		}
		cfgProvider = configurationProvider
	case AUTH_METHOD_RESOURCEPRINCIPAL:
		configurationProvider, err := auth.ResourcePrincipalConfigurationProvider()
		if err != nil {
			return nil, err
		}
		cfgProvider = configurationProvider
	default:
		return nil, fmt.Errorf("unsupported auth method '%s'", authMethod)
	}

	client, err := certificatesmanagement.NewCertificatesManagementClientWithConfigurationProvider(cfgProvider)
	if err != nil {
		return nil, err
	}

	return &client, nil
}
