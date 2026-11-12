package onepanelssl

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/certimate-go/certimate/pkg/core"
	onepanelsdk "github.com/certimate-go/plugins/internal/onepanelsdk"
	onepanelsdk2 "github.com/certimate-go/plugins/internal/onepanelsdk/v2"
)

type (
	Provider      = core.Certmgr
	UploadResult  = core.CertmgrUploadResult
	ReplaceResult = core.CertmgrReplaceResult
)

type CertmgrConfig struct {
	ServerUrl string `json:"serverUrl"`

	ApiVersion string `json:"apiVersion"`

	ApiKey string `json:"apiKey"`

	AllowInsecureConnections bool `json:"allowInsecureConnections,omitempty"`

	NodeName string `json:"nodeName,omitempty"`
}

type Certmgr struct {
	config    *CertmgrConfig
	logger    *slog.Logger
	sdkClient any
}

var _ Provider = (*Certmgr)(nil)

func NewCertmgr(config *CertmgrConfig) (*Certmgr, error) {
	if config == nil {
		return nil, fmt.Errorf("the configuration of the certmgr provider is nil")
	}

	client, err := createSDKClient(config.ServerUrl, config.ApiVersion, config.ApiKey, config.AllowInsecureConnections, config.NodeName)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	return &Certmgr{
		config:    config,
		logger:    slog.Default(),
		sdkClient: client,
	}, nil
}

func (c *Certmgr) SetLogger(logger *slog.Logger) {
	if logger == nil {
		c.logger = slog.New(slog.DiscardHandler)
	} else {
		c.logger = logger
	}
}

func (c *Certmgr) Upload(ctx context.Context, certPEM, privkeyPEM string) (*UploadResult, error) {
	if upres, upok, err := c.tryGetResultIfCertExists(ctx, certPEM, privkeyPEM); err != nil {
		return nil, err
	} else if upok {
		c.logger.Info("ssl certificate already exists")
		return upres, nil
	}

	certName := fmt.Sprintf("certimate-%d", time.Now().UnixMilli())

	switch sdkClient := c.sdkClient.(type) {
	case *onepanelsdk.Client:
		{
			websiteSSLUploadReq := &onepanelsdk.WebsiteSSLUploadRequest{
				Type:        "paste",
				Description: certName,
				Certificate: certPEM,
				PrivateKey:  privkeyPEM,
			}
			_, err := sdkClient.WebsiteSSLUploadWithContext(ctx, websiteSSLUploadReq)
			c.logger.Debug("sdk request 'WebsiteSSLUpload'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'WebsiteSSLUpload': %w", err)
			}
		}

	case *onepanelsdk2.Client:
		{
			websiteSSLUploadReq := &onepanelsdk2.WebsiteSSLUploadRequest{
				Type:        "paste",
				Description: certName,
				Certificate: certPEM,
				PrivateKey:  privkeyPEM,
			}
			_, err := sdkClient.WebsiteSSLUploadWithContext(ctx, websiteSSLUploadReq)
			c.logger.Debug("sdk request 'WebsiteSSLUpload'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'WebsiteSSLUpload': %w", err)
			}
		}

	default:
		panic("unreachable")
	}

	if upres, upok, err := c.tryGetResultIfCertExists(ctx, certPEM, privkeyPEM); err != nil {
		return nil, err
	} else if !upok {
		return nil, fmt.Errorf("could not find ssl certificate, may be upload failed")
	} else {
		return upres, nil
	}
}

func (c *Certmgr) Replace(ctx context.Context, certIdOrName string, certPEM, privkeyPEM string) (*ReplaceResult, error) {
	sslId, err := strconv.ParseInt(certIdOrName, 10, 64)
	if err != nil {
		return nil, err
	}

	switch sdkClient := c.sdkClient.(type) {
	case *onepanelsdk.Client:
		{

			websiteSSLUploadReq := &onepanelsdk.WebsiteSSLUploadRequest{
				SSLID:       sslId,
				Type:        "paste",
				Description: "upload from Certimate",
				Certificate: certPEM,
				PrivateKey:  privkeyPEM,
			}
			_, err := sdkClient.WebsiteSSLUploadWithContext(ctx, websiteSSLUploadReq)
			c.logger.Debug("sdk request 'WebsiteSSLUpload'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'WebsiteSSLUpload': %w", err)
			}
		}

	case *onepanelsdk2.Client:
		{

			websiteSSLUploadReq := &onepanelsdk2.WebsiteSSLUploadRequest{
				SSLID:       sslId,
				Type:        "paste",
				Description: "upload from Certimate",
				Certificate: certPEM,
				PrivateKey:  privkeyPEM,
			}
			_, err := sdkClient.WebsiteSSLUploadWithContext(ctx, websiteSSLUploadReq)
			c.logger.Debug("sdk request 'WebsiteSSLUpload'")
			if err != nil {
				return nil, fmt.Errorf("failed to execute sdk request 'WebsiteSSLUpload': %w", err)
			}
		}

	default:
		panic("unreachable")
	}

	return &ReplaceResult{}, nil
}

func (c *Certmgr) tryGetResultIfCertExists(ctx context.Context, certPEM, privkeyPEM string) (*UploadResult, bool, error) {
	switch sdkClient := c.sdkClient.(type) {
	case *onepanelsdk.Client:
		{
			searchWebsiteSSLPage := 1
			searchWebsiteSSLPageSize := 100
			for {
				select {
				case <-ctx.Done():
					return nil, false, ctx.Err()
				default:
				}

				websiteSSLSearchReq := &onepanelsdk.WebsiteSSLSearchRequest{
					Page:     int32(searchWebsiteSSLPage),
					PageSize: int32(searchWebsiteSSLPageSize),
				}
				websiteSSLSearchResp, err := sdkClient.WebsiteSSLSearchWithContext(ctx, websiteSSLSearchReq)
				c.logger.Debug("sdk request 'WebsiteSSLSearch'")
				if err != nil {
					return nil, false, fmt.Errorf("failed to execute sdk request 'WebsiteSSLSearch': %w", err)
				}

				if websiteSSLSearchResp.Data == nil {
					break
				}

				for _, sslItem := range websiteSSLSearchResp.Data.Items {
					oldCertPEM := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(sslItem.PEM, "\r", ""), "\n", ""))
					oldPrivkeyPEM := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(sslItem.PrivateKey, "\r", ""), "\n", ""))
					newCertPEM := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(certPEM, "\r", ""), "\n", ""))
					newPrivkeyPEM := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(privkeyPEM, "\r", ""), "\n", ""))
					if oldCertPEM == newCertPEM && oldPrivkeyPEM == newPrivkeyPEM {
						return &UploadResult{
							CertId:   fmt.Sprintf("%d", sslItem.ID),
							CertName: sslItem.Description,
						}, true, nil
					}
				}

				if len(websiteSSLSearchResp.Data.Items) < searchWebsiteSSLPageSize ||
					searchWebsiteSSLPage*searchWebsiteSSLPageSize >= int(websiteSSLSearchResp.Data.Total) {
					break
				}

				searchWebsiteSSLPage++
			}
		}

	case *onepanelsdk2.Client:
		{
			searchWebsiteSSLPage := 1
			searchWebsiteSSLPageSize := 100
			for {
				select {
				case <-ctx.Done():
					return nil, false, ctx.Err()
				default:
				}

				websiteSSLSearchReq := &onepanelsdk2.WebsiteSSLSearchRequest{
					Order:    "null",
					OrderBy:  "expire_date",
					Page:     int32(searchWebsiteSSLPage),
					PageSize: int32(searchWebsiteSSLPageSize),
				}
				websiteSSLSearchResp, err := sdkClient.WebsiteSSLSearchWithContext(ctx, websiteSSLSearchReq)
				c.logger.Debug("sdk request 'WebsiteSSLSearch'")
				if err != nil {
					return nil, false, fmt.Errorf("failed to execute sdk request 'WebsiteSSLSearch': %w", err)
				}

				if websiteSSLSearchResp.Data == nil {
					break
				}

				for _, sslItem := range websiteSSLSearchResp.Data.Items {
					oldCertPEM := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(sslItem.PEM, "\r", ""), "\n", ""))
					oldPrivkeyPEM := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(sslItem.PrivateKey, "\r", ""), "\n", ""))
					newCertPEM := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(certPEM, "\r", ""), "\n", ""))
					newPrivkeyPEM := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(privkeyPEM, "\r", ""), "\n", ""))
					if oldCertPEM == newCertPEM && oldPrivkeyPEM == newPrivkeyPEM {
						return &UploadResult{
							CertId:   fmt.Sprintf("%d", sslItem.ID),
							CertName: sslItem.Description,
						}, true, nil
					}
				}

				if len(websiteSSLSearchResp.Data.Items) < searchWebsiteSSLPageSize ||
					searchWebsiteSSLPage*searchWebsiteSSLPageSize >= int(websiteSSLSearchResp.Data.Total) {
					break
				}

				searchWebsiteSSLPage++
			}
		}

	default:
		panic("unreachable")
	}

	return nil, false, nil
}

const (
	sdkVersionV1 = "v1"
	sdkVersionV2 = "v2"
)

func createSDKClient(serverUrl, apiVersion, apiKey string, skipTlsVerify bool, nodeName string) (any, error) {
	switch apiVersion {
	case sdkVersionV1:
		{
			client, err := onepanelsdk.NewClient(
				serverUrl,
				onepanelsdk.WithApiKey(apiKey),
			)
			if err != nil {
				return nil, err
			}

			if skipTlsVerify {
				client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
			}

			return client, nil
		}

	case sdkVersionV2:
		{
			client, err := onepanelsdk2.NewClient(
				serverUrl,
				onepanelsdk2.WithApiKey(apiKey),
				onepanelsdk2.WithNode(nodeName),
			)
			if err != nil {
				return nil, err
			}

			if skipTlsVerify {
				client.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
			}

			return client, nil
		}
	}

	return nil, fmt.Errorf("1panel: invalid api version")
}
