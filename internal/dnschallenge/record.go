package dnschallenge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/go-acme/lego/v5/challenge/dns01"
	"github.com/miekg/dns"

	"github.com/certimate-go/certimate/pkg/plugin"
)

type Record struct {
	Zone  string
	Name  string
	Value string
	TTL   uint32
}

func ForZone(ctx context.Context, zone string, req *plugin.DnsRequest) (Record, error) {
	zone = dns.CanonicalName(strings.TrimSpace(zone))
	if _, ok := dns.IsDomainName(zone); !ok || zone == "." {
		return Record{}, errors.New("a valid DNS zone is required")
	}
	var config struct {
		TTL uint32 `json:"ttl"`
	}
	if req.ExtendedConfigJSON != "" {
		if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &config); err != nil {
			return Record{}, fmt.Errorf("invalid DNS challenge configuration: %w", err)
		}
	}
	if config.TTL == 0 {
		config.TTL = 120
	}
	if config.TTL > 1<<31-1 {
		return Record{}, errors.New("TXT record TTL must be at most 2147483647 seconds")
	}
	info := dns01.GetChallengeInfo(ctx, strings.TrimPrefix(req.Domain, "*."), req.KeyAuth)
	name := dns.CanonicalName(info.EffectiveFQDN)
	if _, ok := dns.IsDomainName(name); !ok || !dns.IsSubDomain(zone, name) {
		return Record{}, fmt.Errorf("challenge name %q is outside DNS zone %q", name, zone)
	}
	return Record{Zone: zone, Name: name, Value: info.Value, TTL: config.TTL}, nil
}
