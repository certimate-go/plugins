package ulb

import (
	"context"
	"io"

	"github.com/ucloud/ucloud-sdk-go/ucloud"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"

	"github.com/certimate-go/plugins/internal/ucloudsdk/transport"
)

type ULBClient struct {
	*ucloud.Client
}

func NewClient(ctx context.Context, config *ucloud.Config, credential *auth.Credential) *ULBClient {
	meta := ucloud.ClientMeta{Product: "ULB"}
	client := ucloud.NewClientWithMeta(config, credential, meta)
	client.GetLogger().SetOutput(io.Discard)
	client.SetTransport(transport.NewContextRoundTripper(ctx, nil))
	return &ULBClient{
		client,
	}
}
