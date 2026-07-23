# 访迹更新历史

[英文版](./update.md)

本文记录各个 VisitorTrace 发布版本面向用户的变更。

## 尚未发布

- 修正多列表单中仅部分字段带补充说明时的控件错位，包括站点统计与保留、管理员密码设置。
- 补齐 `/admin/sites` 站点管理路由，将站点列表与新建页面、运行状态总览分离，并让删除后的落点返回站点列表。
- 单站点页按接入代码、站点设置、Map Preset、最近记录和危险操作重新组织；站点设置统一划分为基本信息、统计与保留、采集与公开三组。
- 对齐一体式与分离式接入结构：Widget、Tracker 和 Map SVG 均提供嵌入代码、资源地址以及融入对应栏位的复制控件。
- 将管理员设置重组为服务配置、GeoIP 数据维护、账户安全和应用更新；公开 Base URL 与全部 GeoIP 下载源/凭证字段现在通过一次密码确认原子保存，并只触发一次受管重启。
- 为 GeoIP 任务摘要补齐完整样式，给内嵌 CSS/JavaScript 增加内容 revision 以避免升级后命中旧缓存，并让配置保存错误直接提示 systemd `ReadWritePaths` 所需目录。
- 配置目录和文件已经具有正确保护权限时，不再重复执行 `chmod`；不符合要求时仍严格收紧到 `0700`/`0600`。
- 管理员设置的自更新新增本地文件方式，可同时上传同一 Release 的签名 `manifest.json` 和对应平台二进制，并保留 Ed25519、大小、SHA-256、候选身份、Schema、备份和回滚检查。
- 补充本地签名更新流程、它与仅支持相同 Schema 的 systemd 手动更新脚本之间的区别，以及反向代理上传大小配置。

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
