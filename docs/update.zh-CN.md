# 访迹更新历史

[英文版](./update.md)

本文记录各个 VisitorTrace 发布版本面向用户的变更。

## 尚未发布

- 暂无未发布变更。

## 0.2.2 - 2026-08-29

- 增加完整日语界面支持，覆盖管理员后台、Public Analytics、交互式 Widget 和 Site 默认语言设置，并支持显式选择与浏览器语言偏好。语言开关统一按中文、日语、英文排列。SQLite Schema 从 11 升级到 12，使已有 Site 可以保存 `ja` 作为公开页语言。

## 0.2.1 - 2026-08-28

- Site 新建与设置新增明确的“限时 / 无限”逐条记录保留策略。无限模式会记住最近的 1–90 天数值，便于切回限时；维护时只跳过明细删除，并明确提示磁盘占用会持续增长。
- 增加可交互的 Map Preset URL 参数说明。支持鼠标悬浮、点击、键盘聚焦和触屏打开，列出全部可覆写参数，并让示例查询实时跟随尚未保存的表单值更新。
- 公共分析页增加“管理员后台”快捷入口，直接打开当前 Site 的管理页面；未登录时沿用现有管理员登录流程。
- SQLite Schema 从 10 升级到 11；已有 Site 迁移后继续使用原天数的限时保留，不改变现有行为。

## 0.2.0 - 2026-07-27

- 所有 Widget（包括管理员 Map Preset 预览）统一跳转 Public Analytics，不再进入管理路由；同时禁用锚点原生拖放，并把移动操作判定为取消点击，使拖动尝试结束后悬停和点击立即恢复。
- 交互式 Widget 精简为圆点悬停/聚焦/触屏详情与点击进入分析页；移除平移、缩放、Reset、手势状态和 Widget 控件注册表，高级地图交互仅保留在完整的 Public Analytics 与 Admin Analytics 中。
- Site 接入区精简为四段按行为命名、可直接使用的代码，并移除重复的资源地址。Map Preset 的 Widget 预览不再经过滚动容器干预，可保持正式 Widget 的完整交互。Site 详情仅加载 8 条最近记录，以六个核心字段紧凑展示，并可带当前 Site 筛选直接进入完整访问明细。
- ECharts 地图的 Reset 控件改用更小的定位样式图标；Image fallback 预览按配置的像素尺寸原样显示，不再缩放；Site 专属危险操作移除重复的 Site ID 确认，同时保留管理员密码验证。
- Map Preset 新增轻量交互式 Widget 与 Image fallback 双模式预览，并支持私有 Site 的管理员专用渲染；ECharts 分析地图开放可扩展控件注册表，便于后续增加操作。
- 将分离式接入从静态 Map SVG 升级为独立的 `map.js` Loader。该 Loader 不执行采集，而是挂载与一体式模式相同的懒加载、响应式交互 Widget，可与 `tracker.js` 组合、独立延迟加载，或单独用于只读展示。
- 将一体式 JavaScript Widget 的静态图片替换为独立渲染、受 sandbox 隔离的 iframe。城市点位支持鼠标、键盘和触屏查看详情及 GeoIP 署名；点击地图会打开公开分析，同时 Pageview 采集仍由父页面 Loader 完成。
- SVG 地图改为只定义一次世界底图，并在白令海峡接缝两侧复用，不再序列化两份相同路径。iframe 会把 Map Preset 的实际尺寸发送给 Loader，使响应式嵌入保持地图投影比例。
- 本版本继续使用 SQLite Schema 10。已有 0.1.4 安装可通过签名自更新或同 Schema 的 systemd 手动更新脚本升级。

## 0.1.4 - 2026-07-27

- 站点接入区的独立多列内容改为顶部对齐，避免较短的代码资源在较高列旁被纵向摊开。
- 每个 Site 管理页直接增加全部历史的可交互访客地图，复用 Public Analytics 的地图交互，私有 Site 仍可查看，并保留管理员认证的 SVG 作为无 JavaScript 回退。
- 新增不依赖 JavaScript 的一体式 Image Widget：有效图片加载会在同一端点记录访问并返回 SVG 地图。该接口会按 Site Allowed Origins 校验 Referer，支持固定 `path` 与全部地图 URL 覆写参数，使用 no-store 客户端缓存，并在来源无法验证或识别为机器人时退化为只绘图。
- 管理后台的逐条记录、精确筛选和 CSV 导出新增采集方式（`js` 或 `image`）。SQLite Schema 从 9 升级到 10，已有明细会自动标记为 `js`。

## 0.1.3 - 2026-07-24

- 将管理员设置重组为服务配置、GeoIP 数据维护、账户安全和应用更新；配置变更现在通过一次密码验证原子保存并只触发一次受管重启，同时统一记录和处理 systemd 可写路径及权限问题。
- 管理后台自更新新增本地签名文件方式，支持清单与平台二进制校验、Schema 检查、备份和回滚；并补充与同 Schema systemd 更新脚本、反向代理上传限制相关的说明。
- 重构站点管理流程，使用独立的 `/admin/sites` 列表，并重新组织站点设置、接入资源、Map Preset、记录、分析和危险操作。一体式与分离式 API 现在对称提供代码片段、资源地址和复制控件。
- 修正多列表单中仅部分字段带补充说明时的控件错位，包括站点统计与保留、管理员密码设置。
- 将新建站点和站点设置中的 IANA 时区字段改为浏览器原生的可搜索下拉框；已有 Pageview 的站点会明确说明统计时区的锁定原因和安全解锁方式。

## 0.1.2 - 2026-07-23

- Site 页面新增地理信息刷新操作，可使用当前 GeoIP 数据库更新保留期内的 Pageview 明细，并在同一事务中重算对应的国家、地区和城市 PV/UV；已经没有明细的历史日期保持不变。
- 在管理员设置中增加图形化 GeoIP 管理：选择后端、自动/仅手动策略、官方源或自定义镜像、凭证替换与清除且不回显秘密、更新状态、立即检查和强制下载。
- 增加 `visitortrace geoip query` 和 `scripts/query-mmdb.sh`，用于查询单个 IP 并输出格式化的原始 MMDB 元数据、命中网段和记录内容，同时明确表示未命中状态。
- 增加 `scripts/update-systemd-binary.sh`，用于从已经下载的本地二进制执行带校验的手动更新，包括升级前备份、原子版本切换、服务重启和可执行文件自动回滚。
- tracker 会记录 hostname；对于部署在多个域名上的同一 Site，UV 按 hostname 独立计算。hostname 可用于 Pageview Record、筛选、CSV 导出、Public Analytics、Admin Analytics 和聚合导出。
- 增加 DB-IP 中国城市标签规范化：在 City Lite 记录提供足够层级信息时移除区、街道等限定词。清理逻辑现为 DB-IP provider adapter 私有，不会作用于 MaxMind 或 IP2Location 记录。
- 将 DB-IP、MaxMind GeoLite2 City 和 IP2Location LITE DB11 作为平等的一等 GeoIP 后端，统一提供地理字段，并分别提供校验、归因、内置官方下载源、凭据处理和自动更新。更新器现可解包原始 MMDB、gzip MMDB、tar.gz 和 ZIP。
- 本版本将 SQLite Schema 从 8 升级到 9。已有安装会自动迁移；历史明细的 hostname 为空，历史聚合无法反推出 hostname。从 0.1.1 自动升级到 0.1.2 应使用签名自更新流程；离线 `update-systemd-binary.sh` 会按设计拒绝跨 Schema 更新。

## 0.1.1 - 2026-07-23

- 修复 Public Analytics 和 Admin Analytics 交互地图的变形问题，保持世界地图比例并使用白令海峡附近的固定边界。
- 修复跨越地图接缝的 GeoJSON 环，多边形不再在美国和俄罗斯位置产生贯穿全图的长条。
- 在分离式接入区域增加地图控件代码，提供带 `loading="lazy"` 的 `<img>` 示例，并与 Tracker 代码一样提供独立复制按钮。
- 增加 `scripts/install-systemd.sh`，一键完成服务账户创建、受保护目录配置、初始化、自更新稳定路径初始化、systemd 单元创建和服务启动。
- 更新 systemd 加固示例，使 `ProtectSystem=strict` 下的后台 Base URL 设置可以持久化。
- 同步更新中英文部署指南和用户指南。

本版本不修改 SQLite Schema，也不要求执行数据迁移。

## 0.1.0 - 2026-07-23

- VisitorTrace / 访迹首次公开发布。
- 提供 Pageview 采集、访客合并周期、持久化聚合、逐条记录保留、本地 GeoIP 查询、SVG 地图、Public Analytics、Admin Console、Site 管理、备份、维护和签名自更新等功能。
