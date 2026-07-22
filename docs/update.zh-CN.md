# 访迹更新历史

[英文版](./update.md)

本文记录各个 VisitorTrace 发布版本面向用户的变更。

## 尚未发布

- 增加 `visitortrace geoip query` 和 `scripts/query-mmdb.sh`，用于查询单个 IP 并输出格式化的原始 MMDB 元数据、命中网段和记录内容，同时明确表示未命中状态。
- 增加 `scripts/update-systemd-binary.sh`，用于从已经下载的本地二进制执行带校验的手动更新，包括升级前备份、原子版本切换、服务重启和可执行文件自动回滚。
- tracker 会记录 hostname；对于部署在多个域名上的同一 Site，UV 按 hostname 独立计算。hostname 可用于 Pageview Record、筛选、CSV 导出、Public Analytics、Admin Analytics 和聚合导出。
- 增加 DB-IP 中国城市标签规范化：在 City Lite 记录提供足够层级信息时移除区、街道等限定词。
- 本次尚未发布的改动将 SQLite Schema 升级到 9。已有安装会自动迁移；历史明细的 hostname 为空，历史聚合无法反推出 hostname。

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
