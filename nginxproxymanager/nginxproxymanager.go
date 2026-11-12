package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/core"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
	cmgrimpl "github.com/certimate-go/plugins/internal/nginxproxymanagercertmgr"
	npmsdk "github.com/certimate-go/plugins/internal/nginxproxymanagersdk"
)

type deploymentConfig struct {
	ServerUrl string `json:"serverUrl"`

	AuthMethod string `json:"authMethod,omitempty"`

	Username string `json:"username,omitempty"`

	Password string `json:"password,omitempty"`

	ApiToken string `json:"apiToken,omitempty"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`

	DeployTarget string `json:"deployTarget"`

	HostMatchPattern string `json:"hostMatchPattern,omitempty"`

	HostType string `json:"hostType,omitempty"`

	HostId int64 `json:"hostId,omitempty"`

	CertificateId int64 `json:"certificateId,omitempty"`
}

type deployment struct {
	config     *deploymentConfig
	logger     *slog.Logger
	sdkClient  *npmsdk.Client
	sdkCertmgr core.Certmgr
}

func newDeployment(config *deploymentConfig) (*deployment, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the deployer provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.AuthMethod, config.Username, config.Password, config.ApiToken, config.AllowInsecureConnections)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	pcertmgr, err := cmgrimpl.NewCertmgr(&cmgrimpl.CertmgrConfig{
		ServerUrl:                config.ServerUrl,
		AuthMethod:               config.AuthMethod,
		Username:                 config.Username,
		Password:                 config.Password,
		ApiToken:                 config.ApiToken,
		AllowInsecureConnections: config.AllowInsecureConnections,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}

	return &deployment{
		config:     config,
		logger:     slog.Default(),
		sdkClient:  client,
		sdkCertmgr: pcertmgr,
	}, nil
}

func (d *deployment) SetLogger(logger *slog.Logger) {
	if logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	} else {
		d.logger = logger
	}

	d.sdkCertmgr.SetLogger(logger)
}

func (d *deployment) deploy(ctx context.Context, certPEM, privkeyPEM string) (*plugin.DeployResult, error) {
	switch d.config.DeployTarget {
	case DEPLOY_TARGET_HOST:
		if err := d.deployToHost(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	case DEPLOY_TARGET_CERTIFICATE:
		if err := d.deployToCertificate(ctx, certPEM, privkeyPEM); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported deploy target '%s'", d.config.DeployTarget)
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func (d *deployment) deployToHost(ctx context.Context, certPEM, privkeyPEM string) error {
	certX509, err := xcert.ParseCertificateFromPEM(certPEM)
	if err != nil {
		return err
	}

	upres, err := d.sdkCertmgr.Upload(ctx, certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to upload certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate uploaded", slog.Any("result", upres))
	}

	hostsByType, err := d.getAllHosts(ctx, d.config.HostType)
	if err != nil {
		return err
	}

	var hostIds []int64
	switch d.config.HostMatchPattern {
	case "", HOST_MATCH_PATTERN_SPECIFIED:
		{
			if d.config.HostId == 0 {
				return fmt.Errorf("config `hostId` is required")
			}

			hostIds = []int64{d.config.HostId}
		}

	case HOST_MATCH_PATTERN_CERTSAN:
		{
			hostCandidates := hostsByType
			hostIds = lo.Map(
				lo.Filter(hostCandidates, func(hostItem *npmsdk.Host, _ int) bool {
					return len(hostItem.DomainNames) > 0 &&
						lo.EveryBy(hostItem.DomainNames, func(domain string) bool {
							return certX509.VerifyHostname(domain) == nil
						})
				}),
				func(hostItem *npmsdk.Host, _ int) int64 {
					return hostItem.Id
				},
			)
			if len(hostIds) == 0 {
				return fmt.Errorf("could not find any hosts matched by certificate")
			}
		}

	default:
		return fmt.Errorf("unsupported host match pattern: '%s'", d.config.HostMatchPattern)
	}

	if len(hostIds) == 0 {
		d.logger.Info("no hosts to deploy")
	} else {
		d.logger.Info("found hosts to deploy", slog.Any("hostIds", hostIds))

		hostIds = lo.Filter(hostIds, func(hostId int64, _ int) bool {
			hostInfo, _ := lo.Find(hostsByType, func(hostItem *npmsdk.Host) bool {
				return hostId == hostItem.Id
			})
			if hostInfo != nil {
				certId, _ := strconv.ParseInt(upres.CertId, 10, 64)
				return hostInfo.CertificateId != certId
			}

			return true
		})

		if err := xloop.ForRangeAllWithContext(ctx, hostIds, func(ctx context.Context, hostId int64, i int) error {
			if i > 0 {
				if err := xwait.DelayWithContext(ctx, 3*time.Second); err != nil {
					return err
				}
			}

			certId, _ := strconv.ParseInt(upres.CertId, 10, 64)
			return d.updateHostCertificate(ctx, d.config.HostType, hostId, certId)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (d *deployment) deployToCertificate(ctx context.Context, certPEM, privkeyPEM string) error {
	if d.config.CertificateId == 0 {
		return fmt.Errorf("config `certificateId` is required")
	}

	rplres, err := d.sdkCertmgr.Replace(ctx, strconv.FormatInt(d.config.CertificateId, 10), certPEM, privkeyPEM)
	if err != nil {
		return fmt.Errorf("failed to replace certificate file: %w", err)
	} else {
		d.logger.Info("ssl certificate replaced", slog.Any("result", rplres))
	}

	settingsGetDefaultSiteReq := &npmsdk.SettingsGetDefaultSiteRequest{}
	settingsGetDefaultSiteResp, err := d.sdkClient.SettingsGetDefaultSiteWithContext(ctx, settingsGetDefaultSiteReq)
	d.logger.Debug("sdk request 'settings.GetDefaultSite'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'settings.GetDefaultSite': %w", err)
	}

	settingsSetDefaultSiteReq := &npmsdk.SettingsSetDefaultSiteRequest{
		Value: settingsGetDefaultSiteResp.Value,
	}
	_, err = d.sdkClient.SettingsSetDefaultSiteWithContext(ctx, settingsSetDefaultSiteReq)
	d.logger.Debug("sdk request 'settings.SetDefaultSite'")
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'settings.SetDefaultSite': %w", err)
	}

	return nil
}

func (d *deployment) getAllHosts(ctx context.Context, cloudHostType string) ([]*npmsdk.Host, error) {
	var hosts []*npmsdk.Host
	switch cloudHostType {
	case HOST_TYPE_PROXY:
		{
			nginxListProxyHostsReq := &npmsdk.NginxListProxyHostsRequest{}
			nginxListProxyHostsResp, err := d.sdkClient.NginxListProxyHostsWithContext(ctx, nginxListProxyHostsReq)
			d.logger.Debug("sdk request 'nginx.ListProxyHosts'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'nginx.ListProxyHosts': %w", err)
			}

			hosts = make([]*npmsdk.Host, 0, len(*nginxListProxyHostsResp))
			for _, hostItem := range *nginxListProxyHostsResp {
				hosts = append(hosts, &hostItem.Host)
			}
		}

	case HOST_TYPE_REDIRECTION:
		{
			nginxListRedirectionHostsReq := &npmsdk.NginxListRedirectionHostsRequest{}
			nginxListRedirectionHostsResp, err := d.sdkClient.NginxListRedirectionHostsWithContext(ctx, nginxListRedirectionHostsReq)
			d.logger.Debug("sdk request 'nginx.ListRedirectionHosts'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'nginx.ListRedirectionHosts': %w", err)
			}

			hosts = make([]*npmsdk.Host, 0, len(*nginxListRedirectionHostsResp))
			for _, hostItem := range *nginxListRedirectionHostsResp {
				hosts = append(hosts, &hostItem.Host)
			}
		}

	case HOST_TYPE_STREAM:
		{
			nginxListStreamsReq := &npmsdk.NginxListStreamsRequest{}
			nginxListStreamsResp, err := d.sdkClient.NginxListStreamsWithContext(ctx, nginxListStreamsReq)
			d.logger.Debug("sdk request 'nginx.ListStreams'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'nginx.ListStreams': %w", err)
			}

			hosts = make([]*npmsdk.Host, 0, len(*nginxListStreamsResp))
			for _, hostItem := range *nginxListStreamsResp {
				hosts = append(hosts, &hostItem.Host)
			}
		}

	case HOST_TYPE_DEAD:
		{
			nginxListDeadHostsReq := &npmsdk.NginxListDeadHostsRequest{}
			nginxListDeadHostsResp, err := d.sdkClient.NginxListDeadHostsWithContext(ctx, nginxListDeadHostsReq)
			d.logger.Debug("sdk request 'nginx.ListDeadHosts'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'nginx.ListDeadHosts': %w", err)
			}

			hosts = make([]*npmsdk.Host, 0, len(*nginxListDeadHostsResp))
			for _, hostItem := range *nginxListDeadHostsResp {
				hosts = append(hosts, &hostItem.Host)
			}
		}

	default:
		return hosts, fmt.Errorf("unsupported host type: '%s'", cloudHostType)
	}

	return hosts, nil
}

func (d *deployment) updateHostCertificate(ctx context.Context, cloudHostType string, cloudHostId int64, cloudCertId int64) error {
	switch cloudHostType {
	case HOST_TYPE_PROXY:
		{
			nginxUpdateProxyHostReq := &npmsdk.NginxUpdateProxyHostRequest{
				CertificateId: lo.ToPtr(cloudCertId),
			}
			_, err := d.sdkClient.NginxUpdateProxyHostWithContext(ctx, cloudHostId, nginxUpdateProxyHostReq)
			d.logger.Debug("sdk request 'nginx.UpdateProxyHost'", slog.Int64("params.hostId", cloudHostId))
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'nginx.UpdateProxyHost': %w", err)
			}
		}

	case HOST_TYPE_REDIRECTION:
		{
			nginxUpdateRedirectionHostReq := &npmsdk.NginxUpdateRedirectionHostRequest{
				CertificateId: lo.ToPtr(cloudCertId),
			}
			_, err := d.sdkClient.NginxUpdateRedirectionHostWithContext(ctx, cloudHostId, nginxUpdateRedirectionHostReq)
			d.logger.Debug("sdk request 'nginx.UpdateRedirectionHost'", slog.Int64("params.hostId", cloudHostId))
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'nginx.UpdateRedirectionHost': %w", err)
			}
		}

	case HOST_TYPE_STREAM:
		{
			nginxUpdateStreamReq := &npmsdk.NginxUpdateStreamRequest{
				CertificateId: lo.ToPtr(cloudCertId),
			}
			_, err := d.sdkClient.NginxUpdateStreamWithContext(ctx, cloudHostId, nginxUpdateStreamReq)
			d.logger.Debug("sdk request 'nginx.UpdateStream'", slog.Int64("params.hostId", cloudHostId))
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'nginx.UpdateStream': %w", err)
			}
		}

	case HOST_TYPE_DEAD:
		{
			nginxUpdateDeadHostReq := &npmsdk.NginxUpdateDeadHostRequest{
				CertificateId: lo.ToPtr(cloudCertId),
			}
			_, err := d.sdkClient.NginxUpdateDeadHostWithContext(ctx, cloudHostId, nginxUpdateDeadHostReq)
			d.logger.Debug("sdk request 'nginx.UpdateDeadHost'", slog.Int64("params.hostId", cloudHostId))
			if err != nil {
				return fmt.Errorf("failed to execute sdk request 'nginx.UpdateDeadHost': %w", err)
			}
		}

	default:
		return fmt.Errorf("unsupported host type: '%s'", cloudHostType)
	}

	return nil
}

func createSDKClient(serverUrl, authMethod, username, password, apiToken string, skipTlsVerify bool) (*npmsdk.Client, error) {
	var client *npmsdk.Client
	var err error

	switch authMethod {
	case "", AUTH_METHOD_PASSWORD:
		client, err = npmsdk.NewClient(serverUrl, npmsdk.WithLogins(username, password))
	case AUTH_METHOD_TOKEN:
		client, err = npmsdk.NewClient(serverUrl, npmsdk.WithJwtToken(apiToken))
	default:
		err = fmt.Errorf("unsupported auth method '%s'", authMethod)
	}

	if err != nil {
		return nil, err
	}

	if skipTlsVerify {
		client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	return client, nil
}
