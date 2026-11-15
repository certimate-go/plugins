package upathx

import (
	"context"
	"io"

	"github.com/ucloud/ucloud-sdk-go/ucloud"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"

	"github.com/certimate-go/plugins/internal/ucloudsdk/transport"
)

type UPathXClient struct {
	*ucloud.Client
}

func NewClient(ctx context.Context, config *ucloud.Config, credential *auth.Credential) *UPathXClient {
	meta := ucloud.ClientMeta{Product: "PathX"}
	client := ucloud.NewClientWithMeta(config, credential, meta)
	client.GetLogger().SetOutput(io.Discard)
	client.SetTransport(transport.NewContextRoundTripper(ctx, nil))
	return &UPathXClient{
		client,
	}
}
