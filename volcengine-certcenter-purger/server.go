package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/certimate-go/certimate/pkg/plugin"
	xmaps "github.com/certimate-go/certimate/pkg/utils/maps"
)

//go:embed schema
var schemaFS embed.FS

const (
	providerType   = "volcengine-certcenter-purger"
	accessType     = "volcengine"
	displayNameKey = "plugin.volcengine-certcenter-purger.name"
)

type pluginPurger struct {
	plugin.NoopFinalizer
}

func (*pluginPurger) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:       providerType,
		AccessProviderType: accessType,
		ProtocolVersion:    plugin.ProtocolVersion,
		Capability:         plugin.CapabilityPurge,
		Category:           "other",
		DisplayNameKey:     displayNameKey,
	}, nil
}

func (*pluginPurger) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
	providerSchema, err := plugin.LoadProviderSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{ProviderSchemaJSON: providerSchema, I18n: i18n}, nil
}

type accessConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	ProjectName     string `json:"projectName,omitempty"`
}

func (*pluginPurger) Process(ctx context.Context, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var rawAccess map[string]any
	if err := json.Unmarshal([]byte(req.AccessConfigJSON), &rawAccess); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}
	var access accessConfig
	if err := xmaps.Populate(rawAccess, &access); err != nil {
		return nil, fmt.Errorf("invalid access config: %w", err)
	}

	var extended map[string]any
	if err := json.Unmarshal([]byte(req.ExtendedConfigJSON), &extended); err != nil {
		return nil, fmt.Errorf("invalid purge config: %w", err)
	}

	purger, err := NewPurger(&PurgerConfig{
		AccessKeyId:     access.AccessKeyId,
		SecretAccessKey: access.SecretAccessKey,
		ProjectName:     access.ProjectName,
		Region:          xmaps.GetString(extended, "region"),
	})
	if err != nil {
		return nil, err
	}
	purger.SetLogger(logger)

	purgeCount, err := purger.Purge(ctx, time.Duration(req.Purge.ExpirySeconds)*time.Second)
	if err != nil {
		return nil, err
	}

	return &plugin.Result{
		Purge: &plugin.PurgeResult{Count: purgeCount, ExtendedDataJSON: "{}"},
	}, nil
}
