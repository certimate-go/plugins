package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/samber/lo"
	"github.com/ucloud/ucloud-sdk-go/services/ulb"
	"github.com/ucloud/ucloud-sdk-go/ucloud"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"

	"github.com/certimate-go/certimate/pkg/core"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	ucloudsdk "github.com/certimate-go/plugins/internal/ucloudsdk/ulb"
	cmgrimpl "github.com/certimate-go/plugins/internal/ucloudulbcertmgr"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	PublicKey string `json:"publicKey"`

	PrivateKey string `json:"privateKey"`

	ProjectId string `json:"projectId,omitempty"`

	Endpoint string `json:"endpoint,omitempty"`

	Region string `json:"region"`

	DeployTarget string `json:"deployTarget"`

	LoadbalancerId string `json:"loadbalancerId,omitempty"`

	VServerId string `json:"vserverId,omitempty"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *ucloudsdk.ULBClient
	sdkCertmgr core.Certmgr

	sslId2PemMap   map[string]string
	sslId2PemMapMu sync.Mutex
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.PublicKey, config.PrivateKey, config.ProjectId, config.Endpoint, config.Region)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		PublicKey:  config.PublicKey,
		PrivateKey: config.PrivateKey,
		ProjectId:  config.ProjectId,
		Endpoint:   config.Endpoint,
		Region:     config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}

	return &Deployer{
		config:     config,
		logger:     slog.Default(),
		sdkClient:  client,
		sdkCertmgr: pcertmgr,

		sslId2PemMap:   make(map[string]string),
		sslId2PemMapMu: sync.Mutex{},
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
	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))

		d.sslId2PemMapMu.Lock()
		d.sslId2PemMap[upres.CertId] = certPEM
		d.sslId2PemMapMu.Unlock()
	}

	switch d.config.DeployTarget {
	case DEPLOY_TARGET_LOADBALANCER:
		if err := d.deployToLoadbalancer(ctx, upres.CertId); err != nil {
			return nil, err
		}

	case DEPLOY_TARGET_VSERVER:
		if err := d.deployToVServer(ctx, upres.CertId); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &DeployResult{}, nil
}

func (d *Deployer) deployToLoadbalancer(ctx context.Context, cloudCertId string) error {
	if d.config.LoadbalancerId == "" {
		return fmt.Errorf("config `loadbalancerId` is required")
	}

	vserverIds := make([]string, 0)
	describeVServerOffset := 0
	describeVServerLimit := 100
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		describeVServerReq := d.sdkClient.NewDescribeVServerRequest()
		describeVServerReq.ULBId = ucloud.String(d.config.LoadbalancerId)
		describeVServerReq.Offset = ucloud.Int(describeVServerOffset)
		describeVServerReq.Limit = ucloud.Int(describeVServerLimit)
		describeVServerResp, err := d.sdkClient.DescribeVServer(describeVServerReq)
		d.logger.Debug("sdk request 'ulb.DescribeVServer'", slog.Any("request", describeVServerReq), slog.Any("response", describeVServerResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'ulb.DescribeVServer': %w", err)
		}

		for _, vserverItem := range describeVServerResp.DataSet {
			if vserverItem.Protocol == "HTTPS" {
				vserverIds = append(vserverIds, vserverItem.VServerId)
			}
		}

		if len(describeVServerResp.DataSet) < describeVServerLimit {
			break
		}

		describeVServerOffset += describeVServerLimit
	}

	if len(vserverIds) == 0 {
		d.logger.Info("no clb vservers to deploy")
	} else {
		d.logger.Info("found clb vservers to deploy", slog.Any("vserverIds", vserverIds))

		if err := xloop.ForRangeAllWithContext(ctx, vserverIds, func(ctx context.Context, vserverId string, _ int) error {
			return d.updateVServerCertificate(ctx, d.config.LoadbalancerId, vserverId, cloudCertId)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (d *Deployer) deployToVServer(ctx context.Context, cloudCertId string) error {
	if d.config.LoadbalancerId == "" {
		return fmt.Errorf("config `loadbalancerId` is required")
	}
	if d.config.VServerId == "" {
		return fmt.Errorf("config `vserverId` is required")
	}

	if err := d.updateVServerCertificate(ctx, d.config.LoadbalancerId, d.config.VServerId, cloudCertId); err != nil {
		return err
	}

	return nil
}

func (d *Deployer) updateVServerCertificate(ctx context.Context, cloudLoadbalancerId, cloudVServerId string, cloudCertId string) error {
	describeVServerReq := d.sdkClient.NewDescribeVServerRequest()
	describeVServerReq.ULBId = ucloud.String(cloudLoadbalancerId)
	describeVServerReq.VServerId = ucloud.String(cloudVServerId)
	describeVServerReq.Limit = ucloud.Int(1)
	describeVServerResp, err := d.sdkClient.DescribeVServer(describeVServerReq)
	d.logger.Debug("sdk request 'ulb.DescribeVServer'", slog.Any("request", describeVServerReq), slog.Any("response", describeVServerResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'ulb.DescribeVServer': %w", err)
	} else if len(describeVServerResp.DataSet) == 0 {
		return fmt.Errorf("could not find uclb vserver '%s'", cloudVServerId)
	}

	vserverInfo := describeVServerResp.DataSet[0]
	if lo.SomeBy(vserverInfo.SSLSet, func(item ulb.ULBSSLSet) bool { return item.SSLId == cloudCertId }) {
		return nil
	}

	for _, sslItem := range vserverInfo.SSLSet {
		unbindSSLReq := d.sdkClient.NewUnbindSSLRequest()
		unbindSSLReq.ULBId = ucloud.String(cloudLoadbalancerId)
		unbindSSLReq.VServerId = ucloud.String(cloudVServerId)
		unbindSSLReq.SSLId = ucloud.String(sslItem.SSLId)
		unbindSSLResp, err := d.sdkClient.UnbindSSL(unbindSSLReq)
		d.logger.Debug("sdk request 'ulb.UnbindSSL'", slog.Any("request", unbindSSLReq), slog.Any("response", unbindSSLResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'ulb.UnbindSSL': %w", err)
		}
	}

	bindSSLReq := d.sdkClient.NewBindSSLRequest()
	bindSSLReq.ULBId = ucloud.String(cloudLoadbalancerId)
	bindSSLReq.VServerId = ucloud.String(cloudVServerId)
	bindSSLReq.SSLId = ucloud.String(cloudCertId)
	bindSSLResp, err := d.sdkClient.BindSSL(bindSSLReq)
	d.logger.Debug("sdk request 'ulb.BindSSL'", slog.Any("request", bindSSLReq), slog.Any("response", bindSSLResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'ulb.BindSSL': %w", err)
	}

	return nil
}

func createSDKClient(publicKey, privateKey, projectId, endpoint, region string) (*ucloudsdk.ULBClient, error) {
	if publicKey == "" {
		return nil, fmt.Errorf("ucloud: invalid public key")
	}
	if privateKey == "" {
		return nil, fmt.Errorf("ucloud: invalid private key")
	}

	cfg := ucloud.NewConfig()
	if projectId != "" {
		cfg.ProjectId = projectId
	}
	if endpoint != "" {
		if strings.Contains(endpoint, "://") {
			cfg.BaseUrl = endpoint
		} else {
			cfg.BaseUrl = "https://" + endpoint
		}
	}
	if region != "" {
		cfg.Region = region
	}

	credential := auth.NewCredential()
	credential.PublicKey = publicKey
	credential.PrivateKey = privateKey

	client := ucloudsdk.NewClient(&cfg, &credential)
	return client, nil
}
