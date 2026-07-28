package main

import (
	githubplugin "github.com/hashicorp/go-plugin"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func main() {
	githubplugin.Serve(&githubplugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig,
		Plugins: map[string]githubplugin.Plugin{
			plugin.PluginName: &plugin.DeployerGRPCPlugin{Impl: &dogeCloudCdnDeployer{}},
		},
		GRPCServer: githubplugin.DefaultGRPCServer,
	})
}
