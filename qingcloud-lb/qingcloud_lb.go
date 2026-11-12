package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/samber/lo"
	qcconfig "github.com/yunify/qingcloud-sdk-go/config"

	"github.com/certimate-go/certimate/pkg/core"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/qingcloudcertmgr"
	qclbsdk "github.com/certimate-go/plugins/internal/qingcloudsdk/lb"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	ZoneId          string `json:"zoneId"`
	DeployTarget    string `json:"deployTarget"`
	LoadbalancerId  string `json:"loadbalancerId,omitempty"`
	ListenerId      string `json:"listenerId,omitempty"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *qclbsdk.LoadBalancerService
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey, config.ZoneId)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		AccessKeyId:     config.AccessKeyId,
		SecretAccessKey: config.SecretAccessKey,
		ZoneId:          config.ZoneId,
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
	switch d.config.DeployTarget {
	case DEPLOY_TARGET_LOADBALANCER:
		if err := d.deployToLoadbalancer(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	case DEPLOY_TARGET_LISTENER:
		if err := d.deployToListener(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &DeployResult{}, nil
}

func (d *Deployer) deployToLoadbalancer(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.LoadbalancerId == "" {
		return fmt.Errorf("config `loadbalancerId` is required")
	}
	listenerIds := make([]string, 0)
	describeLoadBalancerListenersOffset := 0
	describeLoadBalancerListenersLimit := 100
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		listListenersReq := &qclbsdk.DescribeLoadBalancerListenersInput{
			LoadBalancer: lo.ToPtr(d.config.LoadbalancerId),
			Offset:       lo.ToPtr(describeLoadBalancerListenersOffset),
			Limit:        lo.ToPtr(describeLoadBalancerListenersLimit),
		}
		listListenersResp, err := d.sdkClient.DescribeLoadBalancerListeners(listListenersReq)
		d.logger.Debug("sdk request 'lb.DescribeLoadBalancerListeners'", slog.Any("response", listListenersResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'lb.DescribeLoadBalancerListeners': %w", err)
		}

		for _, listener := range listListenersResp.LoadBalancerListenerSet {
			if !strings.EqualFold(lo.FromPtr(listener.ListenerProtocol), "https") {
				continue
			}

			listenerIds = append(listenerIds, lo.FromPtr(listener.LoadBalancerListenerID))
		}

		if len(listListenersResp.LoadBalancerListenerSet) < describeLoadBalancerListenersLimit {
			break
		}

		describeLoadBalancerListenersOffset += describeLoadBalancerListenersLimit
	}
	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}
	if len(listenerIds) == 0 {
		d.logger.Info("no lb listeners to deploy")
	} else {
		d.logger.Info("found lb listeners to deploy", slog.Any("listenerIds", listenerIds))

		if err := xloop.ForRangeAllWithContext(ctx, listenerIds, func(ctx context.Context, listenerId string, _ int) error {
			return d.updateListenerCertificate(ctx, listenerId, upres.CertId)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (d *Deployer) deployToListener(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.ListenerId == "" {
		return fmt.Errorf("config `listenerId` is required")
	}
	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}
	if err := d.updateListenerCertificate(ctx, d.config.ListenerId, upres.CertId); err != nil {
		return err
	}

	return nil
}

func (d *Deployer) updateListenerCertificate(ctx context.Context, cloudListenerId string, cloudCertId string) error {
	associateServerCertsToLBListenerReq := &qclbsdk.AssociateServerCertsToLBListenerInput{
		LoadBalancerListener: lo.ToPtr(cloudListenerId),
		ServerCertificates:   []*string{lo.ToPtr(cloudCertId)},
	}
	associateServerCertsToLBListenerResp, err := d.sdkClient.AssociateServerCertsToLBListener(associateServerCertsToLBListenerReq)
	d.logger.Debug("sdk request 'lb.AssociateServerCertsToLBListener'", slog.Any("response", associateServerCertsToLBListenerResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'lb.AssociateServerCertsToLBListener': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey, zoneId string) (*qclbsdk.LoadBalancerService, error) {
	config, err := qcconfig.New(accessKeyId, secretAccessKey)
	if err != nil {
		return nil, err
	} else {
		config.Zone = zoneId
	}

	service, err := qclbsdk.NewService(config)
	if err != nil {
		return nil, err
	}

	return service, nil
}
