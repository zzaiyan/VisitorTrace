# 访迹（VisitorTrace）

[![CI](https://github.com/zzaiyan/VisitorTrace/actions/workflows/ci.yml/badge.svg)](https://github.com/zzaiyan/VisitorTrace/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/zzaiyan/VisitorTrace?display_name=tag&sort=semver)](https://github.com/zzaiyan/VisitorTrace/releases)
[![License](https://img.shields.io/github/license/zzaiyan/VisitorTrace)](./LICENSE)

面向个人主页、博客和其他小型网站的轻量级自托管访客地图与访问记录服务。

[英文主页](./README.md) · [用户指南](./docs/user-guide.zh-CN.md) · [部署指南](./docs/deployment.zh-CN.md) · [开发指南](./docs/development.zh-CN.md) · [第三方声明](./THIRD_PARTY_NOTICES.zh-CN.md)

## 功能

- 按 Site 隔离的 Pageview 采集
- SQLite 逐条记录和持久化聚合
- 本地 GeoIP 查询与 SVG 访客地图
- 支持日期联动、趋势缩放和交互地图的公开分析页
- 简体中文和英文界面，以及按 Site 配置的公开页默认语言
- 密码保护的管理后台
- 可筛选、游标分页和 CSV 导出的访问明细
- 备份、清理、GeoIP 与资源状态总览
- 带 Ed25519 验签、自动备份和失败回滚的一键自更新
- 可实时预览的地图预设
- 一体式 Widget 和分离式 Tracker 两种接入模式

## 快速预览

```sh
./tools/preview-demo.sh
```

脚本会创建临时数据库、写入带地理坐标的伪数据并启动本地服务。默认后台密码为 `VisitorTrace2026`；按 `Ctrl-C` 停止后会清理临时数据。

## 构建

需要 Go 1.25 或更新版本。

```sh
make check
make build
./bin/visitortrace version
```

正式版本会在 [GitHub Releases](https://github.com/zzaiyan/VisitorTrace/releases) 提供 `visitortrace-<版本>-linux-amd64`、`visitortrace-<版本>-linux-arm64` 二进制、校验文件、许可证和对应源码归档。每个二进制还会内嵌版本、Commit、构建时间和数据库 Schema 信息，可通过 `visitortrace version` 查看。

## 基本启动

```sh
./bin/visitortrace init \
  --data-dir "$HOME/.local/share/visitortrace" \
  --config "$HOME/.config/visitortrace/config.json" \
  --geoip /path/to/geoip.mmdb

./bin/visitortrace serve \
  --config "$HOME/.config/visitortrace/config.json"
```

配置、Site 创建、网站接入和地图参数请参阅[用户指南](./docs/user-guide.zh-CN.md)；systemd 与宝塔 Nginx/SSL 配置见[部署指南](./docs/deployment.zh-CN.md)。

## 许可证

VisitorTrace 是采用 [GNU 通用公共许可证第 3 版](./LICENSE)发布的自由软件。第三方组件和数据继续适用各自的许可证，详见[第三方声明](./THIRD_PARTY_NOTICES.zh-CN.md)。
