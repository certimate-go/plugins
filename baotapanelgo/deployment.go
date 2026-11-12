package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/samber/lo"

	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
	btpanelgosdk "github.com/certimate-go/plugins/internal/btpanelgosdk"
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
	sdkClient *btpanelgosdk.Client
}

var (
	btProjectTypes      = []string{"php", "java", "asp", "go", "python", "nodejs", "proxy", "general"}
	btProjectTypesInIIS = []string{"php", "asp", "aspx"}
)

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

	if d.config.SiteType != "" {
		if !lo.Contains(btProjectTypes, d.config.SiteType) && !lo.Contains(btProjectTypesInIIS, d.config.SiteType) {
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

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) findSiteByName(ctx context.Context, siteType, siteName string) (*btpanelgosdk.SiteData, error) {
	if siteType == "" || lo.Contains(btProjectTypesInIIS, siteType) {

		datalistGetDataListPage := 1
		datalistGetDataListLimit := 10
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			datalistGetDataListReq := &btpanelgosdk.DatalistGetDataListRequest{
				Table:        lo.ToPtr("sites"),
				SearchString: lo.ToPtr(siteName),
				Page:         lo.ToPtr(int32(datalistGetDataListPage)),
				Limit:        lo.ToPtr(int32(datalistGetDataListLimit)),
			}
			datalistGetDataListResp, err := d.sdkClient.DatalistGetDataListWithContext(ctx, datalistGetDataListReq)
			d.logger.Debug("sdk request 'datalist.GetDataList'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'datalist.GetDataList': %w", err)
			}

			for _, siteItem := range datalistGetDataListResp.Data {
				if strings.EqualFold(siteItem.Name, siteName) {
					return siteItem, nil
				}
			}

			if len(datalistGetDataListResp.Data) < datalistGetDataListLimit {
				break
			}

			datalistGetDataListPage++
		}
	} else {

		siteGetProjectListPage := 1
		siteGetProjectListLimit := 10
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			siteGetProjectListReq := &btpanelgosdk.SiteGetProjectListRequest{
				SearchType:   lo.ToPtr(siteType),
				SearchString: lo.ToPtr(siteName),
				Page:         lo.ToPtr(int32(siteGetProjectListPage)),
				Limit:        lo.ToPtr(int32(siteGetProjectListLimit)),
			}
			siteGetProjectListResp, err := d.sdkClient.SiteGetProjectListWithContext(ctx, siteGetProjectListReq)
			d.logger.Debug("sdk request 'site.GetProjectList'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'site.GetProjectList': %w", err)
			}

			for _, siteItem := range siteGetProjectListResp.Data {
				if strings.EqualFold(siteItem.Name, siteName) {
					return siteItem, nil
				}
			}

			if len(siteGetProjectListResp.Data) < siteGetProjectListLimit {
				break
			}

			siteGetProjectListPage++
		}
	}

	return nil, fmt.Errorf("could not find site '%s'", siteName)
}

func (d *deployment) updateSiteCertificate(ctx context.Context, siteType, siteName string, certPEM, privkeyPEM string) error {
	panelGetConfigReq := &btpanelgosdk.PanelGetConfigRequest{}
	panelGetConfigResp, err := d.sdkClient.PanelGetConfigWithContext(ctx, panelGetConfigReq)
	d.logger.Debug("sdk request 'panel.GetConfig'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'panel.GetConfig': %w", err)
	}

	siteData, err := d.findSiteByName(ctx, siteType, siteName)
	if err != nil {
		return err
	}

	pfxRequried := panelGetConfigResp.Site != nil && strings.EqualFold(panelGetConfigResp.Site.WebServer, "iis")
	if pfxRequried {

		certPFXPassword := "certimate"
		certPFX, err := xcert.TransformCertificateFromPEMToPFX(certPEM, privkeyPEM, certPFXPassword, nil)
		if err != nil {
			return fmt.Errorf("failed to transform certificate from PEM to PFX: %w", err)
		}

		certPFXHash := sha256.Sum256([]byte(certPFX))
		certPFXHashHex := hex.EncodeToString(certPFXHash[:])
		certPFXPath := panelGetConfigResp.Paths.Soft + "/temp/ssl/certimate"
		certPFXFileName := fmt.Sprintf("%s.pfx", certPFXHashHex)
		filesUploadReq := &btpanelgosdk.FilesUploadRequest{
			Path:  lo.ToPtr(certPFXPath),
			Name:  lo.ToPtr(certPFXFileName),
			Start: lo.ToPtr(int32(0)),
			Size:  lo.ToPtr(int32(len(certPFX))),
			Blob:  certPFX,
			Force: lo.ToPtr(true),
		}
		_, err = d.sdkClient.FilesUploadWithContext(ctx, filesUploadReq)
		d.logger.Debug("sdk request 'files.Upload'")
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'files.Upload': %w", err)
		}

		siteSetSitePFXSSLReq := &btpanelgosdk.SiteSetSitePFXSSLRequest{
			SiteId:   lo.ToPtr(siteData.Id),
			PFX:      lo.ToPtr(fmt.Sprintf("%s/%s", certPFXPath, certPFXFileName)),
			Password: lo.ToPtr(certPFXPassword),
		}
		_, err = d.sdkClient.SiteSetSitePFXSSLWithContext(ctx, siteSetSitePFXSSLReq)
		d.logger.Debug("sdk request 'site.SetSitePFXSSL'")
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'site.SetSitePFXSSL': %w", err)
		}
	} else {

		siteSetSiteSSLReq := &btpanelgosdk.SiteSetSiteSSLRequest{
			SiteId: lo.ToPtr(siteData.Id),
			Status: lo.ToPtr(true),
			Key:    lo.ToPtr(privkeyPEM),
			Cert:   lo.ToPtr(certPEM),
		}
		_, err := d.sdkClient.SiteSetSiteSSLWithContext(ctx, siteSetSiteSSLReq)
		d.logger.Debug("sdk request 'site.SetSiteSSL'")
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'site.SetSiteSSL': %w", err)
		}
	}

	return nil
}

func createSDKClient(serverUrl, apiKey string, skipTlsVerify bool) (*btpanelgosdk.Client, error) {
	client, err := btpanelgosdk.NewClient(serverUrl,
		btpanelgosdk.WithApiKey(apiKey),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
