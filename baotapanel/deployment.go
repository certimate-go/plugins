package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/samber/lo"

	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
	btpanelsdk "github.com/certimate-go/plugins/internal/btpanelsdk"
)

type deploymentConfig struct {
	ServerUrl string `json:"serverUrl"`

	ApiKey string `json:"apiKey"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`

	SiteType string `json:"siteType"`

	SiteNames []string `json:"siteNames,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *btpanelsdk.Client
}

var btProjectTypes = []string{"php", "java", "nodejs", "go", "python", "proxy", "html", "general"}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiKey, config.AllowInsecureConnections)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return &deployment{
		config:    config,
		logger:    slog.Default(),
		sdkClient: client,
	}, nil
}

func (d *deployment) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}
}

func (d *deployment) deploy(ctx context.Context, certPEM, privkeyPEM string) (*plugin.DeployResult, error) {
	if len(d.config.SiteNames) == 0 {
		return nil, fmt.Errorf("config `siteNames` is required")
	}

	switch d.config.SiteType {
	case "any":
		{
			if err1 := d.updateSitesCertificateByAny(ctx, d.config.SiteNames, certPEM, privkeyPEM); err1 != nil {
				if err2 := d.updateSitesCertificateByAnyV2(ctx, d.config.SiteNames, certPEM, privkeyPEM); err2 != nil {
					return nil, errors.Join(err1, err2)
				}
			}
		}

	default:
		{
			if d.config.SiteType != "" {
				if !lo.Contains(btProjectTypes, d.config.SiteType) {
					return nil, fmt.Errorf("unsupported site type: '%s'", d.config.SiteType)
				}
			}

			if err := xloop.ForRangeAllWithContext(ctx, d.config.SiteNames, func(ctx context.Context, siteName string, i int) error {
				if i > 0 {
					if err := xwait.DelayWithContext(ctx, 3*time.Second); err != nil {
						return err
					}
				}

				return d.updateSiteCertificate(ctx, d.config.SiteType, siteName, certPEM, privkeyPEM)
			}); err != nil {
				return nil, err
			}
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) updateSiteCertificate(ctx context.Context, siteType, siteName string, certPEM, privkeyPEM string) error {
	switch siteType {
	case "proxy":
		{

			modProxyComSetSSLReq := &btpanelsdk.ModProxyComSetSSLRequest{
				SiteName:    siteName,
				Certificate: certPEM,
				PrivateKey:  privkeyPEM,
			}
			_, err := d.sdkClient.ModProxyComSetSSLWithContext(ctx, modProxyComSetSSLReq)
			d.logger.Debug("sdk request 'mod.proxy.com.SetSSL'")
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'mod.proxy.com.SetSSL': %w", err)
			}
		}

	default:
		{

			siteSetSSLReq := &btpanelsdk.SiteSetSSLRequest{
				Type:        "0",
				SiteName:    siteName,
				Certificate: certPEM,
				PrivateKey:  privkeyPEM,
			}
			_, err := d.sdkClient.SiteSetSSLWithContext(ctx, siteSetSSLReq)
			d.logger.Debug("sdk request 'site.SetSSL'")
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'site.SetSSL': %w", err)
			}
		}
	}

	return nil
}

func (d *deployment) updateSitesCertificateByAny(ctx context.Context, siteNames []string, certPEM, privkeyPEM string) error {
	sslCertSaveCertReq := &btpanelsdk.SSLCertSaveCertRequest{
		Certificate: certPEM,
		PrivateKey:  privkeyPEM,
	}
	sslCertSaveCertResp, err := d.sdkClient.SSLCertSaveCertWithContext(ctx, sslCertSaveCertReq)
	d.logger.Debug("sdk request 'ssl.cert.SaveCert'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'ssl.cert.SaveCert': %w", err)
	}

	sslSetBatchCertToSiteReq := &btpanelsdk.SSLSetBatchCertToSiteRequest{
		BatchInfo: lo.Map(siteNames, func(siteName string, _ int) *btpanelsdk.SSLSetBatchCertToSiteRequestBatchInfo {
			return &btpanelsdk.SSLSetBatchCertToSiteRequestBatchInfo{
				SiteName: siteName,
				SSLHash:  sslCertSaveCertResp.SSLHash,
			}
		}),
	}
	_, err = d.sdkClient.SSLSetBatchCertToSiteWithContext(ctx, sslSetBatchCertToSiteReq)
	d.logger.Debug("sdk request 'ssl.SetBatchCertToSite'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'ssl.SetBatchCertToSite': %w", err)
	}

	return nil
}

func (d *deployment) updateSitesCertificateByAnyV2(ctx context.Context, siteNames []string, certPEM, privkeyPEM string) error {
	sslDomainUploadCertV2Req := &btpanelsdk.SSLDomainUploadCertV2Request{
		Certificate: certPEM,
		PrivateKey:  privkeyPEM,
	}
	sslDomainUploadCertV2Resp, err := d.sdkClient.SSLDomainUploadCertV2WithContext(ctx, sslDomainUploadCertV2Req)
	d.logger.Debug("sdk request 'v2.ssldomain.UploadCert'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'v2.ssldomain.UploadCert': %w", err)
	}

	sslDomainCertDeploySitesV2Req := &btpanelsdk.SSLDomainCertDeploySitesV2Request{
		SSLHash: sslDomainUploadCertV2Resp.Message.SSLHash,
		Domains: siteNames,
		Append:  1,
	}
	_, err = d.sdkClient.SSLDomainCertDeploySitesV2WithContext(ctx, sslDomainCertDeploySitesV2Req)
	d.logger.Debug("sdk request 'v2.ssldomain.CertDeploySites'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'v2.ssldomain.CertDeploySites': %w", err)
	}

	return nil
}

func createSDKClient(serverUrl, apiKey string, skipTlsVerify bool) (*btpanelsdk.Client, error) {
	client, err := btpanelsdk.NewClient(serverUrl,
		btpanelsdk.WithApiKey(apiKey),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
