package uewaf

import (
	"context"
	"io"

	"github.com/ucloud/ucloud-sdk-go/ucloud"
	"github.com/ucloud/ucloud-sdk-go/ucloud/auth"

	"github.com/certimate-go/plugins/internal/ucloudsdk/transport"
)

type UEWAFClient struct {
	*ucloud.Client
}

func NewClient(ctx context.Context, config *ucloud.Config, credential *auth.Credential) *UEWAFClient {
	meta := ucloud.ClientMeta{Product: "UEWAF"}
	client := ucloud.NewClientWithMeta(config, credential, meta)
	client.GetLogger().SetOutput(io.Discard)
	client.SetTransport(transport.NewContextRoundTripper(ctx, nil))
	return &UEWAFClient{
		client,
	}
}
