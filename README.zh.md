# certimate plugins

[English](README.md) | 中文

[certimate](https://github.com/certimate-go/certimate) 的外置 provider 插件集合。每个插件是一个独立二进制，certimate 通过 [hashicorp/go-plugin](https://github.com/hashicorp/go-plugin) 以 gRPC 子进程的方式启动它，完成证书的部署、DNS 解析、通知或清除。新增插件**无需改动 certimate 核心**。

本仓库维护官方插件注册表。已准入的插件从各自的 provider 仓库发布 ZIP。Certimate 经 CDN 读取 `main` 上的签名索引 `registry/index.json`，再从各 provider 的正式产物提交下载 ZIP。 [签名索引](https://cdn.jsdelivr.net/gh/certimate-go/plugins@main/registry/index.json)

## 贡献插件

修改已有官方插件时，从准入清单找到对应仓库，将它 fork 到个人账号，再向官方仓库的 `main` 提交 PR。 [官方准入清单](https://github.com/certimate-go/plugins/blob/main/registry/_policy/admissions.json)

申请收录新插件时，先在本仓库提 Issue，写明目标服务、插件能力、拟用的 `provider_type` 和验证方式。已有源码时，附上公开仓库、待审提交和许可证。维护者同意收录后会创建并配置官方仓库。收到仓库链接后，从个人 fork 提交源码 PR。维护者在合并源码后完成准入和首次发布。 [申请官方收录](https://github.com/certimate-go/plugins/issues/new)

如果要在自己的仓库独立发布，请按[自定义市场指南](skills/certimate-plugin-dev/references/third-party-development.md)操作。

## 插件结构

每个插件实现 `certimate/pkg/plugin` 中定义的统一 `Plugin` 接口：

- `GetMetadata` —— 身份信息：provider type、access type、capability、category、显示名。
- `GetConfigSchema` —— form/v1 provider（及可选 access）schema + i18n。
- `Process` —— 根据访问配置、provider 配置和该能力的 typed payload 执行工作。
- `PostProcess` —— 同一次执行的收尾（能力无需清理时嵌入 `plugin.NoopFinalizer`）。

源码中的 `manifest.json` 描述插件。

| 字段 | 含义 |
|---|---|
| `provider_type` | 稳定的插件标识，使用 kebab-case |
| `access_provider_type` | 本插件消费的访问凭据类型 |
| `version` | SemVer 版本号，须与官方发布触发标签对应 |
| `capability` | 四选一：`deploy`、`dns`、`notify`、`purge` |
| `category` | 市场分组（如 `cdn`、`storage`、`other`） |
| `binary` | 构建出的二进制名 |
| `icon` | 随插件发布的图标文件名 |
| `protocol_version` | 必须与 certimate 插件协议匹配 |
| `usages` / `priority` | 市场展示提示 |

## 开发插件

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

参考 `_template` 中的示例。官方 provider 的 `go.mod`、`manifest.json`、Go 入口和 `schema/` 放在各自仓库根目录。将图标一并放在根目录，并把文件名填入 `manifest.icon`。在 provider 仓库运行 `go test ./...`。

插件可以**声明新的** access 类型（添加 `schema/access.json`），也可以复用已有的（内置或来自其它插件）。

## 构建仍位于本仓库的插件

```bash
make build PLUGIN=my-deployer         # 当前主机平台安装目录 -> dist/<plugin>/
make build-all PLUGIN=my-deployer     # 跨平台编译出发布矩阵
make test                             # go test ./...
```

## 发布官方插件

源码 PR 和首次准入 PR 合并后，维护者将 `vX.Y.Z-detach` 标签推到 provider 仓库，指向受保护的 `main` 中与 `manifest.json` 版本对应的源码提交。发布工作流构建 8 个平台的 ZIP，并将 `build/` 写入不可改的正式标签 `vX.Y.Z`。注册表先合并版本登记 PR，再单独合并签名索引发布 PR。CDN 提供通过验证的签名索引后，该版本可从官方市场获取。 [官方发布契约](https://github.com/certimate-go/plugins/blob/main/docs/release-contract.md)

## 从内置 purge provider 升级

原先内置在 certimate 核心里的十六个证书清除 provider，已迁移为本仓库的 `purge` 能力插件：`aliyun-cas-purger`、`aliyun-clb-purger`、`aliyun-esa-purger`、`aws-acm-purger`、`aws-iam-purger`、`azure-keyvault-purger`、`baiducloud-cert-purger`、`huaweicloud-elb-purger`、`huaweicloud-scm-purger`、`huaweicloud-waf-purger`、`jdcloud-ssl-purger`、`tencentcloud-gaap-purger`、`tencentcloud-ssl-purger`、`tencentcloud-tse-purger`、`ucloud-ussl-purger`、`volcengine-certcenter-purger`。

引用旧清除 provider 的工作流节点，在安装映射插件后即可继续工作 —— certimate 会把旧 provider id 自动解析到这些插件；插件安装前该节点会报缺少 provider 的错误。certimate 不会未经请求就下载插件或改写工作流：请从插件市场（或自定义市场）显式安装对应插件；仅当希望节点改用新 provider id 时才需要重新保存工作流。

## 工具

- `cmd/registryctl` 用于校验和登记官方版本，以及发布签名索引。
- `cmd/releaseinfo` 为自定义市场生成 Release 附件元数据。
- `make clean` 清除 `dist/`。

## 环境要求

- Go 1.25+（`GOTOOLCHAIN=auto` 会自动拉取所需工具链）。
- 依赖 `github.com/certimate-go/certimate/pkg/plugin` 提供 `Plugin` 接口与握手协议。
