package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/config"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkvlb/model"
	"github.com/samber/lo"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkvlb"

	"github.com/certimate-go/certimate/pkg/core"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/cmcccloud-vlb"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	AccessKeySecret string `json:"accessKeySecret"`

	PoolId string `json:"poolId"`

	DeployTarget string `json:"deployTarget"`

	LoadbalancerId string `json:"loadbalancerId,omitempty"`

	ListenerId string `json:"listenerId,omitempty"`

	Domain string `json:"domain,omitempty"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *ecloudsdkvlb.Client
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.AccessKeySecret, config.PoolId)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		AccessKeyId:     config.AccessKeyId,
		AccessKeySecret: config.AccessKeySecret,
		PoolId:          config.PoolId,
		IsDefault:       config.Domain == "",
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}

	return &Deployer{
		config:     config,
		logger:     slog.Default(),
		sdkClient:  client,
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
	if d.config.LoadbalancerId == "" {
		return nil, fmt.Errorf("config `loadbalancerId` is required")
	}

	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	switch d.config.DeployTarget {
	case DEPLOY_TARGET_LOADBALANCER:
		if err := d.deployToLoadbalancer(ctx, upres.CertId); err != nil {
			return nil, err
		}

	case DEPLOY_TARGET_LISTENER:
		if err := d.deployToListener(ctx, upres.CertId); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &DeployResult{}, nil
}

func (d *Deployer) deployToLoadbalancer(ctx context.Context, cloudCertId string) error {
	listenerIds := make([]string, 0)
	listLoadBalanceHTTPSListenerPage := 1
	listLoadBalanceHTTPSListenerPageSize := 10
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		listLoadBalanceHTTPSListenerReq := &model.ListLoadBalanceHTTPSListenerRequest{
			ListLoadBalanceHTTPSListenerPath: &model.ListLoadBalanceHTTPSListenerPath{
				LoadBalanceId: lo.ToPtr(d.config.LoadbalancerId),
			},
			ListLoadBalanceHTTPSListenerQuery: &model.ListLoadBalanceHTTPSListenerQuery{
				Page:     lo.ToPtr(int32(listLoadBalanceHTTPSListenerPage)),
				PageSize: lo.ToPtr(int32(listLoadBalanceHTTPSListenerPageSize)),
			},
		}
		listLoadBalanceHTTPSListenerResp, err := d.sdkClient.ListLoadBalanceHTTPSListener(listLoadBalanceHTTPSListenerReq)
		d.logger.Debug("sdk request 'vlb.ListLoadBalanceHTTPSListener'")
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'vlb.ListLoadBalanceHTTPSListener': %w", err)
		}

		if listLoadBalanceHTTPSListenerResp.Body == nil || listLoadBalanceHTTPSListenerResp.Body.Content == nil {
			break
		}

		for _, listener := range *listLoadBalanceHTTPSListenerResp.Body.Content {
			listenerIds = append(listenerIds, lo.FromPtr(listener.Id))
		}

		if len(*listLoadBalanceHTTPSListenerResp.Body.Content) < listLoadBalanceHTTPSListenerPageSize {
			break
		}

		listLoadBalanceHTTPSListenerPage++
	}

	if len(listenerIds) == 0 {
		d.logger.Info("no vlb listeners to deploy")
	} else {
		d.logger.Info("found vlb listeners to deploy", slog.Any("listenerIds", listenerIds))

		if err := xloop.ForRangeAllWithContext(ctx, listenerIds, func(ctx context.Context, listenerId string, _ int) error {
			return d.updateListenerCertificate(ctx, listenerId, cloudCertId)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (d *Deployer) deployToListener(ctx context.Context, cloudCertId string) error {
	if d.config.ListenerId == "" {
		return fmt.Errorf("config `listenerId` is required")
	}

	if err := d.updateListenerCertificate(ctx, d.config.ListenerId, cloudCertId); err != nil {
		return err
	}

	return nil
}

func (d *Deployer) updateListenerCertificate(ctx context.Context, cloudListenerId string, cloudCertId string) error {
	var listenerInfo *model.ListLoadBalanceHTTPSListenerResponseContent
	listLoadBalanceHTTPSListenerPage := 1
	listLoadBalanceHTTPSListenerPageSize := 10
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		listLoadBalanceHTTPSListenerReq := &model.ListLoadBalanceHTTPSListenerRequest{
			ListLoadBalanceHTTPSListenerPath: &model.ListLoadBalanceHTTPSListenerPath{
				LoadBalanceId: lo.ToPtr(d.config.LoadbalancerId),
			},
			ListLoadBalanceHTTPSListenerQuery: &model.ListLoadBalanceHTTPSListenerQuery{
				Page:     lo.ToPtr(int32(listLoadBalanceHTTPSListenerPage)),
				PageSize: lo.ToPtr(int32(listLoadBalanceHTTPSListenerPageSize)),
			},
		}
		listLoadBalanceHTTPSListenerResp, err := d.sdkClient.ListLoadBalanceHTTPSListener(listLoadBalanceHTTPSListenerReq)
		d.logger.Debug("sdk request 'vlb.ListLoadBalanceHTTPSListener'")
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'vlb.ListLoadBalanceHTTPSListener': %w", err)
		}

		if listLoadBalanceHTTPSListenerResp.Body == nil || listLoadBalanceHTTPSListenerResp.Body.Content == nil {
			break
		}

		for _, listener := range *listLoadBalanceHTTPSListenerResp.Body.Content {
			if lo.FromPtr(listener.Id) == cloudListenerId {
				listenerInfo = &listener
				break
			}
		}

		if len(*listLoadBalanceHTTPSListenerResp.Body.Content) < listLoadBalanceHTTPSListenerPageSize {
			break
		}

		listLoadBalanceHTTPSListenerPage++
	}
	if listenerInfo == nil {
		return fmt.Errorf("could not find vlb listener '%s'", cloudListenerId)
	}

	if d.config.Domain == "" {

		if lo.FromPtr(listenerInfo.DefaultTlsContainerId) == cloudCertId {
			d.logger.Info("no need to deploy vlb default certificate")
			return nil
		}
		return d.updateListenerDefaultCertificate(ctx, *listenerInfo, cloudCertId)
	} else {

		if lo.Contains(listenerInfo.SniContainerIdList, cloudCertId) {
			d.logger.Info("no need to deploy vlb sni certificate")
			return nil
		}
		return d.updateListenerSniCertificate(ctx, *listenerInfo, cloudCertId)
	}
}

func (d *Deployer) updateListenerDefaultCertificate(ctx context.Context, cloudListenerInfo model.ListLoadBalanceHTTPSListenerResponseContent, cloudCertId string) error {
	updateListenerReq := &model.UpdateListenerRequest{
		UpdateListenerBody: &model.UpdateListenerBody{
			Id:                    cloudListenerInfo.Id,
			DefaultTlsContainerId: lo.ToPtr(cloudCertId),
		},
	}
	_, err := d.sdkClient.UpdateListener(updateListenerReq)
	d.logger.Debug("sdk request 'vlb.UpdateListener'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'vlb.UpdateListener': %w", err)
	}

	return nil
}

func (d *Deployer) updateListenerSniCertificate(ctx context.Context, cloudListenerInfo model.ListLoadBalanceHTTPSListenerResponseContent, cloudCertId string) error {
	updateListenerReq := &model.UpdateListenerRequest{
		UpdateListenerBody: &model.UpdateListenerBody{
			Id:              cloudListenerInfo.Id,
			SniUp:           lo.ToPtr(true),
			SniContainerIds: append(cloudListenerInfo.SniContainerIdList, cloudCertId),
		},
	}
	_, err := d.sdkClient.UpdateListener(updateListenerReq)
	d.logger.Debug("sdk request 'vlb.UpdateListener'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'vlb.UpdateListener': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, accessKeySecret, poolId string) (*ecloudsdkvlb.Client, error) {
	client := ecloudsdkvlb.NewClient(&config.Config{
		AccessKey: &accessKeyId,
		SecretKey: &accessKeySecret,
		PoolId:    &poolId,
	})

	return client, nil
}
