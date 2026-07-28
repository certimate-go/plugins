package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/qiniu/go-sdk/v7/pili"
	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcerthostname "github.com/certimate-go/certimate/pkg/utils/cert/hostname"
	xloop "github.com/certimate-go/certimate/pkg/utils/loop"
	qiniusslcert "github.com/certimate-go/plugins/internal/qiniusslcert"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "qiniu-pili"
	accessType     = "qiniu"
	displayNameKey = "plugin.qiniu-pili.name"

	domainMatchPatternExact   = "exact"
	domainMatchPatternCertsan = "certsan"
)

type qiniuPiliDeployer struct{}

func (*qiniuPiliDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "av",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*qiniuPiliDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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
	Hub                string `json:"hub"`
	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`
	Domain             string `json:"domain"`
}

func (d *qiniuPiliDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
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
	if extended.Domain == "" {
		return nil, fmt.Errorf("config `domain` is required")
	}

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

	manager := pili.NewManager(pili.ManagerConfig{AccessKey: access.AccessKey, SecretKey: access.SecretKey})

	var domains []string
	switch extended.DomainMatchPattern {
	case "", domainMatchPatternExact:
		domains = []string{extended.Domain}

	case domainMatchPatternCertsan:
		domainCandidates, err := getAllPiliDomainsByHub(ctx, manager, logger, extended.Hub)
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
		logger.Info("no pili domains to deploy")
	} else {
		logger.Info("found pili domains to deploy", slog.Any("domains", domains))
		if err := xloop.ForRangeAllWithContext(ctx, domains, func(ctx context.Context, domain string, _ int) error {
			return updatePiliDomainCertificate(ctx, manager, logger, extended.Hub, domain, upres.CertName)
		}); err != nil {
			return nil, err
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func getAllPiliDomainsByHub(ctx context.Context, m *pili.Manager, logger *slog.Logger, hub string) ([]string, error) {
	domains := make([]string, 0)
	listReq := pili.GetDomainsListRequest{Hub: hub}
	listResp, err := m.GetDomainsList(ctx, listReq)
	logger.Debug("sdk request 'pili.GetDomainsList'", slog.Any("request", listReq), slog.Any("response", listResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'pili.GetDomainsList': %w", err)
	}
	for _, item := range listResp.Domains {
		domains = append(domains, item.Domain)
	}
	return domains, nil
}

func updatePiliDomainCertificate(ctx context.Context, m *pili.Manager, logger *slog.Logger, hub string, domain string, cloudCertName string) error {
	setReq := pili.SetDomainCertRequest{
		Hub:      hub,
		Domain:   domain,
		CertName: cloudCertName,
	}
	err := m.SetDomainCert(ctx, setReq)
	logger.Debug("sdk request 'pili.SetDomainCert'", slog.Any("request", setReq))
	if err != nil {
		return fmt.Errorf("failed to execute sdk request 'pili.SetDomainCert': %w", err)
	}
	return nil
}
