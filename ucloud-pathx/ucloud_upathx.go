package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ucloud/ucloud-sdk-go/services/uaccount"
	"github.com/ucloud/ucloud-sdk-go/ucloud"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"

	"github.com/certimate-go/certimate/pkg/core"
	ucloudsdk "github.com/certimate-go/plugins/internal/ucloudsdk/upathx"
	cmgrimpl "github.com/certimate-go/plugins/internal/ucloudupathxcertmgr"
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

	AcceleratorId string `json:"acceleratorId"`

	ListenerPort int32 `json:"listenerPort"`
}

type Deployer struct {
	config     *DeployerConfig
	logger     *slog.Logger
	sdkClient  *ucloudsdk.UPathXClient
	sdkCertmgr core.Certmgr
}

var _ Provider = (*Deployer)(nil)

func NewDeployer(config *DeployerConfig) (*Deployer, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.PublicKey, config.PrivateKey, config.ProjectId, config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		PublicKey:  config.PublicKey,
		PrivateKey: config.PrivateKey,
		ProjectId:  config.ProjectId,
		Endpoint:   config.Endpoint,
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
	if d.config.AcceleratorId == "" {
		return nil, fmt.Errorf("config `acceleratorId` is required")
	}
	if d.config.ListenerPort == 0 {
		return nil, fmt.Errorf("config `listenerPort` is required")
	}

	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	bindPathXSSLReq := d.sdkClient.NewBindPathXSSLRequest()
	bindPathXSSLReq.UGAId = ucloud.String(d.config.AcceleratorId)
	bindPathXSSLReq.Port = []int{int(d.config.ListenerPort)}
	bindPathXSSLReq.SSLId = ucloud.String(upres.CertId)
	bindPathXSSLResp, err := d.sdkClient.BindPathXSSL(bindPathXSSLReq)
	d.logger.Debug("sdk request 'pathx.BindPathXSSL'", slog.Any("request", bindPathXSSLReq), slog.Any("response", bindPathXSSLResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'pathx.BindPathXSSL': %w", err)
	}

	return &DeployResult{}, nil
}

func createSDKClient(publicKey, privateKey, projectId, endpoint string) (*ucloudsdk.UPathXClient, error) {
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

	if cfg.ProjectId == "" {
		defaultProjectId, err := getSDKDefaultProjectId(publicKey, privateKey, endpoint)
		if err != nil {
			return nil, err
		}

		cfg.ProjectId = defaultProjectId
	}

	credential := auth.NewCredential()
	credential.PublicKey = publicKey
	credential.PrivateKey = privateKey

	client := ucloudsdk.NewClient(&cfg, &credential)
	return client, nil
}

func getSDKDefaultProjectId(publicKey, privateKey, endpoint string) (string, error) {
	cfg := ucloud.NewConfig()
	if endpoint != "" {
		if strings.Contains(endpoint, "://") {
			cfg.BaseUrl = endpoint
		} else {
			cfg.BaseUrl = "https://" + endpoint
		}
	}

	credential := auth.NewCredential()
	credential.PublicKey = publicKey
	credential.PrivateKey = privateKey

	client := uaccount.NewClient(&cfg, &credential)

	request := client.NewGetProjectListRequest()
	response, err := client.GetProjectList(request)
	if err != nil {
		return "", err
	}

	for _, projectItem := range response.ProjectSet {
		if projectItem.IsDefault {
			return projectItem.ProjectId, nil
		}
	}

	return "", fmt.Errorf("ucloud: no default project found")
}
