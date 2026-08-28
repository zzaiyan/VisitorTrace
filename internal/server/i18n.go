package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	adminLanguageCookie  = "visitortrace_admin_language"
	publicLanguageCookie = "visitortrace_public_language"
)

var messages = map[string]map[string]string{
	"zh-CN": {
		"dashboard": "管理总览", "sites": "站点管理", "records": "访问明细", "settings": "管理员设置", "logout": "退出登录", "admin_navigation": "后台导航",
		"site_count": "个 Site", "add_site": "新增 Site", "manage_sites": "管理站点", "site_management_help": "集中查看各站点状态、统计与公开入口。", "operations_attention": "运行状态需要关注", "operations": "运行状态", "backup_now": "立即备份",
		"cleanup_now": "立即清理", "check_geoip": "检查 GeoIP", "version": "版本", "uptime": "运行时长", "started_at": "启动于", "available_disk": "可用磁盘",
		"total": "总计", "unavailable": "不可用", "awaiting_download": "等待下载", "latest_backup": "最近备份", "no_backup": "尚无备份",
		"create_recommended": "建议立即创建", "task": "任务", "status": "状态", "completed_at": "完成时间", "summary": "摘要", "running": "运行中",
		"no_task_runs": "后台任务尚无运行记录", "site_list": "站点列表", "receiving": "接收中", "paused": "已暂停", "public": "公开", "private": "未公开",
		"manage": "管理", "public_analytics": "公开分析", "admin_console": "管理员后台", "manage_site": "管理", "no_sites": "还没有 Site", "create_first_site": "创建第一个 Site",
		"aggregate_analytics": "聚合分析", "path_performance": "路径表现", "geographic_distribution": "地理分布", "back_to_site": "返回 Site",
		"new_site": "新增 Site", "cancel": "取消", "display_name": "显示名称", "origin_hint": "每行一个完整 Origin，例如 https://example.com", "iana_timezone": "IANA 时区", "iana_timezone_help": "输入时区名称可快速匹配，例如 Asia/Shanghai。", "iana_timezone_locked_help": "首条 Pageview 已写入，统计时区已锁定；清空 Site 数据后可重新设置。",
		"dedup_window": "访客合并周期", "record_retention": "逐条记录保留期", "day": "天", "create_site": "创建 Site", "retention_limited": "限时", "retention_unlimited": "无限", "retention_policy_help": "无限保留不会自动删除 Pageview 明细，磁盘占用会持续增长。",
		"dedup_change_help": "修改在下一个站点本地午夜生效；已完成聚合不会重算。", "counting_rule_changes": "计数规则变更", "effective_from": "生效于",
		"credentials_sessions": "服务配置、凭据与安全会话", "settings_page_help": "集中管理服务配置、数据维护、账户安全和版本更新。", "settings_navigation": "设置分区", "service_configuration": "服务配置", "geoip_operations": "GeoIP 数据维护", "account_security": "账户安全", "application_update": "应用更新",
		"service_configuration_help": "公开地址、GeoIP 后端、下载源和凭证会作为一组保存。", "managed_restart": "保存后重启", "public_access": "公开访问", "geoip_resolution": "地理定位", "provider_credentials_help": "凭证只写入受保护的服务配置，不会在页面中回显。", "apply_configuration": "应用全部配置变更", "apply_configuration_help": "一次验证、一次保存，并由进程管理器完成一次重启。", "save_configuration": "保存配置并重启", "configuration_saved": "服务配置已保存，服务即将重启。", "configuration_save_failed": "无法保存服务配置：%v。请确认 systemd 的 ReadWritePaths 包含 %s，并重启 VisitorTrace 服务。",
		"geoip_database_status": "GeoIP 数据状态", "geoip_status_help": "查看当前数据库与最近任务，并按需检查或重新下载。", "account_security_help": "修改管理员登录密码；此操作不需要重启服务。", "change_password": "修改密码", "current_password": "当前密码", "new_password": "新密码", "password_hint": "8 至 128 个字符",
		"base_url": "公开 Base URL", "base_url_help": "用于生成接入代码和公开链接；路径部分也会作为应用路由前缀。留空时使用当前请求地址。", "base_url_effective": "当前生效地址：", "copy": "复制", "copied": "已复制",
		"geoip_database": "GeoIP 数据库", "geoip_settings_help": "管理定位后端、下载源与更新策略。", "database_file": "数据库文件", "last_modified": "更新于", "update_mode": "更新策略",
		"automatic_updates": "自动更新", "manual_only": "仅手动", "last_update": "最近更新", "never_updated": "尚未执行", "manual_update": "手动更新", "manual_update_help": "检查当前版本，或忽略更新周期重新下载。",
		"check_now": "检查更新", "force_download": "强制下载", "geoip_provider": "定位后端", "download_source": "下载源", "official_source": "官方源", "custom_mirror": "自定义镜像",
		"official_source_url": "当前后端官方地址", "custom_download_url": "自定义下载地址", "checksum_url": "SHA-256 校验地址", "checksum_url_help": "可选；用于校验下载容器。", "credential_blank_keeps": "留空即保留已保存值。",
		"provider_credentials": "后端凭证", "saved_credential": "已保存；留空保持不变", "incomplete": "配置不完整", "maxmind_account_id": "Account ID", "maxmind_license_key": "License Key", "ip2location_token": "Download Token", "clear_maxmind_credentials": "清除已保存的 MaxMind 凭证",
		"clear_ip2location_token": "清除已保存的 IP2Location Token",
		"confirm_new_password":    "确认新密码", "update_password": "更新密码", "version_update": "版本更新", "current": "当前", "stable_path": "稳定执行路径",
		"signature_verification": "签名验证", "configured": "已配置", "not_configured": "未配置", "launch_mode": "启动方式", "stable_launch": "稳定路径", "needs_adjustment": "需要调整",
		"administrator_password": "管理员密码", "check_and_update": "检查并更新", "online_update": "在线更新", "online_update_help": "从已配置的发布清单地址获取并安装新版本。",
		"local_update": "本地文件更新", "local_update_help": "上传同一正式 Release 的签名清单与当前平台二进制。", "signed_manifest": "签名清单 manifest.json", "release_binary": "发布二进制", "install_local_update": "验证并更新",
		"sensitive_admin_only": "敏感数据，仅管理员可见", "export_filtered_csv": "导出当前筛选 CSV", "filters": "筛选条件", "clear_filters": "清除筛选",
		"all_sites": "全部 Site", "from_utc": "起始时间（UTC）", "to_utc": "结束时间（UTC）", "hostname": "访问域名", "hostnames": "访问域名", "path": "路径", "original_ip": "原始 IP", "site_label": "Site", "visitor_digest": "Visitor Digest",
		"country_code": "国家代码", "region_code": "地区代码", "per_page": "每页", "apply_filters": "应用筛选", "row_records": "逐条记录", "this_page": "本页",
		"items": "条", "time": "时间", "location": "位置", "system": "系统", "newer_records": "较新记录", "older_records": "较早记录",
		"no_matching_records": "没有符合条件的 Pageview Record", "export_aggregates": "导出聚合", "choose_site": "选择 Site", "dimension": "维度", "overall": "整体",
		"country": "国家", "region": "地区", "download_aggregate_csv": "下载聚合 CSV", "map_controls": "地图控制", "reset_map_view": "重置地图位置和缩放",
		"allowed_origins_count": "个 Allowed Origin", "integration_code": "接入代码", "cumulative_stats": "累计统计", "retention": "保留期", "map_preset": "地图预设",
		"url_overrides": "URL 参数可覆写", "url_override_title": "可覆写的地图参数", "url_override_help": "参数只作用于当前请求，不会改变已保存的 Map Preset。多个参数可组合使用。", "url_param_dimensions": "输出宽度和高度，范围分别为 160–1200 与 90–800。", "url_param_title": "覆写标题内容。", "url_param_labels": "覆写 PV 或 UV 统计标签。", "url_param_show": "控制显示 title、pv、uv，使用逗号分隔；传入 none 可隐藏全部。", "url_param_font": "设置字体大小，范围为 8–32。", "url_param_colors": "使用六位十六进制颜色值；bg 也支持 transparent。", "url_param_metric": "设置地图点的大小指标：pv 或 uv。", "close": "关闭", "preset_preview": "地图预设实时预览", "preview_mode": "预览模式", "interactive_widget_preview": "交互式 Widget", "image_fallback": "Image fallback", "width": "宽度", "height": "高度", "auto_width": "按当前选项自动计算宽度",
		"pv_label": "PV 标签", "uv_label": "UV 标签",
		"auto_height": "按当前选项自动计算高度", "title": "标题", "font_size": "字体大小", "marker_metric": "标记指标", "display_content": "显示内容",
		"transparent_background": "透明背景", "background": "背景色", "land": "陆地", "border": "边界", "text": "文字", "marker": "标记", "save_preset": "保存预设",
		"site_sections": "站点页面分区", "site_settings": "站点设置", "site_identity": "基本信息", "statistics_policy": "统计与保留", "access_publishing": "采集与公开", "allowed_origins": "Allowed Origins",
		"accept_pageviews": "接收 Pageview", "public_ingest_endpoint": "允许 Tracker 与 Widget 写入访问记录", "publish_data": "公开数据", "map_and_analytics": "开放地图接口与公开分析页",
		"save_settings": "保存站点设置", "combined_widget": "一体式接入", "separate_tracker": "分离式接入", "javascript_widget": "交互式地图 + 采集", "image_widget": "图片地图 + 采集", "recommended": "推荐", "interactive": "交互式", "compatibility_mode": "兼容模式", "image_widget_limitations": "无需 JavaScript；不创建浏览器 Visitor ID，路径默认取 Referer。图片代理、预取或缓存可能只展示地图而不计数；固定页面可在 URL 中加入 path 参数。", "tracker_resource": "仅采集", "map_resource": "仅显示交互式地图", "separated_widget_help": "只负责显示，不记录 Pageview；支持懒加载，并与一体式 Widget 共用悬浮、触屏、公开分析跳转和响应式尺寸行为。", "embed_code": "嵌入代码", "recent_records": "最近记录", "view_site_records": "查看该站点全部记录", "visited_page": "访问页面", "client": "客户端", "collection_method": "采集方式", "all_methods": "全部方式",
		"refresh_record_geoip": "刷新地理信息", "refresh_record_geoip_confirm": "使用当前 GeoIP 覆写保留期内的 Pageview 地理信息，并重算这些记录覆盖日期的地理聚合？没有明细的早期日期不会改变。", "geoip_unavailable": "GeoIP 数据库不可用",
		"no_pageview_records": "暂无 Pageview Record", "danger_zone": "危险操作", "irreversible_backup_first": "不可撤销，请先备份", "reset_site_data": "清空 Site 数据",
		"reset_site_help": "删除逐条记录、聚合和地图点，保留 Site 设置；同时关闭采集和公开展示。", "reset_data": "清空数据",
		"delete_site": "永久删除 Site", "delete_site_help": "删除 Site 及其全部记录、聚合、地图数据和设置，Site ID 不会再次分配。", "delete_permanently": "永久删除",
		"admin_login": "管理员登录", "login_console": "登录后台", "request_incomplete": "请求未完成", "return_admin": "返回后台", "switching_version": "正在切换版本",
		"update_ready": "已通过验证并完成准备", "service_restarting": "服务正在重启", "restart_help": "进程管理器重新拉起稳定执行路径后，后台将恢复可用。", "reconnect": "重新连接",
		"date_to": "至", "analytics_range": "统计范围", "today": "今天", "days": "天", "all": "全部", "all_history": "全部历史", "analytics_summary": "统计摘要",
		"cities": "城市", "countries_regions": "国家或地区", "visitor_map": "访客地图", "selected_range": "所选日期范围", "visit_trend": "访问趋势",
		"daily_visit_trend": "每日访问趋势", "no_data": "暂无数据", "active_dates": "个有数据日期", "code": "代码", "city": "城市",
		"browsers": "浏览器", "operating_systems": "操作系统", "type": "类型", "share": "占比", "start_date": "开始日期", "end_date": "结束日期",
		"query": "查询", "aggregate_only": "仅展示非敏感聚合数据", "pageviews": "浏览量", "unique_visitors": "独立访客", "visitors": "访客", "unknown": "未知",
		"public_language": "公开分析默认语言", "language_auto": "自动（跟随访客浏览器）",
		"flash_site": "Site 已创建。", "flash_settings": "站点设置已保存。", "flash_preset": "Map Preset 已保存。", "flash_reset": "Site 数据已清空；采集和公开展示保持关闭。",
		"flash_deleted": "Site 已永久删除。", "flash_backup": "备份已创建并通过完整性检查。", "flash_cleanup": "维护清理已完成。", "flash_geoip": "GeoIP 数据库已更新并热加载。",
		"flash_geoip_current": "GeoIP 数据库当前无需更新。", "flash_update_current": "VisitorTrace 已是最新版本。", "flash_record_geoip": "已处理 %d 条 Pageview（%d 条发生变化）：定位 %d 条，未命中 %d 条，跳过 %d 条无效 IP；已重算 %d 个日期的地理聚合。",
	},
	"en": {
		"dashboard": "Dashboard", "sites": "Sites", "records": "Pageview Records", "settings": "Administrator Settings", "logout": "Log out", "admin_navigation": "Admin navigation",
		"site_count": "Sites", "add_site": "Add Site", "manage_sites": "Manage Sites", "site_management_help": "Review each Site's state, totals, and public entry point.", "operations_attention": "Operations need attention", "operations": "Operations", "backup_now": "Back up now",
		"cleanup_now": "Clean up now", "check_geoip": "Check GeoIP", "version": "Version", "uptime": "Uptime", "started_at": "Started", "available_disk": "Available disk",
		"total": "Total", "unavailable": "Unavailable", "awaiting_download": "Awaiting download", "latest_backup": "Latest backup", "no_backup": "No backup",
		"create_recommended": "Create one now", "task": "Task", "status": "Status", "completed_at": "Completed", "summary": "Summary", "running": "Running",
		"no_task_runs": "No background task runs yet", "site_list": "Site list", "receiving": "Receiving", "paused": "Paused", "public": "Public", "private": "Private",
		"manage": "Manage", "public_analytics": "Public Analytics", "admin_console": "Admin Console", "manage_site": "Manage", "no_sites": "No Sites yet", "create_first_site": "Create the first Site",
		"aggregate_analytics": "Aggregate Analytics", "path_performance": "Path performance", "geographic_distribution": "Geographic distribution", "back_to_site": "Back to Site",
		"new_site": "Add Site", "cancel": "Cancel", "display_name": "Display name", "origin_hint": "One complete Origin per line, for example https://example.com", "iana_timezone": "IANA timezone", "iana_timezone_help": "Type a timezone name to search quickly, for example Asia/Shanghai.", "iana_timezone_locked_help": "The first Pageview has been recorded, so the statistics timezone is locked. Reset Site data to choose it again.",
		"dedup_window": "Visitor merge window", "record_retention": "Record retention", "day": "days", "create_site": "Create Site", "retention_limited": "Timed", "retention_unlimited": "Unlimited", "retention_policy_help": "Unlimited retention never deletes Pageview Records automatically, so disk usage will keep growing.",
		"dedup_change_help": "Changes take effect at the next Site-local midnight; completed aggregates are not recalculated.", "counting_rule_changes": "Counting rule changes", "effective_from": "Effective",
		"credentials_sessions": "Service configuration, credentials, and secure sessions", "settings_page_help": "Manage service configuration, data maintenance, account security, and application updates.", "settings_navigation": "Settings sections", "service_configuration": "Service configuration", "geoip_operations": "GeoIP data", "account_security": "Account security", "application_update": "Application update",
		"service_configuration_help": "The public URL, GeoIP provider, download source, and credentials are saved together.", "managed_restart": "Restarts after save", "public_access": "Public access", "geoip_resolution": "Geolocation", "provider_credentials_help": "Credentials are written only to the protected service configuration and are never echoed here.", "apply_configuration": "Apply all configuration changes", "apply_configuration_help": "Verify once, save once, and let the process supervisor perform one restart.", "save_configuration": "Save configuration and restart", "configuration_saved": "The service configuration was saved and the service will restart.", "configuration_save_failed": "Failed to save the service configuration: %v. Ensure systemd ReadWritePaths includes %s, then restart VisitorTrace.",
		"geoip_database_status": "GeoIP data status", "geoip_status_help": "Review the active database and latest task, then check or download again when needed.", "account_security_help": "Change the Administrator login password without restarting the service.", "change_password": "Change password", "current_password": "Current password", "new_password": "New password", "password_hint": "8 to 128 characters",
		"base_url": "Public Base URL", "base_url_help": "Used for integration code and public links; its path also becomes the application route prefix. Leave it empty to use the current request URL.", "base_url_effective": "Effective URL:", "copy": "Copy", "copied": "Copied",
		"geoip_database": "GeoIP database", "geoip_settings_help": "Manage the location provider, download source, and update policy.", "database_file": "Database file", "last_modified": "Updated", "update_mode": "Update policy",
		"automatic_updates": "Automatic", "manual_only": "Manual only", "last_update": "Latest update", "never_updated": "Not run yet", "manual_update": "Manual update", "manual_update_help": "Check the current version or download again regardless of the update interval.",
		"check_now": "Check now", "force_download": "Force download", "geoip_provider": "Location provider", "download_source": "Download source", "official_source": "Official source", "custom_mirror": "Custom mirror",
		"official_source_url": "Official URL for this provider", "custom_download_url": "Custom download URL", "checksum_url": "SHA-256 checksum URL", "checksum_url_help": "Optional; verifies the downloaded container.", "credential_blank_keeps": "Leave blank to retain the saved value.",
		"provider_credentials": "Provider credentials", "saved_credential": "Saved; leave blank to keep", "incomplete": "Incomplete", "maxmind_account_id": "Account ID", "maxmind_license_key": "License Key", "ip2location_token": "Download Token", "clear_maxmind_credentials": "Remove saved MaxMind credentials",
		"clear_ip2location_token": "Remove saved IP2Location token",
		"confirm_new_password":    "Confirm new password", "update_password": "Update password", "version_update": "Version update", "current": "Current", "stable_path": "Stable executable path",
		"signature_verification": "Signature verification", "configured": "Configured", "not_configured": "Not configured", "launch_mode": "Launch mode", "stable_launch": "Stable path", "needs_adjustment": "Needs adjustment",
		"administrator_password": "Administrator password", "check_and_update": "Check and update", "online_update": "Online update", "online_update_help": "Fetch and install a release from the configured manifest URL.",
		"local_update": "Local files", "local_update_help": "Upload the signed manifest and platform binary from the same official Release.", "signed_manifest": "Signed manifest.json", "release_binary": "Release binary", "install_local_update": "Verify and update",
		"sensitive_admin_only": "Sensitive data, visible to administrators only", "export_filtered_csv": "Export filtered CSV", "filters": "Filters", "clear_filters": "Clear filters",
		"all_sites": "All Sites", "from_utc": "Start time (UTC)", "to_utc": "End time (UTC)", "hostname": "Hostname", "hostnames": "Hostnames", "path": "Path", "original_ip": "Original IP", "site_label": "Site", "visitor_digest": "Visitor Digest",
		"country_code": "Country code", "region_code": "Region code", "per_page": "Per page", "apply_filters": "Apply filters", "row_records": "Individual records", "this_page": "This page",
		"items": "items", "time": "Time", "location": "Location", "system": "System", "newer_records": "Newer records", "older_records": "Older records",
		"no_matching_records": "No matching Pageview Records", "export_aggregates": "Export aggregates", "choose_site": "Choose Site", "dimension": "Dimension", "overall": "Overall",
		"country": "Country", "region": "Region", "download_aggregate_csv": "Download aggregate CSV", "map_controls": "Map controls", "reset_map_view": "Reset map position and zoom",
		"allowed_origins_count": "Allowed Origins", "integration_code": "Integration code", "cumulative_stats": "Cumulative statistics", "retention": "Retention", "map_preset": "Map Preset",
		"url_overrides": "URL parameters can override defaults", "url_override_title": "Overridable map parameters", "url_override_help": "Parameters affect only the current request and do not change the saved Map Preset. Multiple parameters can be combined.", "url_param_dimensions": "Output width and height, limited to 160–1200 and 90–800.", "url_param_title": "Override the title text.", "url_param_labels": "Override the Pageview or Unique Visitor labels.", "url_param_show": "Show title, pv, or uv as a comma-separated list; use none to hide all.", "url_param_font": "Set the font size from 8 to 32.", "url_param_colors": "Use six-digit hexadecimal colors; bg also accepts transparent.", "url_param_metric": "Set the marker size metric to pv or uv.", "close": "Close", "preset_preview": "Live Map Preset preview", "preview_mode": "Preview mode", "interactive_widget_preview": "Interactive Widget", "image_fallback": "Image fallback", "width": "Width", "height": "Height", "auto_width": "Calculate width from current options",
		"pv_label": "PV label", "uv_label": "UV label",
		"auto_height": "Calculate height from current options", "title": "Title", "font_size": "Font size", "marker_metric": "Marker metric", "display_content": "Displayed content",
		"transparent_background": "Transparent background", "background": "Background", "land": "Land", "border": "Borders", "text": "Text", "marker": "Markers", "save_preset": "Save preset",
		"site_sections": "Site page sections", "site_settings": "Site settings", "site_identity": "Identity", "statistics_policy": "Counting and retention", "access_publishing": "Collection and publishing", "allowed_origins": "Allowed Origins",
		"accept_pageviews": "Accept Pageviews", "public_ingest_endpoint": "Allow Tracker and Widget ingestion", "publish_data": "Publish data", "map_and_analytics": "Expose the map endpoint and Public Analytics",
		"save_settings": "Save Site settings", "combined_widget": "Integrated", "separate_tracker": "Separated", "javascript_widget": "Interactive map + tracking", "image_widget": "Image map + tracking", "recommended": "Recommended", "interactive": "Interactive", "compatibility_mode": "Compatibility", "image_widget_limitations": "No JavaScript required. It creates no browser Visitor ID and uses Referer for the default path. Image proxies, prefetching, or caches may render without counting; add a path parameter for a fixed page.", "tracker_resource": "Tracking only", "map_resource": "Interactive map only", "separated_widget_help": "Display only; it records no Pageview. It lazy-loads the same hover, touch, Public Analytics navigation, and responsive sizing experience as the Integrated Widget.", "embed_code": "Embed code", "recent_records": "Recent records", "view_site_records": "View all Site records", "visited_page": "Visited page", "client": "Client", "collection_method": "Collection method", "all_methods": "All methods",
		"refresh_record_geoip": "Refresh geography", "refresh_record_geoip_confirm": "Overwrite retained Pageview geography with the current GeoIP database and rebuild geographic aggregates for the dates those records cover? Earlier dates without records will not change.", "geoip_unavailable": "GeoIP database unavailable",
		"no_pageview_records": "No Pageview Records", "danger_zone": "Danger zone", "irreversible_backup_first": "Irreversible; create a backup first", "reset_site_data": "Reset Site data",
		"reset_site_help": "Delete individual records, aggregates, and map points while keeping Site settings; ingestion and public views are disabled.", "reset_data": "Reset data",
		"delete_site": "Permanently delete Site", "delete_site_help": "Delete the Site and all records, aggregates, map data, and settings. The Site ID will not be reused.", "delete_permanently": "Delete permanently",
		"admin_login": "Administrator login", "login_console": "Sign in", "request_incomplete": "Request not completed", "return_admin": "Return to Admin Console", "switching_version": "Switching version",
		"update_ready": "has been verified and prepared", "service_restarting": "Service is restarting", "restart_help": "The Admin Console will return after the process manager launches the stable executable path.", "reconnect": "Reconnect",
		"date_to": "to", "analytics_range": "Analytics range", "today": "Today", "days": "days", "all": "All", "all_history": "All history", "analytics_summary": "Analytics summary",
		"cities": "Cities", "countries_regions": "Countries or regions", "visitor_map": "Visitor map", "selected_range": "Selected date range", "visit_trend": "Visit trend",
		"daily_visit_trend": "Daily visit trend", "no_data": "No data", "active_dates": "dates with data", "code": "Code", "city": "City",
		"browsers": "Browsers", "operating_systems": "Operating systems", "type": "Type", "share": "Share", "start_date": "Start date", "end_date": "End date",
		"query": "Query", "aggregate_only": "Aggregate-only public view", "pageviews": "Pageviews", "unique_visitors": "Unique Visitors", "visitors": "Visitors", "unknown": "Unknown",
		"public_language": "Default Public Analytics language", "language_auto": "Automatic (visitor browser preference)",
		"flash_site": "Site created.", "flash_settings": "Site settings saved.", "flash_preset": "Map Preset saved.", "flash_reset": "Site data reset; ingestion and public views remain disabled.",
		"flash_deleted": "Site permanently deleted.", "flash_backup": "Backup created and verified.", "flash_cleanup": "Maintenance cleanup completed.", "flash_geoip": "GeoIP database updated and hot-loaded.",
		"flash_geoip_current": "The GeoIP database is already current.", "flash_update_current": "VisitorTrace is up to date.", "flash_record_geoip": "Processed %d Pageviews (%d changed): %d located, %d unmatched, %d invalid IPs skipped; geographic aggregates rebuilt for %d dates.",
	},
	"ja": {
		"dashboard": "ダッシュボード", "sites": "サイト管理", "records": "Pageview 記録", "settings": "管理者設定", "logout": "ログアウト", "admin_navigation": "管理者ナビゲーション",
		"site_count": "サイト", "add_site": "サイトを追加", "manage_sites": "サイトを管理", "site_management_help": "各サイトの状態、集計、公開ページを確認します。", "operations_attention": "要確認の運用項目", "operations": "運用状況", "backup_now": "今すぐバックアップ",
		"cleanup_now": "今すぐクリーンアップ", "check_geoip": "GeoIP を確認", "version": "バージョン", "uptime": "稼働時間", "started_at": "起動日時", "available_disk": "空きディスク容量",
		"total": "合計", "unavailable": "利用不可", "awaiting_download": "ダウンロード待ち", "latest_backup": "最新バックアップ", "no_backup": "バックアップなし",
		"create_recommended": "今すぐ作成", "task": "タスク", "status": "状態", "completed_at": "完了日時", "summary": "概要", "running": "実行中",
		"no_task_runs": "バックグラウンドタスクの実行履歴はありません", "site_list": "サイト一覧", "receiving": "受信中", "paused": "一時停止", "public": "公開", "private": "非公開",
		"manage": "管理", "public_analytics": "公開分析", "admin_console": "管理者コンソール", "manage_site": "管理", "no_sites": "サイトはまだありません", "create_first_site": "最初のサイトを作成",
		"aggregate_analytics": "集計分析", "path_performance": "パス別の状況", "geographic_distribution": "地理分布", "back_to_site": "サイトに戻る",
		"new_site": "サイトを追加", "cancel": "キャンセル", "display_name": "表示名", "origin_hint": "完全な Origin を1行に1つ入力（例: https://example.com）", "iana_timezone": "IANA タイムゾーン", "iana_timezone_help": "タイムゾーン名を入力して検索できます（例: Asia/Shanghai）。", "iana_timezone_locked_help": "最初の Pageview が記録されたため、統計タイムゾーンは固定されています。再設定するにはサイトデータをリセットしてください。",
		"dedup_window": "訪問者の統合期間", "record_retention": "詳細記録の保存期間", "day": "日", "create_site": "サイトを作成", "retention_limited": "期間指定", "retention_unlimited": "無期限", "retention_policy_help": "無期限にすると Pageview の詳細記録は自動削除されず、ディスク使用量が増え続けます。",
		"dedup_change_help": "変更は次のサイト現地時間の午前0時から有効です。完了済みの集計は再計算されません。", "counting_rule_changes": "集計ルールの変更", "effective_from": "有効開始",
		"credentials_sessions": "サービス設定、認証情報、安全なセッション", "settings_page_help": "サービス設定、データ管理、アカウント保護、アプリ更新を管理します。", "settings_navigation": "設定セクション", "service_configuration": "サービス設定", "geoip_operations": "GeoIP データ管理", "account_security": "アカウント保護", "application_update": "アプリケーション更新",
		"service_configuration_help": "公開 URL、GeoIP プロバイダー、ダウンロード元、認証情報をまとめて保存します。", "managed_restart": "保存後に再起動", "public_access": "公開アクセス", "geoip_resolution": "位置情報", "provider_credentials_help": "認証情報は保護されたサービス設定にのみ保存され、画面には表示されません。", "apply_configuration": "すべての設定変更を適用", "apply_configuration_help": "1回の認証と保存で、プロセスマネージャーが1回だけ再起動します。", "save_configuration": "設定を保存して再起動", "configuration_saved": "サービス設定を保存しました。サービスを再起動します。", "configuration_save_failed": "サービス設定を保存できませんでした: %v。systemd の ReadWritePaths に %s が含まれていることを確認し、VisitorTrace サービスを再起動してください。",
		"geoip_database_status": "GeoIP データの状態", "geoip_status_help": "現在のデータベースと最新タスクを確認し、必要に応じて確認または再ダウンロードします。", "account_security_help": "サービスを再起動せずに管理者ログインパスワードを変更します。", "change_password": "パスワードを変更", "current_password": "現在のパスワード", "new_password": "新しいパスワード", "password_hint": "8～128文字",
		"base_url": "公開 Base URL", "base_url_help": "接入コードと公開リンクの生成に使用します。パス部分はアプリのルートパスにもなります。空欄の場合は現在のリクエスト URL を使用します。", "base_url_effective": "現在有効な URL:", "copy": "コピー", "copied": "コピーしました",
		"geoip_database": "GeoIP データベース", "geoip_settings_help": "位置情報プロバイダー、ダウンロード元、更新ポリシーを管理します。", "database_file": "データベースファイル", "last_modified": "更新日時", "update_mode": "更新ポリシー",
		"automatic_updates": "自動更新", "manual_only": "手動のみ", "last_update": "最終更新", "never_updated": "未実行", "manual_update": "手動更新", "manual_update_help": "現在のバージョンを確認するか、更新間隔を無視して再ダウンロードします。",
		"check_now": "今すぐ確認", "force_download": "強制ダウンロード", "geoip_provider": "位置情報プロバイダー", "download_source": "ダウンロード元", "official_source": "公式ソース", "custom_mirror": "カスタムミラー",
		"official_source_url": "このプロバイダーの公式 URL", "custom_download_url": "カスタムダウンロード URL", "checksum_url": "SHA-256 チェックサム URL", "checksum_url_help": "任意。ダウンロードしたコンテナを検証します。", "credential_blank_keeps": "空欄のままにすると保存済みの値を保持します。",
		"provider_credentials": "プロバイダー認証情報", "saved_credential": "保存済み。保持する場合は空欄", "incomplete": "未完了", "maxmind_account_id": "Account ID", "maxmind_license_key": "License Key", "ip2location_token": "Download Token", "clear_maxmind_credentials": "保存済み MaxMind 認証情報を削除",
		"clear_ip2location_token": "保存済み IP2Location Token を削除", "confirm_new_password": "新しいパスワードの確認", "update_password": "パスワードを更新", "version_update": "バージョン更新", "current": "現在", "stable_path": "安定実行パス",
		"signature_verification": "署名検証", "configured": "設定済み", "not_configured": "未設定", "launch_mode": "起動方式", "stable_launch": "安定パス", "needs_adjustment": "要調整",
		"administrator_password": "管理者パスワード", "check_and_update": "確認して更新", "online_update": "オンライン更新", "online_update_help": "設定済みのマニフェスト URL からリリースを取得してインストールします。",
		"local_update": "ローカルファイル更新", "local_update_help": "同じ公式 Release の署名済みマニフェストと現在のプラットフォーム用バイナリをアップロードします。", "signed_manifest": "署名済み manifest.json", "release_binary": "Release バイナリ", "install_local_update": "検証して更新",
		"sensitive_admin_only": "機密データ。管理者のみ表示", "export_filtered_csv": "現在のフィルターを CSV 出力", "filters": "フィルター", "clear_filters": "フィルターを解除",
		"all_sites": "すべてのサイト", "from_utc": "開始時刻 (UTC)", "to_utc": "終了時刻 (UTC)", "hostname": "ホスト名", "hostnames": "ホスト名", "path": "パス", "original_ip": "元の IP", "site_label": "サイト", "visitor_digest": "Visitor Digest",
		"country_code": "国コード", "region_code": "地域コード", "per_page": "1ページあたり", "apply_filters": "フィルターを適用", "row_records": "詳細記録", "this_page": "このページ",
		"items": "件", "time": "時刻", "location": "場所", "system": "システム", "newer_records": "新しい記録", "older_records": "古い記録",
		"no_matching_records": "条件に一致する Pageview 記録はありません", "export_aggregates": "集計を出力", "choose_site": "サイトを選択", "dimension": "ディメンション", "overall": "全体",
		"country": "国", "region": "地域", "download_aggregate_csv": "集計 CSV をダウンロード", "map_controls": "地図操作", "reset_map_view": "地図の位置とズームをリセット",
		"allowed_origins_count": "件の Allowed Origin", "integration_code": "埋め込みコード", "cumulative_stats": "累積統計", "retention": "保存期間", "map_preset": "Map Preset",
		"url_overrides": "URL パラメーターで上書き可能", "url_override_title": "上書き可能な地図パラメーター", "url_override_help": "パラメーターは現在のリクエストにだけ適用され、保存済みの Map Preset は変更しません。複数指定できます。", "url_param_dimensions": "出力の幅と高さ。範囲はそれぞれ 160～1200、90～800。", "url_param_title": "タイトルを上書きします。", "url_param_labels": "Pageview または Unique Visitor のラベルを上書きします。", "url_param_show": "title、pv、uv をカンマ区切りで表示指定します。none ですべて非表示。", "url_param_font": "フォントサイズを 8～32 に設定します。", "url_param_colors": "6桁の16進カラーを使用します。bg には transparent も指定できます。", "url_param_metric": "地図マーカーのサイズ指標を pv または uv に設定します。", "close": "閉じる",
		"preset_preview": "Map Preset のライブプレビュー", "preview_mode": "プレビューモード", "interactive_widget_preview": "インタラクティブ Widget", "image_fallback": "Image fallback", "width": "幅", "height": "高さ", "auto_width": "現在の設定から幅を計算",
		"pv_label": "PV ラベル", "uv_label": "UV ラベル", "auto_height": "現在の設定から高さを計算", "title": "タイトル", "font_size": "フォントサイズ", "marker_metric": "マーカー指標", "display_content": "表示内容",
		"transparent_background": "透明な背景", "background": "背景", "land": "陸地", "border": "境界線", "text": "テキスト", "marker": "マーカー", "save_preset": "プリセットを保存",
		"site_sections": "サイトページのセクション", "site_settings": "サイト設定", "site_identity": "基本情報", "statistics_policy": "集計と保存", "access_publishing": "収集と公開", "allowed_origins": "Allowed Origins",
		"accept_pageviews": "Pageview を受信", "public_ingest_endpoint": "Tracker と Widget のデータ送信を許可", "publish_data": "データを公開", "map_and_analytics": "地図 API と公開分析ページを公開",
		"save_settings": "サイト設定を保存", "combined_widget": "統合", "separate_tracker": "分離", "javascript_widget": "インタラクティブ地図 + 収集", "image_widget": "画像地図 + 収集", "recommended": "推奨", "interactive": "インタラクティブ", "compatibility_mode": "互換", "image_widget_limitations": "JavaScript 不要。ブラウザー Visitor ID は作成せず、既定のパスには Referer を使用します。画像プロキシ、先読み、キャッシュでは地図だけ表示され、カウントされない場合があります。固定ページには URL の path パラメーターを追加してください。", "tracker_resource": "収集のみ", "map_resource": "インタラクティブ地図のみ", "separated_widget_help": "表示専用で、Pageview は記録しません。遅延読み込みに対応し、統合 Widget と同じホバー、タッチ、公開分析への遷移、レスポンシブ表示を提供します。", "embed_code": "埋め込みコード", "recent_records": "最近の記録", "view_site_records": "このサイトの全記録を見る", "visited_page": "訪問ページ", "client": "クライアント", "collection_method": "収集方法", "all_methods": "すべての方法",
		"refresh_record_geoip": "位置情報を更新", "refresh_record_geoip_confirm": "現在の GeoIP データベースで保存期間内の Pageview 位置情報を上書きし、対象記録の日付の地理集計を再構築しますか？詳細記録のない過去の日付は変更されません。", "geoip_unavailable": "GeoIP データベースを利用できません",
		"no_pageview_records": "Pageview 記録はありません", "danger_zone": "危険な操作", "irreversible_backup_first": "取り消せません。先にバックアップしてください", "reset_site_data": "サイトデータをリセット",
		"reset_site_help": "サイト設定を残して詳細記録、集計、地図データを削除し、収集と公開表示を無効にします。", "reset_data": "データをリセット",
		"delete_site": "サイトを完全に削除", "delete_site_help": "サイトとすべての記録、集計、地図データ、設定を削除します。Site ID は再利用されません。", "delete_permanently": "完全に削除",
		"admin_login": "管理者ログイン", "login_console": "ログイン", "request_incomplete": "リクエストを完了できませんでした", "return_admin": "管理者コンソールに戻る", "switching_version": "バージョンを切り替えています",
		"update_ready": "検証と準備が完了しました", "service_restarting": "サービスを再起動しています", "restart_help": "プロセスマネージャーが安定実行パスを起動すると、管理者コンソールが再び利用できます。", "reconnect": "再接続",
		"date_to": "～", "analytics_range": "分析期間", "today": "今日", "days": "日間", "all": "すべて", "all_history": "全期間", "analytics_summary": "分析概要",
		"cities": "都市", "countries_regions": "国または地域", "visitor_map": "訪問者マップ", "selected_range": "選択期間", "visit_trend": "訪問傾向",
		"daily_visit_trend": "日別訪問傾向", "no_data": "データなし", "active_dates": "データのある日", "code": "コード", "city": "都市",
		"browsers": "ブラウザー", "operating_systems": "OS", "type": "種類", "share": "割合", "start_date": "開始日", "end_date": "終了日",
		"query": "検索", "aggregate_only": "機密情報を含まない集計ビュー", "pageviews": "Pageview", "unique_visitors": "ユニーク訪問者", "visitors": "訪問者", "unknown": "不明",
		"public_language": "公開分析の既定言語", "language_auto": "自動（訪問者のブラウザー設定）",
		"flash_site": "サイトを作成しました。", "flash_settings": "サイト設定を保存しました。", "flash_preset": "Map Preset を保存しました。", "flash_reset": "サイトデータをリセットしました。収集と公開表示は無効のままです。",
		"flash_deleted": "サイトを完全に削除しました。", "flash_backup": "バックアップを作成し、整合性を確認しました。", "flash_cleanup": "メンテナンスのクリーンアップが完了しました。", "flash_geoip": "GeoIP データベースを更新してホットロードしました。",
		"flash_geoip_current": "GeoIP データベースは最新です。", "flash_update_current": "VisitorTrace は最新です。", "flash_record_geoip": "%d 件の Pageview を処理しました（%d 件を変更）: 位置情報あり %d 件、未一致 %d 件、無効な IP を %d 件スキップ。%d 日分の地理集計を再構築しました。",
	},
}

func translate(lang, key string) string {
	if value := messages[lang][key]; value != "" {
		return value
	}
	if value := messages["zh-CN"][key]; value != "" {
		return value
	}
	return key
}

func validLanguage(value string) bool { return value == "zh-CN" || value == "ja" || value == "en" }

func requestedLanguage(r *http.Request, cookieName string) string {
	if value := r.URL.Query().Get("lang"); validLanguage(value) {
		return value
	}
	if cookie, err := r.Cookie(cookieName); err == nil && validLanguage(cookie.Value) {
		return cookie.Value
	}
	return ""
}

func adminLanguage(r *http.Request) string {
	if value := requestedLanguage(r, adminLanguageCookie); value != "" {
		return value
	}
	return "zh-CN"
}

func publicLanguage(r *http.Request, siteDefault string) string {
	if value := requestedLanguage(r, publicLanguageCookie); value != "" {
		return value
	}
	if validLanguage(siteDefault) {
		return siteDefault
	}
	if accepted := preferredLanguage(r.Header.Get("Accept-Language")); accepted != "" {
		return accepted
	}
	return "zh-CN"
}

func preferredLanguage(header string) string {
	bestLanguage := ""
	bestQuality := -1.0
	for _, item := range strings.Split(header, ",") {
		parts := strings.Split(item, ";")
		languageTag := strings.ToLower(strings.TrimSpace(parts[0]))
		if languageTag == "" {
			continue
		}
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil || parsed < 0 {
				quality = 0
			} else {
				quality = parsed
			}
		}
		if quality <= 0 {
			continue
		}

		language := ""
		switch {
		case strings.HasPrefix(languageTag, "zh"):
			language = "zh-CN"
		case strings.HasPrefix(languageTag, "ja"):
			language = "ja"
		case strings.HasPrefix(languageTag, "en"):
			language = "en"
		}
		if language != "" && quality > bestQuality {
			bestLanguage = language
			bestQuality = quality
		}
	}
	return bestLanguage
}

func (s *Server) rememberRequestedLanguage(w http.ResponseWriter, r *http.Request) {
	value := r.URL.Query().Get("lang")
	if !validLanguage(value) {
		return
	}
	cookieName := publicLanguageCookie
	if strings.HasPrefix(r.URL.Path, "/admin") {
		cookieName = adminLanguageCookie
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: value, Path: s.cookiePath(), MaxAge: 365 * 24 * 60 * 60,
		Expires: time.Now().Add(365 * 24 * time.Hour), SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil || s.forwardedHTTPS(r),
	})
}

func languageURL(r *http.Request, lang string) string {
	query := cloneValues(r.URL.Query())
	query.Set("lang", lang)
	return r.URL.Path + "?" + query.Encode()
}

func cloneValues(values url.Values) url.Values {
	result := make(url.Values, len(values))
	for key, value := range values {
		result[key] = append([]string(nil), value...)
	}
	return result
}
