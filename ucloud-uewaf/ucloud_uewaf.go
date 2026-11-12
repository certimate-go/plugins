package main

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ucloud/ucloud-sdk-go/ucloud"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"

	"github.com/certimate-go/certimate/pkg/core"
	ucloudsdk "github.com/certimate-go/plugins/internal/ucloudsdk/uewaf"
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

	Domain string `json:"domain"`
}

type Deployer struct {
	config    *DeployerConfig
	logger    *slog.Logger
	sdkClient *ucloudsdk.UEWAFClient
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

	return &Deployer{
		config:    config,
		logger:    slog.Default(),
		sdkClient: client,
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
	if d.config.Domain == "" {
		return nil, fmt.Errorf("config `domain` is required")
	}

	certPEMBase64 := base64.StdEncoding.EncodeToString([]byte(certPEM))
	privkeyPEMBase64 := base64.StdEncoding.EncodeToString([]byte(privkeyPEM))
	certMd5 := md5.Sum([]byte(certPEMBase64 + privkeyPEMBase64))
	certMd5Hex := hex.EncodeToString(certMd5[:])
	certName := fmt.Sprintf("certimate_%d", time.Now().UnixMilli())

	addWafDomainCertificateInfoReq := d.sdkClient.NewAddWafDomainCertificateInfoRequest()
	addWafDomainCertificateInfoReq.Domain = ucloud.String(d.config.Domain)
	addWafDomainCertificateInfoReq.CertificateName = ucloud.String(certName)
	addWafDomainCertificateInfoReq.SslPublicKey = ucloud.String(certPEMBase64)
	addWafDomainCertificateInfoReq.SslPrivateKey = ucloud.String(privkeyPEMBase64)
	addWafDomainCertificateInfoReq.SslMD = ucloud.String(certMd5Hex)
	addWafDomainCertificateInfoReq.SslKeyLess = ucloud.String("off")
	addWafDomainCertificateInfoResp, err := d.sdkClient.AddWafDomainCertificateInfo(addWafDomainCertificateInfoReq)
	d.logger.Debug("sdk request 'uewaf.AddWafDomainCertificateInfo'", slog.Any("response", addWafDomainCertificateInfoResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'uewaf.AddWafDomainCertificateInfo': %w", err)
	}

	return &DeployResult{}, nil
}

func createSDKClient(publicKey, privateKey, projectId, endpoint string) (*ucloudsdk.UEWAFClient, error) {
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

	credential := auth.NewCredential()
	credential.PublicKey = publicKey
	credential.PrivateKey = privateKey

	client := ucloudsdk.NewClient(&cfg, &credential)
	return client, nil
}
