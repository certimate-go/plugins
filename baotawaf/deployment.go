package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/samber/lo"

	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
	btwafsdk "github.com/certimate-go/plugins/internal/btwafsdk"
)

type deploymentConfig struct {
	ServerUrl string `json:"serverUrl"`

	ApiKey string `json:"apiKey"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`

	SiteNames []string `json:"siteNames"`

	SitePort int32 `json:"sitePort,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *btwafsdk.Client
}

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

	if err := xloop.ForRangeAllWithContext(ctx, d.config.SiteNames, func(ctx context.Context, siteName string, i int) error {
		if i > 0 {
			if err := xwait.DelayWithContext(ctx, 3*time.Second); err != nil {
				return err
			}
		}

		return d.updateSiteCertificate(ctx, siteName, d.config.SitePort, certPEM, privkeyPEM)
	}); err != nil {
		return nil, err
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) findSiteByName(ctx context.Context, siteName string) (*btwafsdk.SiteData, error) {
	getSiteListPage := 1
	getSiteListPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		getSiteListReq := &btwafsdk.GetSiteListRequest{
			SiteName: lo.ToPtr(siteName),
			Page:     lo.ToPtr(int32(getSiteListPage)),
			PageSize: lo.ToPtr(int32(getSiteListPageSize)),
		}
		getSiteListResp, err := d.sdkClient.GetSiteListWithContext(ctx, getSiteListReq)
		d.logger.Debug("sdk request 'wafmastersite.GetSiteList'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'wafmastersite.GetSiteList': %w", err)
		}

		if getSiteListResp.Result == nil {
			break
		}

		for _, siteItem := range getSiteListResp.Result.List {
			if siteItem.SiteName == siteName {
				return siteItem, nil
			}
		}

		if len(getSiteListResp.Result.List) < getSiteListPageSize {
			break
		}

		getSiteListPage++
	}

	return nil, fmt.Errorf("could not find site '%s'", siteName)
}

func (d *deployment) updateSiteCertificate(ctx context.Context, siteName string, sitePort int32, certPEM, privkeyPEM string) error {
	if sitePort == 0 {
		sitePort = 443
	}

	siteData, err := d.findSiteByName(ctx, siteName)
	if err != nil {
		return err
	}

	modifySiteReq := &btwafsdk.ModifySiteRequest{
		SiteId: lo.ToPtr(siteData.SiteId),
		Type:   lo.ToPtr("openCert"),
		Server: &btwafsdk.SiteServerInfoMod{
			ListenSSLPorts: []*string{lo.ToPtr(fmt.Sprintf("%d", d.config.SitePort))},
			SSL: &btwafsdk.SiteServerSSLInfo{
				IsSSL:      lo.ToPtr(int32(1)),
				FullChain:  lo.ToPtr(certPEM),
				PrivateKey: lo.ToPtr(privkeyPEM),
			},
		},
	}
	_, err = d.sdkClient.ModifySiteWithContext(ctx, modifySiteReq)
	d.logger.Debug("sdk request 'wafmastersite.ModifySite'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'wafmastersite.ModifySite': %w", err)
	}

	return nil
}

func createSDKClient(serverUrl, apiKey string, skipTlsVerify bool) (*btwafsdk.Client, error) {
	client, err := btwafsdk.NewClient(serverUrl,
		btwafsdk.WithApiKey(apiKey),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
