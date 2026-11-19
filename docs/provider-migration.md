# 官方插件迁移清单

新仓库使用 `certimate-go/plugin-provider-` 前缀，名称体现实际服务与插件能力。注册表通过准入清单把原有 `provider_type` 绑定到对应仓库。workspace 根目录用同名 submodule 固定源码提交。证书中心共享源码仓库由实际消费者再以子模块固定同一版本。

以下条目的目标为统一的八平台构建矩阵。

外部 Go 依赖按各仓库的 `go.mod` 和 `go.sum` 固定。清单列出随仓迁移的内部源码依赖。

| 系统 | 架构 |
| --- | --- |
| macOS | amd64、arm64 |
| Linux | amd64、arm64、arm v7 |
| Windows | 386、amd64、arm64 |

只有从正式签名目录安装并验证能力后，才把条目更新为已切换。若决定下线，保留条目并记录决定和负责人。

## 火山引擎

证书中心上传源码仓库为 `certimate-go/plugin-module-volcengine-certcenter-certmgr`。ALB、API Gateway、证书中心部署、CLB、DCDN、ImageX、TOS、VOD 和 WAF 消费该源码。BytePlus API Gateway 也固定此共享源码。Live 的证书管理保留在 Live 仓库。证书中心清理只消费它自己的查询和删除 API。

| Provider type | 新仓库（certimate-go） | 负责人 | 随仓源码依赖 | 状态 |
| --- | --- | --- | --- | --- |
| `volcengine-alb` | `plugin-provider-volcengine-alb-deployer` | 待分配 | 证书中心共享子模块、仓内 SDK | 源码已拆分，待发布验收 |
| `volcengine-apig` | `plugin-provider-volcengine-apig-deployer` | 待分配 | 证书中心共享子模块、仓内 SDK、`internal/volcengineapig` | 源码已拆分，待发布验收 |
| `volcengine-cdn` | `plugin-provider-volcengine-cdn-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/volcengine-cdn` | 公开源码已推送，待发布验收 |
| `volcengine-certcenter` | `plugin-provider-volcengine-certcenter-deployer` | 待分配 | 证书中心共享子模块 | 源码已拆分，待发布验收 |
| `volcengine-certcenter-purger` | `plugin-provider-volcengine-certcenter-purger` | 待分配 | 仓内 SDK | 公开源码已推送，待发布验收 |
| `volcengine-clb` | `plugin-provider-volcengine-clb-deployer` | 待分配 | 证书中心共享子模块、仓内 SDK | 源码已拆分，待发布验收 |
| `volcengine-dcdn` | `plugin-provider-volcengine-dcdn-deployer` | 待分配 | 证书中心共享子模块、仓内 SDK | 源码已拆分，待发布验收 |
| `volcengine-dns` | `plugin-provider-volcengine-dns-acmedns01challenger` | 待分配 | Go 模块 | 公开源码已推送，待发布验收 |
| `volcengine-imagex` | `plugin-provider-volcengine-imagex-deployer` | 待分配 | 证书中心共享子模块 | 源码已拆分，待发布验收 |
| `volcengine-live` | `plugin-provider-volcengine-live-deployer` | 待分配 | `internal/certmgr/volcengine-live` | 源码已拆分，待发布验收 |
| `volcengine-tos` | `plugin-provider-volcengine-tos-deployer` | 待分配 | 证书中心共享子模块、仓内 SDK | 源码已拆分，待发布验收 |
| `volcengine-vod` | `plugin-provider-volcengine-vod-deployer` | 待分配 | 证书中心共享子模块、仓内 SDK | 源码已拆分，待发布验收 |
| `volcengine-waf` | `plugin-provider-volcengine-waf-deployer` | 待分配 | 证书中心共享子模块、仓内 SDK | 源码已拆分，待发布验收 |

## 其他官方插件

| Provider type | 新仓库（certimate-go） | 负责人 | 随仓源码依赖 | 状态 |
| --- | --- | --- | --- | --- |
| `1panel` | `plugin-provider-1panel-deployer` | 待分配 | `internal/onepanelcertmgr`、`internal/onepanelsdk`、`internal/onepanelsdk/v2` | 源码已拆分，待发布验收 |
| `1panel-console` | `plugin-provider-1panel-console-deployer` | 待分配 | `internal/onepanelsdk`、`internal/onepanelsdk/v2` | 源码已拆分，待发布验收 |
| `aliyun-cas-purger` | `plugin-provider-aliyun-cas-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `aliyun-clb-purger` | `plugin-provider-aliyun-clb-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `aliyun-esa-purger` | `plugin-provider-aliyun-esa-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `apisix` | `plugin-provider-apisix-deployer` | 待分配 | `internal/apisixsdk` | 源码已拆分，待发布验收 |
| `asiaispcdn` | `plugin-provider-asiaispcdn-deployer` | 待分配 | `internal/asiaispcdncertmgr`、`internal/asiaispcdnsdk` | 源码已拆分，待发布验收 |
| `aws-acm-purger` | `plugin-provider-aws-acm-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `aws-iam-purger` | `plugin-provider-aws-iam-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `axisnow` | `plugin-provider-axisnow-cdn-deployer` | 待分配 | `internal/axisnowcertmgr`、`internal/axisnowsdk` | 源码已拆分，待发布验收 |
| `azure-keyvault-purger` | `plugin-provider-azure-keyvault-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `baiducloud-appblb` | `plugin-provider-baiducloud-appblb-deployer` | 待分配 | `internal/baiducloudcertmgr` | 源码已拆分，待发布验收 |
| `baiducloud-blb` | `plugin-provider-baiducloud-blb-deployer` | 待分配 | `internal/baiducloudcertmgr` | 源码已拆分，待发布验收 |
| `baiducloud-cdn` | `plugin-provider-baiducloud-cdn-deployer` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `baiducloud-cert` | `plugin-provider-baiducloud-cert-deployer` | 待分配 | `internal/baiducloudcertmgr` | 源码已拆分，待发布验收 |
| `baiducloud-cert-purger` | `plugin-provider-baiducloud-cert-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `baishan-cdn` | `plugin-provider-baishan-cdn-deployer` | 待分配 | `internal/baishancertmgr`、`internal/baishansdk` | 源码已拆分，待发布验收 |
| `baotapanel` | `plugin-provider-baotapanel-deployer` | 待分配 | `internal/btpanelsdk` | 源码已拆分，待发布验收 |
| `baotapanel-console` | `plugin-provider-baotapanel-console-deployer` | 待分配 | `internal/btpanelsdk` | 源码已拆分，待发布验收 |
| `baotapanelgo` | `plugin-provider-baotapanelgo-deployer` | 待分配 | `internal/btpanelgosdk` | 源码已拆分，待发布验收 |
| `baotapanelgo-console` | `plugin-provider-baotapanelgo-console-deployer` | 待分配 | `internal/btpanelgosdk` | 源码已拆分，待发布验收 |
| `baotawaf` | `plugin-provider-baotawaf-deployer` | 待分配 | `internal/btwafsdk` | 源码已拆分，待发布验收 |
| `baotawaf-console` | `plugin-provider-baotawaf-console-deployer` | 待分配 | `internal/btwafsdk` | 源码已拆分，待发布验收 |
| `bunny-cdn` | `plugin-provider-bunny-cdn-deployer` | 待分配 | `internal/bunnysdk` | 源码已拆分，待发布验收 |
| `byteplus-alb` | `plugin-provider-byteplus-alb-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/byteplus-certcenter` | 源码已拆分，待发布验收 |
| `byteplus-apig` | `plugin-provider-byteplus-apig-deployer` | 待分配 | 证书中心共享子模块、仓内 SDK、`internal/volcengineapig` | 源码已拆分，待发布验收 |
| `byteplus-cdn` | `plugin-provider-byteplus-cdn-deployer` | 待分配 | `internal/certmgr/byteplus-cdn` | 源码已拆分，待发布验收 |
| `byteplus-certcenter` | `plugin-provider-byteplus-certcenter-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/byteplus-certcenter` | 源码已拆分，待发布验收 |
| `byteplus-clb` | `plugin-provider-byteplus-clb-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/byteplus-certcenter` | 源码已拆分，待发布验收 |
| `byteplus-medialive` | `plugin-provider-byteplus-medialive-deployer` | 待分配 | `internal/certmgr/byteplus-medialive` | 源码已拆分，待发布验收 |
| `byteplus-tos` | `plugin-provider-byteplus-tos-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/byteplus-certcenter` | 源码已拆分，待发布验收 |
| `cachefly` | `plugin-provider-cachefly-deployer` | 待分配 | `internal/cacheflysdk` | 源码已拆分，待发布验收 |
| `cdnfly` | `plugin-provider-cdnfly-deployer` | 待分配 | `internal/cdnflysdk` | 源码已拆分，待发布验收 |
| `cloudflare-ssl` | `plugin-provider-cloudflare-ssl-deployer` | 待分配 | `internal/cloudflaresdk` | 源码已拆分，待发布验收 |
| `cmcccloud-cdn` | `plugin-provider-cmcccloud-cdn-deployer` | 待分配 | 仓内 SDK | 源码已拆分，待发布验收 |
| `cmcccloud-dns` | `plugin-provider-cmcccloud-dns-acmedns01challenger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `cmcccloud-vlb` | `plugin-provider-cmcccloud-vlb-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/cmcccloud-vlb` | 源码已拆分，待发布验收 |
| `cpanel` | `plugin-provider-cpanel-deployer` | 待分配 | `internal/cpanelsdk` | 源码已拆分，待发布验收 |
| `ctcccloud-ao` | `plugin-provider-ctcccloud-ao-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/ctcccloud-ao` | 源码已拆分，待发布验收 |
| `ctcccloud-cdn` | `plugin-provider-ctcccloud-cdn-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/ctcccloud-cdn` | 源码已拆分，待发布验收 |
| `ctcccloud-cms` | `plugin-provider-ctcccloud-cms-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/ctcccloud-cms` | 源码已拆分，待发布验收 |
| `ctcccloud-elb` | `plugin-provider-ctcccloud-elb-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/ctcccloud-elb` | 源码已拆分，待发布验收 |
| `ctcccloud-faas` | `plugin-provider-ctcccloud-faas-deployer` | 待分配 | 仓内 SDK | 源码已拆分，待发布验收 |
| `ctcccloud-icdn` | `plugin-provider-ctcccloud-icdn-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/ctcccloud-icdn` | 源码已拆分，待发布验收 |
| `ctcccloud-lvdn` | `plugin-provider-ctcccloud-lvdn-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/ctcccloud-lvdn` | 源码已拆分，待发布验收 |
| `digitalocean-certificate` | `plugin-provider-digitalocean-certificate-deployer` | 待分配 | `internal/digitaloceancertmgr`、`internal/digitaloceansdk` | 源码已拆分，待发布验收 |
| `dingtalkbot` | `plugin-provider-dingtalkbot-notifier` | 待分配 | `internal/notifier` | 源码已拆分，待发布验收 |
| `discordbot` | `plugin-provider-discordbot-notifier` | 待分配 | `internal/notifier` | 源码已拆分，待发布验收 |
| `dogecloud-cdn` | `plugin-provider-dogecloud-cdn-deployer` | 待分配 | `internal/dogecloudcertmgr`、`internal/dogecloudsdk` | 源码已拆分，待发布验收 |
| `dokploy` | `plugin-provider-dokploy-deployer` | 待分配 | `internal/dokploycertmgr`、`internal/dokploysdk` | 源码已拆分，待发布验收 |
| `flexcdn` | `plugin-provider-flexcdn-deployer` | 待分配 | `internal/flexcdnsdk` | 源码已拆分，待发布验收 |
| `flyio` | `plugin-provider-flyio-deployer` | 待分配 | `internal/flyiosdk` | 源码已拆分，待发布验收 |
| `gcore-cdn` | `plugin-provider-gcore-cdn-deployer` | 待分配 | `internal/gcoreauth`、`internal/gcorecertmgr` | 源码已拆分，待发布验收 |
| `goedge` | `plugin-provider-goedge-deployer` | 待分配 | `internal/goedgesdk` | 源码已拆分，待发布验收 |
| `googlecloud-certificatemanager` | `plugin-provider-googlecloud-certificatemanager-deployer` | 待分配 | `internal/googlecloudcertmgr` | 源码已拆分，待发布验收 |
| `googlecloud-dns` | `plugin-provider-googlecloud-dns-acmedns01challenger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `gotify` | `plugin-provider-gotify-notifier` | 待分配 | `internal/pluginhttp` | 源码已拆分，待发布验收 |
| `huaweicloud-aad` | `plugin-provider-huaweicloud-aad-deployer` | 待分配 | 仓内 SDK | 源码已拆分，待发布验收 |
| `huaweicloud-apig` | `plugin-provider-huaweicloud-apig-deployer` | 待分配 | 仓内 SDK | 源码已拆分，待发布验收 |
| `huaweicloud-cdn` | `plugin-provider-huaweicloud-cdn-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/huaweicloud-scm` | 源码已拆分，待发布验收 |
| `huaweicloud-elb` | `plugin-provider-huaweicloud-elb-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/huaweicloud-elb` | 源码已拆分，待发布验收 |
| `huaweicloud-elb-purger` | `plugin-provider-huaweicloud-elb-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `huaweicloud-live` | `plugin-provider-huaweicloud-live-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/huaweicloud-scm` | 源码已拆分，待发布验收 |
| `huaweicloud-obs` | `plugin-provider-huaweicloud-obs-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/huaweicloud-scm` | 源码已拆分，待发布验收 |
| `huaweicloud-scm` | `plugin-provider-huaweicloud-scm-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/huaweicloud-scm` | 源码已拆分，待发布验收 |
| `huaweicloud-scm-purger` | `plugin-provider-huaweicloud-scm-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `huaweicloud-vod` | `plugin-provider-huaweicloud-vod-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/huaweicloud-scm` | 源码已拆分，待发布验收 |
| `huaweicloud-waf` | `plugin-provider-huaweicloud-waf-deployer` | 待分配 | 仓内 SDK、`internal/certmgr/huaweicloud-waf` | 源码已拆分，待发布验收 |
| `huaweicloud-waf-purger` | `plugin-provider-huaweicloud-waf-purger` | 待分配 | 仓内 SDK | 源码已拆分，待发布验收 |
| `huaweiibmc` | `plugin-provider-huaweiibmc-deployer` | 待分配 | `internal/huaweiibmcsdk` | 源码已拆分，待发布验收 |
| `jdcloud-alb` | `plugin-provider-jdcloud-alb-deployer` | 待分配 | 仓内 SDK、`internal/jdcloudsslcert` | 源码已拆分，待发布验收 |
| `jdcloud-cdn` | `plugin-provider-jdcloud-cdn-deployer` | 待分配 | 仓内 SDK、`internal/jdcloudsslcert` | 源码已拆分，待发布验收 |
| `jdcloud-live` | `plugin-provider-jdcloud-live-deployer` | 待分配 | 仓内 SDK | 源码已拆分，待发布验收 |
| `jdcloud-ssl` | `plugin-provider-jdcloud-ssl-deployer` | 待分配 | `internal/jdcloudsslcert` | 源码已拆分，待发布验收 |
| `jdcloud-ssl-purger` | `plugin-provider-jdcloud-ssl-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `jdcloud-vod` | `plugin-provider-jdcloud-vod-deployer` | 待分配 | 仓内 SDK | 源码已拆分，待发布验收 |
| `jdcloud-waf` | `plugin-provider-jdcloud-waf-deployer` | 待分配 | 仓内 SDK、`internal/jdcloudsslcert` | 源码已拆分，待发布验收 |
| `k8s-secret` | `plugin-provider-k8s-secret-deployer` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `kong` | `plugin-provider-kong-deployer` | 待分配 | `internal/kongsdk` | 源码已拆分，待发布验收 |
| `ksyun-cdn` | `plugin-provider-ksyun-cdn-deployer` | 待分配 | `internal/ksyunsdk/cdn`、`internal/ksyunsdk/zz-shared-common` | 源码已拆分，待发布验收 |
| `ksyun-kcm` | `plugin-provider-ksyun-kcm-deployer` | 待分配 | `internal/ksyuncertmgr/kcm`、`internal/ksyunsdk/kcm`、`internal/ksyunsdk/zz-shared-common` | 源码已拆分，待发布验收 |
| `ksyun-slb` | `plugin-provider-ksyun-slb-deployer` | 待分配 | `internal/ksyuncertmgr/kcm`、`internal/ksyuncertmgr/slb`、`internal/ksyunsdk/kcm`、`internal/ksyunsdk/zz-shared-common` | 源码已拆分，待发布验收 |
| `larkbot` | `plugin-provider-larkbot-notifier` | 待分配 | `internal/notifier` | 源码已拆分，待发布验收 |
| `lecdn` | `plugin-provider-lecdn-deployer` | 待分配 | `internal/lecdnsdk/client/v3`、`internal/lecdnsdk/client/v4`、`internal/lecdnsdk/master/v3`、`internal/lecdnsdk/master/v4` | 源码已拆分，待发布验收 |
| `linode-los` | `plugin-provider-linode-los-deployer` | 待分配 | `internal/linodesdk` | 源码已拆分，待发布验收 |
| `matrix` | `plugin-provider-matrix-notifier` | 待分配 | `internal/notifier` | 源码已拆分，待发布验收 |
| `mattermost` | `plugin-provider-mattermost-notifier` | 待分配 | `internal/notifier` | 源码已拆分，待发布验收 |
| `mohua-mvh` | `plugin-provider-mohua-mvh-deployer` | 待分配 | `internal/mohuasdk` | 源码已拆分，待发布验收 |
| `netlify` | `plugin-provider-netlify-deployer` | 待分配 | `internal/netlifysdk` | 源码已拆分，待发布验收 |
| `nginxproxymanager` | `plugin-provider-nginxproxymanager-deployer` | 待分配 | `internal/nginxproxymanagercertmgr`、`internal/nginxproxymanagersdk` | 源码已拆分，待发布验收 |
| `ntfy` | `plugin-provider-ntfy-notifier` | 待分配 | `internal/pluginhttp` | 源码已拆分，待发布验收 |
| `oraclecloud-certificatesmgmt` | `plugin-provider-oraclecloud-certificatesmgmt-deployer` | 待分配 | `internal/oraclecloudcertmgr` | 源码已拆分，待发布验收 |
| `powerdns` | `plugin-provider-powerdns-dns-acmedns01challenger` | 待分配 | `internal/dnschallenge`、`internal/pluginhttp` | 源码已拆分，待发布验收 |
| `proxmoxbs` | `plugin-provider-proxmoxbs-deployer` | 待分配 | `internal/proxmoxbssdk` | 源码已拆分，待发布验收 |
| `proxmoxve` | `plugin-provider-proxmoxve-deployer` | 待分配 | `internal/proxmoxvesdk` | 源码已拆分，待发布验收 |
| `qingcloud-lb` | `plugin-provider-qingcloud-lb-deployer` | 待分配 | `internal/qingcloudcertmgr`、`internal/qingcloudsdk/lb` | 源码已拆分，待发布验收 |
| `qiniu-cdn` | `plugin-provider-qiniu-cdn-deployer` | 待分配 | `internal/qiniusdk`、`internal/qiniusslcert` | 源码已拆分，待发布验收 |
| `qiniu-kodo` | `plugin-provider-qiniu-kodo-deployer` | 待分配 | `internal/qiniusdk`、`internal/qiniusslcert` | 源码已拆分，待发布验收 |
| `qiniu-pili` | `plugin-provider-qiniu-pili-deployer` | 待分配 | `internal/qiniusdk`、`internal/qiniusslcert` | 源码已拆分，待发布验收 |
| `rainyun-rcdn` | `plugin-provider-rainyun-rcdn-deployer` | 待分配 | `internal/rainyun` | 源码已拆分，待发布验收 |
| `rainyun-sslcenter` | `plugin-provider-rainyun-sslcenter-deployer` | 待分配 | `internal/rainyun` | 源码已拆分，待发布验收 |
| `ratpanel` | `plugin-provider-ratpanel-deployer` | 待分配 | `internal/ratpanelsdk` | 源码已拆分，待发布验收 |
| `ratpanel-console` | `plugin-provider-ratpanel-console-deployer` | 待分配 | `internal/ratpanelsdk` | 源码已拆分，待发布验收 |
| `rfc2136` | `plugin-provider-rfc2136-dns-acmedns01challenger` | 待分配 | `internal/dnschallenge` | 源码已拆分，待发布验收 |
| `safeline` | `plugin-provider-safeline-deployer` | 待分配 | `internal/safelinesdk` | 源码已拆分，待发布验收 |
| `samwaf` | `plugin-provider-samwaf-deployer` | 待分配 | `internal/samwafsdk` | 源码已拆分，待发布验收 |
| `samwaf-console` | `plugin-provider-samwaf-console-deployer` | 待分配 | `internal/samwafsdk` | 源码已拆分，待发布验收 |
| `slackbot` | `plugin-provider-slackbot-notifier` | 待分配 | `internal/notifier` | 源码已拆分，待发布验收 |
| `synologydsm` | `plugin-provider-synologydsm-deployer` | 待分配 | `internal/synologydsmsdk` | 源码已拆分，待发布验收 |
| `telegrambot` | `plugin-provider-telegrambot-notifier` | 待分配 | `internal/notifier` | 源码已拆分，待发布验收 |
| `tencentcloud-gaap-purger` | `plugin-provider-tencentcloud-gaap-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `tencentcloud-ssl-purger` | `plugin-provider-tencentcloud-ssl-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `tencentcloud-tse-purger` | `plugin-provider-tencentcloud-tse-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `ucloud-pathx` | `plugin-provider-ucloud-pathx-deployer` | 待分配 | `internal/ucloudsdk/transport`、`internal/ucloudsdk/upathx`、`internal/ucloudupathxcertmgr` | 源码已拆分，待发布验收 |
| `ucloud-ualb` | `plugin-provider-ucloud-ualb-deployer` | 待分配 | `internal/ucloudsdk/transport`、`internal/ucloudsdk/ulb`、`internal/ucloudulbcertmgr` | 源码已拆分，待发布验收 |
| `ucloud-ucdn` | `plugin-provider-ucloud-ucdn-deployer` | 待分配 | `internal/ucloudsdk/transport`、`internal/ucloudsdk/ucdn`、`internal/ucloudusslcertmgr` | 源码已拆分，待发布验收 |
| `ucloud-uclb` | `plugin-provider-ucloud-uclb-deployer` | 待分配 | `internal/ucloudsdk/transport`、`internal/ucloudsdk/ulb`、`internal/ucloudulbcertmgr` | 源码已拆分，待发布验收 |
| `ucloud-uewaf` | `plugin-provider-ucloud-uewaf-deployer` | 待分配 | `internal/ucloudsdk/transport`、`internal/ucloudsdk/uewaf` | 源码已拆分，待发布验收 |
| `ucloud-us3` | `plugin-provider-ucloud-us3-deployer` | 待分配 | `internal/ucloudsdk/transport`、`internal/ucloudsdk/ufile`、`internal/ucloudusslcertmgr` | 源码已拆分，待发布验收 |
| `ucloud-ussl-purger` | `plugin-provider-ucloud-ussl-purger` | 待分配 | Go 模块 | 源码已拆分，待发布验收 |
| `unicloud-webhost` | `plugin-provider-unicloud-webhost-deployer` | 待分配 | `internal/unicloudsdk` | 源码已拆分，待发布验收 |
| `upyun-cdn` | `plugin-provider-upyun-cdn-deployer` | 待分配 | `internal/upyuncertmgr`、`internal/upyunsdk` | 源码已拆分，待发布验收 |
| `upyun-file` | `plugin-provider-upyun-file-deployer` | 待分配 | `internal/upyuncertmgr`、`internal/upyunsdk` | 源码已拆分，待发布验收 |
| `vercel` | `plugin-provider-vercel-deployer` | 待分配 | `internal/vercelsdk` | 源码已拆分，待发布验收 |
| `wangsu-cdn` | `plugin-provider-wangsu-cdn-deployer` | 待分配 | `internal/wangsucertmgr`、`internal/wangsusdk/cdn`、`internal/wangsusdk/certificate`、`internal/wangsusdk/zz-shared-common` | 源码已拆分，待发布验收 |
| `wangsu-cdnpro` | `plugin-provider-wangsu-cdnpro-deployer` | 待分配 | `internal/wangsusdk/cdnpro`、`internal/wangsusdk/zz-shared-common` | 源码已拆分，待发布验收 |
| `wangsu-certificate` | `plugin-provider-wangsu-certificate-deployer` | 待分配 | `internal/wangsucertmgr`、`internal/wangsusdk/certificate`、`internal/wangsusdk/zz-shared-common` | 源码已拆分，待发布验收 |
| `wecombot` | `plugin-provider-wecombot-notifier` | 待分配 | `internal/notifier` | 源码已拆分，待发布验收 |
| `yandexcloud-certificatemanager` | `plugin-provider-yandexcloud-certificatemanager-deployer` | 待分配 | `internal/yandexcloudcertmgr` | 源码已拆分，待发布验收 |
| `zenlayer-cdn` | `plugin-provider-zenlayer-cdn-deployer` | 待分配 | `internal/zenlayercertmgr/cdn`、`internal/zenlayersdk/cdn` | 源码已拆分，待发布验收 |
| `zenlayer-ga` | `plugin-provider-zenlayer-ga-deployer` | 待分配 | `internal/zenlayercertmgr/zga`、`internal/zenlayersdk/zga` | 源码已拆分，待发布验收 |

中央仓库和拆分后的阿里云 provider 中，只有 `aliyun-cas-purger` 调用 `cas-20200407`，证书清理逻辑也位于该 provider 内。其余阿里云插件使用各自的产品 API。本批无需建立 `plugin-module-aliyun-cas-certmgr`。
