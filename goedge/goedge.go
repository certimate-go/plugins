package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"

	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	goedgesdk "github.com/certimate-go/plugins/internal/goedgesdk"
)

type deploymentConfig struct {
	ServerUrl string `json:"serverUrl"`

	ApiRole string `json:"apiRole"`

	AccessKeyId string `json:"accessKeyId"`

	AccessKey string `json:"accessKey"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`

	DeployTarget string `json:"deployTarget"`

	CertificateId int64 `json:"certificateId,omitempty"`
}

type deployment struct {
	config    *deploymentConfig
	logger    *slog.Logger
	sdkClient *goedgesdk.Client
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiRole, config.AccessKeyId, config.AccessKey, config.AllowInsecureConnections)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return &deployment{
		config:    config,
		logger:    slog.Default(),
		sdkClient: client,
	}, nil
}

func (d *deployment) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}
}

func (d *deployment) deploy(ctx context.Context, certPEM, privkeyPEM string) (*plugin.DeployResult, error) {
	switch d.config.DeployTarget {
	case DEPLOY_TARGET_CERTIFICATE:
		if err := d.deployToCertificate(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.CertificateId == 0 {
		return fmt.Errorf("config `certificateId` is required")
	}

	certX509, err := xcert.ParseCertificateFromPEM(certPEM)
	if err != nil {
		return err
	}

	updateSSLCertReq := &goedgesdk.UpdateSSLCertRequest{
		SSLCertId:   d.config.CertificateId,
		IsOn:        true,
		Name:        fmt.Sprintf("certimate-%d", time.Now().UnixMilli()),
		Description: "upload from Certimate",
		ServerName:  certX509.Subject.CommonName,
		IsCA:        false,
		CertData:    base64.StdEncoding.EncodeToString([]byte(certPEM)),
		KeyData:     base64.StdEncoding.EncodeToString([]byte(privkeyPEM)),
		TimeBeginAt: certX509.NotBefore.Unix(),
		TimeEndAt:   certX509.NotAfter.Unix(),
		DNSNames:    certX509.DNSNames,
		CommonNames: []string{certX509.Subject.CommonName},
	}
	_, err = d.sdkClient.UpdateSSLCertWithContext(ctx, updateSSLCertReq)
	d.logger.Debug("sdk request 'UpdateSSLCert'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'UpdateSSLCert': %w", err)
	}

	return nil
}

func createSDKClient(serverUrl, apiRole, accessKeyId, accessKey string, skipTlsVerify bool) (*goedgesdk.Client, error) {
	client, err := goedgesdk.NewClient(serverUrl,
		goedgesdk.WithRole(apiRole),
		goedgesdk.WithAccessKey(accessKeyId, accessKey),
	)
	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
