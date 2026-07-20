# VisitorTrace · 访迹 用户指南

[English](./user-guide.en.md)

VisitorTrace 是面向个人主页、博客和其他小型网站的轻量级自托管访客地图与 Pageview 记录服务。生产环境只需要一个 Go 可执行文件、SQLite 数据库和本地 GeoIP MMDB。

## 快速预览

仓库提供一次性测试环境，自动创建 Site 并写入带经纬度的伪数据：

```sh
./tools/preview-demo.sh
```

默认后台地址为 `http://127.0.0.1:8790/admin/login`，密码为 `VisitorTrace2026`。按 `Ctrl-C` 停止后，临时数据库会自动删除。

端口冲突时：

```sh
VISITORTRACE_LISTEN=127.0.0.1:8791 ./tools/preview-demo.sh
```

## 构建

需要 Go 1.25 或更新版本。

```sh
make check
make build
./bin/visitortrace version
```

## 初始化

```sh
./bin/visitortrace init \
  --data-dir "$HOME/.local/share/visitortrace" \
  --config "$HOME/.config/visitortrace/config.json" \
  --geoip /path/to/geoip.mmdb
```

初始化过程会要求输入至少 8 个字符的管理员密码。配置文件、SQLite 数据和 Site HMAC 密钥应始终位于受保护的持久化目录中。

## 创建 Site

```sh
./bin/visitortrace site create \
  --config "$HOME/.config/visitortrace/config.json" \
  --name "Personal homepage" \
  --origin "https://example.com"
```

每个 Site 都有独立的 Site ID、Allowed Origins、统计时区、访客合并周期、逐条记录保留期和 Map Preset。

## 启动服务

```sh
./bin/visitortrace serve \
  --config "$HOME/.config/visitortrace/config.json"
```

默认监听 `127.0.0.1:8790`。生产环境应由反向代理终止 HTTPS，并且只有显式配置的 `trusted_proxies` 才能提供转发客户端 IP 和 HTTPS 协议信息。

## 后台与公开页面

- Admin Console：`/admin/login`
- Public Analytics：`/public/<SITE-ID>/analytics`
- Public Map：`/api/v1/sites/<SITE-ID>/map.svg`
- 健康检查：`/health/live`、`/health/ready`

Admin Console 可管理 Site 设置、Pageview 接收和公开状态、Map Preset，并查看原始 IP、路径、浏览器、操作系统和 Visitor Digest。Public Analytics 只展示聚合统计。

## 网站接入

一体式 Widget 会同时记录 Pageview 并插入地图：

```html
<script async src="https://stats.example.com/embed/widget.js?site_id=SITE_ID"></script>
```

分离式 Tracker 只记录 Pageview：

```html
<script async src="https://stats.example.com/embed/tracker.js?site_id=SITE_ID"></script>
```

地图可单独作为图片加载：

```html
<img loading="lazy"
     src="https://stats.example.com/api/v1/sites/SITE_ID/map.svg"
     alt="Visitor map">
```

## Map Preset 与 URL 覆写

后台支持尺寸、标题、PV/UV 标签、字体大小、显示内容、背景、陆地、边界、文字、标记颜色和标记指标。宽度和高度旁的自动按钮会根据当前标题、统计栏和字体大小计算保持世界地图投影比例所需的另一维度。

底图不包含南极洲，左右接缝位于白令海峡附近的 `170°W`，避免使用 `180°` 经线作为页面边界。

公开地图支持以下参数：

```text
w h title pv_label uv_label show fs bg land border text marker metric
```

颜色使用六位十六进制值，透明背景使用：

```text
bg=transparent
```

URL 参数只覆写当前请求，不会改变保存的 Map Preset。

## GeoIP

生产环境需要有效的 DB-IP City Lite MMDB。GeoIP 不可用时，服务仍可启动并显示已有聚合与底图，但 `/health/ready` 返回不可用，新 Pageview 不会获得地理位置。

## 当前状态

当前版本已经实现 Pageview 采集、聚合统计、SVG 地图、Public Analytics、管理员认证和 Map Preset。自动记录清理、备份恢复、GeoIP 自动更新、密码重置和一键自更新仍在后续里程碑中。
