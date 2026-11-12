package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/errs"
)

type NetHttpClientBuilder struct {
	goClient       *http.Client
	ignoreSSL      bool
	certFile       string
	clientCertFile string
	clientKeyFile  string
}

type Proxy struct {
	Protocol *string
	Host     *string
	Port     *int32
	Username *string
	Password *string
}

func NewHttpClientBuilder() *NetHttpClientBuilder {
	return &NetHttpClientBuilder{ignoreSSL: true}
}

func defaultGoClient() *http.Client {
	return &http.Client{
		Transport: defaultTransport(),
	}
}

func defaultTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
}

func (hcb *NetHttpClientBuilder) Build() *http.Client {
	if hcb.goClient == nil {
		return defaultGoClient()
	}
	return hcb.goClient
}

func (hcb *NetHttpClientBuilder) SetReadTimeout(timeout int32) {
	if hcb.goClient == nil {
		hcb.goClient = defaultGoClient()
	}
	hcb.goClient.Timeout = time.Duration(timeout) * time.Second
}

func (hcb *NetHttpClientBuilder) SetConnectTimeout(timeout int32) {
	if hcb.goClient == nil {
		hcb.goClient = defaultGoClient()
	}
	transport := hcb.goClient.Transport.(*http.Transport)
	transport.DialContext = hcb.setDialContext(timeout)
	hcb.goClient.Transport = transport
}

func (hcb *NetHttpClientBuilder) SetClientProxy(proxy Proxy) {
	if hcb.goClient == nil {
		hcb.goClient = defaultGoClient()
	}
	transport := hcb.goClient.Transport.(*http.Transport)
	proxyUrl := proxy.GetProxyUrl()
	if proxyUrl != "" {
		proxy, _ := url.Parse(proxyUrl)
		transport.Proxy = http.ProxyURL(proxy)
	}
	hcb.goClient.Transport = transport
}

func (p *Proxy) GetProxyUrl() string {
	var proxyUrl string
	if p.Username != nil {
		proxyUrl = fmt.Sprintf("%s://%s:%s@%s", *p.Protocol, *p.Username, *p.Password, *p.Host)
	} else {
		proxyUrl = fmt.Sprintf("%s://%s", *p.Protocol, *p.Host)
	}
	if p.Port != nil {
		proxyUrl = fmt.Sprintf("%s:%d", proxyUrl, *p.Port)
	}
	return proxyUrl
}

func (hcb *NetHttpClientBuilder) setDialContext(timeout int32) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{
			Timeout:   time.Duration(timeout) * time.Second,
			DualStack: true,
		}).DialContext(ctx, network, address)
	}
}

func (hcb *NetHttpClientBuilder) SetIgnoreSSL(ignoreSSL bool) {
	if ignoreSSL != hcb.ignoreSSL {
		transport := defaultTransport()
		transport.TLSClientConfig.InsecureSkipVerify = ignoreSSL
		hcb.goClient.Transport = transport
		hcb.ignoreSSL = ignoreSSL
	}
}

func (hcb *NetHttpClientBuilder) SetCertFile(certFile string) error {
	if certFile != hcb.certFile {
		return hcb.ApplySSLSettings(hcb.ignoreSSL, certFile, hcb.clientCertFile, hcb.clientKeyFile)
	}
	return nil
}

func (hcb *NetHttpClientBuilder) SetClientKeyPairFile(clientCertFile string, clientKeyFile string) error {
	if clientCertFile != hcb.clientCertFile || clientKeyFile != hcb.clientKeyFile {
		return hcb.ApplySSLSettings(hcb.ignoreSSL, hcb.certFile,
			hcb.clientCertFile, hcb.clientKeyFile)
	}
	return nil
}

func (hcb *NetHttpClientBuilder) ApplySSLSettings(ignoreSSL bool, certFile string, clientCertFile string, clientKeyFile string) error {
	if ignoreSSL && hcb.ignoreSSL {
		return nil
	}
	if ignoreSSL && !hcb.ignoreSSL {
		hcb.ignoreSSL = ignoreSSL
		transport := hcb.goClient.Transport.(*http.Transport)
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: ignoreSSL,
		}
		hcb.goClient.Transport = transport
		return nil
	}
	if !ignoreSSL && !hcb.ignoreSSL {
		if certFile == hcb.certFile && clientCertFile == hcb.clientCertFile &&
			clientKeyFile == hcb.clientKeyFile {
			return nil
		}
	}

	var certP *x509.CertPool = nil
	if len(certFile) > 0 {
		certP = &x509.CertPool{}
		pemCert, err := ioutil.ReadFile(certFile)
		if err != nil {
			return errs.NewSslHandShakeError(fmt.Sprintf("can't read certfile: %s", certFile), err)
		}
		certP.AppendCertsFromPEM(pemCert)
	}
	transport := hcb.goClient.Transport.(*http.Transport)
	var clientCert *tls.Certificate = nil
	if len(clientKeyFile) > 0 && len(clientCertFile) > 0 {
		cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return errs.NewSslHandShakeError(fmt.Sprintf("can't load keypair of http cert file: %s, http key file: %s",
				clientCertFile, clientKeyFile), err)
		}
		clientCert = &cert
	}

	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: ignoreSSL,
	}
	if certP != nil {
		transport.TLSClientConfig.RootCAs = certP
	}
	if clientCert != nil {
		transport.TLSClientConfig.Certificates = []tls.Certificate{*clientCert}
	}
	hcb.goClient.Transport = transport
	hcb.certFile = certFile
	hcb.clientCertFile = clientCertFile
	hcb.clientKeyFile = clientKeyFile
	return nil
}
