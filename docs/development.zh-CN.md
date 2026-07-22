# 访迹（VisitorTrace）开发指南

[英文版](./development.md)

本文面向参与 VisitorTrace 开发和部署集成的贡献者。面向站点管理员的命令和接入说明见[用户指南](./user-guide.zh-CN.md)。

## 设计边界

VisitorTrace 是一个单进程 Go 服务。生产运行只依赖可执行文件、SQLite 数据库和本地 GeoIP MMDB，不需要 Node.js、Redis、消息队列或独立任务服务。Admin Console 和 Public Analytics 使用服务端渲染，浏览器资源嵌入可执行文件。

主要包职责：

- `cmd/visitortrace`：命令行入口和服务生命周期；
- `internal/store`：SQLite 模型、迁移、事务和查询；
- `internal/server`：HTTP API、嵌入脚本与管理/公开页面；
- `internal/maprender`：无外部运行时依赖的 SVG 地图渲染；
- `internal/backup`：一致性快照、归档校验与恢复；
- `internal/maintenance`：进程内定期维护和有界清理调度；
- `internal/geoip`：面向后端的本地 MMDB 查询，以及 DB-IP、MaxMind、IP2Location 到统一地理位置字段的映射。
- `internal/geoipupdate`：按后端下载、归档解包、校验、原子替换、热加载与回滚。
- `internal/selfupdate`：签名清单、候选验证、版本切换、启动确认与失败回滚。

## 开发校验

需要 Go 1.25 或更新版本：

```sh
go test ./...
go test -race ./...
go vet ./...
go mod verify
```

`make check` 执行常规测试和静态检查，`tools/preview-demo.sh` 提供带伪数据的本地人工预览环境。

只有修改 Public Analytics 交互前端或底图时才需要 Node.js。`web/analytics-entry.js` 使用 ECharts 的按需模块，`web/assets/world.geo.json` 与 SVG 底图来自同一锁定版本的 Natural Earth 数据。执行：

```sh
make frontend
```

该命令使用 `package-lock.json` 安装依赖，生成并提交 `internal/server/assets/analytics.js` 及预压缩的 `.br`、`.gz` 文件。Go 构建直接嵌入这些产物，生产服务器不需要 Node.js，也不会在请求时消耗 CPU 压缩脚本。交互增强失败时必须保留服务端统计、同日期范围 SVG 和基础趋势图。

## 数据库演进

迁移按版本顺序嵌入 `internal/store/migrations.go`，必须在事务内完成。服务启动会运行正向迁移，不提供向下迁移。涉及破坏性升级时，应先使用 `visitortrace backup` 创建可验证快照。

Pageview 写入事务同时保存逐条记录、访客窗口登记和持久化聚合。过期逐条记录不得反向修改已经形成的聚合。

采集请求先从已经通过 `Allowed Origin` 校验的 `Origin` 请求头提取规范化 `hostname`；内嵌 tracker 也会上报 `window.location.hostname`，两者不一致时服务端拒绝非空上报。每条 Pageview Record 都保存 hostname，并新增 hostname 聚合维度。访客登记的内部作用域为 `(Site、hostname、窗口、维度、访客)`，因此共享一个配置 Site 的不同域名会分别计 UV，即使它们产生了相同的 Visitor Digest。Migration 9 增加明细列和索引；迁移前的明细 hostname 为空，迁移前的聚合无法反推出域名维度。

`site_deduplication_rules` 以 Site 本地日期保存计数规则历史。修改周期时，事务会在下一本地日期 upsert 新规则；Pageview 按其本地日期选择最近已生效规则，并从该规则的生效日计算新窗口锚点。已有 `visitor_registrations.window_end` 保持原值，仅可能延后临时登记的清理，不会改变新规则下的计数。

公开和后台聚合查询共用 Site 本地日期边界。公开查询必须先检查发布状态，且不读取 Path 维度；后台查询要求管理员认证，可在 Site 未公开时读取 Path 聚合。前端 JSON 只由服务端已经裁定的聚合结果生成，不包含逐条记录、原始 IP 或 Visitor Digest。

Hostname 聚合不属于敏感数据，Public Analytics 和 Admin Analytics 都会展示。管理员访问明细筛选和 CSV 导出包含已保存的 hostname；聚合导出支持 `hostname` 维度。

`serve` 启动一个轻量级维护循环，启动时和每小时调用同一套清理逻辑。`visitortrace maintenance` 为人工诊断和外部调度提供等价入口。每个删除事务限制批量大小；`operation_status` 保存最近一次维护状态，供运行状态页面读取。

管理员密码以 Argon2id 哈希保存。后台修改和 `visitortrace password reset` 都在同一事务中更新凭据并撤销全部 Session。Session 记录保存最近密码验证时间，供更新等高风险操作实施短时重新验证。

Site 数据清空会在一个事务中先关闭采集和公开展示，再删除逐条记录、访客登记、聚合和地图位置，并轮换 Site HMAC 密钥。永久删除同样先关闭对外能力，再依靠外键级联清理。HTTP 层还要求管理员密码与 Site ID 双重确认。

GeoIP Resolver 每次只打开配置中的一个后端。`provider_dbip.go`、`provider_maxmind.go` 和 `provider_ip2location.go` 分别负责数据库校验、Schema 映射、归因信息和官方更新 profile；通用 Resolver 只负责打开 MMDB、选择 adapter 和返回统一的 `geoip.Location` 字段。DB-IP 与 MaxMind 使用嵌套的 country/subdivision/city/location 结构；IP2Location 主要使用扁平的 country/region/city/latitude/longitude 结构，同时兼容其 MaxMind 结构 MMDB 变体。

更新器在启动时和每 24 小时运行。provider profile 为 DB-IP/IP2Location 选择按自然月判断，为 MaxMind 选择 72 小时新鲜度。MaxMind 官方请求使用 Basic Authentication，IP2Location 通过查询参数接收 Download Token；认证与官方主机绑定，不会发送给自定义镜像。更新器按内容识别原始 MMDB、gzip MMDB、tar.gz 和 ZIP，归档中必须且只能包含一个 MMDB。`{YYYY-MM}` 使用 UTC 月份展开。下载输入限制为 1 GiB，展开后的 MMDB 限制为 2 GiB。配置 SHA-256 sidecar 时先校验下载容器，随后始终执行 MMDB 完整验证。候选文件与目标位于同一文件系统，通过重命名激活，上一版保留为 `.previous`。服务通过互斥保护的 Resolver 热交换，避免关闭仍在查询的旧句柄。

DB-IP City Lite 的 `city.names` 可能同时包含城市和区、街道等限定词，而 Lite Schema 不提供可用于选择行政层级的 feature code。`normalizeDBIPCity` 是 `provider_dbip.go` 的私有逻辑：它清理中国地名中的下级限定词，并对北京、上海、天津、重庆使用上级直辖市名称。通用 Resolver 不执行这项清理，因此 MaxMind 和 IP2Location 的城市名称在结构映射后保持原值。

Pageview Record 列表使用 `(occurred_at, id)` 复合游标，查询顺序由服务端固定，游标携带规范化筛选指纹，不能跨筛选复用。每页最多 200 条。明细和聚合导出直接遍历 SQLite Rows 并写入 `encoding/csv`，不生成临时导出文件；敏感导出只挂载在管理员认证路由并设置 `Cache-Control: no-store`。

`internal/operations` 聚合只读运行信息：构建元数据、进程运行时长、SQLite/WAL/SHM 占用、文件系统空间、GeoIP 与本地备份状态，以及 `operation_status` 中的任务结果。Linux 使用 `statfs`，其他平台明确返回不可用而不伪造数值。后台手动操作复用 CLI 相同的备份、维护和 GeoIP 实现，并受管理员 Session 与 CSRF 保护。GeoIP 和维护 Runner 使用进程级互斥避免重复执行。

Public Map 的有效 Options `CacheKey` 与 Site ID 组成缓存键。内存 LRU 的 TTL 为 5 分钟，每个 Site 最多 256 个变体，全局 SVG body 最多 32 MiB；同键 miss 通过进程内 flight 合并。Map Preset 更新、Site 清空和删除会递增 Site 缓存代次，避免正在渲染的旧结果在失效后重新写回。

## 发布签名与自更新

项目使用 `.github/workflows/ci.yml` 校验 Go 测试、Race Detector、Vet、依赖完整性，以及前端锁定依赖、漏洞和提交产物的一致性。普通 CI 只有仓库只读权限。

首次正式发布前，在离线或受保护环境生成一次项目发布密钥：

```sh
go run ./tools/release-manifest keygen \
  --private-key .release-secrets/update.ed25519 \
  --public-key .release-secrets/update.pub
```

`.release-secrets` 已被 Git 忽略。私钥必须保存在仓库外的受保护备份中，不得提交或放到应用服务器。丢失私钥后无法为现有客户端发布可信更新；替换公钥也不能让已安装版本自动信任新密钥。

在 GitHub 仓库中创建名为 `release` 的 Environment，建议配置 Required reviewers，然后添加：

- Environment secret `UPDATE_PRIVATE_KEY`：私钥文件中的单行 Base64 内容；
- Environment variable `UPDATE_PUBLIC_KEY`：公钥文件中的单行 Base64 内容。

Release 工作流只把公钥嵌入正式二进制。私钥仅暴露给受 Environment 保护的签名步骤；构建和签名 job 只有 `contents: read`，独立发布 job 才有 `contents: write`。

本地仍可构建带自更新能力的正式二进制：

```sh
make build \
  VERSION=0.2.0 \
  UPDATE_PUBLIC_KEY="BASE64_RAW_ED25519_PUBLIC_KEY"
```

发布使用语义化版本标签。确认 `main` 的 CI 通过后创建并推送标签：

```sh
git tag -a v0.1.1 -m "VisitorTrace v0.1.1"
git push origin v0.1.1
```

`.github/workflows/release.yml` 会重新执行测试，使用 `CGO_ENABLED=0` 构建 `visitortrace-<版本>-linux-amd64` 和 `visitortrace-<版本>-linux-arm64`，并将版本、Commit、构建时间、数据库 Schema 和公钥嵌入二进制。工作流从实际文件生成 SHA-256、大小和更新清单，再用最终公钥验签。每个 Release 还会包含未经修改的 GPL 文本和由同一标签 Commit 生成的源码归档，所有文件都纳入 `checksums.txt`。验证完成的文件先上传到草稿 Release，全部成功后才公开；任务重跑可以刷新同一草稿，不能覆盖已经发布的版本。含 `-` 的 SemVer 标签会发布为 prerelease，不会替换稳定版 `releases/latest`。

本地生成清单时使用：

```sh
go run ./tools/release-manifest generate \
  --version 0.1.1 \
  --published-at 2026-07-23T00:00:00Z \
  --asset linux-amd64=dist/visitortrace-0.1.1-linux-amd64 \
  --asset linux-arm64=dist/visitortrace-0.1.1-linux-arm64 \
  --output manifest.unsigned.json

go run ./tools/release-manifest sign \
  --private-key .release-secrets/update.ed25519 \
  --manifest manifest.unsigned.json \
  --output manifest.json

go run ./tools/release-manifest verify \
  --public-key "BASE64_RAW_ED25519_PUBLIC_KEY" \
  --manifest manifest.json
```

签名载荷是去除 `signature` 后的固定 Go 结构 JSON；Assets map 由 `encoding/json` 按键排序。`manifest.json` 使用相对资产 URL，整组 Release 文件可原样同步到国内镜像。发布后不得原地修改清单或二进制。

更新器将候选文件放入 `data_dir/releases/v<version>/visitortrace`，并通过 `releases/current` 相对符号链接切换。`data_dir/.update-pending.json` 记录旧/新目标、升级前快照和启动次数。`serve` 在打开 SQLite 前登记启动；第三次失败会使用不执行正向迁移的恢复路径还原快照。只有 HTTP 服务的 SQLite、Schema 和 GeoIP 就绪后，pending 状态才会完成。后台发起更新需要在当前请求中重新验证密码，然后在响应完成后请求进程优雅退出。

## 备份格式

`.vtbackup` 是 ZIP 容器，包含：

- `visitortrace.sqlite3`：通过 SQLite `VACUUM INTO` 创建的一致性快照；
- `config.json`：备份时的受保护配置副本；
- `manifest.json`：格式版本、应用版本、Schema 版本、文件大小和 SHA-256。

归档旁的 `.sha256` 校验整个容器。恢复在激活数据库前验证两层校验和、执行 SQLite 完整性检查、运行正向迁移并撤销全部管理员 Session。

## 贡献许可

VisitorTrace 采用 GPL-3.0。贡献内容必须能够按与该许可证兼容的条款提供；复制的代码或资产必须保留原始声明。所有打包进入项目的第三方组件和数据都应记录在[第三方声明](../THIRD_PARTY_NOTICES.zh-CN.md)中。依赖项位于公开仓库并不代表其许可证必然兼容。

## 发布文档边界

中英文 README、用户指南、部署指南和开发指南随仓库发布。项目根目录的 `ARCHITECTURE.md`、`CONTEXT.md`，以及 `docs/adr`、`docs/research`、`docs/internal` 是本地设计资料，受 `.gitignore` 排除，不应提交到 GitHub。
