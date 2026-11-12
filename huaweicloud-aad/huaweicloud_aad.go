package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	hwaadmodelv1 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/aad/v1/model"
	hwaadregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/aad/v1/region"
	hwaadmodelv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/aad/v2/model"
	"github.com/samber/lo"

	hwaadv1 "github.com/certimate-go/plugins/internal/cloudsdk/github.com/huaweicloud/huaweicloud-sdk-go-v3/services/aad/v1"
	hwaadv2 "github.com/certimate-go/plugins/internal/cloudsdk/github.com/huaweicloud/huaweicloud-sdk-go-v3/services/aad/v2"
	xhuaweicloud "github.com/certimate-go/plugins/internal/cloudsdk/huaweicloud/auth"

	"github.com/certimate-go/certimate/pkg/core"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
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

	InstanceId string `json:"instanceId"`

	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`

	Domain string `json:"domain"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClients *wSDKClients
}

var _ Provider = (*Deployer)(nil)

type wSDKClients struct {
	AADv1 *hwaadv1.AadClient
	AADv2 *hwaadv2.AadClient
}

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	clientsV1, err := createSDKClientV1(config.AccessKeyId, config.SecretAccessKey, config.Region)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	clientsV2, err := createSDKClientV2(config.AccessKeyId, config.SecretAccessKey, config.Region)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return &Deployer{
		config:     config,
		logger:     slog.Default(),
		sdkClients: &wSDKClients{AADv1: clientsV1, AADv2: clientsV2},
	}, nil
}

func (d *Deployer) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}
}

func (d *Deployer) Deploy(ctx context.Context, certPEM, privkeyPEM string) (*DeployResult, error) {
	if d.config.InstanceId == "" {
		return nil, fmt.Errorf("config `instanceId` is required")
	}

	var domainIds []string
	switch d.config.DomainMatchPattern {
	case "", DOMAIN_MATCH_PATTERN_EXACT:
		{
			if d.config.Domain == "" {
				return nil, fmt.Errorf("config `domain` is required")
			}

			domainCandidates, err := d.getAllDomainsByInstanceId(ctx, d.config.InstanceId)
			if err != nil {
				return nil, err
			}

			domains := lo.Filter(domainCandidates, func(domainItem *hwaadmodelv2.InstanceDomainItem, _ int) bool {
				return d.config.Domain == lo.FromPtr(domainItem.DomainName)
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find domain")
			}

			domainIds = lo.Map(domains, func(domainItem *hwaadmodelv2.InstanceDomainItem, _ int) string {
				return lo.FromPtr(domainItem.DomainId)
			})
		}

	case DOMAIN_MATCH_PATTERN_WILDCARD:
		{
			if d.config.Domain == "" {
				return nil, fmt.Errorf("config `domain` is required")
			}

			domainCandidates, err := d.getAllDomainsByInstanceId(ctx, d.config.InstanceId)
			if err != nil {
				return nil, err
			}

			domains := lo.Filter(domainCandidates, func(domainItem *hwaadmodelv2.InstanceDomainItem, _ int) bool {
				return xcerthostname.IsMatch(d.config.Domain, lo.FromPtr(domainItem.DomainName))
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find any domains matched by wildcard")
			}

			domainIds = lo.Map(domains, func(domainItem *hwaadmodelv2.InstanceDomainItem, _ int) string {
				return lo.FromPtr(domainItem.DomainId)
			})
		}

	case DOMAIN_MATCH_PATTERN_CERTSAN:
		{
			domainCandidates, err := d.getAllDomainsByInstanceId(ctx, d.config.InstanceId)
			if err != nil {
				return nil, err
			}

			domains := lo.Filter(domainCandidates, func(domainItem *hwaadmodelv2.InstanceDomainItem, _ int) bool {
				return xcerthostname.IsMatchByCertificatePEM(certPEM, lo.FromPtr(domainItem.DomainName))
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find any domains matched by certificate")
			}

			domainIds = lo.Map(domains, func(domainItem *hwaadmodelv2.InstanceDomainItem, _ int) string {
				return lo.FromPtr(domainItem.DomainId)
			})
		}

	default:
		return nil, fmt.Errorf("unsupported domain match pattern: '%s'", d.config.DomainMatchPattern)
	}

	if len(domainIds) == 0 {
		d.logger.Info("no aad domains to deploy")
	} else {
		d.logger.Info("found aad domains to deploy", slog.Any("domainIds", domainIds))

		if err := xloop.ForRangeAllWithContext(ctx, domainIds, func(ctx context.Context, domainId string, _ int) error {
			return d.updateDomainCertificate(ctx, domainId, certPEM, privkeyPEM)
		}); err != nil {
			return nil, err
		}
	}

	return &DeployResult{}, nil
}

func (d *Deployer) getAllDomainsByInstanceId(ctx context.Context, cloudInstanceId string) ([]*hwaadmodelv2.InstanceDomainItem, error) {
	domains := make([]*hwaadmodelv2.InstanceDomainItem, 0)

	listInstanceDomainsOffset := 0
	listInstanceDomainsLimit := 10
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		listInstanceDomainsReq := &hwaadmodelv2.ListInstanceDomainsRequest{
			InstanceId: cloudInstanceId,
			Offset:     lo.ToPtr(int32(listInstanceDomainsOffset)),
			Limit:      lo.ToPtr(int32(listInstanceDomainsLimit)),
		}
		listInstanceDomainsResp, err := d.sdkClients.AADv2.ListInstanceDomains(listInstanceDomainsReq)
		d.logger.Debug("sdk request 'aad.ListInstanceDomains'")
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'aad.ListInstanceDomains': %w", err)
		}

		if listInstanceDomainsResp.Domains == nil {
			break
		}

		ignoredStatuses := []string{"1"}
		for _, domainItem := range *listInstanceDomainsResp.Domains {
			if lo.Contains(ignoredStatuses, lo.FromPtr(domainItem.DomainStatus)) {
				continue
			}

			domains = append(domains, &domainItem)
		}

		if len(*listInstanceDomainsResp.Domains) < listInstanceDomainsLimit {
			break
		}

		listInstanceDomainsOffset += listInstanceDomainsLimit
	}

	return domains, nil
}

func (d *Deployer) updateDomainCertificate(ctx context.Context, cloudDomainId string, certPEM, privkeyPEM string) error {
	setCertForDomainReq := &hwaadmodelv1.SetCertForDomainRequest{
		Body: &hwaadmodelv1.CertificateBody{
			OpType:      0,
			DomainId:    cloudDomainId,
			CertName:    fmt.Sprintf("certimate_%d", time.Now().UnixMilli()),
			CertFile:    lo.ToPtr(certPEM),
			CertKeyFile: lo.ToPtr(privkeyPEM),
		},
	}
	_, err := d.sdkClients.AADv1.SetCertForDomain(setCertForDomainReq)
	d.logger.Debug("sdk request 'aad.SetCertForDomain'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'aad.SetCertForDomain': %w", err)
	}

	return nil
}

func createSDKClientV1(accessKeyId, secretAccessKey, region string) (*hwaadv1.AadClient, error) {
	if region == "" {
		region = "cn-north-4"
	}

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

	hcRegion, err := hwaadregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := hwaadv1.AadClientBuilder().
		WithRegion(hcRegion).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	client := hwaadv1.NewAadClient(hcClient)
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

func createSDKClientV2(accessKeyId, secretAccessKey, region string) (*hwaadv2.AadClient, error) {
	if region == "" {
		region = "cn-north-4"
	}

	auth, err := global.NewCredentialsBuilder().
		WithAk(accessKeyId).
		WithSk(secretAccessKey).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	hcRegion, err := hwaadregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := hwaadv2.AadClientBuilder().
		WithRegion(hcRegion).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	client := hwaadv2.NewAadClient(hcClient)
	return client, nil
}
