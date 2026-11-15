package ucdn

import (
	"context"
	"io"

	"github.com/ucloud/ucloud-sdk-go/ucloud"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"

	"github.com/certimate-go/plugins/internal/ucloudsdk/transport"
)

type UCDNClient struct {
	*ucloud.Client
}

func NewClient(ctx context.Context, config *ucloud.Config, credential *auth.Credential) *UCDNClient {
	meta := ucloud.ClientMeta{Product: "UCDN"}
	client := ucloud.NewClientWithMeta(config, credential, meta)
	client.GetLogger().SetOutput(io.Discard)
	client.SetTransport(transport.NewContextRoundTripper(ctx, nil))
	return &UCDNClient{
		client,
	}
}
