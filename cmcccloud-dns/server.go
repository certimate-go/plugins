package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/go-acme/lego/v5/challenge/dns01"
	"github.com/go-acme/lego/v5/platform/env"

	"github.com/certimate-go/certimate/pkg/plugin"
)

type server struct {
	mu        sync.Mutex
	recordIDs map[string]string
}

type challengeConfig struct {
	RecordName  string   `json:"recordName"`
	RecordValue string   `json:"recordValue"`
	Nameservers []string `json:"nameservers"`
	TTL         int32    `json:"ttl"`
}

type createRecord struct {
	LineID      string `json:"lineId"`
	Name        string `json:"rr"`
	Zone        string `json:"domainName"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Value       string `json:"value"`
	TTL         int32  `json:"ttl"`
}

type deleteRecords struct {
	IDs []string `json:"recordIdList"`
}

func (s *server) Present(ctx context.Context, req *plugin.DnsRequest) error {
	var cfg challengeConfig
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &cfg); err != nil {
		return fmt.Errorf("invalid CMCC Cloud DNS challenge configuration: %w", err)
	}
	if cfg.RecordName == "" || cfg.RecordValue == "" {
		return errors.New("CMCC Cloud DNS requires challenge record information from Certimate")
	}
	if cfg.TTL == 0 {
		cfg.TTL = int32(env.GetOrDefaultInt("CMCCCLOUD_TTL", 600))
	}

	client := dns01.NewClient(&dns01.Options{RecursiveNameservers: cfg.Nameservers})
	zone, err := client.FindZoneByFqdn(ctx, cfg.RecordName)
	if err != nil {
		return fmt.Errorf("find CMCC Cloud DNS zone for %q: %w", cfg.RecordName, err)
	}
	name, err := dns01.ExtractSubDomain(cfg.RecordName, zone)
	if err != nil {
		return err
	}

	body, err := callDNS(ctx, req.AccessConfigJSON, "createRecordOpenapi", createRecord{
		LineID:      "0",
		Name:        name,
		Zone:        dns01.UnFqdn(zone),
		Description: "certimate acme",
		Type:        "TXT",
		Value:       cfg.RecordValue,
		TTL:         cfg.TTL,
	})
	if err != nil {
		return err
	}
	var record struct {
		ID string `json:"recordId"`
	}
	if err := json.Unmarshal(body, &record); err != nil || record.ID == "" {
		return errors.New("CMCC Cloud DNS did not return a record ID")
	}

	s.mu.Lock()
	s.recordIDs[req.Token] = record.ID
	s.mu.Unlock()
	return nil
}

func (s *server) CleanUp(ctx context.Context, req *plugin.DnsRequest) error {
	s.mu.Lock()
	recordID := s.recordIDs[req.Token]
	s.mu.Unlock()
	if recordID == "" {
		return nil
	}
	if _, err := callDNS(ctx, req.AccessConfigJSON, "deleteRecordOpenapi", deleteRecords{IDs: []string{recordID}}); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.recordIDs, req.Token)
	s.mu.Unlock()
	return nil
}
