package main

import (
	"context"
	"embed"

	githubplugin "github.com/hashicorp/go-plugin"

	"github.com/certimate-go/certimate/pkg/plugin"
)

//go:embed schema
var schemaFS embed.FS

const providerType = "matrix"

type server struct{}

func main() {
	githubplugin.Serve(&githubplugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig,
		Plugins: plugin.PluginSet{
			plugin.NotifyPluginName: &plugin.NotifyGRPCPlugin{Impl: &server{}},
		},
		GRPCServer: githubplugin.DefaultGRPCServer,
	})
}

func (*server) GetMetadata(context.Context) (*plugin.Metadata, error) {
	return &plugin.Metadata{
		ProviderType:         providerType,
		AccessProviderType:   providerType,
		ProtocolVersion:      plugin.ProtocolVersion,
		DeployDisplayNameKey: "plugin." + providerType + ".name",
		AccessDisplayNameKey: "plugin." + providerType + ".name",
	}, nil
}

func (*server) GetConfigSchema(context.Context) (*plugin.ConfigSchema, error) {
	access, err := plugin.LoadAccessSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	config, err := plugin.LoadProviderSchema(schemaFS)
	if err != nil {
		return nil, err
	}
	i18n, err := plugin.LoadI18n(schemaFS)
	if err != nil {
		return nil, err
	}
	return &plugin.ConfigSchema{
		AccessSchemaJSON: access,
		DeploySchemaJSON: config,
		I18n:             i18n,
	}, nil
}
