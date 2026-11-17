package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
)

const accessCheckAdminSSLSPath = "/apisix/admin/ssls"

const (
	accessCheckItemConnect = "connect"
	accessCheckItemAuth    = "auth"
)

const (
	accessCheckReasonConfigInvalid  = "config_invalid"
	accessCheckReasonConnectFailed  = "connect_failed"
	accessCheckReasonTLSFailed      = "tls_failed"
	accessCheckReasonAuthRejected   = "auth_rejected"
	accessCheckReasonProbeForbidden = "probe_forbidden"
	accessCheckReasonRemoteError    = "remote_error"
	accessCheckReasonTimeout        = "timeout"
	accessCheckReasonCanceled       = "canceled"
)

type accessCheckOutcome struct {
	status      string
	reasonCode  string
	connect     string
	auth        string
	diagnostics []string
}

func (o accessCheckOutcome) toResult() *plugin.AccessCheckResult {
	return &plugin.AccessCheckResult{
		Status:     o.status,
		ReasonCode: o.reasonCode,
		Items: []plugin.AccessCheckItem{
			{ID: accessCheckItemConnect, Status: o.connect},
			{ID: accessCheckItemAuth, Status: o.auth},
		},
		Diagnostics: o.diagnostics,
	}
}

func accessCheckSkippedOutcome(reasonCode string, diagnostics ...string) accessCheckOutcome {
	return accessCheckOutcome{
		status:      plugin.AccessCheckStatusInconclusive,
		reasonCode:  reasonCode,
		connect:     plugin.AccessCheckItemStatusSkipped,
		auth:        plugin.AccessCheckItemStatusSkipped,
		diagnostics: diagnostics,
	}
}

func accessCheckReachedOutcome(status, reasonCode string, diagnostics ...string) accessCheckOutcome {
	return accessCheckOutcome{
		status:      status,
		reasonCode:  reasonCode,
		connect:     plugin.AccessCheckItemStatusPassed,
		auth:        plugin.AccessCheckItemStatusSkipped,
		diagnostics: diagnostics,
	}
}

func (*pluginDeployer) CheckAccess(ctx context.Context, req *plugin.AccessCheckRequest, logger *slog.Logger) (*plugin.AccessCheckResult, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	access, err := parseAccessConfig(req.AccessConfigJSON)
	if err != nil {
		return accessCheckOutcome{
			status:      plugin.AccessCheckStatusFailed,
			reasonCode:  accessCheckReasonConfigInvalid,
			connect:     plugin.AccessCheckItemStatusSkipped,
			auth:        plugin.AccessCheckItemStatusSkipped,
			diagnostics: []string{err.Error()},
		}.toResult(), nil
	}

	return checkAccess(ctx, access, logger), nil
}

func parseAccessConfig(configJSON string) (*accessConfig, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(configJSON), &raw); err != nil {
		return nil, fmt.Errorf("malformed access config JSON: %w", err)
	}
	var access accessConfig
	if err := xmaps.Populate(raw, &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	if access.ServerUrl == "" {
		return nil, errors.New("missing required field 'serverUrl'")
	}
	serverURL, err := url.Parse(access.ServerUrl)
	if err != nil || serverURL.Host == "" || (serverURL.Scheme != "http" && serverURL.Scheme != "https") {
		return nil, errors.New("invalid field 'serverUrl': an HTTP(S) endpoint URL is required")
	}
	if access.ApiKey == "" {
		return nil, errors.New("missing required field 'apiKey'")
	}
	return &access, nil
}

func checkAccess(ctx context.Context, access *accessConfig, logger *slog.Logger) *plugin.AccessCheckResult {
	probeURL := strings.TrimSuffix(access.ServerUrl, "/") + accessCheckAdminSSLSPath + "?page=1&page_size=1"

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return accessCheckOutcome{
			status:      plugin.AccessCheckStatusFailed,
			reasonCode:  accessCheckReasonConfigInvalid,
			connect:     plugin.AccessCheckItemStatusSkipped,
			auth:        plugin.AccessCheckItemStatusSkipped,
			diagnostics: []string{fmt.Sprintf("invalid field 'serverUrl': %v", err)},
		}.toResult()
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("X-API-KEY", access.ApiKey)

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: access.AllowInsecureConnections},
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	httpResponse, err := httpClient.Do(httpRequest)
	if err != nil {
		return accessCheckTransportOutcome(err).toResult()
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, 1<<20))
	if err != nil {
		return accessCheckReadOutcome(ctx, err).toResult()
	}

	parsedURL, _ := url.Parse(probeURL)
	logger.Debug("apisix admin api probe", slog.String("method", http.MethodGet), slog.String("path", accessCheckAdminSSLSPath), slog.Int("status", httpResponse.StatusCode))
	return accessCheckVerdict(httpResponse.StatusCode, body, parsedURL.Host).toResult()
}

func accessCheckTransportOutcome(err error) accessCheckOutcome {
	switch {
	case errors.Is(err, context.Canceled):
		return accessCheckSkippedOutcome(accessCheckReasonCanceled, "the access check was canceled before a verdict")
	case isTimeoutError(err):
		return accessCheckSkippedOutcome(accessCheckReasonTimeout, "the request to the admin API timed out")
	case isTLSError(err):
		return accessCheckOutcome{
			status:      plugin.AccessCheckStatusFailed,
			reasonCode:  accessCheckReasonTLSFailed,
			connect:     plugin.AccessCheckItemStatusFailed,
			auth:        plugin.AccessCheckItemStatusSkipped,
			diagnostics: []string{fmt.Sprintf("the TLS handshake with the admin API failed: %s", transportMessage(err))},
		}
	default:
		return accessCheckOutcome{
			status:      plugin.AccessCheckStatusFailed,
			reasonCode:  accessCheckReasonConnectFailed,
			connect:     plugin.AccessCheckItemStatusFailed,
			auth:        plugin.AccessCheckItemStatusSkipped,
			diagnostics: []string{fmt.Sprintf("could not reach the admin API: %s", transportMessage(err))},
		}
	}
}

func accessCheckReadOutcome(ctx context.Context, err error) accessCheckOutcome {
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return accessCheckSkippedOutcome(accessCheckReasonCanceled, "the access check was canceled before a verdict")
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return accessCheckSkippedOutcome(accessCheckReasonTimeout, "the response from the admin API timed out")
	default:
		return accessCheckReachedOutcome(plugin.AccessCheckStatusInconclusive, accessCheckReasonRemoteError, fmt.Sprintf("failed to read the response from the admin API: %s", transportMessage(err)))
	}
}

func accessCheckVerdict(statusCode int, body []byte, host string) accessCheckOutcome {
	switch {
	case statusCode == http.StatusOK:
		var payload any
		if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
			return accessCheckReachedOutcome(plugin.AccessCheckStatusInconclusive, accessCheckReasonRemoteError, "the admin API returned an unparseable body")
		}
		return accessCheckOutcome{
			status:  plugin.AccessCheckStatusPassed,
			connect: plugin.AccessCheckItemStatusPassed,
			auth:    plugin.AccessCheckItemStatusPassed,
			diagnostics: []string{
				fmt.Sprintf("connected to the admin API at %s over HTTP", host),
				fmt.Sprintf("X-API-KEY authentication accepted by GET %s", accessCheckAdminSSLSPath),
			},
		}
	case statusCode == http.StatusUnauthorized:
		return accessCheckOutcome{
			status:      plugin.AccessCheckStatusFailed,
			reasonCode:  accessCheckReasonAuthRejected,
			connect:     plugin.AccessCheckItemStatusPassed,
			auth:        plugin.AccessCheckItemStatusFailed,
			diagnostics: []string{fmt.Sprintf("the admin API rejected the API key (HTTP %d on GET %s)", statusCode, accessCheckAdminSSLSPath)},
		}
	case statusCode == http.StatusForbidden:
		return accessCheckReachedOutcome(
			plugin.AccessCheckStatusInconclusive,
			accessCheckReasonProbeForbidden,
			fmt.Sprintf("the admin API denied the read-only probe (HTTP %d on GET %s); the cause cannot be attributed to the credentials", statusCode, accessCheckAdminSSLSPath),
		)
	default:
		return accessCheckReachedOutcome(
			plugin.AccessCheckStatusInconclusive,
			accessCheckReasonRemoteError,
			fmt.Sprintf("the admin API returned unexpected HTTP %d for GET %s", statusCode, accessCheckAdminSSLSPath),
		)
	}
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isTLSError(err error) bool {
	var certVerifyErr *tls.CertificateVerificationError
	var recordHeaderErr tls.RecordHeaderError
	var unknownAuthorityErr x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	return errors.As(err, &certVerifyErr) ||
		errors.As(err, &recordHeaderErr) ||
		errors.As(err, &unknownAuthorityErr) ||
		errors.As(err, &hostnameErr)
}

func transportMessage(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err.Error()
	}
	return err.Error()
}
