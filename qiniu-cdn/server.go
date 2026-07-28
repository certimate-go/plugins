package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	qiniusslcert "github.com/certimate-go/plugins/internal/qiniusslcert"
	qiniusdk "github.com/certimate-go/plugins/internal/qiniusdk"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "qiniu-cdn"
	accessType     = "qiniu"
	displayNameKey = "plugin.qiniu-cdn.name"

	domainMatchPatternExact    = "exact"
	domainMatchPatternWildcard = "wildcard"
	domainMatchPatternCertsan  = "certsan"
)

type qiniuCdnDeployer struct{}

func (*qiniuCdnDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*qiniuCdnDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	accessSchema, _ := plugin.LoadAccessSchema(schemaFS)
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{
		AccessSchemaJSON: accessSchema,
		DeploySchemaJSON: deploySchema,
		I18n:             i18n,
	}, nil
}

type accessConfig struct {
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

type extendedConfig struct {
	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`
	Domain             string `json:"domain"`
}

func (d *qiniuCdnDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}

	var extended extendedConfig
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}

	mac := auth.New(access.AccessKey, access.SecretKey)
	certmgr, err := qiniusslcert.NewCertmgr(&qiniusslcert.CertmgrConfig{
		AccessKey: access.AccessKey,
		SecretKey: access.SecretKey,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create certmgr: %w", err)
	}
	certmgr.SetLogger(logger)

	upres, err := certmgr.Upload(ctx, req.CertificatePEM, req.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to upload certificate file: %w", err)
	}
	logger.Info("ssl certificate uploaded", slog.Any("result", upres))

	cdnManager := qiniusdk.NewCdnManager(mac)

	var domains []string
	switch extended.DomainMatchPattern {
	case "", domainMatchPatternExact:
		domain := normalizeDomain(extended.Domain)
		if domain == "" {
			return nil, fmt.Errorf("config `domain` is required")
		}
		domains = []string{domain}

	case domainMatchPatternWildcard:
		if extended.Domain == "" {
			return nil, fmt.Errorf("config `domain` is required")
		}
		if strings.HasPrefix(extended.Domain, "*.") {
			domainCandidates, err := getAllCdnDomains(ctx, cdnManager, logger)
			if err != nil {
				return nil, err
			}
			domains = lo.Filter(domainCandidates, func(domain string, _ int) bool {
				return xcerthostname.IsMatch(extended.Domain, domain)
			})
			if len(domains) == 0 {
				return nil, fmt.Errorf("could not find any domains matched by wildcard")
			}
		} else {
			domains = []string{extended.Domain}
		}

	case domainMatchPatternCertsan:
		domainCandidates, err := getAllCdnDomains(ctx, cdnManager, logger)
		if err != nil {
			return nil, err
		}
		domains = lo.Filter(domainCandidates, func(domain string, _ int) bool {
			return xcerthostname.IsMatchByCertificatePEM(req.CertificatePEM, domain)
		})
		if len(domains) == 0 {
			return nil, fmt.Errorf("could not find any domains matched by certificate")
		}

	default:
		return nil, fmt.Errorf("unsupported domain match pattern: '%s'", extended.DomainMatchPattern)
	}

	if len(domains) == 0 {
		logger.Info("no cdn domains to deploy")
	} else {
		logger.Info("found cdn domains to deploy", slog.Any("domains", domains))
		if err := xloop.ForRangeAllWithContext(ctx, domains, func(ctx context.Context, domain string, _ int) error {
			return updateCdnDomainCertificate(ctx, cdnManager, logger, domain, upres.CertId)
		}); err != nil {
			return nil, err
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func getAllCdnDomains(ctx context.Context, m *qiniusdk.CdnManager, logger *slog.Logger) ([]string, error) {
	domains := make([]string, 0)
	marker := ""
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := m.GetDomainList(ctx, marker, 100)
		logger.Debug("sdk request 'cdn.GetDomainList'", slog.String("params.marker", marker), slog.Any("response", resp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdn.GetDomainList': %w", err)
		}

		ignoredStatuses := []string{"frozen", "offlined"}
		for _, item := range resp.Domains {
			if lo.Contains(ignoredStatuses, item.OperatingState) {
				continue
			}
			domains = append(domains, item.Name)
		}

		if len(resp.Domains) == 0 || resp.Marker == "" {
			break
		}
		marker = resp.Marker
	}
	return domains, nil
}

func updateCdnDomainCertificate(ctx context.Context, m *qiniusdk.CdnManager, logger *slog.Logger, domain string, cloudCertId string) error {
	info, err := m.GetDomainInfo(ctx, domain)
	logger.Debug("sdk request 'cdn.GetDomainInfo'", slog.String("params.domain", domain), slog.Any("response", info))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'cdn.GetDomainInfo': %w", err)
	}

	if info.Https == nil || info.Https.CertID == "" {
		enableResp, err := m.EnableDomainHttps(ctx, domain, cloudCertId, true, true)
		logger.Debug("sdk request 'cdn.EnableDomainHttps'", slog.String("params.domain", domain), slog.String("params.certId", cloudCertId), slog.Any("response", enableResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'cdn.EnableDomainHttps': %w", err)
		}
	} else if info.Https.CertID != cloudCertId {
		modifyResp, err := m.ModifyDomainHttpsConf(ctx, domain, cloudCertId, info.Https.ForceHttps, info.Https.Http2Enable)
		logger.Debug("sdk request 'cdn.ModifyDomainHttpsConf'", slog.String("params.domain", domain), slog.String("params.certId", cloudCertId), slog.Any("response", modifyResp))
		if err != nil {
			return fmt.Errorf("failed to execute sdk request 'cdn.ModifyDomainHttpsConf': %w", err)
		}
	}
	return nil
}

func normalizeDomain(domain string) string {
	if strings.HasPrefix(domain, "*.") {
		return strings.TrimPrefix(domain, "*")
	}
	return domain
}
