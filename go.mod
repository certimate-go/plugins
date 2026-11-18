module github.com/certimate-go/plugins

go 1.27.4

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.23.3
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.15.0
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azcertificates v1.5.0
	github.com/G-Core/gcorelabscdn-go v1.0.37
	github.com/alibabacloud-go/darabonba-openapi/v2 v2.2.4
	github.com/alibabacloud-go/tea v1.5.3
	github.com/aws/aws-sdk-go-v2 v1.47.5
	github.com/aws/aws-sdk-go-v2/config v1.33.12
	github.com/aws/aws-sdk-go-v2/credentials v1.20.12
	github.com/aws/aws-sdk-go-v2/service/acm v1.50.4
	github.com/aws/aws-sdk-go-v2/service/iam v1.64.4
	github.com/aws/smithy-go v1.28.7
	github.com/baidubce/bce-sdk-go v0.9.277
	github.com/byteplus-sdk/byteplus-go-sdk-v2 v1.0.80
	github.com/byteplus-sdk/byteplus-sdk-golang v1.0.73
	github.com/certimate-go/certimate v0.4.39-0.20261117024256-d35882e429db
	github.com/go-acme/lego/v5 v5.6.0
	github.com/go-resty/resty/v2 v2.17.2
	github.com/google/go-querystring v1.2.0
	github.com/google/uuid v1.6.0
	github.com/hashicorp/go-plugin v1.8.0
	github.com/huaweicloud/huaweicloud-sdk-go-v3 v0.1.220
	github.com/jdcloud-api/jdcloud-sdk-go v1.67.0
	github.com/miekg/dns v1.1.73
	github.com/nrdcg/oci-go-sdk/certificatesmanagement/v1065 v1065.127.0
	github.com/nrdcg/oci-go-sdk/common/v1065 v1065.128.0
	github.com/pocketbase/pocketbase v0.39.10
	github.com/pquerna/otp v1.5.0
	github.com/qiniu/go-sdk/v7 v7.26.18
	github.com/samber/lo v1.53.0
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.3.202
	github.com/ucloud/ucloud-sdk-go v0.22.141
	github.com/volcengine/volc-sdk-golang v1.0.258
	github.com/volcengine/volcengine-go-sdk v1.2.58
	github.com/yandex-cloud/go-genproto v0.133.0
	github.com/yandex-cloud/go-sdk/services/certificatemanager v0.0.102
	github.com/yandex-cloud/go-sdk/v2 v2.185.0
	github.com/yunify/qingcloud-sdk-go v2.0.0-alpha.38+incompatible
	github.com/zenlayer/zenlayercloud-sdk-go v0.2.49
	golang.org/x/oauth2 v0.38.0
	google.golang.org/api v0.309.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
	k8s.io/api v0.35.3
	k8s.io/apimachinery v0.35.3
	k8s.io/client-go v0.35.3
	software.sslmate.com/src/go-pkcs12 v0.7.4
)

require (
	cloud.google.com/go/auth v0.24.1-0.20261001053825-dbc26066f70a // indirect
	cloud.google.com/go/auth/oauth2adapt v0.3.0 // indirect
	cloud.google.com/go/compute/metadata v0.10.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.13.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.2.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.10.1 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/alexbrainman/sspi v0.0.0-20250919150558-7d374ff0d59e // indirect
	github.com/alibabacloud-go/alibabacloud-gateway-spi v0.0.5 // indirect
	github.com/alibabacloud-go/cas-20200407/v4 v4.7.3 // indirect
	github.com/alibabacloud-go/debug v1.0.1 // indirect
	github.com/alibabacloud-go/esa-20240910/v3 v3.13.1 // indirect
	github.com/alibabacloud-go/slb-20140515/v4 v4.0.14 // indirect
	github.com/alibabacloud-go/tea-utils/v2 v2.0.9 // indirect
	github.com/aliyun/credentials-go v1.4.12 // indirect
	github.com/avast/retry-go v3.0.0+incompatible // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.5 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.8 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.8 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.12.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.39.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.44.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.51.6 // indirect
	github.com/bodgit/gssapi v0.0.4 // indirect
	github.com/bodgit/tsig v1.3.1 // indirect
	github.com/boombuler/barcode v1.1.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clbanning/mxj/v2 v2.7.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.16 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.6 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v1.0.0 // indirect
	github.com/go-playground/locales v0.14.2 // indirect
	github.com/go-playground/universal-translator v0.18.2 // indirect
	github.com/go-playground/validator/v10 v10.30.5 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/gofrs/flock v0.13.1 // indirect
	github.com/gofrs/uuid v4.4.0+incompatible // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/s2a-go v0.1.11 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.24 // indirect
	github.com/googleapis/gax-go/v2 v2.26.2 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/go-uuid v1.0.4 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/goidentity/v6 v6.0.1 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.13-0.20220915233716-71ac16282d12 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/leodido/go-urn v1.5.0 // indirect
	github.com/mailru/easyjson v0.9.2 // indirect
	github.com/mattn/go-colorable v0.1.16 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/openshift/gssapi v0.0.0-20161010215902-5fb4217df13b // indirect
	github.com/pavlo-v-chernykh/keystore-go/v4 v4.5.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/qiniu/dyn v1.3.0 // indirect
	github.com/qiniu/x v1.17.0 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/sony/gobreaker/v2 v2.4.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/pflag v1.1.0 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/gaap v1.3.34 // indirect
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ssl v1.3.147 // indirect
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tse v1.3.182 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.mongodb.org/mongo-driver v1.17.10 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0 // indirect
	go.opentelemetry.io/otel v1.45.0 // indirect
	go.opentelemetry.io/otel/metric v1.45.0 // indirect
	go.opentelemetry.io/otel/trace v1.45.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/term v0.47.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260715232425-e75dac1f907d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261019180522-ce908d29299b // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.3 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	k8s.io/kube-openapi v0.0.0-20260330154417-16be699c7b31 // indirect
	k8s.io/utils v0.0.0-20260319190234-28399d86e0b5 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.2 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

replace (
	gitlab.ecloud.com/ecloud/ecloudsdkcloudcore v1.0.0 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcloudcore@v1.0.0+mod
	gitlab.ecloud.com/ecloud/ecloudsdkclouddns v1.0.1 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkclouddns@v1.0.1+mod
	gitlab.ecloud.com/ecloud/ecloudsdkcmcdn v1.0.0 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcmcdn@v1.0.0
	gitlab.ecloud.com/ecloud/ecloudsdkcore v1.0.2 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcore@v1.0.2
	gitlab.ecloud.com/ecloud/ecloudsdkcore v1.0.6 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkcore@v1.0.6
	gitlab.ecloud.com/ecloud/ecloudsdkvlb v1.0.7 => ./pkg/sdk3rd-forked/gitlab.ecloud.com/ecloud/ecloudsdkvlb@v1.0.7
)
