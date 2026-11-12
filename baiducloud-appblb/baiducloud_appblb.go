package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	bceappblb "github.com/baidubce/bce-sdk-go/services/appblb"
	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/baiducloudcertmgr"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	Region string `json:"region"`

	DeployTarget string `json:"deployTarget"`

	LoadbalancerId string `json:"loadbalancerId,omitempty"`

	ListenerPort int32 `json:"listenerPort,omitempty"`

	Domain string `json:"domain,omitempty"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *bceappblb.Client
	sdkCertmgr *cmgrimpl.Certmgr
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

	describeLoadBalancerDetailResp, err := d.sdkClient.DescribeLoadBalancerDetail(d.config.LoadbalancerId)
	d.logger.Debug("sdk request 'appblb.DescribeLoadBalancerAttribute'", slog.String("params.blbId", d.config.LoadbalancerId), slog.Any("response", describeLoadBalancerDetailResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'appblb.DescribeLoadBalancerDetail': %w", err)
	}

	type listenerEntry struct {
		Type string
		Port int32
	}
	listeners := make([]listenerEntry, 0)
	for _, listener := range describeLoadBalancerDetailResp.Listener {
		if listener.Type == "HTTPS" || listener.Type == "SSL" {
			listenerPort, err := strconv.Atoi(listener.Port)
			if err != nil {
				continue
			}

			listeners = append(listeners, listenerEntry{
				Type: listener.Type,
				Port: int32(listenerPort),
			})
		}
	}

	if len(listeners) == 0 {
		d.logger.Info("no appblb listeners to deploy")
	} else {
		d.logger.Info("found appblb listeners to deploy", slog.Any("listeners", listeners))

		if err := xloop.ForRangeAllWithContext(ctx, listeners, func(ctx context.Context, listener listenerEntry, _ int) error {
			return d.updateListenerCertificate(ctx, d.config.LoadbalancerId, listener.Type, listener.Port, cloudCertId)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (d *Deployer) deployToListener(ctx context.Context, cloudCertId string) error {
	if d.config.LoadbalancerId == "" {
		return fmt.Errorf("config `loadbalancerId` is required")
	}
	if d.config.ListenerPort == 0 {
		return fmt.Errorf("config `listenerPort` is required")
	}

	describeAppAllListenersRequest := &bceappblb.DescribeAppListenerArgs{
		ListenerPort: uint16(d.config.ListenerPort),
	}
	describeAppAllListenersResp, err := d.sdkClient.DescribeAppAllListeners(d.config.LoadbalancerId, describeAppAllListenersRequest)
	d.logger.Debug("sdk request 'appblb.DescribeAppAllListeners'", slog.String("params.blbId", d.config.LoadbalancerId), slog.Any("request", describeAppAllListenersRequest), slog.Any("response", describeAppAllListenersResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'appblb.DescribeAppAllListeners': %w", err)
	}

	type listenerEntry struct {
		Type string
		Port int32
	}
	listeners := make([]listenerEntry, 0)
	for _, listener := range describeAppAllListenersResp.ListenerList {
		if listener.ListenerType == "HTTPS" || listener.ListenerType == "SSL" {
			listeners = append(listeners, listenerEntry{
				Type: listener.ListenerType,
				Port: int32(listener.ListenerPort),
			})
		}
	}

	if err := xloop.ForRangeAllWithContext(ctx, listeners, func(ctx context.Context, listener listenerEntry, _ int) error {
		return d.updateListenerCertificate(ctx, d.config.LoadbalancerId, listener.Type, listener.Port, cloudCertId)
	}); err != nil {
		return err
	}

	return nil
}

func (d *Deployer) updateListenerCertificate(ctx context.Context, cloudLoadbalancerId string, cloudListenerType string, cloudListenerPort int32, cloudCertId string) error {
	switch strings.ToUpper(cloudListenerType) {
	case "HTTPS":
		return d.updateHttpsListenerCertificate(ctx, cloudLoadbalancerId, cloudListenerPort, cloudCertId)
	case "SSL":
		return d.updateSslListenerCertificate(ctx, cloudLoadbalancerId, cloudListenerPort, cloudCertId)
	default:
		return fmt.Errorf("unsupported listener type '%s'", cloudListenerType)
	}
}

func (d *Deployer) updateHttpsListenerCertificate(ctx context.Context, cloudLoadbalancerId string, cloudHttpsListenerPort int32, cloudCertId string) error {
	describeAppHTTPSListenersReq := &bceappblb.DescribeAppListenerArgs{
		ListenerPort: uint16(cloudHttpsListenerPort),
		MaxKeys:      1,
	}
	describeAppHTTPSListenersResp, err := d.sdkClient.DescribeAppHTTPSListeners(cloudLoadbalancerId, describeAppHTTPSListenersReq)
	d.logger.Debug("sdk request 'appblb.DescribeAppHTTPSListeners'", slog.String("params.blbId", cloudLoadbalancerId), slog.Any("request", describeAppHTTPSListenersReq), slog.Any("response", describeAppHTTPSListenersResp))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'appblb.DescribeAppHTTPSListeners': %w", err)
	} else if len(describeAppHTTPSListenersResp.ListenerList) == 0 {
		return fmt.Errorf("could not find appblb listener '%s:%d'", cloudLoadbalancerId, cloudHttpsListenerPort)
	}

	listenerInfo := describeAppHTTPSListenersResp.ListenerList[0]
	if d.config.Domain == "" {

		if lo.SomeBy(listenerInfo.CertIds, func(item string) bool { return item == cloudCertId }) {
			d.logger.Info("no need to deploy appblb listener default certificate")
			return nil
		}

		return d.updateHttpsListenerDefaultCertificate(ctx, cloudLoadbalancerId, &listenerInfo, cloudCertId)
	} else {
		return d.updateHttpsListenerSniCertificate(ctx, cloudLoadbalancerId, &listenerInfo, cloudCertId)
	}
}

func (d *Deployer) updateHttpsListenerDefaultCertificate(ctx context.Context, cloudLoadbalancerId string, cloudHttpsListenerInfo *bceappblb.AppHTTPSListenerModel, cloudCertId string) error {
	updateAppHTTPSListenerReq := &bceappblb.UpdateAppHTTPSListenerArgs{
		ClientToken:  rand.Text(),
		ListenerPort: cloudHttpsListenerInfo.ListenerPort,
		Scheduler:    cloudHttpsListenerInfo.Scheduler,
		CertIds:      []string{cloudCertId},
	}
	err := d.sdkClient.UpdateAppHTTPSListener(cloudLoadbalancerId, updateAppHTTPSListenerReq)
	d.logger.Debug("sdk request 'appblb.UpdateAppHTTPSListener'", slog.Any("request", updateAppHTTPSListenerReq))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'appblb.UpdateAppHTTPSListener': %w", err)
	}

	return nil
}

func (d *Deployer) updateHttpsListenerSniCertificate(ctx context.Context, cloudLoadbalancerId string, cloudHttpsListenerInfo *bceappblb.AppHTTPSListenerModel, cloudCertId string) error {
	updateAppHTTPSListenerReq := &bceappblb.UpdateAppHTTPSListenerArgs{
		ClientToken:  rand.Text(),
		ListenerPort: cloudHttpsListenerInfo.ListenerPort,
		Scheduler:    cloudHttpsListenerInfo.Scheduler,
		CertIds:      cloudHttpsListenerInfo.CertIds,
		AdditionalCertDomains: lo.Map(cloudHttpsListenerInfo.AdditionalCertDomains, func(domain bceappblb.AdditionalCertDomainsModel, _ int) bceappblb.AdditionalCertDomainsModel {
			if domain.Host == d.config.Domain {
				return bceappblb.AdditionalCertDomainsModel{
					Host:   domain.Host,
					CertId: cloudCertId,
				}
			}

			return bceappblb.AdditionalCertDomainsModel{
				Host:   domain.Host,
				CertId: domain.CertId,
			}
		}),
	}
	err := d.sdkClient.UpdateAppHTTPSListener(cloudLoadbalancerId, updateAppHTTPSListenerReq)
	d.logger.Debug("sdk request 'appblb.UpdateAppHTTPSListener'", slog.Any("request", updateAppHTTPSListenerReq))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'appblb.UpdateAppHTTPSListener': %w", err)
	}

	return nil
}

func (d *Deployer) updateSslListenerCertificate(ctx context.Context, cloudLoadbalancerId string, cloudHttpsListenerPort int32, cloudCertId string) error {
	updateAppSSLListenerReq := &bceappblb.UpdateAppSSLListenerArgs{
		ClientToken:  rand.Text(),
		ListenerPort: uint16(cloudHttpsListenerPort),
		CertIds:      []string{cloudCertId},
	}
	err := d.sdkClient.UpdateAppSSLListener(cloudLoadbalancerId, updateAppSSLListenerReq)
	d.logger.Debug("sdk request 'appblb.UpdateAppSSLListener'", slog.Any("request", updateAppSSLListenerReq))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'appblb.UpdateAppSSLListener': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*bceappblb.Client, error) {
	endpoint := ""
	if region != "" {
		endpoint = fmt.Sprintf("blb.%s.baidubce.com", region)
	}

	client, err := bceappblb.NewClient(accessKeyId, secretAccessKey, endpoint)
	if err != nil {
		return nil, err
	}

	return client, nil
}
