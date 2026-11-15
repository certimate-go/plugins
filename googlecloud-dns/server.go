package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"

	"github.com/go-acme/lego/v5/challenge/dns01"
	"github.com/go-acme/lego/v5/providers/dns/gcloud"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/dns/v1"

	"github.com/certimate-go/certimate/pkg/plugin"
	xgcp "github.com/certimate-go/certimate/pkg/utils/third-party/gcp"
)

type server struct {
	initOnce sync.Once
	provider *gcloud.DNSProvider
	initErr  error
}

type accessConfig struct {
	ProjectID         string `json:"projectId"`
	ServiceAccountKey string `json:"serviceAccountKey"`
}

type challengeConfig struct {
	Nameservers        []string `json:"nameservers"`
	TTL                int      `json:"ttl"`
	DisableFollowCNAME bool     `json:"disableFollowCNAME"`
}

func (s *server) Process(ctx context.Context, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error) {
	provider, err := s.dnsProvider(req)
	if err != nil {
		return nil, err
	}
	if err := provider.Present(ctx, req.Dns.Domain, req.Dns.Token, req.Dns.KeyAuth); err != nil {
		return nil, err
	}
	return &plugin.Result{Dns: &plugin.DnsResult{}}, nil
}

func (s *server) PostProcess(ctx context.Context, req *plugin.Request, logger *slog.Logger) error {
	provider, err := s.dnsProvider(req)
	if err != nil {
		return err
	}
	return provider.CleanUp(ctx, req.Dns.Domain, req.Dns.Token, req.Dns.KeyAuth)
}

func (s *server) dnsProvider(req *plugin.Request) (*gcloud.DNSProvider, error) {
	s.initOnce.Do(func() {
		s.provider, s.initErr = newDNSProvider(req)
	})
	return s.provider, s.initErr
}

func newDNSProvider(req *plugin.Request) (*gcloud.DNSProvider, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid Google Cloud access configuration: %w", err)
	}
	var challenge challengeConfig
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &challenge); err != nil {
		return nil, fmt.Errorf("invalid Google Cloud DNS challenge configuration: %w", err)
	}

	projectID, err := xgcp.GetProjectIDFromServiceAccountKey(access.ServiceAccountKey)
	if err != nil {
		return nil, err
	}
	if projectID != access.ProjectID {
		return nil, fmt.Errorf("googlecloud: invalid project ID: expected '%s', got '%s'", projectID, access.ProjectID)
	}

	account, err := google.JWTConfigFromJSON([]byte(access.ServiceAccountKey), dns.NdevClouddnsReadwriteScope)
	if err != nil {
		return nil, fmt.Errorf("googlecloud: unable to acquire service account config: %w", err)
	}
	config := gcloud.NewDefaultConfig()
	config.Project = projectID
	config.HTTPClient = account.Client(context.Background())
	if challenge.TTL != 0 {
		config.TTL = challenge.TTL
	}
	provider, err := gcloud.NewDNSProviderConfig(config)
	if err != nil {
		return nil, err
	}
	if err := os.Setenv("LEGO_DISABLE_CNAME_SUPPORT", strconv.FormatBool(challenge.DisableFollowCNAME)); err != nil {
		return nil, err
	}
	dns01.SetDefaultClient(dns01.NewClient(&dns01.Options{RecursiveNameservers: challenge.Nameservers}))
	return provider, nil
}
