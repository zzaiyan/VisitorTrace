package server

import (
	"net/http"
	"net/url"
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
		"site_count": "个 Site", "add_site": "新增 Site", "operations_attention": "运行状态需要关注", "operations": "运行状态", "backup_now": "立即备份",
		"cleanup_now": "立即清理", "check_geoip": "检查 GeoIP", "version": "版本", "uptime": "运行时长", "started_at": "启动于", "available_disk": "可用磁盘",
		"total": "总计", "unavailable": "不可用", "awaiting_download": "等待下载", "latest_backup": "最近备份", "no_backup": "尚无备份",
		"create_recommended": "建议立即创建", "task": "任务", "status": "状态", "completed_at": "完成时间", "summary": "摘要", "running": "运行中",
		"no_task_runs": "后台任务尚无运行记录", "site_list": "站点列表", "receiving": "接收中", "paused": "已暂停", "public": "公开", "private": "未公开",
		"manage": "管理", "public_analytics": "公开分析", "manage_site": "管理", "no_sites": "还没有 Site", "create_first_site": "创建第一个 Site",
		"aggregate_analytics": "聚合分析", "path_performance": "路径表现", "geographic_distribution": "地理分布", "back_to_site": "返回 Site",
		"new_site": "新增 Site", "cancel": "取消", "display_name": "显示名称", "origin_hint": "每行一个完整 Origin，例如 https://example.com", "iana_timezone": "IANA 时区",
		"dedup_window": "访客合并周期", "record_retention": "逐条记录保留期", "day": "天", "create_site": "创建 Site",
		"dedup_change_help": "修改在下一个站点本地午夜生效；已完成聚合不会重算。", "counting_rule_changes": "计数规则变更", "effective_from": "生效于",
		"credentials_sessions": "服务配置、凭据与安全会话", "change_password": "修改密码", "current_password": "当前密码", "new_password": "新密码", "password_hint": "8 至 128 个字符",
		"base_url": "公开 Base URL", "base_url_help": "用于生成接入代码和公开链接；路径部分也会作为应用路由前缀。留空时使用当前请求地址。", "base_url_effective": "当前生效地址：", "save_base_url": "保存并重启服务", "base_url_saved": "Base URL 已保存，服务即将重启。", "copy": "复制", "copied": "已复制",
		"geoip_database": "GeoIP 数据库", "geoip_settings_help": "管理定位后端、下载源与更新策略。", "database_file": "数据库文件", "last_modified": "更新于", "update_mode": "更新策略",
		"automatic_updates": "自动更新", "manual_only": "仅手动", "last_update": "最近更新", "never_updated": "尚未执行", "manual_update": "手动更新", "manual_update_help": "检查当前版本，或忽略更新周期重新下载。",
		"check_now": "检查更新", "force_download": "强制下载", "geoip_provider": "定位后端", "download_source": "下载源", "official_source": "官方源", "custom_mirror": "自定义镜像",
		"official_source_url": "当前后端官方地址", "custom_download_url": "自定义下载地址", "checksum_url": "SHA-256 校验地址", "checksum_url_help": "可选；用于校验下载容器。", "credential_blank_keeps": "留空即保留已保存值。",
		"provider_credentials": "后端凭证", "saved_credential": "已保存；留空保持不变", "incomplete": "配置不完整", "maxmind_account_id": "Account ID", "maxmind_license_key": "License Key", "ip2location_token": "Download Token", "clear_maxmind_credentials": "清除已保存的 MaxMind 凭证",
		"clear_ip2location_token": "清除已保存的 IP2Location Token", "save_geoip_settings": "保存并重启服务", "geoip_settings_saved": "GeoIP 设置已保存，服务即将重启。",
		"confirm_new_password": "确认新密码", "update_password": "更新密码", "version_update": "版本更新", "current": "当前", "stable_path": "稳定执行路径",
		"signature_verification": "签名验证", "configured": "已配置", "not_configured": "未配置", "launch_mode": "启动方式", "stable_launch": "稳定路径", "needs_adjustment": "需要调整",
		"administrator_password": "管理员密码", "check_and_update": "检查并更新", "online_update": "在线更新", "online_update_help": "从已配置的发布清单地址获取并安装新版本。",
		"local_update": "本地文件更新", "local_update_help": "上传同一正式 Release 的签名清单与当前平台二进制。", "signed_manifest": "签名清单 manifest.json", "release_binary": "发布二进制", "install_local_update": "验证并更新",
		"sensitive_admin_only": "敏感数据，仅管理员可见", "export_filtered_csv": "导出当前筛选 CSV", "filters": "筛选条件", "clear_filters": "清除筛选",
		"all_sites": "全部 Site", "from_utc": "起始时间（UTC）", "to_utc": "结束时间（UTC）", "hostname": "访问域名", "hostnames": "访问域名", "path": "路径", "original_ip": "原始 IP",
		"country_code": "国家代码", "region_code": "地区代码", "per_page": "每页", "apply_filters": "应用筛选", "row_records": "逐条记录", "this_page": "本页",
		"items": "条", "time": "时间", "location": "位置", "system": "系统", "newer_records": "较新记录", "older_records": "较早记录",
		"no_matching_records": "没有符合条件的 Pageview Record", "export_aggregates": "导出聚合", "choose_site": "选择 Site", "dimension": "维度", "overall": "整体",
		"country": "国家", "region": "地区", "download_aggregate_csv": "下载聚合 CSV",
		"allowed_origins_count": "个 Allowed Origin", "integration_code": "接入代码", "cumulative_stats": "累计统计", "retention": "保留期", "map_preset": "地图预设",
		"url_overrides": "URL 参数可覆写", "preset_preview": "地图预设实时预览", "width": "宽度", "height": "高度", "auto_width": "按当前选项自动计算宽度",
		"pv_label": "PV 标签", "uv_label": "UV 标签",
		"auto_height": "按当前选项自动计算高度", "title": "标题", "font_size": "字体大小", "marker_metric": "标记指标", "display_content": "显示内容",
		"transparent_background": "透明背景", "background": "背景色", "land": "陆地", "border": "边界", "text": "文字", "marker": "标记", "save_preset": "保存预设",
		"site_settings": "站点设置", "accept_pageviews": "接收 Pageview", "public_ingest_endpoint": "公开采集端点", "publish_data": "公开数据", "map_and_analytics": "地图与分析页",
		"save_settings": "保存设置", "combined_widget": "一体式 Widget", "separate_tracker": "分离式 Tracker", "map_control": "地图控件代码", "recent_records": "最近记录", "filter_page_export": "筛选、分页与导出",
		"refresh_record_geoip": "刷新地理信息", "refresh_record_geoip_confirm": "使用当前 GeoIP 覆写保留期内的 Pageview 地理信息，并重算这些记录覆盖日期的地理聚合？没有明细的早期日期不会改变。", "geoip_unavailable": "GeoIP 数据库不可用",
		"no_pageview_records": "暂无 Pageview Record", "danger_zone": "危险操作", "irreversible_backup_first": "不可撤销，请先备份", "reset_site_data": "清空 Site 数据",
		"reset_site_help": "删除逐条记录、聚合和地图点，保留 Site 设置；同时关闭采集和公开展示。", "enter_site_id": "输入 Site ID", "reset_data": "清空数据",
		"delete_site": "永久删除 Site", "delete_site_help": "删除 Site 及其全部记录、聚合、地图数据和设置，Site ID 不会再次分配。", "delete_permanently": "永久删除",
		"admin_login": "管理员登录", "login_console": "登录后台", "request_incomplete": "请求未完成", "return_admin": "返回后台", "switching_version": "正在切换版本",
		"update_ready": "已通过验证并完成准备", "service_restarting": "服务正在重启", "restart_help": "进程管理器重新拉起稳定执行路径后，后台将恢复可用。", "reconnect": "重新连接",
		"date_to": "至", "analytics_range": "统计范围", "today": "今天", "days": "天", "all": "全部", "analytics_summary": "统计摘要",
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
		"site_count": "Sites", "add_site": "Add Site", "operations_attention": "Operations need attention", "operations": "Operations", "backup_now": "Back up now",
		"cleanup_now": "Clean up now", "check_geoip": "Check GeoIP", "version": "Version", "uptime": "Uptime", "started_at": "Started", "available_disk": "Available disk",
		"total": "Total", "unavailable": "Unavailable", "awaiting_download": "Awaiting download", "latest_backup": "Latest backup", "no_backup": "No backup",
		"create_recommended": "Create one now", "task": "Task", "status": "Status", "completed_at": "Completed", "summary": "Summary", "running": "Running",
		"no_task_runs": "No background task runs yet", "site_list": "Site list", "receiving": "Receiving", "paused": "Paused", "public": "Public", "private": "Private",
		"manage": "Manage", "public_analytics": "Public Analytics", "manage_site": "Manage", "no_sites": "No Sites yet", "create_first_site": "Create the first Site",
		"aggregate_analytics": "Aggregate Analytics", "path_performance": "Path performance", "geographic_distribution": "Geographic distribution", "back_to_site": "Back to Site",
		"new_site": "Add Site", "cancel": "Cancel", "display_name": "Display name", "origin_hint": "One complete Origin per line, for example https://example.com", "iana_timezone": "IANA timezone",
		"dedup_window": "Visitor merge window", "record_retention": "Record retention", "day": "days", "create_site": "Create Site",
		"dedup_change_help": "Changes take effect at the next Site-local midnight; completed aggregates are not recalculated.", "counting_rule_changes": "Counting rule changes", "effective_from": "Effective",
		"credentials_sessions": "Service configuration, credentials, and secure sessions", "change_password": "Change password", "current_password": "Current password", "new_password": "New password", "password_hint": "8 to 128 characters",
		"base_url": "Public Base URL", "base_url_help": "Used for integration code and public links; its path also becomes the application route prefix. Leave it empty to use the current request URL.", "base_url_effective": "Effective URL:", "save_base_url": "Save and restart service", "base_url_saved": "The Base URL was saved and the service will restart.", "copy": "Copy", "copied": "Copied",
		"geoip_database": "GeoIP database", "geoip_settings_help": "Manage the location provider, download source, and update policy.", "database_file": "Database file", "last_modified": "Updated", "update_mode": "Update policy",
		"automatic_updates": "Automatic", "manual_only": "Manual only", "last_update": "Latest update", "never_updated": "Not run yet", "manual_update": "Manual update", "manual_update_help": "Check the current version or download again regardless of the update interval.",
		"check_now": "Check now", "force_download": "Force download", "geoip_provider": "Location provider", "download_source": "Download source", "official_source": "Official source", "custom_mirror": "Custom mirror",
		"official_source_url": "Official URL for this provider", "custom_download_url": "Custom download URL", "checksum_url": "SHA-256 checksum URL", "checksum_url_help": "Optional; verifies the downloaded container.", "credential_blank_keeps": "Leave blank to retain the saved value.",
		"provider_credentials": "Provider credentials", "saved_credential": "Saved; leave blank to keep", "incomplete": "Incomplete", "maxmind_account_id": "Account ID", "maxmind_license_key": "License Key", "ip2location_token": "Download Token", "clear_maxmind_credentials": "Remove saved MaxMind credentials",
		"clear_ip2location_token": "Remove saved IP2Location token", "save_geoip_settings": "Save and restart service", "geoip_settings_saved": "GeoIP settings were saved and the service will restart.",
		"confirm_new_password": "Confirm new password", "update_password": "Update password", "version_update": "Version update", "current": "Current", "stable_path": "Stable executable path",
		"signature_verification": "Signature verification", "configured": "Configured", "not_configured": "Not configured", "launch_mode": "Launch mode", "stable_launch": "Stable path", "needs_adjustment": "Needs adjustment",
		"administrator_password": "Administrator password", "check_and_update": "Check and update", "online_update": "Online update", "online_update_help": "Fetch and install a release from the configured manifest URL.",
		"local_update": "Local files", "local_update_help": "Upload the signed manifest and platform binary from the same official Release.", "signed_manifest": "Signed manifest.json", "release_binary": "Release binary", "install_local_update": "Verify and update",
		"sensitive_admin_only": "Sensitive data, visible to administrators only", "export_filtered_csv": "Export filtered CSV", "filters": "Filters", "clear_filters": "Clear filters",
		"all_sites": "All Sites", "from_utc": "Start time (UTC)", "to_utc": "End time (UTC)", "hostname": "Hostname", "hostnames": "Hostnames", "path": "Path", "original_ip": "Original IP",
		"country_code": "Country code", "region_code": "Region code", "per_page": "Per page", "apply_filters": "Apply filters", "row_records": "Individual records", "this_page": "This page",
		"items": "items", "time": "Time", "location": "Location", "system": "System", "newer_records": "Newer records", "older_records": "Older records",
		"no_matching_records": "No matching Pageview Records", "export_aggregates": "Export aggregates", "choose_site": "Choose Site", "dimension": "Dimension", "overall": "Overall",
		"country": "Country", "region": "Region", "download_aggregate_csv": "Download aggregate CSV",
		"allowed_origins_count": "Allowed Origins", "integration_code": "Integration code", "cumulative_stats": "Cumulative statistics", "retention": "Retention", "map_preset": "Map Preset",
		"url_overrides": "URL parameters can override defaults", "preset_preview": "Live Map Preset preview", "width": "Width", "height": "Height", "auto_width": "Calculate width from current options",
		"pv_label": "PV label", "uv_label": "UV label",
		"auto_height": "Calculate height from current options", "title": "Title", "font_size": "Font size", "marker_metric": "Marker metric", "display_content": "Displayed content",
		"transparent_background": "Transparent background", "background": "Background", "land": "Land", "border": "Borders", "text": "Text", "marker": "Markers", "save_preset": "Save preset",
		"site_settings": "Site settings", "accept_pageviews": "Accept Pageviews", "public_ingest_endpoint": "Public ingestion endpoint", "publish_data": "Publish data", "map_and_analytics": "Map and analytics pages",
		"save_settings": "Save settings", "combined_widget": "Combined Widget", "separate_tracker": "Separate Tracker", "map_control": "Map control code", "recent_records": "Recent records", "filter_page_export": "Filter, paginate, and export",
		"refresh_record_geoip": "Refresh geography", "refresh_record_geoip_confirm": "Overwrite retained Pageview geography with the current GeoIP database and rebuild geographic aggregates for the dates those records cover? Earlier dates without records will not change.", "geoip_unavailable": "GeoIP database unavailable",
		"no_pageview_records": "No Pageview Records", "danger_zone": "Danger zone", "irreversible_backup_first": "Irreversible; create a backup first", "reset_site_data": "Reset Site data",
		"reset_site_help": "Delete individual records, aggregates, and map points while keeping Site settings; ingestion and public views are disabled.", "enter_site_id": "Enter Site ID", "reset_data": "Reset data",
		"delete_site": "Permanently delete Site", "delete_site_help": "Delete the Site and all records, aggregates, map data, and settings. The Site ID will not be reused.", "delete_permanently": "Delete permanently",
		"admin_login": "Administrator login", "login_console": "Sign in", "request_incomplete": "Request not completed", "return_admin": "Return to Admin Console", "switching_version": "Switching version",
		"update_ready": "has been verified and prepared", "service_restarting": "Service is restarting", "restart_help": "The Admin Console will return after the process manager launches the stable executable path.", "reconnect": "Reconnect",
		"date_to": "to", "analytics_range": "Analytics range", "today": "Today", "days": "days", "all": "All", "analytics_summary": "Analytics summary",
		"cities": "Cities", "countries_regions": "Countries or regions", "visitor_map": "Visitor map", "selected_range": "Selected date range", "visit_trend": "Visit trend",
		"daily_visit_trend": "Daily visit trend", "no_data": "No data", "active_dates": "dates with data", "code": "Code", "city": "City",
		"browsers": "Browsers", "operating_systems": "Operating systems", "type": "Type", "share": "Share", "start_date": "Start date", "end_date": "End date",
		"query": "Query", "aggregate_only": "Aggregate-only public view", "pageviews": "Pageviews", "unique_visitors": "Unique Visitors", "visitors": "Visitors", "unknown": "Unknown",
		"public_language": "Default Public Analytics language", "language_auto": "Automatic (visitor browser preference)",
		"flash_site": "Site created.", "flash_settings": "Site settings saved.", "flash_preset": "Map Preset saved.", "flash_reset": "Site data reset; ingestion and public views remain disabled.",
		"flash_deleted": "Site permanently deleted.", "flash_backup": "Backup created and verified.", "flash_cleanup": "Maintenance cleanup completed.", "flash_geoip": "GeoIP database updated and hot-loaded.",
		"flash_geoip_current": "The GeoIP database is already current.", "flash_update_current": "VisitorTrace is up to date.", "flash_record_geoip": "Processed %d Pageviews (%d changed): %d located, %d unmatched, %d invalid IPs skipped; geographic aggregates rebuilt for %d dates.",
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

func validLanguage(value string) bool { return value == "zh-CN" || value == "en" }

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
	accepted := strings.ToLower(r.Header.Get("Accept-Language"))
	if strings.Contains(accepted, "zh") || !strings.Contains(accepted, "en") {
		return "zh-CN"
	}
	return "en"
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
