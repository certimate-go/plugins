package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xhttp "github.com/certimate-go/certimate/pkg/utils/http"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "webhook-deployer"
	accessType     = "webhook"
	displayNameKey = "plugin.webhook-deployer.name"

	defaultTimeout = 30
)

var (
	contentTypeJSON      = "application/json"
	contentTypeForm      = "application/x-www-form-urlencoded"
	contentTypeMultipart = "multipart/form-data"

	allowedContentTypes = map[string]bool{
		contentTypeJSON:      true,
		contentTypeForm:      true,
		contentTypeMultipart: true,
	}
	allowedMethods = map[string]bool{
		http.MethodGet:    true,
		http.MethodPost:   true,
		http.MethodPut:    true,
		http.MethodPatch:  true,
		http.MethodDelete: true,
	}
)

type webhookDeployer struct{}

func (*webhookDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "other",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*webhookDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{
		AccessSchemaJSON: nil, // reuses built-in webhook access type
		DeploySchemaJSON: deploySchema,
		I18n:             i18n,
	}, nil
}

type accessConfig struct {
	URL                      string `json:"url"`
	Method                   string `json:"method,omitempty"`
	HeadersString            string `json:"headers,omitempty"`
	DataString               string `json:"data,omitempty"`
	AllowInsecureConnections bool   `json:"allowInsecureConnections,omitempty"`
}

type extendedConfig struct {
	WebhookData string `json:"webhookData,omitempty"`
	Timeout     int    `json:"timeout,omitempty"`
}

func (d *webhookDeployer) Deploy(_ context.Context, req *plugin.DeployRequest) (*plugin.DeployResult, error) {
	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	if access.URL == "" {
		return nil, fmt.Errorf("access config missing url")
	}

	var extended extendedConfig
	if req.ExtendedConfigJSON != "" {
		if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
			return nil, fmt.Errorf("invalid deploy config: %w", err)
		}
	}

	webhookURL, err := url.Parse(access.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse webhook url: %w", err)
	}
	if webhookURL.Scheme != "http" && webhookURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported webhook url scheme %q", webhookURL.Scheme)
	}

	method := strings.ToUpper(strings.TrimSpace(access.Method))
	if method == "" {
		method = http.MethodPost
	} else if !allowedMethods[method] {
		return nil, fmt.Errorf("unsupported webhook request method %q", method)
	}

	header, err := xhttp.ParseHeaders(access.HeadersString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse webhook headers: %w", err)
	}

	contentType := header.Get("Content-Type")
	if contentType == "" {
		contentType = contentTypeJSON
		header.Set("Content-Type", contentTypeJSON)
	} else if mediaType, _, err := mime.ParseMediaType(contentType); err != nil || !allowedContentTypes[mediaType] {
		return nil, fmt.Errorf("unsupported webhook content type %q", contentType)
	}

	webhookData := extended.WebhookData
	if webhookData == "" {
		webhookData = access.DataString
	}

	timeout := defaultTimeout
	if extended.Timeout > 0 {
		timeout = extended.Timeout
	}

	certX509, err := parseCertificate(req.CertificatePEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	serverCertPEM, issuerCertPEM, err := extractLeafAndIntermediatePEM(req.CertificatePEM)
	if err != nil {
		return nil, fmt.Errorf("failed to extract certificates: %w", err)
	}
	commonName := subjectCommonName(certX509)
	subjectAltNames := strings.Join(subjectAltNames(certX509), ";")

	var data any
	if strings.TrimSpace(webhookData) == "" {
		data = map[string]string{
			"name":    subjectAltNames,
			"cert":    req.CertificatePEM,
			"privkey": req.PrivateKeyPEM,
		}
	} else {
		if err := json.Unmarshal([]byte(webhookData), &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal webhook data: %w", err)
		}
		if method == http.MethodGet || contentType == contentTypeForm || contentType == contentTypeMultipart {
			temp := make(map[string]string)
			jsonb, err := json.Marshal(data)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal webhook data: %w", err)
			}
			if err := json.Unmarshal(jsonb, &temp); err != nil {
				return nil, fmt.Errorf("failed to coerce webhook data to string map: %w", err)
			}
			data = temp
		}
	}

	webhookURL.Path = strings.ReplaceAll(webhookURL.Path, "${CERTIMATE_DEPLOYER_COMMONNAME}", url.PathEscape(commonName))
	deepReplaceString(data, "${CERTIMATE_DEPLOYER_COMMONNAME}", commonName)
	deepReplaceString(data, "${CERTIMATE_DEPLOYER_SUBJECTALTNAMES}", subjectAltNames)
	deepReplaceString(data, "${CERTIMATE_DEPLOYER_CERTIFICATE}", req.CertificatePEM)
	deepReplaceString(data, "${CERTIMATE_DEPLOYER_CERTIFICATE_SERVER}", serverCertPEM)
	deepReplaceString(data, "${CERTIMATE_DEPLOYER_CERTIFICATE_INTERMEDIA}", issuerCertPEM)
	deepReplaceString(data, "${CERTIMATE_DEPLOYER_PRIVATEKEY}", req.PrivateKeyPEM)
	// legacy variables
	webhookURL.Path = strings.ReplaceAll(webhookURL.Path, "${DOMAIN}", url.PathEscape(commonName))
	deepReplaceString(data, "${DOMAIN}", commonName)
	deepReplaceString(data, "${DOMAINS}", subjectAltNames)
	deepReplaceString(data, "${CERTIFICATE}", req.CertificatePEM)
	deepReplaceString(data, "${SERVER_CERTIFICATE}", serverCertPEM)
	deepReplaceString(data, "${INTERMEDIA_CERTIFICATE}", issuerCertPEM)
	deepReplaceString(data, "${PRIVATE_KEY}", req.PrivateKeyPEM)

	var body io.Reader
	switch {
	case method == http.MethodGet:
		query := webhookURL.Query()
		for k, v := range data.(map[string]string) {
			query.Set(k, v)
		}
		webhookURL.RawQuery = query.Encode()
	case contentType == contentTypeJSON:
		jsonb, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal webhook body: %w", err)
		}
		body = bytes.NewReader(jsonb)
	case contentType == contentTypeForm:
		form := url.Values{}
		for k, v := range data.(map[string]string) {
			form.Set(k, v)
		}
		body = strings.NewReader(form.Encode())
	case contentType == contentTypeMultipart:
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		for k, v := range data.(map[string]string) {
			if err := writer.WriteField(k, v); err != nil {
				return nil, fmt.Errorf("failed to write multipart field: %w", err)
			}
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("failed to close multipart writer: %w", err)
		}
		header.Set("Content-Type", writer.FormDataContentType())
		body = &buf
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), method, webhookURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header = header

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	if access.AllowInsecureConnections {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	extendedData, _ := json.Marshal(map[string]any{"statusCode": resp.StatusCode, "target": webhookURL.String()})
	return &plugin.DeployResult{ExtendedDataJSON: string(extendedData)}, nil
}

var oidSubjectAlternativeName = asn1.ObjectIdentifier{2, 5, 29, 17}

func parseCertificate(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

func extractLeafAndIntermediatePEM(certPEM string) (leaf, intermediate string, err error) {
	blocks := decodePEMBlocks([]byte(certPEM))
	if len(blocks) == 0 {
		return "", "", fmt.Errorf("failed to decode PEM block")
	}
	for i, block := range blocks {
		if block.Type != "CERTIFICATE" {
			return "", "", fmt.Errorf("invalid PEM block type at %d, expected 'CERTIFICATE', got '%s'", i, block.Type)
		}
	}
	leaf = string(pem.EncodeToMemory(blocks[0]))
	for i := 1; i < len(blocks); i++ {
		intermediate += string(pem.EncodeToMemory(blocks[i]))
	}
	return leaf, intermediate, nil
}

func decodePEMBlocks(data []byte) []*pem.Block {
	var blocks []*pem.Block
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		blocks = append(blocks, block)
		data = rest
	}
	return blocks
}

func subjectCommonName(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	if sans := subjectAltNames(cert); len(sans) > 0 {
		return sans[0]
	}
	return ""
}

func subjectAltNames(cert *x509.Certificate) []string {
	sans := []string{}
	if cert == nil {
		return sans
	}
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(oidSubjectAlternativeName) {
			continue
		}
		var seq asn1.RawValue
		if _, err := asn1.Unmarshal(ext.Value, &seq); err != nil {
			continue
		}
		rest := seq.Bytes
		for len(rest) > 0 {
			var name asn1.RawValue
			var err error
			rest, err = asn1.Unmarshal(rest, &name)
			if err != nil {
				break
			}
			switch name.Tag {
			case 7:
				sans = append(sans, net.IP(name.Bytes).String())
			case 1, 2, 6:
				sans = append(sans, string(name.Bytes))
			}
		}
	}
	return sans
}

func deepReplaceString(v any, oldStr, newStr string) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = deepReplaceValue(val, oldStr, newStr)
		}
	case []any:
		for i, val := range t {
			t[i] = deepReplaceValue(val, oldStr, newStr)
		}
	}
}

func deepReplaceValue(v any, oldStr, newStr string) any {
	switch t := v.(type) {
	case string:
		return strings.ReplaceAll(t, oldStr, newStr)
	default:
		deepReplaceString(v, oldStr, newStr)
		return v
	}
}
