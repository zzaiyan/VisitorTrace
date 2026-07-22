# VisitorTrace · 访迹

轻量、自托管的访客地图与访问记录服务。

A tiny self-hosted visitor map and Pageview tracker.

[中文用户指南](./docs/user-guide.zh-CN.md) · [English User Guide](./docs/user-guide.en.md) · [中文开发指南](./docs/development.zh-CN.md) · [English Development Guide](./docs/development.en.md) · [Third-Party Notices](./THIRD_PARTY_NOTICES.md)

## 中文

VisitorTrace 面向个人主页、博客和其他小型网站，在一个 Go 服务中提供：

- Pageview 采集与 Site 隔离
- SQLite 逐条记录和持久化聚合
- 本地 GeoIP 查询与 SVG 访客地图
- Public Analytics
- 密码保护的 Admin Console
- 可筛选、分页和 CSV 导出的访问明细
- 备份、清理、GeoIP 与资源状态总览
- Ed25519 签名验证、自动备份和失败回滚的一键自更新
- 可实时预览的 Map Preset
- 一体式 Widget 和分离式 Tracker

### 快速预览

```sh
./tools/preview-demo.sh
```

脚本会创建临时数据库、生成带地理坐标的伪数据并启动本地服务。默认后台密码为 `VisitorTrace2026`，按 `Ctrl-C` 后自动清理。

### 构建

需要 Go 1.25 或更新版本。

```sh
make check
make build
./bin/visitortrace version
```

### 基本启动

```sh
./bin/visitortrace init \
  --data-dir "$HOME/.local/share/visitortrace" \
  --config "$HOME/.config/visitortrace/config.json" \
  --geoip /path/to/geoip.mmdb

./bin/visitortrace serve \
  --config "$HOME/.config/visitortrace/config.json"
```

完整配置、Site 创建、网站接入、地图参数和部署说明请参阅[中文用户指南](./docs/user-guide.zh-CN.md)。

## English

VisitorTrace is designed for personal homepages, blogs, and other small websites. One Go service provides:

- Site-isolated Pageview ingestion
- SQLite Pageview Records and durable aggregates
- Local GeoIP lookup and SVG visitor maps
- Public Analytics
- A password-protected Admin Console
- Filterable, cursor-paginated, CSV-exportable Pageview Records
- Backup, cleanup, GeoIP, and resource health overview
- One-click self-update with Ed25519 verification, automatic backup, and rollback
- Live Map Preset previews
- Integrated Widget and separated Tracker modes

### Quick Preview

```sh
./tools/preview-demo.sh
```

The launcher creates a temporary database, seeds geographically distributed fake data, and starts the local service. The default Admin password is `VisitorTrace2026`; pressing `Ctrl-C` removes the temporary data.

### Build

Go 1.25 or newer is required.

```sh
make check
make build
./bin/visitortrace version
```

### Basic Startup

```sh
./bin/visitortrace init \
  --data-dir "$HOME/.local/share/visitortrace" \
  --config "$HOME/.config/visitortrace/config.json" \
  --geoip /path/to/geoip.mmdb

./bin/visitortrace serve \
  --config "$HOME/.config/visitortrace/config.json"
```

See the [English User Guide](./docs/user-guide.en.md) for complete configuration, Site creation, integration, map parameters, and deployment guidance.

## License Status

The project license has not been selected yet. Third-party data and notices are documented separately in [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md).
