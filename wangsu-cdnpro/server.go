package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/samber/lo"

	"github.com/certimate-go/certimate/pkg/plugin"
	xwait "github.com/certimate-go/certimate/pkg/utils/wait"
	wangsucdnprosdk "github.com/certimate-go/plugins/internal/wangsusdk/cdnpro"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "wangsu-cdnpro"
	accessType     = "wangsu"
	displayNameKey = "plugin.wangsu-cdnpro.name"

	environmentProduction = "production"
)

type wangsuCdnproDeployer struct{}

func (*wangsuCdnproDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "cdn",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*wangsuCdnproDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
	deploySchema, err := plugin.LoadDeploySchema(schemaFS)
	if err != nil {
		return nil, err
	}
	accessSchema, _ := plugin.LoadAccessSchema(schemaFS)
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{
		AccessSchemaJSON: accessSchema,
		DeploySchemaJSON: deploySchema,
		I18n:             i18n,
	}, nil
}

type accessConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	ApiKey          string `json:"apiKey"`
}

type extendedConfig struct {
	Environment        string `json:"environment"`
	DomainMatchPattern string `json:"domainMatchPattern,omitempty"`
	Domain             string `json:"domain"`
	CertificateId      string `json:"certificateId,omitempty"`
	WebhookId          string `json:"webhookId,omitempty"`
}

func parseExtendedConfig(raw string) (*extendedConfig, error) {
	var extended extendedConfig
	if err := json.Unmarshal([]byte(raw), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}

	if extended.Environment == "" {
		extended.Environment = environmentProduction
	}

	return &extended, nil
}

func (d *wangsuCdnproDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}

	extended, err := parseExtendedConfig(req.ExtendedConfigJSON)
	if err != nil {
		return nil, err
	}

	sdkClient, err := createSDKClient(access.AccessKeyId, access.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}

	if extended.Domain == "" {
		return nil, fmt.Errorf("config `domain` is required")
	}

	getHostnameDetailResp, err := sdkClient.GetHostnameDetailWithContext(ctx, extended.Domain)
	logger.Debug("sdk request 'cdnpro.GetHostnameDetail'", slog.String("params.hostname", extended.Domain), slog.Any("response", getHostnameDetailResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cdnpro.GetHostnameDetail': %w", err)
	}

	encryptedPrivateKey, err := encryptPrivateKey(req.PrivateKeyPEM, access.ApiKey, time.Now().Unix())
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt private key: %w", err)
	}
	certificateNewVersionInfo := &wangsucdnprosdk.CertificateVersion{
		PrivateKey:  lo.ToPtr(encryptedPrivateKey),
		Certificate: lo.ToPtr(req.CertificatePEM),
	}

	var wangsuCertUrl string
	var wangsuCertId string
	var wangsuCertVer int32

	timestamp := time.Now().Unix()
	if extended.CertificateId == "" {
		createCertificateReq := &wangsucdnprosdk.CreateCertificateRequest{
			Timestamp:  timestamp,
			Name:       lo.ToPtr(fmt.Sprintf("certimate_%d", time.Now().UnixMilli())),
			AutoRenew:  lo.ToPtr("Off"),
			NewVersion: certificateNewVersionInfo,
		}
		createCertificateResp, err := sdkClient.CreateCertificateWithContext(ctx, createCertificateReq)
		logger.Debug("sdk request 'cdnpro.CreateCertificate'", slog.Any("request", createCertificateReq), slog.Any("response", createCertificateResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdnpro.CreateCertificate': %w", err)
		}

		wangsuCertUrl = createCertificateResp.CertificateLocation
		logger.Info("ssl certificate uploaded", slog.Any("certUrl", wangsuCertUrl))

		wangsuCertIdMatches := regexp.MustCompile(`/certificates/([a-zA-Z0-9-]+)`).FindStringSubmatch(wangsuCertUrl)
		if len(wangsuCertIdMatches) > 1 {
			wangsuCertId = wangsuCertIdMatches[1]
		}

		wangsuCertVer = 1
	} else {
		updateCertificateReq := &wangsucdnprosdk.UpdateCertificateRequest{
			Timestamp:  timestamp,
			Name:       lo.ToPtr(fmt.Sprintf("certimate_%d", time.Now().UnixMilli())),
			AutoRenew:  lo.ToPtr("Off"),
			NewVersion: certificateNewVersionInfo,
		}
		updateCertificateResp, err := sdkClient.UpdateCertificateWithContext(ctx, extended.CertificateId, updateCertificateReq)
		logger.Debug("sdk request 'cdnpro.UpdateCertificate'", slog.String("params.certificateId", extended.CertificateId), slog.Any("request", updateCertificateReq), slog.Any("response", updateCertificateResp))
		if err != nil {
			return nil, fmt.Errorf("failed to execute sdk request 'cdnpro.UpdateCertificate': %w", err)
		}

		wangsuCertUrl = updateCertificateResp.CertificateLocation
		logger.Info("ssl certificate uploaded", slog.Any("certUrl", wangsuCertUrl))

		wangsuCertIdMatches := regexp.MustCompile(`/certificates/([a-zA-Z0-9-]+)`).FindStringSubmatch(wangsuCertUrl)
		if len(wangsuCertIdMatches) > 1 {
			wangsuCertId = wangsuCertIdMatches[1]
		}

		wangsuCertVerMatches := regexp.MustCompile(`/versions/(\d+)`).FindStringSubmatch(wangsuCertUrl)
		if len(wangsuCertVerMatches) > 1 {
			n, _ := strconv.ParseInt(wangsuCertVerMatches[1], 10, 32)
			wangsuCertVer = int32(n)
		}
	}

	var wangsuTaskId string
	createDeploymentTaskReq := &wangsucdnprosdk.CreateDeploymentTaskRequest{
		Name:   lo.ToPtr(fmt.Sprintf("certimate_%d", time.Now().UnixMilli())),
		Target: lo.ToPtr(extended.Environment),
		Actions: &[]wangsucdnprosdk.DeploymentTaskAction{
			{
				Action:        lo.ToPtr("deploy_cert"),
				CertificateId: lo.ToPtr(wangsuCertId),
				Version:       lo.ToPtr(wangsuCertVer),
			},
		},
		Webhook: lo.EmptyableToPtr(extended.WebhookId),
	}
	createDeploymentTaskResp, err := sdkClient.CreateDeploymentTaskWithContext(ctx, createDeploymentTaskReq)
	logger.Debug("sdk request 'cdnpro.CreateCertificate'", slog.Any("request", createDeploymentTaskReq), slog.Any("response", createDeploymentTaskResp))
	if err != nil {
		return nil, fmt.Errorf("failed to execute sdk request 'cdnpro.CreateDeploymentTask': %w", err)
	} else {
		wangsuTaskMatches := regexp.MustCompile(`/deploymentTasks/([a-zA-Z0-9-]+)`).FindStringSubmatch(createDeploymentTaskResp.DeploymentTaskLocation)
		if len(wangsuTaskMatches) > 1 {
			wangsuTaskId = wangsuTaskMatches[1]
		}
	}

	if _, err := xwait.UntilWithContext(ctx, func(_ context.Context, _ int) (bool, error) {
		getDeploymentTaskDetailResp, err := sdkClient.GetDeploymentTaskDetailWithContext(ctx, wangsuTaskId)
		logger.Info("sdk request 'cdnpro.GetDeploymentTaskDetail'", slog.String("params.taskId", wangsuTaskId), slog.Any("response", getDeploymentTaskDetailResp))
		if err != nil {
			return false, fmt.Errorf("failed to execute sdk request 'cdnpro.GetDeploymentTaskDetail': %w", err)
		}

		if getDeploymentTaskDetailResp.Status == "failed" {
			return false, fmt.Errorf("unexpected deployment task status")
		} else if getDeploymentTaskDetailResp.Status == "succeeded" || getDeploymentTaskDetailResp.FinishTime != "" {
			return true, nil
		}

		logger.Info(fmt.Sprintf("waiting for deployment task completion (current status: %s) ...", getDeploymentTaskDetailResp.Status))
		return false, nil
	}, 10*time.Second); err != nil {
		return nil, err
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createSDKClient(accessKeyId, accessKeySecret string) (*wangsucdnprosdk.Client, error) {
	client, err := wangsucdnprosdk.NewClient(
		wangsucdnprosdk.WithAkSk(accessKeyId, accessKeySecret),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func encryptPrivateKey(privkeyPEM string, apiKey string, timestamp int64) (string, error) {
	date := time.Unix(timestamp, 0).UTC()
	dateStr := date.Format(http.TimeFormat)

	h := hmac.New(sha256.New, []byte(apiKey))
	h.Write([]byte(dateStr))
	aesivkey := h.Sum(nil)
	aesivkeyHex := hex.EncodeToString(aesivkey)

	if len(aesivkeyHex) != 64 {
		return "", fmt.Errorf("invalid hmac length: %d", len(aesivkeyHex))
	}
	ivHex := aesivkeyHex[:32]
	keyHex := aesivkeyHex[32:64]

	iv, err := hex.DecodeString(ivHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode iv: %w", err)
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	plainBytes := []byte(privkeyPEM)
	padlen := aes.BlockSize - len(plainBytes)%aes.BlockSize
	if padlen > 0 {
		paddata := bytes.Repeat([]byte{byte(padlen)}, padlen)
		plainBytes = append(plainBytes, paddata...)
	}

	encBytes := make([]byte, len(plainBytes))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(encBytes, plainBytes)

	return base64.StdEncoding.EncodeToString(encBytes), nil
}
