package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	hwelbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
	hwelbregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/region"
	"github.com/samber/lo"

	hwelb "github.com/certimate-go/plugins/internal/cloudsdk/github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3"

	"github.com/certimate-go/certimate/pkg/core"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	cmgrimpl "github.com/certimate-go/plugins/internal/certmgr/huaweicloud-elb"
	xhuaweicloud "github.com/certimate-go/plugins/internal/cloudsdk/huaweicloud/auth"
)

type (
	Provider     = core.Deployer
	DeployResult = core.DeployerDeployResult
)

type DeployerConfig struct {
	AccessKeyId string `json:"accessKeyId"`

	SecretAccessKey string `json:"secretAccessKey"`

	EnterpriseProjectId string `json:"enterpriseProjectId,omitempty"`

	Region string `json:"region"`

	DeployTarget string `json:"deployTarget"`

	LoadbalancerId string `json:"loadbalancerId,omitempty"`

	ListenerId string `json:"listenerId,omitempty"`

	CertificateId string `json:"certificateId,omitempty"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *hwelb.ElbClient
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
		AccessKeyId:         config.AccessKeyId,
		SecretAccessKey:     config.SecretAccessKey,
		EnterpriseProjectId: config.EnterpriseProjectId,
		Region:              config.Region,
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

	case DEPLOY_TARGET_CERTIFICATE:
		if err := d.deployToCertificate(ctx, certPEM, privkeyPEM); err != nil {
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

	showLoadBalancerReq := &hwelbmodel.ShowLoadBalancerRequest{
		LoadbalancerId: d.config.LoadbalancerId,
	}
	showLoadBalancerResp, err := d.sdkClient.ShowLoadBalancer(showLoadBalancerReq)
	d.logger.Debug("sdk request 'elb.ShowLoadBalancer'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'elb.ShowLoadBalancer': %w", err)
	}

	listenerIds := make([]string, 0)
	listListenersMarker := (*string)(nil)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		listListenersReq := &hwelbmodel.ListListenersRequest{
			EnterpriseProjectId: lo.IfF(d.config.EnterpriseProjectId != "", func() *[]string { return lo.ToPtr([]string{d.config.EnterpriseProjectId}) }).Else(nil),
			Marker:              listListenersMarker,
			Limit:               lo.ToPtr(int32(2000)),
			Protocol:            &[]string{"HTTPS", "TERMINATED_HTTPS"},
			LoadbalancerId:      &[]string{showLoadBalancerResp.Loadbalancer.Id},
		}
		listListenersResp, err := d.sdkClient.ListListeners(listListenersReq)
		d.logger.Debug("sdk request 'elb.ListListeners'")
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'elb.ListListeners': %w", err)
		}

		if listListenersResp.Listeners == nil {
			break
		}

		for _, listener := range *listListenersResp.Listeners {
			listenerIds = append(listenerIds, listener.Id)
		}

		if len(*listListenersResp.Listeners) == 0 || listListenersResp.PageInfo.NextMarker == nil {
			break
		}

		listListenersMarker = listListenersResp.PageInfo.NextMarker
	}

	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	if len(listenerIds) == 0 {
		d.logger.Info("no elb listeners to deploy")
	} else {
		d.logger.Info("found elb listeners to deploy", slog.Any("listenerIds", listenerIds))

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

func (d *Deployer) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.CertificateId == "" {
		return fmt.Errorf("config `certificateId` is required")
	}

	rplres, err := d.sdkCertmgr.Replace(ctx, d.config.CertificateId, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to replace certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate replaced", slog.Any("result", rplres))
	}

	return nil
}

func (d *Deployer) updateListenerCertificate(ctx context.Context, cloudListenerId string, cloudCertId string) error {
	showListenerReq := &hwelbmodel.ShowListenerRequest{
		ListenerId: cloudListenerId,
	}
	showListenerResp, err := d.sdkClient.ShowListener(showListenerReq)
	d.logger.Debug("sdk request 'elb.ShowListener'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'elb.ShowListener': %w", err)
	}

	updateListenerReq := &hwelbmodel.UpdateListenerRequest{
		ListenerId: cloudListenerId,
		Body: &hwelbmodel.UpdateListenerRequestBody{
			Listener: &hwelbmodel.UpdateListenerOption{
				DefaultTlsContainerRef: lo.ToPtr(cloudCertId),
			},
		},
	}
	if showListenerResp.Listener.SniContainerRefs != nil {
		if len(showListenerResp.Listener.SniContainerRefs) > 0 {

			sniCertIds := make([]string, 0)
			sniCertIds = append(sniCertIds, cloudCertId)

			listOldCertificateReq := &hwelbmodel.ListCertificatesRequest{
				Id: &showListenerResp.Listener.SniContainerRefs,
			}
			listOldCertificateResp, err := d.sdkClient.ListCertificates(listOldCertificateReq)
			d.logger.Debug("sdk request 'elb.ListCertificates'")
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'elb.ListCertificates': %w", err)
			}

			showNewCertificateReq := &hwelbmodel.ShowCertificateRequest{
				CertificateId: cloudCertId,
			}
			showNewCertificateResp, err := d.sdkClient.ShowCertificate(showNewCertificateReq)
			d.logger.Debug("sdk request 'elb.ShowCertificate'")
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'elb.ShowCertificate': %w", err)
			}

			for _, oldCertInfo := range *listOldCertificateResp.Certificates {
				newCertInfo := showNewCertificateResp.Certificate

				if oldCertInfo.SubjectAlternativeNames != nil && newCertInfo.SubjectAlternativeNames != nil {
					if strings.Join(*oldCertInfo.SubjectAlternativeNames, ",") == strings.Join(*newCertInfo.SubjectAlternativeNames, ",") {
						continue
					}
				} else {
					if oldCertInfo.Domain == newCertInfo.Domain {
						continue
					}
				}

				sniCertIds = append(sniCertIds, oldCertInfo.Id)
			}

			updateListenerReq.Body.Listener.SniContainerRefs = &sniCertIds
		}

		if showListenerResp.Listener.SniMatchAlgo != "" {
			updateListenerReq.Body.Listener.SniMatchAlgo = lo.ToPtr(showListenerResp.Listener.SniMatchAlgo)
		}
	}
	_, err = d.sdkClient.UpdateListener(updateListenerReq)
	d.logger.Debug("sdk request 'elb.UpdateListener'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'elb.UpdateListener': %w", err)
	}

	return nil
}

func createSDKClient(accessKeyId, secretAccessKey, region string) (*hwelb.ElbClient, error) {
	projectId, err := getSDKProjectId(accessKeyId, secretAccessKey, region)
	if err != nil {
		return nil, err
	}

	auth, err := basic.NewCredentialsBuilder().
		WithAk(accessKeyId).
		WithSk(secretAccessKey).
		WithProjectId(projectId).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	hcRegion, err := hwelbregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := hwelb.ElbClientBuilder().
		WithRegion(hcRegion).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	client := hwelb.NewElbClient(hcClient)
	return client, nil
}

func getSDKProjectId(accessKeyId, secretAccessKey, region string) (string, error) {
	auth, err := global.NewCredentialsBuilder().
		WithAk(accessKeyId).
		WithSk(secretAccessKey).
		SafeBuild()
	if err != nil {
		return "", err
	}

	return xhuaweicloud.GetKeystoneProjectIDWithRegion(auth, region)
}
