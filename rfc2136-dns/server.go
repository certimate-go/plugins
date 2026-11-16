package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bodgit/tsig"
	"github.com/bodgit/tsig/gss"
	"github.com/go-acme/lego/v5/challenge/dns01"
	"github.com/go-acme/lego/v5/providers/dns/dnsupdate"
	"github.com/miekg/dns"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/dnschallenge"
)

type server struct {
	initOnce sync.Once
	initErr  error
	access   accessConfig
	timeout  time.Duration
	ttl      int
}

type accessConfig struct {
	Server          string `json:"server"`
	Zone            string `json:"zone"`
	KeyName         string `json:"keyName"`
	KeySecret       string `json:"keySecret"`
	Algorithm       string `json:"algorithm"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	TSIGAlgorithm   string `json:"tsigAlgorithm"`
	TSIGKey         string `json:"tsigKey"`
	TSIGSecret      string `json:"tsigSecret"`
	TSIGGSSRealm    string `json:"tsigGssRealm"`
	TSIGGSSUsername string `json:"tsigGssUsername"`
	TSIGGSSPassword string `json:"tsigGssPassword"`
}

func (s *server) Process(ctx context.Context, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error) {
	if err := s.update(ctx, req, (*dns.Msg).Insert); err != nil {
		return nil, err
	}
	return &plugin.Result{Dns: &plugin.DnsResult{}}, nil
}

func (s *server) PostProcess(ctx context.Context, req *plugin.Request, logger *slog.Logger) error {
	return s.update(ctx, req, (*dns.Msg).Remove)
}

func (s *server) configure(req *plugin.Request) error {
	access := &s.access
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), access); err != nil {
		return fmt.Errorf("invalid RFC 2136 access configuration: %w", err)
	}
	ttl, err := dnschallenge.ConfigureResolver(req)
	if err != nil {
		return err
	}
	s.timeout = 10 * time.Second
	if access.Host != "" {
		if access.Port == 0 {
			access.Port = 53
		}
		access.Server = net.JoinHostPort(access.Host, strconv.Itoa(access.Port))
		access.KeyName = access.TSIGKey
		access.KeySecret = access.TSIGSecret
		access.Algorithm = access.TSIGAlgorithm
		if access.Algorithm == "" {
			access.Algorithm = dns.HmacSHA1
		}
		defaults := dnsupdate.NewDefaultConfig()
		s.timeout = defaults.DNSTimeout
		s.ttl = ttl
		if s.ttl == 0 {
			s.ttl = defaults.TTL
		}
	}
	host, port, err := net.SplitHostPort(access.Server)
	if err != nil || host == "" || port == "" {
		return errors.New("primary DNS server must be in host:port format")
	}
	access.Algorithm = dns.CanonicalName(strings.TrimSpace(access.Algorithm))
	switch access.Algorithm {
	case ".":
		access.Algorithm = dns.HmacSHA256
	case tsig.GSS:
		if access.Host == "" || access.TSIGGSSUsername == "" || access.TSIGGSSPassword == "" {
			return errors.New("GSS-TSIG requires a username and password")
		}
		return nil
	case dns.HmacSHA1, dns.HmacSHA224, dns.HmacSHA256, dns.HmacSHA384, dns.HmacSHA512:
	default:
		return errors.New("unsupported TSIG algorithm")
	}
	if (access.KeyName == "") != (access.KeySecret == "") {
		return errors.New("TSIG key name and secret must be provided together")
	}
	if access.KeyName != "" {
		access.KeyName = dns.CanonicalName(strings.TrimSpace(access.KeyName))
		if _, ok := dns.IsDomainName(access.KeyName); !ok || access.KeyName == "." {
			return errors.New("a valid TSIG key name is required")
		}
		if secret, err := base64.StdEncoding.DecodeString(access.KeySecret); err != nil || len(secret) == 0 {
			return errors.New("TSIG secret must be a nonempty Base64 value")
		}
	}
	return nil
}

func (s *server) update(ctx context.Context, req *plugin.Request, change func(*dns.Msg, []dns.RR)) error {
	s.initOnce.Do(func() {
		s.initErr = s.configure(req)
	})
	if s.initErr != nil {
		return s.initErr
	}
	record, err := s.challengeRecord(ctx, req)
	if err != nil {
		return err
	}
	message := new(dns.Msg).SetUpdate(record.Zone)
	change(message, []dns.RR{&dns.TXT{
		Hdr: dns.RR_Header{Name: record.Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: record.TTL},
		Txt: []string{record.Value},
	}})
	return s.sendUpdate(ctx, message)
}

func (s *server) challengeRecord(ctx context.Context, req *plugin.Request) (dnschallenge.Record, error) {
	if s.access.Host == "" {
		return dnschallenge.ForZone(ctx, s.access.Zone, req)
	}
	info := dns01.GetChallengeInfo(ctx, strings.TrimPrefix(req.Dns.Domain, "*."), req.Dns.KeyAuth)
	zone, err := dns01.DefaultClient().FindZoneByFqdnCustom(ctx, info.EffectiveFQDN, []string{s.access.Server})
	if err != nil {
		return dnschallenge.Record{}, fmt.Errorf("find RFC 2136 DNS zone: %w", err)
	}
	return dnschallenge.Record{Zone: zone, Name: info.EffectiveFQDN, Value: info.Value, TTL: uint32(s.ttl)}, nil
}

func (s *server) sendUpdate(ctx context.Context, message *dns.Msg) error {
	access := s.access
	client := &dns.Client{Net: "tcp", Timeout: s.timeout}
	if access.Host != "" && access.Algorithm != tsig.GSS {
		client.Net = "udp"
	}
	switch {
	case access.Algorithm == tsig.GSS:
		gssClient, err := gss.NewClient(client)
		if err != nil {
			return fmt.Errorf("create GSS client: %w", err)
		}
		defer gssClient.Close()
		keyName, _, err := gssClient.NegotiateContextWithCredentials(access.Server, access.TSIGGSSRealm, access.TSIGGSSUsername, access.TSIGGSSPassword)
		if err != nil {
			return fmt.Errorf("negotiate GSS-TSIG context: %w", err)
		}
		defer gssClient.DeleteContext(keyName)
		client.TsigProvider = gssClient
		message.SetTsig(keyName, tsig.GSS, 300, time.Now().Unix())
	case access.KeyName != "":
		message.SetTsig(access.KeyName, access.Algorithm, 300, time.Now().Unix())
		client.TsigSecret = map[string]string{access.KeyName: access.KeySecret}
	}
	reply, _, err := client.ExchangeContext(ctx, message, access.Server)
	if err != nil {
		return fmt.Errorf("DNS update failed: %w", err)
	}
	if reply.Rcode != dns.RcodeSuccess {
		return fmt.Errorf("DNS update rejected with response code %d", reply.Rcode)
	}
	if message.IsTsig() != nil && reply.IsTsig() == nil {
		return errors.New("DNS update reply is missing TSIG authentication")
	}
	return nil
}
