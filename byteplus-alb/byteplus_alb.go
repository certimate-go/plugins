package main

import (
	"context"
	"fmt"
	"log/slog"

	bp "github.com/byteplus-sdk/byteplus-go-sdk-v2/byteplus"
	bpsession "github.com/byteplus-sdk/byteplus-go-sdk-v2/byteplus/session"
	"github.com/samber/lo"

	bpalb "github.com/certimate-go/plugins/internal/cloudsdk/github.com/byteplus-sdk/byteplus-go-sdk-v2/service/alb"

	"github.com/certimate-go/certimate/pkg/core"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/byteplus-certcenter"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	ProjectName string `json:"projectName,omitempty"`

	Region string `json:"region"`

	DeployTarget string `json:"deployTarget"`

	LoadbalancerId string `json:"loadbalancerId,omitempty"`

	ListenerId string `json:"listenerId,omitempty"`

	Domain string `json:"domain,omitempty"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *bpalb.ALB
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.AccessKeyId, config.SecretAccessKey, config.Region)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		AccessKeyId:     config.AccessKeyId,
		SecretAccessKey: config.SecretAccessKey,
		ProjectName:     config.ProjectName,
		Region:          "ap-singapore-1",
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

	describeLoadBalancerAttributesReq := &bpalb.DescribeLoadBalancerAttributesInput{
		LoadBalancerId: bp.String(d.config.LoadbalancerId),
	}
	_, err := d.sdkClient.DescribeLoadBalancerAttributes(describeLoadBalancerAttributesReq)
	d.logger.Debug("sdk request 'alb.DescribeLoadBalancerAttributes'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'alb.DescribeLoadBalancerAttributes': %w", err)
	}

	listenerIds := make([]string, 0)
	describeListenersPageSize := 100
	describeListenersPageNumber := 1
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		describeListenersReq := &bpalb.DescribeListenersInput{
			ProjectName:    lo.EmptyableToPtr(d.config.ProjectName),
			LoadBalancerId: bp.String(d.config.LoadbalancerId),
			Protocol:       bp.String("HTTPS"),
			PageNumber:     bp.Int64(int64(describeListenersPageNumber)),
			PageSize:       bp.Int64(int64(describeListenersPageSize)),
		}
		describeListenersResp, err := d.sdkClient.DescribeListeners(describeListenersReq)
		d.logger.Debug("sdk request 'alb.DescribeListeners'")
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'alb.DescribeListeners': %w", err)
		}

		for _, listener := range describeListenersResp.Listeners {
			listenerIds = append(listenerIds, *listener.ListenerId)
		}

		if len(describeListenersResp.Listeners) < describeListenersPageSize {
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
	describeListenerAttributesReq := &bpalb.DescribeListenerAttributesInput{
		ListenerId: bp.String(cloudListenerId),
	}
	describeListenerAttributesResp, err := d.sdkClient.DescribeListenerAttributesWithContext(ctx, describeListenerAttributesReq)
	d.logger.Debug("sdk request 'alb.DescribeListenerAttributes'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'alb.DescribeListenerAttributes': %w", err)
	}

	if d.config.Domain == "" {

		if bp.StringValue(describeListenerAttributesResp.CertificateId) == cloudCertId {
			d.logger.Info("no need to deploy alb listener default certificate")
			return nil
		}

		return d.updateListenerDefaultCertificate(ctx, *describeListenerAttributesResp, cloudCertId)
	} else {
		return d.updateListenerSniCertificate(ctx, *describeListenerAttributesResp, cloudCertId)
	}
}

func (d *Deployer) updateListenerDefaultCertificate(ctx context.Context, cloudListenerInfo bpalb.DescribeListenerAttributesOutput, cloudCertId string) error {
	modifyListenerAttributesReq := &bpalb.ModifyListenerAttributesInput{
		ListenerId:              cloudListenerInfo.ListenerId,
		CertificateSource:       bp.String("cert_center"),
		CertCenterCertificateId: bp.String(cloudCertId),
	}
	_, err := d.sdkClient.ModifyListenerAttributesWithContext(ctx, modifyListenerAttributesReq)
	d.logger.Debug("sdk request 'alb.ModifyListenerAttributes'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'alb.ModifyListenerAttributes': %w", err)
	}

	return nil
}

func (d *Deployer) updateListenerSniCertificate(ctx context.Context, cloudListenerInfo bpalb.DescribeListenerAttributesOutput, cloudCertId string) error {
	modifyListenerAttributesReq := &bpalb.ModifyListenerAttributesInput{
		ListenerId: cloudListenerInfo.ListenerId,
		DomainExtensions: lo.Map(
			lo.Filter(cloudListenerInfo.DomainExtensions, func(domain *bpalb.DomainExtensionForDescribeListenerAttributesOutput, _ int) bool {
				return bp.StringValue(domain.Domain) == d.config.Domain
			}),
			func(domain *bpalb.DomainExtensionForDescribeListenerAttributesOutput, _ int) *bpalb.DomainExtensionForModifyListenerAttributesInput {
				return &bpalb.DomainExtensionForModifyListenerAttributesInput{
					DomainExtensionId:       domain.DomainExtensionId,
					Domain:                  domain.Domain,
					CertificateSource:       bp.String("cert_center"),
					CertCenterCertificateId: bp.String(cloudCertId),
					Action:                  bp.String("modify"),
				}
			},
		),
	}
	_, err := d.sdkClient.ModifyListenerAttributesWithContext(ctx, modifyListenerAttributesReq)
	d.logger.Debug("sdk request 'alb.ModifyListenerAttributes'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'alb.ModifyListenerAttributes': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*bpalb.ALB, error) {
	config := bp.NewConfig().
		WithAkSk(accessKeyId, secretAccessKey).
		WithRegion(region)

	session, err := bpsession.NewSession(config)
	if err != nil {
		return nil, err
	}

	client := bpalb.New(session)
	return client, nil
}
