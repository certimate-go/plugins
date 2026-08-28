package main

import (
	githubplugin "github.com/hashicorp/go-plugin"

	"github.com/certimate-go/certimate/pkg/plugin"
)

func main() {
	deployer := &upyunFileDeployer{}
	githubplugin.Serve(&githubplugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig,
		Plugins: map[string]githubplugin.Plugin{
			plugin.PluginName: &plugin.DeployerGRPCPlugin{Impl: deployer},
		},
		GRPCServer: githubplugin.DefaultGRPCServer,
	})
}
