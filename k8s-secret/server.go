package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	k8score "k8s.io/api/core/v1"
	k8serrs "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/certimate-go/certimate/pkg/plugin"
	xcert "github.com/certimate-go/certimate/pkg/utils/cert"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "k8s-secret"
	accessType     = "k8s"
	displayNameKey = "plugin.k8s-secret.name"

	defaultNamespace        = "default"
	defaultSecretType       = "kubernetes.io/tls"
	defaultSecretDataKeyKey = "tls.key"
	defaultSecretDataKeyCrt = "tls.crt"
)

type k8sSecretDeployer struct{}

func (*k8sSecretDeployer) GetMetadata(_ context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   accessType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployCategory:       "other",
		DeployDisplayNameKey: displayNameKey,
	}, nil
}

func (*k8sSecretDeployer) GetConfigSchema(_ context.Context) (*plugin.ConfigSchema, error) {
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
	KubeConfig string `json:"kubeConfig,omitempty"`
}

type extendedConfig struct {
	Namespace                         string `json:"namespace,omitempty"`
	SecretName                        string `json:"secretName"`
	SecretType                        string `json:"secretType,omitempty"`
	SecretDataKeyForKey               string `json:"secretDataKeyForKey,omitempty"`
	SecretDataKeyForCrt               string `json:"secretDataKeyForCrt,omitempty"`
	SecretDataKeyForCrtOnlyServer     string `json:"secretDataKeyForCrtOnlyServer,omitempty"`
	SecretDataKeyForCrtOnlyIntermedia string `json:"secretDataKeyForCrtOnlyIntermedia,omitempty"`
	SecretAnnotations                 string `json:"secretAnnotations,omitempty"`
	SecretLabels                      string `json:"secretLabels,omitempty"`
}

func (d *k8sSecretDeployer) Deploy(ctx context.Context, req *plugin.DeployRequest, logger *slog.Logger) (*plugin.DeployResult, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	var access accessConfig
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}

	var extended extendedConfig
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid deploy config: %w", err)
	}

	if extended.Namespace == "" {
		extended.Namespace = defaultNamespace
	}
	if extended.SecretType == "" {
		extended.SecretType = defaultSecretType
	}
	if extended.SecretDataKeyForKey == "" {
		extended.SecretDataKeyForKey = defaultSecretDataKeyKey
	}
	if extended.SecretDataKeyForCrt == "" {
		extended.SecretDataKeyForCrt = defaultSecretDataKeyCrt
	}

	if extended.SecretName == "" {
		return nil, fmt.Errorf("config `secretName` is required")
	}

	secretAnnotations, err := parseKeyValueMap(extended.SecretAnnotations)
	if err != nil {
		return nil, fmt.Errorf("failed to parse kubernetes secret annotations: %w", err)
	}
	secretLabels, err := parseKeyValueMap(extended.SecretLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to parse kubernetes secret labels: %w", err)
	}

	certX509, err := xcert.ParseCertificateFromPEM(req.CertificatePEM)
	if err != nil {
		return nil, err
	}

	serverCertPEM, issuerCertPEM, err := xcert.ExtractCertificatesFromPEM(req.CertificatePEM)
	if err != nil {
		return nil, fmt.Errorf("failed to extract certs: %w", err)
	}

	client, err := createK8sClient(access.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	secretPayload := &k8score.Secret{}
	secretIsNew := false
	secretGetResp := client.Get().
		Namespace(extended.Namespace).
		Resource("secrets").
		Name(extended.SecretName).
		VersionedParams(&meta.GetOptions{}, meta.ParameterCodec).
		Do(ctx)
	if err := secretGetResp.Error(); err != nil {
		if !k8serrs.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get kubernetes secret: %w", err)
		}
		secretPayload = &k8score.Secret{
			Type: k8score.SecretType(extended.SecretType),
			TypeMeta: meta.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: meta.ObjectMeta{
				Name: extended.SecretName,
			},
		}
		secretIsNew = true
	} else if err := secretGetResp.Into(secretPayload); err != nil {
		return nil, fmt.Errorf("failed to parse kubernetes secret: %w", err)
	}
	logger.Debug("kubernetes operate 'Secrets.Get'", slog.String("namespace", extended.Namespace), slog.Any("secret", extended.SecretName))

	builtAnnotations := map[string]string{
		"certimate/common-name":       certX509.Subject.CommonName,
		"certimate/subject-sn":        certX509.Subject.SerialNumber,
		"certimate/subject-alt-names": strings.Join(certX509.DNSNames, ","),
		"certimate/issuer-sn":         certX509.Issuer.SerialNumber,
		"certimate/issuer-org":        strings.Join(certX509.Issuer.Organization, ","),
	}
	builtLabels := map[string]string{}
	xmaps.CopyTo(secretAnnotations, builtAnnotations)
	xmaps.CopyTo(secretLabels, builtLabels)

	secretPayload.Type = k8score.SecretType(extended.SecretType)
	if secretPayload.ObjectMeta.Annotations == nil {
		secretPayload.ObjectMeta.Annotations = builtAnnotations
	} else {
		xmaps.CopyTo(builtAnnotations, secretPayload.ObjectMeta.Annotations)
	}
	if secretPayload.ObjectMeta.Labels == nil {
		secretPayload.ObjectMeta.Labels = builtLabels
	} else {
		xmaps.CopyTo(builtLabels, secretPayload.ObjectMeta.Labels)
	}
	if secretPayload.Data == nil {
		secretPayload.Data = make(map[string][]byte)
	}
	if extended.SecretDataKeyForKey != "" {
		secretPayload.Data[extended.SecretDataKeyForKey] = []byte(req.PrivateKeyPEM)
	}
	if extended.SecretDataKeyForCrt != "" {
		secretPayload.Data[extended.SecretDataKeyForCrt] = []byte(req.CertificatePEM)
	}
	if extended.SecretDataKeyForCrtOnlyServer != "" {
		secretPayload.Data[extended.SecretDataKeyForCrtOnlyServer] = []byte(serverCertPEM)
	}
	if extended.SecretDataKeyForCrtOnlyIntermedia != "" {
		secretPayload.Data[extended.SecretDataKeyForCrtOnlyIntermedia] = []byte(issuerCertPEM)
	}

	if secretIsNew {
		secretPostResp := client.Post().
			Namespace(extended.Namespace).
			Resource("secrets").
			Name(extended.SecretName).
			VersionedParams(&meta.GetOptions{}, meta.ParameterCodec).
			Body(secretPayload).
			Do(ctx)
		logger.Debug("kubernetes operate 'Secrets.Post'", slog.String("namespace", extended.Namespace), slog.Any("secret", extended.SecretName))
		if err := secretPostResp.Error(); err != nil {
			return nil, fmt.Errorf("failed to create kubernetes secret: %w", err)
		}
	} else {
		secretPutResp := client.Put().
			Namespace(extended.Namespace).
			Resource("secrets").
			Name(extended.SecretName).
			VersionedParams(&meta.GetOptions{}, meta.ParameterCodec).
			Body(secretPayload).
			Do(ctx)
		logger.Debug("kubernetes operate 'Secrets.Put'", slog.String("namespace", extended.Namespace), slog.Any("secret", extended.SecretName))
		if err := secretPutResp.Error(); err != nil {
			return nil, fmt.Errorf("failed to update kubernetes secret: %w", err)
		}
	}

	return &plugin.DeployResult{ExtendedDataJSON: "{}"}, nil
}

func createK8sClient(kubeConfig string) (*rest.RESTClient, error) {
	var config *rest.Config
	var err error
	if kubeConfig == "" {
		config, err = rest.InClusterConfig()
	} else {
		kubeConfig, err := clientcmd.NewClientConfigFromBytes([]byte(kubeConfig))
		if err != nil {
			return nil, err
		}
		config, err = kubeConfig.ClientConfig()
	}
	if err != nil {
		return nil, err
	}

	// InClusterConfig does not set GroupVersion or NegotiatedSerializer,
	// but both are required by rest.RESTClientFor.
	if config.GroupVersion == nil {
		config.GroupVersion = &schema.GroupVersion{Group: "", Version: "v1"}
	}
	if config.NegotiatedSerializer == nil {
		config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	}

	return rest.RESTClientFor(config)
}

func parseKeyValueMap(s string) (map[string]string, error) {
	result := make(map[string]string)

	for i, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pos := strings.Index(line, ":")
		if pos == -1 {
			return nil, fmt.Errorf("invalid line format at line %d", i+1)
		}
		key := strings.TrimSpace(line[:pos])
		value := strings.TrimSpace(line[pos+1:])
		if key == "" {
			return nil, fmt.Errorf("invalid key at line %d", i+1)
		}
		result[key] = value
	}

	return result, nil
}
