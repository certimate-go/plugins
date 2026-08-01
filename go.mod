module github.com/certimate-go/plugins

go 1.25.12

require (
	github.com/go-resty/resty/v2 v2.17.2
	github.com/hashicorp/go-plugin v1.8.0
	github.com/qiniu/go-sdk/v7 v7.26.18
	github.com/samber/lo v1.53.0
	k8s.io/api v0.35.3
	k8s.io/apimachinery v0.35.3
	k8s.io/client-go v0.35.3
)

require github.com/go-openapi/jsonpointer v1.0.0 // indirect

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/certimate-go/certimate v0.4.30-0.20260801043400-926b29dea6b3
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-acme/lego/v5 v5.3.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.3 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/json-iterator/go v1.1.13-0.20220915233716-71ac16282d12 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.23 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/pavlo-v-chernykh/keystore-go/v4 v4.5.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/qiniu/dyn v1.3.0 // indirect
	github.com/qiniu/x v1.17.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260724162435-b2f20204f0df // indirect
	google.golang.org/grpc v1.82.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	k8s.io/kube-openapi v0.0.0-20260330154417-16be699c7b31 // indirect
	k8s.io/utils v0.0.0-20260319190234-28399d86e0b5 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.2 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
	software.sslmate.com/src/go-pkcs12 v0.7.3 // indirect
)

replace (
	gitlab.ecloud.com/ecloud/ecloudsdkcloudcore v1.0.0 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcloudcore@v1.0.0+mod
	gitlab.ecloud.com/ecloud/ecloudsdkclouddns v1.0.1 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkclouddns@v1.0.1+mod
	gitlab.ecloud.com/ecloud/ecloudsdkcmcdn v1.0.0 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcmcdn@v1.0.0
	gitlab.ecloud.com/ecloud/ecloudsdkcore v1.0.2 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcore@v1.0.2
	gitlab.ecloud.com/ecloud/ecloudsdkcore v1.0.6 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcore@v1.0.6
	gitlab.ecloud.com/ecloud/ecloudsdkvlb v1.0.7 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkvlb@v1.0.7
)
