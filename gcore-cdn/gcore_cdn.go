package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/G-Core/gcorelabscdn-go/gcore"
	"github.com/G-Core/gcorelabscdn-go/gcore/provider"
	"github.com/G-Core/gcorelabscdn-go/resources"
	"github.com/G-Core/gcorelabscdn-go/sslcerts"
	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	xgcore "github.com/certimate-go/plugins/internal/gcoreauth"
	cmgrimpl "github.com/certimate-go/plugins/internal/gcorecertmgr"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	ApiToken      string `json:"apiToken"`
	ResourceId    int64  `json:"resourceId"`
	CertificateId int64  `json:"certificateId,omitempty"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClients *wSDKClients
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

type wSDKClients struct {
	Resources *resources.Service
	SSLCerts  *sslcerts.Service
}

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	clientRES, err := createSDKClientRES(config.ApiToken)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	clientSSL, err := createSDKClientSSL(config.ApiToken)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		ApiToken: config.ApiToken,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}

	return &Deployer{
		config:     config,
		logger:     slog.Default(),
		sdkClients: &wSDKClients{Resources: clientRES, SSLCerts: clientSSL},
		sdkCertmgr: pcertmgr,
	}, nil
}

func (d *Deployer) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}

	d.sdkCertmgr.SetLogger(logger)
}

func (d *Deployer) Deploy(ctx context.Context, certPEM, privkeyPEM string) (*DeployResult, error) {
	if d.config.ResourceId == 0 {
		return nil, fmt.Errorf("config `resourceId` is required")
	}
	var cloudCertId int64
	if d.config.CertificateId == 0 {
		upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to upload certificate file: %w", err)
		} else {
			d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
		}

		cloudCertId, _ = strconv.ParseInt(upres.CertId, 10, 64)
	} else {
		cloudCertId = d.config.CertificateId
		rplres, err := d.sdkCertmgr.Replace(ctx, strconv.FormatInt(cloudCertId, 10), certPEM, privkeyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to replace certificate file: %w", err)
		} else {
			d.logger.Info("ssl certificate replaced", slog.Any("result", rplres))
		}
	}
	getResourceResp, err := d.sdkClients.Resources.Get(ctx, d.config.ResourceId)
	d.logger.Debug("sdk request 'resources.Get'", slog.Any("resourceId", d.config.ResourceId))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'resources.Get': %w", err)
	}
	updateResourceReq := &resources.UpdateRequest{
		Description:        getResourceResp.Description,
		Active:             getResourceResp.Active,
		OriginGroup:        int(getResourceResp.OriginGroup),
		OriginProtocol:     getResourceResp.OriginProtocol,
		SecondaryHostnames: getResourceResp.SecondaryHostnames,
		SSlEnabled:         true,
		SSLData:            int(cloudCertId),
		ProxySSLEnabled:    getResourceResp.ProxySSLEnabled,
		ProxySSLCA:         lo.Ternary(getResourceResp.ProxySSLCA != 0, &getResourceResp.ProxySSLCA, nil),
		ProxySSLData:       lo.Ternary(getResourceResp.ProxySSLData != 0, &getResourceResp.ProxySSLData, nil),
		Options:            &gcore.Options{},
	}
	_, err = d.sdkClients.Resources.Update(ctx, d.config.ResourceId, updateResourceReq)
	d.logger.Debug("sdk request 'resources.Update'", slog.Int64("params.resourceId", d.config.ResourceId))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'resources.Update': %w", err)
	}

	return &DeployResult{}, nil
}

func createSDKClientRES(apiToken string) (*resources.Service, error) {
	if apiToken == "" {
		return nil, fmt.Errorf("gcore: invalid api token")
	}

	requester := provider.NewClient(
		xgcore.BaseURL,
		provider.WithSigner(xgcore.NewAuthRequestSigner(apiToken)),
	)
	return resources.NewService(requester), nil
}

func createSDKClientSSL(apiToken string) (*sslcerts.Service, error) {
	if apiToken == "" {
		return nil, fmt.Errorf("gcore: invalid api token")
	}

	requester := provider.NewClient(
		xgcore.BaseURL,
		provider.WithSigner(xgcore.NewAuthRequestSigner(apiToken)),
	)
	return sslcerts.NewService(requester), nil
}
