package gcorecdn

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	gcore "github.com/G-Core/gcorelabscdn-go/gcore/provider"
	"github.com/G-Core/gcorelabscdn-go/sslcerts"

	"github.com/certimate-go/certimate/pkg/core"
	xgcore "github.com/certimate-go/plugins/internal/gcoreauth"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	ApiToken string `json:"apiToken"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient *sslcerts.Service
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.ApiToken)
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
	addSSLDataReq := &sslcerts.CreateRequest{
		Name:           fmt.Sprintf("certimate_%d", time.Now().UnixMilli()),
		Cert:           certPEM,
		PrivateKey:     privkeyPEM,
		Automated:      false,
		ValidateRootCA: false,
	}
	addSSLDataResp, err := c.sdkClient.Create(ctx, addSSLDataReq)
	c.logger.Debug("sdk request 'sslData.Add'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'sslData.Add': %w", err)
	}

	return &UploadResult{
		CertId:   fmt.Sprintf("%d", addSSLDataResp.ID),
		CertName: addSSLDataResp.Name,
	}, nil
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	sslId, err := strconv.ParseInt(certIdOrName, 10, 64)
	if err != nil {
		return nil, err
	}
	getSSLDataDetailResp, err := c.sdkClient.Get(ctx, sslId)
	c.logger.Debug("sdk request 'sslData.GetDetail'", slog.Any("params.sslId", sslId))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'sslData.GetDetail': %w", err)
	}
	changeSSLDataReq := &sslcerts.UpdateRequest{
		Name:           getSSLDataDetailResp.Name,
		Cert:           certPEM,
		PrivateKey:     privkeyPEM,
		ValidateRootCA: false,
	}
	_, err = c.sdkClient.Update(ctx, sslId, changeSSLDataReq)
	c.logger.Debug("sdk request 'sslData.Change'")
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'sslData.Change': %w", err)
	}

	return &ReplaceResult{}, nil
}

func createSDKClient(apiToken string) (*sslcerts.Service, error) {
	if apiToken == "" {
		return nil, fmt.Errorf("gcore: invalid api token")
	}

	requester := gcore.NewClient(
		xgcore.BaseURL,
		gcore.WithSigner(xgcore.NewAuthRequestSigner(apiToken)),
	)
	service := sslcerts.NewService(requester)
	return service, nil
}
