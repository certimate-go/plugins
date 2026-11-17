package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-acme/lego/v5/providers/dns/pdns"

	"github.com/certimate-go/certimate/pkg/plugin"
	xhttp "github.com/certimate-go/certimate/pkg/utils/http"
	"github.com/certimate-go/plugins/internal/dnschallenge"
	"github.com/certimate-go/plugins/internal/pluginhttp"
)

type server struct {
	initOnce sync.Once
	initErr  error
	access   accessConfig
	legacy   *pdns.DNSProvider
	legacyMu sync.Mutex
}

type accessConfig struct {
	ServerURL                string `json:"serverUrl"`
	Zone                     string `json:"zone"`
	APIKey                   string `json:"apiKey"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections"`
}

type txtRecord struct {
	Content  string `json:"content"`
	Disabled bool   `json:"disabled"`
}

type rrsetUpdate struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	ChangeType string      `json:"changetype"`
	TTL        uint32      `json:"ttl"`
	Records    []txtRecord `json:"records"`
}

func (s *server) Process(ctx context.Context, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error) {
	if err := s.setup(req); err != nil {
		return nil, err
	}
	var err error
	if s.legacy != nil {
		s.legacyMu.Lock()
		defer s.legacyMu.Unlock()
		err = s.legacy.Present(ctx, req.Dns.Domain, req.Dns.Token, req.Dns.KeyAuth)
	} else {
		err = s.update(ctx, req, "EXTEND")
	}
	if err != nil {
		return nil, err
	}
	return &plugin.Result{Dns: &plugin.DnsResult{}}, nil
}

func (s *server) PostProcess(ctx context.Context, req *plugin.Request, logger *slog.Logger) error {
	if err := s.setup(req); err != nil {
		return err
	}
	if s.legacy != nil {
		s.legacyMu.Lock()
		defer s.legacyMu.Unlock()
		return s.legacy.CleanUp(ctx, req.Dns.Domain, req.Dns.Token, req.Dns.KeyAuth)
	}
	return s.update(ctx, req, "PRUNE")
}

func (s *server) setup(req *plugin.Request) error {
	s.initOnce.Do(func() {
		s.initErr = s.configure(req)
	})
	return s.initErr
}

func (s *server) configure(req *plugin.Request) error {
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &s.access); err != nil {
		return fmt.Errorf("invalid PowerDNS access configuration: %w", err)
	}
	ttl, err := dnschallenge.ConfigureResolver(req)
	if err != nil || s.access.Zone != "" {
		return err
	}
	endpoint, err := pluginhttp.ParseServerURL(s.access.ServerURL)
	if err != nil {
		return err
	}
	config := pdns.NewDefaultConfig()
	config.Host = endpoint
	config.APIKey = s.access.APIKey
	if ttl != 0 {
		config.TTL = ttl
	}
	if s.access.AllowInsecureConnections {
		transport := xhttp.NewDefaultTransport()
		transport.DisableKeepAlives = true
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		config.HTTPClient.Transport = transport
	}
	s.legacy, err = pdns.NewDNSProviderConfig(config)
	return err
}

func (s *server) update(ctx context.Context, req *plugin.Request, change string) error {
	access := s.access
	if access.APIKey == "" {
		return errors.New("PowerDNS API key is required")
	}
	endpoint, err := pluginhttp.ParseServerURL(access.ServerURL)
	if err != nil {
		return err
	}
	record, err := dnschallenge.ForZone(ctx, access.Zone, req)
	if err != nil {
		return err
	}
	rrset := rrsetUpdate{
		Name: record.Name, Type: "TXT", ChangeType: change, TTL: record.TTL,
		Records: []txtRecord{{Content: strconv.Quote(record.Value)}},
	}
	payload := struct {
		RRSets []rrsetUpdate `json:"rrsets"`
	}{RRSets: []rrsetUpdate{rrset}}
	endpoint = endpoint.JoinPath("api", "v1", "servers", "localhost", "zones", record.Zone)
	return pluginhttp.SendJSON(ctx, http.MethodPatch, endpoint, "X-API-Key", access.APIKey, payload)
}
