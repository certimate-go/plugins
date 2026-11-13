package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"

	"github.com/certimate-go/certimate/pkg/plugin"
	"github.com/certimate-go/plugins/internal/dnschallenge"
)

type accessConfig struct {
	Server    string `json:"server"`
	Zone      string `json:"zone"`
	KeyName   string `json:"keyName"`
	KeySecret string `json:"keySecret"`
	Algorithm string `json:"algorithm"`
}

func (*server) Present(ctx context.Context, req *plugin.DnsRequest) error {
	return update(ctx, req, (*dns.Msg).Insert)
}

func (*server) CleanUp(ctx context.Context, req *plugin.DnsRequest) error {
	return update(ctx, req, (*dns.Msg).Remove)
}

func update(ctx context.Context, req *plugin.DnsRequest, change func(*dns.Msg, []dns.RR)) error {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return fmt.Errorf("invalid RFC 2136 access configuration: %w", err)
	}
	host, port, err := net.SplitHostPort(access.Server)
	if err != nil || host == "" || port == "" {
		return errors.New("primary DNS server must be in host:port format")
	}
	keyName := access.KeyName
	if (keyName == "") != (access.KeySecret == "") {
		return errors.New("TSIG key name and secret must be provided together")
	}
	if keyName != "" {
		keyName = dns.CanonicalName(strings.TrimSpace(keyName))
		if _, ok := dns.IsDomainName(keyName); !ok || keyName == "." {
			return errors.New("a valid TSIG key name is required")
		}
		if secret, err := base64.StdEncoding.DecodeString(access.KeySecret); err != nil || len(secret) == 0 {
			return errors.New("TSIG secret must be a nonempty Base64 value")
		}
	}
	algorithm := dns.HmacSHA256
	if value := strings.TrimSpace(access.Algorithm); value != "" {
		algorithm = dns.CanonicalName(value)
	}
	switch algorithm {
	case dns.HmacSHA1, dns.HmacSHA224, dns.HmacSHA256, dns.HmacSHA384, dns.HmacSHA512:
	default:
		return errors.New("unsupported TSIG algorithm")
	}
	record, err := dnschallenge.ForZone(ctx, access.Zone, req)
	if err != nil {
		return err
	}
	message := new(dns.Msg).SetUpdate(record.Zone)
	change(message, []dns.RR{&dns.TXT{
		Hdr: dns.RR_Header{Name: record.Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: record.TTL},
		Txt: []string{record.Value},
	}})
	client := &dns.Client{Net: "tcp", Timeout: 10 * time.Second}
	if keyName != "" {
		message.SetTsig(keyName, algorithm, 300, time.Now().Unix())
		client.TsigSecret = map[string]string{keyName: access.KeySecret}
	}
	reply, _, err := client.ExchangeContext(ctx, message, access.Server)
	if err != nil {
		return fmt.Errorf("DNS update failed: %w", err)
	}
	if reply.Rcode != dns.RcodeSuccess {
		return fmt.Errorf("DNS update rejected with response code %d", reply.Rcode)
	}
	if keyName != "" && reply.IsTsig() == nil {
		return errors.New("DNS update reply is missing TSIG authentication")
	}
	return nil
}
