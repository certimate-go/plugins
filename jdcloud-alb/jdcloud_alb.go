package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	jdcore "github.com/jdcloud-api/jdcloud-sdk-go/core"
	jdcommon "github.com/jdcloud-api/jdcloud-sdk-go/services/common/models"
	jdlbapis "github.com/jdcloud-api/jdcloud-sdk-go/services/lb/apis"
	jdlbmodels "github.com/jdcloud-api/jdcloud-sdk-go/services/lb/models"
	"github.com/samber/lo"

	jdlb "github.com/certimate-go/certimate/pkg/sdk3rd-trimmed/github.com/jdcloud-api/jdcloud-sdk-go/services/lb/client"

	"github.com/certimate-go/certimate/pkg/core"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/jdcloudsslcert"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	AccessKeySecret string `json:"accessKeySecret"`

	RegionId string `json:"regionId"`

	DeployTarget string `json:"deployTarget"`

	LoadbalancerId string `json:"loadbalancerId,omitempty"`

	ListenerId string `json:"listenerId,omitempty"`

	Domain string `json:"domain,omitempty"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *jdlb.LbClient
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		AccessKeyId:     config.AccessKeyId,
		AccessKeySecret: config.AccessKeySecret,
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
	if d.config.LoadbalancerId == "" {
		return fmt.Errorf("config `loadbalancerId` is required")
	}

	describeLoadBalancerReq := jdlbapis.NewDescribeLoadBalancerRequestWithoutParam()
	describeLoadBalancerReq.SetRegionId(d.config.RegionId)
	describeLoadBalancerReq.SetLoadBalancerId(d.config.LoadbalancerId)
	describeLoadBalancerResp, err := d.sdkClient.DescribeLoadBalancer(describeLoadBalancerReq)
	if err == nil && describeLoadBalancerResp.Error.Code != 0 {
		err = fmt.Errorf("JDCloud API error %d (%s): %s", describeLoadBalancerResp.Error.Code, describeLoadBalancerResp.Error.Status, describeLoadBalancerResp.Error.Message)
	}
	d.logger.Debug("sdk request 'lb.DescribeLoadBalancer'", slog.Any("request", describeLoadBalancerReq), slog.Any("response", describeLoadBalancerResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'lb.DescribeLoadBalancer': %w", err)
	}

	listenerIds := make([]string, 0)
	describeListenersPageNumber := 1
	describeListenersPageSize := 100
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		describeListenersReq := jdlbapis.NewDescribeListenersRequestWithoutParam()
		describeListenersReq.SetRegionId(d.config.RegionId)
		describeListenersReq.SetFilters([]jdcommon.Filter{{Name: "loadBalancerId", Values: []string{d.config.LoadbalancerId}}})
		describeListenersReq.SetPageSize(describeListenersPageNumber)
		describeListenersReq.SetPageSize(describeListenersPageSize)
		describeListenersResp, err := d.sdkClient.DescribeListeners(describeListenersReq)
		if err == nil && describeListenersResp.Error.Code != 0 {
			err = fmt.Errorf("JDCloud API error %d (%s): %s", describeListenersResp.Error.Code, describeListenersResp.Error.Status, describeListenersResp.Error.Message)
		}
		d.logger.Debug("sdk request 'lb.DescribeListeners'", slog.Any("request", describeListenersReq), slog.Any("response", describeListenersResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'lb.DescribeListeners': %w", err)
		}

		for _, listener := range describeListenersResp.Result.Listeners {
			if strings.EqualFold(listener.Protocol, "https") || strings.EqualFold(listener.Protocol, "tls") {
				listenerIds = append(listenerIds, listener.ListenerId)
			}
		}

		if len(describeListenersResp.Result.Listeners) < describeListenersPageSize {
			break
		}

		describeListenersPageNumber++
	}

	if len(listenerIds) == 0 {
		d.logger.Info("no alb listeners to deploy")
	} else {
		d.logger.Info("found alb listeners to deploy", slog.Any("listenerIds", listenerIds))

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
	if d.config.Domain == "" {
		return d.updateListenerDefaultCertificate(ctx, cloudListenerId, cloudCertId)
	} else {
		return d.updateListenerSniCertificate(ctx, cloudListenerId, cloudCertId)
	}
}

func (d *Deployer) updateListenerDefaultCertificate(ctx context.Context, cloudListenerId string, cloudCertId string) error {
	updateListenerReq := jdlbapis.NewUpdateListenerRequestWithoutParam()
	updateListenerReq.SetRegionId(d.config.RegionId)
	updateListenerReq.SetListenerId(cloudListenerId)
	updateListenerReq.SetCertificateSpecs([]jdlbmodels.CertificateSpec{{CertificateId: cloudCertId}})
	updateListenerResp, err := d.sdkClient.UpdateListener(updateListenerReq)
	if err == nil && updateListenerResp.Error.Code != 0 {
		err = fmt.Errorf("JDCloud API error %d (%s): %s", updateListenerResp.Error.Code, updateListenerResp.Error.Status, updateListenerResp.Error.Message)
	}
	d.logger.Debug("sdk request 'lb.UpdateListener'", slog.Any("request", updateListenerReq), slog.Any("response", updateListenerResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'lb.UpdateListener': %w", err)
	}

	return nil
}

func (d *Deployer) updateListenerSniCertificate(ctx context.Context, cloudListenerId string, cloudCertId string) error {
	describeListenerReq := jdlbapis.NewDescribeListenerRequestWithoutParam()
	describeListenerReq.SetRegionId(d.config.RegionId)
	describeListenerReq.SetListenerId(cloudListenerId)
	describeListenerResp, err := d.sdkClient.DescribeListener(describeListenerReq)
	if err == nil && describeListenerResp.Error.Code != 0 {
		err = fmt.Errorf("JDCloud API error %d (%s): %s", describeListenerResp.Error.Code, describeListenerResp.Error.Status, describeListenerResp.Error.Message)
	}
	d.logger.Debug("sdk request 'lb.DescribeListener'", slog.Any("request", describeListenerReq), slog.Any("response", describeListenerResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'lb.DescribeListener': %w", err)
	}

	extCertSpecs := lo.Filter(describeListenerResp.Result.Listener.ExtensionCertificateSpecs, func(extCertSpec jdlbmodels.ExtensionCertificateSpec, _ int) bool {
		return extCertSpec.Domain == d.config.Domain
	})
	if len(extCertSpecs) == 0 {
		addListenerCertificatesReq := jdlbapis.NewAddListenerCertificatesRequestWithoutParam()
		addListenerCertificatesReq.SetRegionId(d.config.RegionId)
		addListenerCertificatesReq.SetListenerId(cloudListenerId)
		addListenerCertificatesReq.SetCertificates([]jdlbmodels.ExtCertificateSpec{
			{
				CertificateId: cloudCertId,
				Domain:        d.config.Domain,
			},
		})
		addListenerCertificatesResp, err := d.sdkClient.AddListenerCertificates(addListenerCertificatesReq)
		if err == nil && addListenerCertificatesResp.Error.Code != 0 {
			err = fmt.Errorf("JDCloud API error %d (%s): %s", addListenerCertificatesResp.Error.Code, addListenerCertificatesResp.Error.Status, addListenerCertificatesResp.Error.Message)
		}
		d.logger.Debug("sdk request 'lb.AddListenerCertificates'", slog.Any("request", addListenerCertificatesReq), slog.Any("response", addListenerCertificatesResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'lb.AddListenerCertificates': %w", err)
		}
	}

	updateListenerCertificatesReq := jdlbapis.NewUpdateListenerCertificatesRequestWithoutParam()
	updateListenerCertificatesReq.SetRegionId(d.config.RegionId)
	updateListenerCertificatesReq.SetListenerId(cloudListenerId)
	updateListenerCertificatesReq.SetCertificates(lo.Map(extCertSpecs, func(extCertSpec jdlbmodels.ExtensionCertificateSpec, _ int) jdlbmodels.ExtCertificateUpdateSpec {
		return jdlbmodels.ExtCertificateUpdateSpec{
			CertificateBindId: extCertSpec.CertificateBindId,
			CertificateId:     &cloudCertId,
			Domain:            &extCertSpec.Domain,
		}
	}))
	updateListenerCertificatesResp, err := d.sdkClient.UpdateListenerCertificates(updateListenerCertificatesReq)
	if err == nil && updateListenerCertificatesResp.Error.Code != 0 {
		err = fmt.Errorf("JDCloud API error %d (%s): %s", updateListenerCertificatesResp.Error.Code, updateListenerCertificatesResp.Error.Status, updateListenerCertificatesResp.Error.Message)
	}
	d.logger.Debug("sdk request 'lb.UpdateListenerCertificates'", slog.Any("request", updateListenerCertificatesReq), slog.Any("response", updateListenerCertificatesResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'lb.UpdateListenerCertificates': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, accessKeySecret string) (*jdlb.LbClient, error) {
	clientCredentials := jdcore.NewCredentials(accessKeyId, accessKeySecret)
	client := jdlb.NewLbClient(clientCredentials)
	client.DisableLogger()
	return client, nil
}
