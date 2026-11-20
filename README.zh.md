# certimate plugins

[English](README.md) | 中文

[certimate](https://github.com/certimate-go/certimate) 的外置 provider 插件集合。每个插件是一个独立二进制，certimate 通过 [hashicorp/go-plugin](https://github.com/hashicorp/go-plugin) 以 gRPC 子进程的方式启动它，完成证书的部署、DNS 解析、通知或清除。新增插件**无需改动 certimate 核心**。

本仓库维护官方插件索引。各 provider 仓库自行构建和发布。Certimate 读取 [签名索引](https://cdn.jsdelivr.net/gh/certimate-go/plugins@latest/registry/index.json)。每次发布推送 semver 标签 `v<epoch>.<generation>.0`，并清理这个 `@latest` 别名。

## 仓库结构

官方 provider 源码在各自的 `certimate-go/plugin-provider-*` 仓库。通过版本回执的 `repository` 字段定位源码。

| 路径 | 用途 |
| --- | --- |
| `registry/` | 版本回执、签名目录、详情和图标 |
| `.github/workflows/` | 可复用的 provider CI、打包、登记和签名发布 |
| `cmd/` | 插件和注册表工具 |
| `_template/` | 插件脚手架 |
| `integration/` | 使用独立 provider checkout 的 Core 集成测试 |

## 插件结构

每个插件实现 `certimate/pkg/plugin` 中定义的统一 `Plugin` 接口：

- `GetMetadata` —— 身份信息：provider type、access type、capability、category、显示名。
- `GetConfigSchema` —— form/v1 provider（及可选 access）schema + i18n。
- `Process` —— 根据访问配置、provider 配置和该能力的 typed payload 执行工作。
- `PostProcess` —— 同一次执行的收尾（能力无需清理时嵌入 `plugin.NoopFinalizer`）。

`manifest.json` 携带市场元数据：

| 字段 | 含义 |
|---|---|
| `provider_type` | 插件唯一 id（kebab-case）；同时也是目录名和二进制名 |
| `access_provider_type` | 本插件消费的访问凭据类型 |
| `version` | semver 版本号；**bump 它才会触发新发版** |
| `capability` | 四选一：`deploy`、`dns`、`notify`、`purge` |
| `category` | 市场分组（如 `cdn`、`storage`、`other`） |
| `binary` | 构建出的二进制名 |
| `icon` | 随插件发布的图标文件名 |
| `protocol_version` | 必须与 certimate 插件协议匹配 |
| `usages` / `priority` | 市场展示提示 |
| `release` | 生成的 GitHub Release 指针（repo/tag/assets/checksums） |

## 新增插件

仓库提供 [certimate-plugin-dev skill](skills/certimate-plugin-dev/SKILL.md)，指导 AI 编程助手开发 deploy、dns、notify、purge 插件，覆盖凭据复用、表单与中英文翻译、构建及真实宿主验证。在本仓库中使用时，可直接给助手以下指令：

```text
请读取 skills/certimate-plugin-dev/SKILL.md，为 <目标平台> 开发一个 <deploy/dns/notify/purge> 插件。
目标行为：<部署目标 / DNS 服务 / 通知渠道 / 过期证书清除>；API 文档：<文档地址>；凭据类型：<已有类型或新建>。
```

安装到 Codex：在本仓库根目录执行以下命令，将整个 skill 目录复制到个人 skills 目录（如已安装同名 skill，请先备份再更新）：

```sh
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
cp -R skills/certimate-plugin-dev "${CODEX_HOME:-$HOME/.codex}/skills/"
```

安装后，在你自己的项目中调用：“用 `$certimate-plugin-dev` 为我的服务开发 DNS 插件，并把本仓库配置成自定义市场。”其他支持 `SKILL.md` 的助手可将整个目录复制到各自的 skills 目录。参见[自定义市场发布指南](docs/releasing.md#distributing-from-your-own-market)。

```bash
make init PLUGIN=my-deployer          # 从 _template 生成脚手架
```

将脚手架复制到独立 Go 模块，再编辑 `my-deployer/manifest.json`（设置 `provider_type`、`access_provider_type`、`category`、`icon` 等），在 `server.go` 实现 `Process()`，并编写 `schema/config.json`。把图标文件（svg/png）放进插件目录，并在 `manifest.icon` 指向它。

插件可以**声明新的** access 类型（添加 `schema/access.json`），也可以复用已有的（内置或来自其它插件）。

## 构建与测试

初始化 provider 自己的 Git 子模块，然后构建到本地 Core 插件目录。

```sh
make dev PROVIDER_DIR=../plugin-provider-volcengine-dns-acmedns01challenger
```

在 provider 仓库运行 `go test ./...`。在注册表仓库运行 `make test` 前，先初始化 workspace 的独立 provider 子模块，或将 `CERTIMATE_PLUGIN_WORKSPACE` 指向这些 checkout。 [集成测试](integration/README.md)

## 发版

在 provider 仓库推送发版请求标签。正式标签出现后，本仓库为该版本开 PR。合并后，签名索引自动发布到 `main`。 [官方发布指南](docs/releasing.md)

## 从内置 purge provider 升级

原先内置在 certimate 核心里的十六个证书清除 provider，已迁移为 `purge` 能力插件：`aliyun-cas-purger`、`aliyun-clb-purger`、`aliyun-esa-purger`、`aws-acm-purger`、`aws-iam-purger`、`azure-keyvault-purger`、`baiducloud-cert-purger`、`huaweicloud-elb-purger`、`huaweicloud-scm-purger`、`huaweicloud-waf-purger`、`jdcloud-ssl-purger`、`tencentcloud-gaap-purger`、`tencentcloud-ssl-purger`、`tencentcloud-tse-purger`、`ucloud-ussl-purger`、`volcengine-certcenter-purger`。

引用旧清除 provider 的工作流节点，在安装映射插件后即可继续工作 —— certimate 会把旧 provider id 自动解析到这些插件；插件安装前该节点会报缺少 provider 的错误。certimate 不会未经请求就下载插件或改写工作流：请从插件市场（或自定义市场）显式安装对应插件；仅当希望节点改用新 provider id 时才需要重新保存工作流。

## 工具

- `make index` 使用注册表回执和 `READY` 指定的 CDN 工件观测结果暂存目录。
- [第三方发布模板](docs/releasing.md#distributing-from-your-own-market)固定其使用的 `releaseinfo` 和 `genindex` 工具版本。
- `make clean` 清除本地 `dist/` 输出。

## 环境要求

- Go 1.25+（`GOTOOLCHAIN=auto` 会自动拉取所需工具链）。
- 依赖 `github.com/certimate-go/certimate/pkg/plugin` 提供 `Deployer` 接口与握手协议。
