module github.com/certimate-go/plugins

go 1.27.4

require (
	github.com/baidubce/bce-sdk-go v0.9.277
	github.com/certimate-go/certimate v0.4.39-0.20261117024256-d35882e429db
	github.com/ucloud/ucloud-sdk-go v0.22.141
	github.com/yandex-cloud/go-genproto v0.133.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
	software.sslmate.com/src/go-pkcs12 v0.7.4
)

require (
	github.com/fatih/color v1.19.0 // indirect
	github.com/go-acme/lego/v5 v5.6.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-plugin v1.8.0 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/mattn/go-colorable v0.1.16 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/pavlo-v-chernykh/keystore-go/v4 v4.5.0 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260715232425-e75dac1f907d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261019180522-ce908d29299b // indirect
)

replace (
	gitlab.ecloud.com/ecloud/ecloudsdkcloudcore v1.0.0 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcloudcore@v1.0.0+mod
	gitlab.ecloud.com/ecloud/ecloudsdkclouddns v1.0.1 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkclouddns@v1.0.1+mod
	gitlab.ecloud.com/ecloud/ecloudsdkcmcdn v1.0.0 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcmcdn@v1.0.0
	gitlab.ecloud.com/ecloud/ecloudsdkcore v1.0.2 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcore@v1.0.2
	gitlab.ecloud.com/ecloud/ecloudsdkcore v1.0.6 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcore@v1.0.6
	gitlab.ecloud.com/ecloud/ecloudsdkvlb v1.0.7 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkvlb@v1.0.7
)
