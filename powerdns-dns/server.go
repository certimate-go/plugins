package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/dnschallenge"
	"github.com/certimate-go/plugins/internal/pluginhttp"
)

type accessConfig struct {
	ServerURL string `json:"serverUrl"`
	Zone      string `json:"zone"`
	APIKey    string `json:"apiKey"`
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

func (*server) Process(ctx context.Context, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error) {
	if err := update(ctx, req, "EXTEND"); err != nil {
		return nil, err
	}
	return &plugin.Result{Dns: &plugin.DnsResult{}}, nil
}

func (*server) PostProcess(ctx context.Context, req *plugin.Request, logger *slog.Logger) error {
	return update(ctx, req, "PRUNE")
}

func update(ctx context.Context, req *plugin.Request, change string) error {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return fmt.Errorf("invalid PowerDNS access configuration: %w", err)
	}
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
