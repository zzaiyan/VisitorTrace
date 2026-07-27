# 访迹（VisitorTrace）用户指南

[英文版](./user-guide.md)

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

正式版本在 GitHub Releases 提供带版本号且无需 Go 环境的 Linux 二进制。根据服务器架构选择 `visitortrace-<版本>-linux-amd64` 或 `visitortrace-<版本>-linux-arm64`，同时下载 `checksums.txt`。例如，校验 `0.1.4` 的 AMD64 版本：

```sh
grep ' visitortrace-0.1.4-linux-amd64$' checksums.txt | sha256sum -c -
install -Dm700 visitortrace-0.1.4-linux-amd64 "$HOME/.local/bin/visitortrace"
"$HOME/.local/bin/visitortrace" version
```

实际安装时将 `0.1.4` 替换为下载的版本号；ARM64 服务器使用 `linux-arm64` 文件名。每个 Release 还会提供 GPL 文本和来自同一标签的对应源码归档。Release 清单另有 Ed25519 签名，供内置自更新器验证；手工安装时仍应先核对 `checksums.txt`。使用发布版时，后续示例中的 `./bin/visitortrace` 对应 `$HOME/.local/bin/visitortrace`。

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

时区字段使用可搜索的 IANA 时区下拉框，输入城市或时区片段即可筛选浏览器支持的候选项。首条 Pageview 写入后会锁定统计时区，因为修改时区会改变历史聚合日期；如需更换，应先清空 Site 数据。

修改访客合并周期后，新规则在 Site 时区的下一个午夜生效，并以该日期作为新周期锚点；当前窗口不会在保存设置的瞬间改变，已经完成的聚合也不会重算。管理员聚合趋势会标出规则生效日。

## 启动服务

```sh
./bin/visitortrace serve \
  --config "$HOME/.config/visitortrace/config.json"
```

使用 systemd 运行服务，并使用宝塔管理 Nginx/SSL 时，请继续阅读[部署指南](./deployment.zh-CN.md)。

默认监听 `127.0.0.1:8790`。生产环境应由反向代理终止 HTTPS，并且只有显式配置的 `trusted_proxies` 才能提供转发客户端 IP 和 HTTPS 协议信息。

### Base URL 与子路径路由

在“管理员设置 > 服务配置 > 公开访问”中填写服务的公开地址，例如 `https://stats.example.com/visitortrace`。该值用于生成 Site 接入代码和公开链接，其中的路径也会成为应用路由前缀。根路径部署时可以留空。

配置 Base URL 后，所有应用路由都会带上此前缀，例如 `/visitortrace/admin/login`、`/visitortrace/health/live` 和 `/visitortrace/embed/tracker.js`。反向代理必须保留此前缀。该设置也能避免接入代码退回到 `127.0.0.1` 等本机地址。

## 后台与公开页面

- Admin Console：`/admin/login`
- 站点管理：`/admin/sites`
- Public Analytics：`/public/<SITE-ID>/analytics`
- Public Map：`/api/v1/sites/<SITE-ID>/map.svg`
- 健康检查：`/health/live`、`/health/ready`

如果 Base URL 包含路径，请在以上各路由前加上该路径。

Admin Console 可管理 Site 设置、Pageview 接收和公开状态、Map Preset，并查看原始 IP、路径、浏览器、操作系统和 Visitor Digest。Public Analytics 只展示聚合统计。

站点管理使用独立的 `/admin/sites` 列表，新建 Site 是列表中的单独操作。每个 Site 页面按全部历史的可交互访客地图、接入代码、站点设置、Map Preset、最近记录和危险操作组织；地图读取管理员权限下的聚合数据，即使 Site 已关闭公开展示仍可使用。站点设置进一步分为基本信息与 Allowed Origins、统计与保留规则、采集与公开控制。精简后的最近记录最多读取 8 条，仅保留时间、采集方式、访问页面、位置、客户端和原始 IP；点击“查看该站点全部记录”会进入完整版访问明细，并自动选中当前 Site。

Public Analytics 的日期范围会同时作用于 PV/UV 摘要、趋势、地理地图和各维度表格。支持今天、7/30/90 天、全部及自定义日期；启用 JavaScript 时，趋势图可缩放，地图可平移和缩放，左下角 Reset 控件可恢复初始位置与比例。脚本不可用时会自动保留同一日期范围的服务端 SVG 地图和基础趋势图。

Site 管理页的“聚合分析”使用相同日期范围和交互组件，并额外展示 Path 聚合；该页面受管理员 Session 保护，即使 Site 已关闭公开展示仍可使用。Path 聚合不会出现在 Public Analytics。

后台默认使用简体中文，语言开关会把中文或英文偏好保存在浏览器中。每个 Site 可把 Public Analytics 默认语言设为“自动”、简体中文或英文；自动模式读取访客的 `Accept-Language`。公开页的语言开关以及 `lang=zh-CN`、`lang=en` URL 参数可覆写默认值。Map Preset 中的 SVG 标题和 PV/UV 标签不随界面语言自动改写。

管理总览顶部显示应用版本与运行时长、SQLite 版本/Schema/占用、可用磁盘空间、GeoIP 文件和最近本地备份。任务表记录最近一次备份、维护清理和 GeoIP 更新的结果；低磁盘、超过 48 小时没有新备份、超过 35 天的 GeoIP、清理停滞或任务失败会显示告警。页面可直接触发立即备份、立即清理和 GeoIP 检查。

“管理员设置 > 服务配置”把公开 Base URL、GeoIP 后端、更新策略、官方源/自定义镜像、可选校验地址和后端凭证合并为一个表单。已保存的秘密不会回显到浏览器：凭证输入框留空表示保持原值，显式勾选清除项才会删除。一次管理员密码验证会原子保存所有变更，并只请求一次受管重启。“GeoIP 数据维护”是独立的操作区，用于查看数据库状态与最近任务摘要、立即检查，或在“仅手动”策略下强制重新下载。

### 访问明细与导出

后台“访问明细”页面显示全部 Site 的 Pageview Record，默认每页 100 条，可选择 50 或 200 条。页面使用与当前筛选绑定的游标向较早或较新记录翻页，避免数据持续写入时页码偏移。

可组合使用以下精确筛选：Site、采集方式（`js` 或 `image`）、访问 hostname、UTC 起止时间、规范化路径、原始 IP、Visitor Digest、国家代码、地区代码、城市、浏览器和操作系统。页面时间按对应 Site 时区显示，悬浮可查看 UTC 时间。

“导出当前筛选 CSV”会流式输出所有符合条件的记录，不受当前页大小影响。文件同时包含 UTC 时间和 Site 本地时间，以及采集方式、经纬度、原始 IP 和 Visitor Digest 等全部明细字段。以 `=`、`+`、`-` 或 `@` 开头的文本会增加前导单引号，避免电子表格将外部数据解释为公式。

聚合导出要求选择一个 Site，可按整体、hostname、路径、国家、地区、城市、浏览器或操作系统分别导出，并可限制 Site 本地日期范围。

同一个配置 Site 如果用于多个域名，每个 hostname 都会作为独立聚合行显示。Pageview Record 也会保存 tracker 上报且服务端从 Allowed Origin 确认的 hostname，因此不同域名上的同一访客会分别计入各自的 UV。

Site 页面中的“刷新地理信息”会使用当前 GeoIP 数据库重新查询全部仍在保留期内的 Pageview Record。操作会更新逐条记录，并对这些记录覆盖的每个 Site 本地日期重算国家、地区和城市 PV/UV；已经没有任何明细的早期日期保持不变，整体、hostname、路径、浏览器和操作系统聚合也不会改变。有效 IP 如果在新数据库中未命中，会清除旧地理信息并计入未知国家；格式无效的已保存 IP 会被跳过并保留原地理信息。只有 GeoIP 数据库已加载时才能执行该操作，事务成功后地图缓存会立即失效。

## 网站接入

推荐的一体式交互 Widget 会记录 Pageview，并挂载一个受 sandbox 隔离的 iframe：

```html
<script async src="https://stats.example.com/embed/widget.js?site_id=SITE_ID"></script>
```

子路径部署时，使用后台显示的 Base URL：

```html
<script async src="https://stats.example.com/visitortrace/embed/widget.js?site_id=SITE_ID"></script>
```

Loader 从父页面发送 Pageview，因此能保留当前 hostname、path、Origin 校验和 localStorage Visitor ID，随后懒加载独立渲染的 iframe。iframe 使用 Map Preset 或 URL 中的 `w`/`h` 覆写，并把实际尺寸发送给 Loader，使响应式布局保持地图投影比例。鼠标悬浮或键盘聚焦城市点位时会显示 PV、UV 和 GeoIP 署名；地图支持指针拖拽、滚轮缩放和触屏平移/双指缩放，左下角 Reset 控件可恢复世界视图。在纯触屏设备上，第一次点击点位显示详情，第二次打开公开分析；未发生拖拽时点击地图其他位置会直接打开公开分析。

`GET /embed/widget?site_id=SITE_ID` 是 Loader 使用的 iframe 文档。该端点只读，不会重复记录 Pageview；应用也可以把它与分离式 Tracker 配合，自行控制 iframe 的放置。文档仅包含内联 SVG、CSS 和少量原生脚本，不加载 ECharts、字体、图标或 CDN 资源。

如果接入网站启用了 Content Security Policy，需要在 `script-src`、`connect-src` 和 `frame-src` 中允许 VisitorTrace Base URL；Image Widget 还需要 `img-src`。例如：

```text
script-src 'self' https://stats.example.com;
connect-src 'self' https://stats.example.com;
frame-src https://stats.example.com;
img-src 'self' https://stats.example.com;
```

一体式 Image Widget 是不依赖 JavaScript 的兼容变体，适用于静态 HTML、允许远程图片的 Markdown 渲染器或接入限制较多的发布系统：

```html
<a href="https://stats.example.com/public/SITE_ID/analytics"
   target="_blank" rel="noopener">
  <img src="https://stats.example.com/embed/widget.svg?site_id=SITE_ID"
       width="300" height="168"
       alt="Visitor map"
       title="VisitorTrace Public Map | IP geolocation data">
</a>
```

每个有效图片请求会先记录一条 Pageview，再返回 SVG。请求 Referer 必须能还原为该 Site 的某个 Allowed Origin；Referer 缺失、格式错误或来源不允许时仍会返回地图，但不会计数。由于浏览器不会运行 VisitorTrace 代码，该变体无法创建 localStorage Visitor ID，UV 会退化为使用客户端 IP 与 User-Agent 的组合指纹。路径默认取自 Referer；现代浏览器的跨域 Referrer Policy 通常只发送 Origin，因此默认路径经常是 `/`。固定页面可显式加入规范化路径，例如 `&path=%2Fresearch`（写在 HTML 属性中时使用 `&amp;path=%2Fresearch`）。

VisitorTrace 为 Image Widget 返回 `Cache-Control: private, no-store`，但上游 Markdown/图片代理、预取器和缓存仍可能隐藏访客 IP、替换 Referer，或把多次浏览合并为一次请求。因此 GitHub Camo 等服务可以正常显示地图，却不适合精确统计 PV/UV。普通网页应优先使用交互式 Widget；如果需要每次页面加载都计数，不要为 Image Widget 增加 `loading="lazy"`。

分离式 Tracker 只记录 Pageview：

```html
<script async src="https://stats.example.com/embed/tracker.js?site_id=SITE_ID"></script>
```

Tracker 会上报当前页面 hostname。服务端以已经通过 Origin 校验的 hostname 为权威值，因此共享一个 Site 的不同域名会在 hostname 统计和独立访客计数中保持隔离。

分离式接入区域同时提供只负责显示的交互式地图 Loader。需要分别控制采集和渲染生命周期时，可将其与 Tracker 组合，也可以等地图进入视口后再加载：

```html
<script async src="https://stats.example.com/embed/map.js?site_id=SITE_ID"></script>
```

`map.js` 不记录 Pageview。它与一体式 Widget 挂载相同的 sandbox iframe，因此完整保留懒加载、Map Preset 与 URL 覆写、响应式比例更新、鼠标/触屏点位详情和公开分析跳转。无需显示地图时只加载 `tracker.js`，是占用最低的方案；只加载 `map.js` 则可展示公开统计而不采集宿主页面。

Site 页面为四段可直接使用的代码提供一键复制，分别是“交互式地图 + 采集”“图片地图 + 采集”“仅采集”和“仅显示交互式地图”。资源地址已经包含在完整代码中，因此不再重复显示；需要编程调用时，可从本节文档查看各原始端点形式。

## Map Preset 与 URL 覆写

后台支持尺寸、标题、PV/UV 标签、字体大小、显示内容、背景、陆地、边界、文字、标记颜色和标记指标。实时预览可在交互式 Widget 与 Image fallback 之间切换，无需先保存。Widget 预览与正式接入一致：悬停或聚焦圆点显示详情，触屏设备先展示详情，点击后进入 Public Analytics。拖动尝试既不会移动地图或触发跳转，也不会影响后续悬停和点击。Widget 有意不提供平移、缩放和 Reset；完整的 Public Analytics 与 Admin Analytics 地图继续保留这些高级交互。私有 Site 通过管理员专用端点获得相同的 Widget 行为，但只有开启公开展示后，其 Public Analytics 链接才可访问。宽度和高度旁的自动按钮会根据当前标题、统计栏和字体大小计算保持世界地图投影比例所需的另一维度。

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

VisitorTrace 同时只启用一个本地 MMDB 后端，并把不同数据库结构统一映射为国家代码/名称、地区代码/名称、城市、纬度和经度。通过 `geoip_provider` 选择后端：

| 后端 | 数据库格式 | 默认更新方式 |
| --- | --- | --- |
| `dbip` | DB-IP City Lite（`.mmdb.gz`） | 每月自动更新，无需凭据 |
| `maxmind` | MaxMind GeoLite2 City（`.tar.gz`） | 自动更新，需要 Account ID 和 License Key |
| `ip2location` | IP2Location LITE DB11 MMDB（`.zip`） | 每月自动更新，需要 Download Token |

默认后端是 `dbip`。三种后端使用同一套自动更新、校验和原子激活流程。MaxMind 可使用账户后台提供的凭据初始化：

```sh
visitortrace init \
  --data-dir /var/lib/visitortrace \
  --config /etc/visitortrace/config.json \
  --geoip-provider maxmind \
  --maxmind-account-id ACCOUNT_ID \
  --maxmind-license-key LICENSE_KEY
```

MaxMind 官方下载使用 HTTP Basic Authentication。IP2Location 使用账户后台的 Download Token：

```sh
visitortrace init \
  --data-dir /var/lib/visitortrace \
  --config /etc/visitortrace/config.json \
  --geoip-provider ip2location \
  --ip2location-token DOWNLOAD_TOKEN
```

对应的受保护配置字段为 `maxmind_account_id`、`maxmind_license_key` 和 `ip2location_download_token`。三个内置源为：

```text
DB-IP:       https://download.db-ip.com/free/dbip-city-lite-{YYYY-MM}.mmdb.gz
MaxMind:     https://download.maxmind.com/geoip/databases/GeoLite2-City/download?suffix=tar.gz
IP2Location: https://www.ip2location.com/download?file=DB11LITEMMDB
```

IP2Location 会在账户 Download 页面给出准确的下载 code。内置的 LITE DB11 MMDB code 为 `DB11LITEMMDB`；若账户页面显示其他 code，可使用页面给出的 URL 覆写 `geoip_update_url`。更新器会跟随 MaxMind 和 IP2Location 使用的 HTTPS 重定向。

VisitorTrace 在启动时检查，并每 24 小时再次检查。DB-IP 与 IP2Location 按自然月判断新鲜度，MaxMind 在本地文件超过 72 小时后检查。更新器按文件签名识别原始 MMDB、gzip MMDB、tar.gz 和 ZIP。下载完成后会限制下载与解压后的文件大小、验证完整 MMDB 搜索树和数据区、校验当前后端对应的数据库结构，再原子替换当前文件并热加载。上一版保存在 `<geoip_path>.previous`，激活失败时自动回滚。

可人工检查并更新：

```sh
./bin/visitortrace geoip update \
  --config "$HOME/.config/visitortrace/config.json"
```

使用 `--force` 可忽略当前后端的新鲜度策略重新下载。命令行更新发生在另一个进程中，若服务正在运行，更新后需要通过 systemd 重启服务；服务内置的自动更新会直接热加载。

需要诊断某个 IP 为什么显示成特定城市时，可以查询原始 MMDB 记录：

```sh
# 使用配置文件中的 geoip_path。
./scripts/query-mmdb.sh --binary ./bin/visitortrace \
  --config "$HOME/.config/visitortrace/config.json" \
  1.2.3.4

# 或直接指定 MMDB，绕过配置文件。
./scripts/query-mmdb.sh --binary ./bin/visitortrace \
  --mmdb /path/to/geoip.mmdb \
  1.2.3.4
```

命令输出格式化 JSON，包括数据库元数据、命中的 CIDR、`found` 状态和未修改的 MMDB `record` 字段树。它不会应用 VisitorTrace 的城市级标签规范化。地址未命中时返回 `found: false` 和 `null` 的 `record`。在已部署的服务器上，也可以直接使用已安装的可执行文件，例如 `sudo -u visitortrace /var/lib/visitortrace/releases/current/visitortrace geoip query --config /etc/visitortrace/config.json 1.2.3.4`。

更新器可以使用提供任一受支持容器和可选 SHA-256 sidecar 的 HTTPS 镜像。后端凭据只会附加到对应后端的准确官方主机名，不会发送给自定义镜像。使用私有或国内镜像时可显式配置：

```json
{
  "geoip_update": "automatic",
  "geoip_update_url": "https://mirror.example.com/dbip-city-lite-{YYYY-MM}.mmdb.gz",
  "geoip_checksum_url": "https://mirror.example.com/dbip-city-lite-{YYYY-MM}.mmdb.gz.sha256"
}
```

`geoip_checksum_url` 可省略；配置后会在解压前校验下载容器的 SHA-256。远程源必须使用 HTTPS，本机回环测试地址例外。设置 `"geoip_update": "disabled"` 可关闭下载。读取旧配置时，`"monthly"` 会迁移为 `"automatic"`。

账户凭据属于敏感信息。配置文件权限应保持为 `0600`；备份中包含配置文件，也需要限制访问；不要把凭据直接写入更新 URL。

已有安装不需要重新执行 `init` 即可切换后端：进入“管理员设置 > 服务配置”，按需填写新凭证并保存合并配置，服务会使用新后端重启。其他后端已经保存的凭证会继续保留，除非显式清除。

GeoIP 不可用时，服务仍可启动并显示已有聚合与底图，但 `/health/ready` 返回不可用，新 Pageview 不会获得地理位置。地图悬浮提示、后台预览和 Public Analytics 会展示当前后端的归因信息。DB-IP 中国城市标签规范化只作用于 DB-IP 记录；MaxMind 和 IP2Location 的城市名称按数据库原值映射。

## 备份与恢复

创建一致性 SQLite 快照和配置归档：

```sh
./bin/visitortrace backup \
  --config "$HOME/.config/visitortrace/config.json"
```

备份默认保存在配置中的 `backup_dir`，未显式配置时为数据目录下的 `backups`。每个 `.vtbackup` 归档都有配套的 `.sha256` 文件，归档内的数据库和配置也分别记录 SHA-256。命令会执行 SQLite 完整性检查，并默认只保留最近三份；可使用 `--output` 和 `--keep` 覆写。

恢复前必须先通过 systemd 停止 VisitorTrace：

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

每个 Site 管理页底部提供两项危险操作。由于当前页面已限定到该 Site，两项操作均只要求输入当前管理员密码：

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

将 systemd 服务的启动路径改为该稳定路径，并保持“进程退出后自动重启”。宝塔只负责 Nginx 和 SSL，VisitorTrace 不依赖宝塔 API。

随后可在服务器检查或应用更新：

```sh
visitortrace update check --config "$HOME/.config/visitortrace/config.json"
visitortrace update apply --config "$HOME/.config/visitortrace/config.json"
```

如果已经手动下载了新版二进制，可以使用仓库脚本在不联网的情况下完成更新：

```sh
sudo ./scripts/update-systemd-binary.sh \
  --binary ./visitortrace-0.1.4-linux-amd64 \
  --checksum-file ./checksums.txt
```

脚本会在提供校验文件时验证本地二进制，运行候选版本的 `doctor --upgrade-check`，创建带校验的升级前备份，原子切换稳定版本链接并重启 systemd 服务。如果新进程无法保持运行，会恢复到旧版本；如果服务原本处于停止状态，脚本也会保持停止。为避免自动回滚时混用可执行文件和数据库版本，本地更新要求数据库 Schema 不变；改变 Schema 的版本应使用签名更新器。默认参数适用于部署指南中的目录；自定义安装可使用 `--user`、`--data-dir`、`--config` 或 `--service-name`。

“管理员设置”提供两种签名更新方式。“在线更新”从配置的清单地址获取清单和平台资产；“本地文件更新”接收同一 Release 中的 `manifest.json` 和界面所示平台的二进制，适合服务器无法连接发布站点的场景。两种方式都会在本次请求中重新验证管理员密码。与 `update-systemd-binary.sh` 不同，后台本地文件方式使用完整签名更新器，可以安装改变数据库 Schema 的版本。

任一方式准备好候选版本后，当前进程都会优雅退出，由进程管理器从稳定路径拉起新版本。反向代理必须允许本地文件上传；部署指南使用 `client_max_body_size 210m`，以覆盖签名清单规定的 200 MiB 资产上限和 multipart 开销。

更新流程会依次验证 Ed25519 清单签名、平台资产大小和 SHA-256、候选版本/Schema 身份，并运行候选二进制的 `doctor --upgrade-check`。全部通过后才创建升级前数据库快照、写入 pending 状态并原子切换 `current`。新版本达到 ready 后确认更新并保留最近两个旧版本；连续三次未能达到 ready 时，会恢复升级前数据库并切回旧版本。

正式发布构建必须嵌入项目发布公钥。未嵌入公钥的开发构建会禁用两种更新方式。更新清单地址默认为 GitHub Release，也可在受保护配置中改为国内镜像：

```json
{
  "update_manifest_url": "https://mirror.example.com/visitortrace/manifest.json"
}
```

镜像不能替换签名信任根；无论下载地址如何配置，清单都必须通过二进制内嵌公钥验证。

## 许可证

VisitorTrace 采用 [GNU 通用公共许可证第 3 版](../LICENSE)发布。第三方组件和数据继续适用[第三方声明](../THIRD_PARTY_NOTICES.zh-CN.md)所列的各自条款。具体权利与义务以许可证原文为准，而不是以本指南为准。

## 当前状态

当前版本已经实现此前确定的首版功能，包括 Pageview 采集与聚合、规则生效历史、自动清理、GeoIP 自动更新、带有界缓存的 SVG 地图、双语交互分析、管理员数据与运行状态、密码和 Site 生命周期、备份恢复，以及签名验证的一键自更新。
