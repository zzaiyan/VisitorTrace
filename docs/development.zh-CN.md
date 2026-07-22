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

## 备份格式

`.vtbackup` 是 ZIP 容器，包含：

- `visitortrace.sqlite3`：通过 SQLite `VACUUM INTO` 创建的一致性快照；
- `config.json`：备份时的受保护配置副本；
- `manifest.json`：格式版本、应用版本、Schema 版本、文件大小和 SHA-256。

归档旁的 `.sha256` 校验整个容器。恢复在激活数据库前验证两层校验和、执行 SQLite 完整性检查、运行正向迁移并撤销全部管理员 Session。

## 发布文档边界

本指南和中英文用户指南随仓库发布。项目根目录的 `ARCHITECTURE.md`、`CONTEXT.md`，以及 `docs/adr`、`docs/research`、`docs/internal` 是本地设计资料，受 `.gitignore` 排除，不应提交到 GitHub。
