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
./bin/visitortrace doctor --config "$HOME/.config/visitortrace/config.json"
```

## 安装发布版

正式版本在 GitHub Releases 提供无需 Go 环境的 Linux 二进制。根据服务器架构选择 `visitortrace-linux-amd64` 或 `visitortrace-linux-arm64`，同时下载 `checksums.txt`，在同一目录校验：

```sh
grep ' visitortrace-linux-amd64$' checksums.txt | sha256sum -c -
install -Dm700 visitortrace-linux-amd64 "$HOME/.local/bin/visitortrace"
"$HOME/.local/bin/visitortrace" version
```

ARM64 服务器把命令中的文件名替换为 `visitortrace-linux-arm64`。Release 清单另有 Ed25519 签名，供内置自更新器验证；手工安装时仍应先核对 `checksums.txt`。使用发布版时，后续示例中的 `./bin/visitortrace` 对应 `$HOME/.local/bin/visitortrace`。

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

修改访客合并周期后，新规则在 Site 时区的下一个午夜生效，并以该日期作为新周期锚点；当前窗口不会在保存设置的瞬间改变，已经完成的聚合也不会重算。管理员聚合趋势会标出规则生效日。

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

Public Analytics 的日期范围会同时作用于 PV/UV 摘要、趋势、地理地图和各维度表格。支持今天、7/30/90 天、全部及自定义日期；启用 JavaScript 时，趋势图可缩放，地图可平移和缩放。脚本不可用时会自动保留同一日期范围的服务端 SVG 地图和基础趋势图。

Site 管理页的“聚合分析”使用相同日期范围和交互组件，并额外展示 Path 聚合；该页面受管理员 Session 保护，即使 Site 已关闭公开展示仍可使用。Path 聚合不会出现在 Public Analytics。

后台默认使用简体中文，语言开关会把中文或英文偏好保存在浏览器中。每个 Site 可把 Public Analytics 默认语言设为“自动”、简体中文或英文；自动模式读取访客的 `Accept-Language`。公开页的语言开关以及 `lang=zh-CN`、`lang=en` URL 参数可覆写默认值。Map Preset 中的 SVG 标题和 PV/UV 标签不随界面语言自动改写。

管理总览顶部显示应用版本与运行时长、SQLite 版本/Schema/占用、可用磁盘空间、GeoIP 文件和最近本地备份。任务表记录最近一次备份、维护清理和 GeoIP 更新的结果；低磁盘、超过 48 小时没有新备份、超过 35 天的 GeoIP、清理停滞或任务失败会显示告警。页面可直接触发立即备份、立即清理和 GeoIP 检查。

### 访问明细与导出

后台“访问明细”页面显示全部 Site 的 Pageview Record，默认每页 100 条，可选择 50 或 200 条。页面使用与当前筛选绑定的游标向较早或较新记录翻页，避免数据持续写入时页码偏移。

可组合使用以下精确筛选：Site、UTC 起止时间、规范化路径、原始 IP、Visitor Digest、国家代码、地区代码、城市、浏览器和操作系统。页面时间按对应 Site 时区显示，悬浮可查看 UTC 时间。

“导出当前筛选 CSV”会流式输出所有符合条件的记录，不受当前页大小影响。文件同时包含 UTC 时间和 Site 本地时间，以及经纬度、原始 IP 和 Visitor Digest 等全部明细字段。以 `=`、`+`、`-` 或 `@` 开头的文本会增加前导单引号，避免电子表格将外部数据解释为公式。

聚合导出要求选择一个 Site，可按整体、路径、国家、地区、城市、浏览器或操作系统分别导出，并可限制 Site 本地日期范围。

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

等价参数会归一化为同一个 SVG 缓存项。公开地图返回 `ETag` 并缓存 5 分钟，因此新 Pageview 最多需要约 5 分钟反映到已有地图 URL。服务限制每个 Site 最多 256 个变体、全局最多 32 MiB，并合并同一变体的并发首次渲染。

## GeoIP

生产环境需要有效的 DB-IP City Lite MMDB。默认配置会在启动时检查数据库，并每天检查一次；当本地文件缺失、无效或不是当月版本时，下载：

```text
https://download.db-ip.com/free/dbip-city-lite-{YYYY-MM}.mmdb.gz
```

下载完成后，VisitorTrace 会限制压缩包和解压后文件大小、验证完整 MMDB 搜索树与数据区、确认数据库类型为 City/Location，再原子替换当前文件并热加载。上一版保存在 `<geoip_path>.previous`，激活失败时自动回滚。

可人工检查并更新：

```sh
./bin/visitortrace geoip update \
  --config "$HOME/.config/visitortrace/config.json"
```

使用 `--force` 可忽略当月文件状态重新下载。命令行更新发生在另一个进程中，若服务正在运行，更新后需要由宝塔或其他进程管理器重启服务；服务内置的自动更新会直接热加载。

国内镜像可在配置文件中设置：

```json
{
  "geoip_update": "monthly",
  "geoip_update_url": "https://mirror.example.com/dbip-city-lite-{YYYY-MM}.mmdb.gz",
  "geoip_checksum_url": "https://mirror.example.com/dbip-city-lite-{YYYY-MM}.mmdb.gz.sha256"
}
```

`geoip_checksum_url` 可省略；配置后会在解压前校验压缩文件 SHA-256。远程源必须使用 HTTPS，本机回环测试地址例外。设置 `"geoip_update": "disabled"` 可关闭下载。

GeoIP 不可用时，服务仍可启动并显示已有聚合与底图，但 `/health/ready` 返回不可用，新 Pageview 不会获得地理位置。DB-IP City Lite 每月更新并采用 CC BY 4.0，VisitorTrace 在地图悬浮提示、后台预览和 Public Analytics 中保留 DB-IP 归因链接。

## 备份与恢复

创建一致性 SQLite 快照和配置归档：

```sh
./bin/visitortrace backup \
  --config "$HOME/.config/visitortrace/config.json"
```

备份默认保存在配置中的 `backup_dir`，未显式配置时为数据目录下的 `backups`。每个 `.vtbackup` 归档都有配套的 `.sha256` 文件，归档内的数据库和配置也分别记录 SHA-256。命令会执行 SQLite 完整性检查，并默认只保留最近三份；可使用 `--output` 和 `--keep` 覆写。

恢复前必须先在宝塔或其他进程管理器中停止 VisitorTrace：

```sh
./bin/visitortrace restore \
  --config "$HOME/.config/visitortrace/config.json" \
  --from /path/to/visitortrace-20260722T033000.000000000Z.vtbackup \
  --confirm
```

恢复命令会先在 `backup_dir/pre-restore` 中创建当前数据库的安全快照，然后验证归档外校验和、归档内文件校验和与数据库完整性。恢复的数据库会迁移到当前版本并撤销所有管理员 Session。归档包含初始化时的配置副本，但常规恢复不会覆盖当前配置文件。

如需定时备份，可由系统计划任务每天调用 `visitortrace backup`；服务本身不依赖特定面板或定时任务实现。

## 自动维护与保留期

服务启动后会立即执行一次维护，此后每小时检查一次。维护任务按 Site 删除：

- 实际年龄超过“逐条记录保留期”的 Pageview Record；
- 已经结束的访客合并窗口登记；
- 过期或超过 12 小时未活动的管理员 Session。

删除采用有上限的小批次事务，避免长时间阻塞采集。每日聚合和地图统计不会随逐条记录过期而删除。缩短保留期会让新超出范围的记录在下一轮维护中被清理，延长保留期不能恢复已经删除的记录。

可人工运行同一维护流程：

```sh
./bin/visitortrace maintenance \
  --config "$HOME/.config/visitortrace/config.json"
```

## 管理员密码

登录后台后可在“管理员设置”中输入当前密码并设置新密码。密码长度为 8 至 128 个字符；修改成功后全部管理员 Session 都会失效，需要重新登录。

忘记密码时，可在服务器上重置：

```sh
./bin/visitortrace password reset \
  --config "$HOME/.config/visitortrace/config.json"
```

命令会交互式读取并确认新密码。自动化环境可通过权限为 `0600` 的 `--password-file` 提供密码；重置同样会撤销全部 Session。

## Site 清空与删除

每个 Site 管理页底部提供两项危险操作，均要求输入完整 Site ID 和当前管理员密码：

- “清空 Site 数据”删除 Pageview Record、全部聚合和地图位置，保留 Site 设置，轮换 HMAC 密钥并解除统计时区锁；采集和公开展示会保持关闭，检查设置后再手动开启。
- “永久删除 Site”删除 Site 及其全部数据和设置，原 Site ID 不会重新分配。

两项操作都不可撤销，执行前应先创建备份。

## 一键自更新

自更新使用并列版本目录和稳定符号链接，不覆盖正在运行的文件。首次启用时运行：

```sh
./bin/visitortrace update bootstrap \
  --config "$HOME/.config/visitortrace/config.json"
```

命令会输出类似以下稳定执行路径：

```text
$HOME/.local/share/visitortrace/releases/current/visitortrace
```

将宝塔或其他进程管理器的启动命令改为该稳定路径，并保持“进程退出后自动重启”。这只是通用进程管理契约，VisitorTrace 不依赖宝塔 API。

随后可在服务器检查或应用更新：

```sh
visitortrace update check --config "$HOME/.config/visitortrace/config.json"
visitortrace update apply --config "$HOME/.config/visitortrace/config.json"
```

也可在“管理员设置”输入当前密码后点击“检查并更新”。后台方式要求密码在本次请求中重新验证；候选版本准备完成后，当前进程会优雅退出，由进程管理器从稳定路径拉起新版本。

更新流程会依次验证 Ed25519 清单签名、平台资产大小和 SHA-256、候选版本/Schema 身份，并运行候选二进制的 `doctor --upgrade-check`。全部通过后才创建升级前数据库快照、写入 pending 状态并原子切换 `current`。新版本达到 ready 后确认更新并保留最近两个旧版本；连续三次未能达到 ready 时，会恢复升级前数据库并切回旧版本。

正式发布构建必须嵌入项目发布公钥。未嵌入公钥的开发构建会禁用更新按钮。更新清单地址默认为 GitHub Release，也可在受保护配置中改为国内镜像：

```json
{
  "update_manifest_url": "https://mirror.example.com/visitortrace/manifest.json"
}
```

镜像不能替换签名信任根；无论下载地址如何配置，清单都必须通过二进制内嵌公钥验证。

## 当前状态

当前版本已经实现此前确定的首版功能，包括 Pageview 采集与聚合、规则生效历史、自动清理、GeoIP 自动更新、带有界缓存的 SVG 地图、双语交互分析、管理员数据与运行状态、密码和 Site 生命周期、备份恢复，以及签名验证的一键自更新。
