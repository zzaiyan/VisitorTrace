# VisitorTrace · 访迹开发指南

[English](./development.en.md)

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
- `internal/geoip`：本地 MMDB 查询。
- `internal/geoipupdate`：按月下载、校验、原子替换、热加载与回滚。

## 开发校验

需要 Go 1.25 或更新版本：

```sh
go test ./...
go test -race ./...
go vet ./...
go mod verify
```

`make check` 执行常规测试和静态检查，`tools/preview-demo.sh` 提供带伪数据的本地人工预览环境。

## 数据库演进

迁移按版本顺序嵌入 `internal/store/migrations.go`，必须在事务内完成。服务启动会运行正向迁移，不提供向下迁移。涉及破坏性升级时，应先使用 `visitortrace backup` 创建可验证快照。

Pageview 写入事务同时保存逐条记录、访客窗口登记和持久化聚合。过期逐条记录不得反向修改已经形成的聚合。

`serve` 启动一个轻量级维护循环，启动时和每小时调用同一套清理逻辑。`visitortrace maintenance` 为人工诊断和外部调度提供等价入口。每个删除事务限制批量大小；`operation_status` 保存最近一次维护状态，供运行状态页面读取。

管理员密码以 Argon2id 哈希保存。后台修改和 `visitortrace password reset` 都在同一事务中更新凭据并撤销全部 Session。Session 记录保存最近密码验证时间，供更新等高风险操作实施短时重新验证。

Site 数据清空会在一个事务中先关闭采集和公开展示，再删除逐条记录、访客登记、聚合和地图位置，并轮换 Site HMAC 密钥。永久删除同样先关闭对外能力，再依靠外键级联清理。HTTP 层还要求管理员密码与 Site ID 双重确认。

GeoIP 更新器在启动时和每 24 小时运行，`{YYYY-MM}` 使用 UTC 月份展开。压缩输入限制为 1 GiB，展开后的 MMDB 限制为 2 GiB。配置 SHA-256 sidecar 时先校验下载容器；随后始终调用 MMDB 完整验证。候选文件与目标位于同一文件系统，通过重命名激活，上一版保留为 `.previous`。服务通过互斥保护的 Resolver 热交换，避免关闭仍在查询的旧句柄。

Pageview Record 列表使用 `(occurred_at, id)` 复合游标，查询顺序由服务端固定，游标携带规范化筛选指纹，不能跨筛选复用。每页最多 200 条。明细和聚合导出直接遍历 SQLite Rows 并写入 `encoding/csv`，不生成临时导出文件；敏感导出只挂载在管理员认证路由并设置 `Cache-Control: no-store`。

## 备份格式

`.vtbackup` 是 ZIP 容器，包含：

- `visitortrace.sqlite3`：通过 SQLite `VACUUM INTO` 创建的一致性快照；
- `config.json`：备份时的受保护配置副本；
- `manifest.json`：格式版本、应用版本、Schema 版本、文件大小和 SHA-256。

归档旁的 `.sha256` 校验整个容器。恢复在激活数据库前验证两层校验和、执行 SQLite 完整性检查、运行正向迁移并撤销全部管理员 Session。

## 发布文档边界

本指南和中英文用户指南随仓库发布。项目根目录的 `ARCHITECTURE.md`、`CONTEXT.md`，以及 `docs/adr`、`docs/research`、`docs/internal` 是本地设计资料，受 `.gitignore` 排除，不应提交到 GitHub。
