package server

import (
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/maprender"
	"github.com/zzaiyan/VisitorTrace/internal/operations"
	"github.com/zzaiyan/VisitorTrace/internal/password"
	"github.com/zzaiyan/VisitorTrace/internal/selfupdate"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

//go:embed templates/*.html assets/admin.css assets/analytics.js assets/analytics.js.gz assets/analytics.js.br
var pageAssets embed.FS

type pageLayout struct {
	Title       string
	Admin       bool
	CSRF        string
	Flash       string
	Error       string
	Active      string
	CurrentPath string
	Lang        string
}

func (p pageLayout) PageLanguage() string { return p.Lang }

type loginPageData struct {
	pageLayout
	Next string
}

type errorPageData struct {
	pageLayout
	Status  int
	Message string
}

type siteSummary struct {
	Site          store.Site
	Overview      store.SiteOverview
	Preset        maprender.Options
	MapPreviewURL string
}

type adminDashboardData struct {
	pageLayout
	Sites      []siteSummary
	Operations operations.Snapshot
}

type adminSiteData struct {
	pageLayout
	Site          store.Site
	Overview      store.SiteOverview
	Preset        maprender.Options
	Recent        []store.PageviewRecord
	OriginsText   string
	MapPreviewURL string
	MapAspect     float64
	BaseURL       string
	Saved         string
}

type newSiteData struct {
	pageLayout
	Name            string
	Timezone        string
	Origins         string
	DedupWindowDays int
	RetentionDays   int
}

type adminSettingsData struct {
	pageLayout
	CurrentVersion        string
	StableExecutable      string
	UpdateKeyReady        bool
	RunningFromStablePath bool
	BaseURL               string
	EffectiveBaseURL      string
}

type publicAnalyticsData struct {
	pageLayout
	Site      store.Site
	Analytics store.PublicAnalyticsData
	MapURL    string
	Range     string
	ChartJSON template.JS
}

type adminAnalyticsData struct {
	pageLayout
	Site      store.Site
	Analytics store.PublicAnalyticsData
	Range     string
	ChartJSON template.JS
}

func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, page string, data any) {
	lang := "zh-CN"
	if provider, ok := data.(interface{ PageLanguage() string }); ok && validLanguage(provider.PageLanguage()) {
		lang = provider.PageLanguage()
	}
	s.rememberRequestedLanguage(w, r)
	templates, err := template.New("layout.html").Funcs(template.FuncMap{
		"t":                func(key string) string { return translate(lang, key) },
		"language":         func() string { return lang },
		"languageIs":       func(want string) bool { return lang == want },
		"languageURL":      func(want string) string { return s.appPath(languageURL(r, want)) },
		"appPath":          s.appPath,
		"formatCount":      func(value int64) string { return strconv.FormatInt(value, 10) },
		"formatTime":       func(value time.Time) string { return value.Local().Format("2006-01-02 15:04:05") },
		"formatUTC":        formatUTCValue,
		"formatRecordTime": formatRecordTime,
		"formatBytes":      formatBytes,
		"formatDuration":   func(value time.Duration) string { return formatDuration(value, lang) },
		"operationWarning": func(value string) string { return operationWarning(value, lang) },
		"operationLabel":   func(value string) string { return operationLabel(value, lang) },
		"operationState":   func(value string) string { return operationState(value, lang) },
		"geoAttribution":   func() geoip.Attribution { return geoip.AttributionForProvider(s.Config.GeoIPProvider) },
		"geoLabel":         geoLabel,
		"metricPercent": func(value, total int64) string {
			if total <= 0 {
				return "0%"
			}
			return fmt.Sprintf("%.1f%%", float64(value)*100/float64(total))
		},
		"mapShow":  func(options maprender.Options, key string) bool { return options.Show[key] },
		"selected": func(actual, want string) bool { return actual == want },
	}).ParseFS(pageAssets, "templates/layout.html", "templates/"+page+".html")
	if err != nil {
		http.Error(w, "template unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "layout", data); err != nil {
		s.logger.Error("render page failed", "page", page, "error", err)
	}
}

func (s *Server) adminLayout(r *http.Request, session store.AdministratorSession, title, active string) pageLayout {
	return pageLayout{
		Title: title, Admin: true, CSRF: hex.EncodeToString(session.CSRFToken),
		Active: active, CurrentPath: r.URL.Path, Lang: adminLanguage(r),
	}
}

func (s *Server) adminDashboard(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	sites, err := s.Store.ListSites(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法读取站点。")
		return
	}
	result := adminDashboardData{pageLayout: s.adminLayout(r, session, "管理总览", "dashboard")}
	result.Operations = operations.Collect(r.Context(), s.Config, s.Store, s.Started, time.Now())
	for _, site := range sites {
		overview, err := s.Store.SiteOverview(r.Context(), site.ID)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, "无法读取站点统计。")
			return
		}
		preset, err := maprender.ParsePresetJSON(site.MapPresetJSON)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, "无法读取 Map Preset。")
			return
		}
		result.Sites = append(result.Sites, siteSummary{Site: site, Overview: overview, Preset: preset, MapPreviewURL: s.appPath("/admin/sites/" + site.ID + "/preset-preview.svg")})
	}
	result.Flash = adminFlash(r)
	s.renderPage(w, r, "dashboard", result)
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	manager := selfupdate.New(s.Config, s.ConfigPath, s.Store)
	data := adminSettingsData{
		pageLayout:     s.adminLayout(r, session, "管理员设置", "settings"),
		CurrentVersion: manager.CurrentVersion, StableExecutable: manager.StableBinaryPath(),
		UpdateKeyReady: len(manager.PublicKey) > 0, RunningFromStablePath: manager.RunningFromStablePath(),
		BaseURL: s.Config.BaseURL, EffectiveBaseURL: s.externalBaseURL(r),
	}
	data.Flash = adminFlash(r)
	data.Error = r.URL.Query().Get("error")
	s.renderPage(w, r, "settings", data)
}

type baseURLRestartData struct {
	pageLayout
	ReconnectURL string
}

func (s *Server) adminUpdateBaseURL(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	if s.ConfigPath == "" {
		s.redirectWithError(w, r, "/admin/settings", "服务配置路径不可用。")
		return
	}
	baseURL, err := config.NormalizeBaseURL(r.FormValue("base_url"))
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings", err.Error())
		return
	}
	updated := s.Config
	updated.BaseURL = baseURL
	if err := config.Save(s.ConfigPath, updated); err != nil {
		s.redirectWithError(w, r, "/admin/settings", "无法保存 Base URL："+err.Error())
		return
	}
	reconnectURL := s.requestOrigin(r) + "/admin/settings"
	if baseURL != "" {
		reconnectURL = strings.TrimSuffix(baseURL, "/") + "/admin/settings"
	}
	s.renderPage(w, r, "settings-restarting", baseURLRestartData{
		pageLayout:   s.adminLayout(r, session, "正在重启", "settings"),
		ReconnectURL: reconnectURL,
	})
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.RequestRestart()
	}()
}

func (s *Server) adminChangePassword(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectWithError(w, r, "/admin/settings", "密码表单无效。")
		return
	}
	if !s.administratorPasswordMatches(r.Context(), r.FormValue("current_password")) {
		s.redirectWithError(w, r, "/admin/settings", "当前密码不正确。")
		return
	}
	newPassword, err := password.Validate(r.FormValue("new_password"))
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings", err.Error())
		return
	}
	confirmation := []byte(r.FormValue("confirm_password"))
	if len(newPassword) != len(confirmation) || subtle.ConstantTimeCompare(newPassword, confirmation) != 1 {
		s.redirectWithError(w, r, "/admin/settings", "两次输入的新密码不一致。")
		return
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法生成密码凭据。")
		return
	}
	if err := s.Store.UpdateAdministratorPassword(r.Context(), hash); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法更新管理员密码。")
		return
	}
	s.clearAdminCookie(w, r)
	s.redirect(w, r, "/admin/login?changed=1", http.StatusSeeOther)
}

func (s *Server) adminNewSite(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	data := newSiteData{pageLayout: s.adminLayout(r, session, "新增 Site", "sites"), Timezone: "Asia/Shanghai", DedupWindowDays: 1, RetentionDays: 30}
	s.renderPage(w, r, "new-site", data)
}

func (s *Server) adminCreateSite(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, "站点表单无效。")
		return
	}
	data := newSiteData{pageLayout: s.adminLayout(r, session, "新增 Site", "sites"), Name: r.FormValue("name"), Timezone: r.FormValue("timezone"), Origins: r.FormValue("origins"), DedupWindowDays: intFormValue(r, "dedup_window_days", 1), RetentionDays: intFormValue(r, "retention_days", 30)}
	created, err := s.Store.CreateSite(r.Context(), store.CreateSiteParams{
		Name: data.Name, Timezone: data.Timezone, AllowedOrigins: splitLines(data.Origins),
		DedupWindowDays: data.DedupWindowDays, RetentionDays: data.RetentionDays,
	})
	if err != nil {
		data.Error = err.Error()
		s.renderPage(w, r, "new-site", data)
		return
	}
	s.redirect(w, r, "/admin/sites/"+created.ID+"?saved=site", http.StatusSeeOther)
}

func (s *Server) adminSite(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	siteID := r.PathValue("siteID")
	site, err := s.Store.GetSite(r.Context(), siteID)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Site 不存在。")
		return
	}
	overview, err := s.Store.SiteOverview(r.Context(), siteID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法读取站点统计。")
		return
	}
	preset, err := maprender.ParsePresetJSON(site.MapPresetJSON)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法读取 Map Preset。")
		return
	}
	recent, err := s.Store.RecentPageviewRecords(r.Context(), siteID, 80)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法读取 Pageview Record。")
		return
	}
	data := adminSiteData{
		pageLayout: s.adminLayout(r, session, site.Name, "sites"), Site: site, Overview: overview,
		Preset: preset, Recent: recent, OriginsText: strings.Join(site.AllowedOrigins, "\n"),
		MapPreviewURL: s.appPath("/admin/sites/" + site.ID + "/preset-preview.svg"), MapAspect: maprender.MapAspect, BaseURL: s.externalBaseURL(r), Saved: adminFlash(r),
	}
	data.Error = r.URL.Query().Get("error")
	s.renderPage(w, r, "site", data)
}

func (s *Server) adminUpdateSite(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, "站点表单无效。")
		return
	}
	siteID := r.PathValue("siteID")
	_, err := s.Store.UpdateSite(r.Context(), siteID, store.UpdateSiteParams{
		Name: r.FormValue("name"), Timezone: r.FormValue("timezone"), AllowedOrigins: splitLines(r.FormValue("origins")),
		AcceptPageviews: r.FormValue("accept_pageviews") == "on", PublishPublic: r.FormValue("publish_public") == "on", PublicLanguage: r.FormValue("public_language"),
		DedupWindowDays: intFormValue(r, "dedup_window_days", 1), RetentionDays: intFormValue(r, "retention_days", 30),
	})
	if err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+siteID, err.Error())
		return
	}
	s.redirect(w, r, "/admin/sites/"+siteID+"?saved=settings#settings", http.StatusSeeOther)
}

func (s *Server) adminUpdatePreset(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Map Preset 表单无效。")
		return
	}
	values := url.Values{}
	for _, key := range []string{"w", "h", "title", "pv_label", "uv_label", "fs", "land", "border", "text", "marker", "metric"} {
		values.Set(key, r.FormValue(key))
	}
	if r.FormValue("bg_transparent") == "on" {
		values.Set("bg", "transparent")
	} else {
		values.Set("bg", r.FormValue("bg_color"))
	}
	show := make([]string, 0, 3)
	for _, key := range []string{"title", "pv", "uv"} {
		if r.FormValue("show_"+key) == "on" {
			show = append(show, key)
		}
	}
	if len(show) == 0 {
		values.Set("show", "none")
	} else {
		values.Set("show", strings.Join(show, ","))
	}
	options, err := maprender.ParseOptions(values)
	if err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+r.PathValue("siteID"), err.Error())
		return
	}
	presetJSON, err := maprender.PresetJSON(options)
	if err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+r.PathValue("siteID"), err.Error())
		return
	}
	if err := s.Store.UpdateMapPreset(r.Context(), r.PathValue("siteID"), presetJSON); err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+r.PathValue("siteID"), err.Error())
		return
	}
	s.mapCache.deleteSite(r.PathValue("siteID"))
	s.redirect(w, r, "/admin/sites/"+r.PathValue("siteID")+"?saved=preset#preset", http.StatusSeeOther)
}

func (s *Server) adminResetSite(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+r.PathValue("siteID")+"#danger", "确认表单无效。")
		return
	}
	siteID := r.PathValue("siteID")
	if r.FormValue("site_id") != siteID || !s.administratorPasswordMatches(r.Context(), r.FormValue("password")) {
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#danger", "Site ID 或管理员密码不正确。")
		return
	}
	if err := s.Store.ResetSiteData(r.Context(), siteID); err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#danger", err.Error())
		return
	}
	s.mapCache.deleteSite(siteID)
	s.redirect(w, r, "/admin/sites/"+siteID+"?saved=reset#danger", http.StatusSeeOther)
}

func (s *Server) adminDeleteSite(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+r.PathValue("siteID")+"#danger", "确认表单无效。")
		return
	}
	siteID := r.PathValue("siteID")
	if r.FormValue("site_id") != siteID || !s.administratorPasswordMatches(r.Context(), r.FormValue("password")) {
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#danger", "Site ID 或管理员密码不正确。")
		return
	}
	if err := s.Store.DeleteSite(r.Context(), siteID); err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#danger", err.Error())
		return
	}
	s.mapCache.deleteSite(siteID)
	s.redirect(w, r, "/admin?saved=deleted", http.StatusSeeOther)
}

func (s *Server) adminPresetPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	siteID := r.PathValue("siteID")
	site, err := s.Store.GetSite(r.Context(), siteID)
	if err != nil {
		http.Error(w, "unknown Site", http.StatusNotFound)
		return
	}
	defaults, err := maprender.ParsePresetJSON(site.MapPresetJSON)
	if err != nil {
		http.Error(w, "could not load Map Preset", http.StatusInternalServerError)
		return
	}
	options, err := maprender.ParseOptionsWithDefaults(r.URL.Query(), defaults)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := s.Store.AdminMapData(r.Context(), siteID)
	if err != nil {
		http.Error(w, "could not read map data", http.StatusInternalServerError)
		return
	}
	body, err := maprender.Render(data, options)
	if err != nil {
		http.Error(w, "could not render map", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func (s *Server) publicAnalytics(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("siteID")
	site, err := s.Store.GetSite(r.Context(), siteID)
	if err != nil || !site.PublishPublic {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	start, end, rangeName, err := s.analyticsRange(r, site)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	analytics, err := s.Store.PublicAnalytics(r.Context(), siteID, start, end)
	if err != nil {
		http.Error(w, "could not read Public Analytics", http.StatusInternalServerError)
		return
	}
	lang := publicLanguage(r, site.PublicLanguage)
	chartJSON, err := analyticsChartJSON(analytics, lang)
	if err != nil {
		http.Error(w, "could not encode Public Analytics", http.StatusInternalServerError)
		return
	}
	mapQuery := url.Values{"start": {analytics.StartDate}, "end": {analytics.EndDate}}
	data := publicAnalyticsData{
		pageLayout: pageLayout{Title: site.Name + " · Public Analytics", CurrentPath: r.URL.Path, Lang: lang},
		Site:       site, Analytics: analytics, Range: rangeName,
		MapURL: s.appPath("/public/" + site.ID + "/analytics-map.svg?" + mapQuery.Encode()), ChartJSON: chartJSON,
	}
	s.renderPage(w, r, "public-analytics", data)
}

func (s *Server) adminSiteAnalytics(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	siteID := r.PathValue("siteID")
	site, err := s.Store.GetSite(r.Context(), siteID)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Site 不存在。")
		return
	}
	start, end, rangeName, err := s.analyticsRange(r, site)
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	analytics, err := s.Store.AdminAnalytics(r.Context(), siteID, start, end)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法读取聚合分析。")
		return
	}
	lang := adminLanguage(r)
	chartJSON, err := analyticsChartJSON(analytics, lang)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法编码聚合分析。")
		return
	}
	s.renderPage(w, r, "admin-analytics", adminAnalyticsData{
		pageLayout: s.adminLayout(r, session, site.Name+" · Analytics", "sites"),
		Site:       site, Analytics: analytics, Range: rangeName, ChartJSON: chartJSON,
	})
}

func (s *Server) publicAnalyticsMap(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("siteID")
	configuredSite, err := s.Store.GetSite(r.Context(), siteID)
	if err != nil || !configuredSite.PublishPublic {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	data, err := s.Store.PublicMapDataRange(r.Context(), siteID, r.URL.Query().Get("start"), r.URL.Query().Get("end"))
	if err != nil {
		http.Error(w, "invalid analytics range", http.StatusBadRequest)
		return
	}
	options, err := maprender.ParsePresetJSON(configuredSite.MapPresetJSON)
	if err != nil {
		http.Error(w, "could not load Map Preset", http.StatusInternalServerError)
		return
	}
	options.Width = 720
	options.Height = 400
	body, err := maprender.Render(data, options)
	if err != nil {
		http.Error(w, "could not render analytics map", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write(body)
}

func analyticsChartJSON(analytics store.PublicAnalyticsData, lang string) (template.JS, error) {
	type chartPoint struct {
		Name string  `json:"name"`
		Lon  float64 `json:"lon"`
		Lat  float64 `json:"lat"`
		PV   int64   `json:"pv"`
		UV   int64   `json:"uv"`
	}
	payload := struct {
		Daily    []store.DailyMetric             `json:"daily"`
		Points   []chartPoint                    `json:"points"`
		Paths    []store.AnalyticsMetric         `json:"paths,omitempty"`
		Browsers []store.AnalyticsMetric         `json:"browsers,omitempty"`
		OS       []store.AnalyticsMetric         `json:"operating_systems,omitempty"`
		Rules    []store.DeduplicationRuleChange `json:"rules,omitempty"`
		Labels   map[string]string               `json:"labels"`
	}{Daily: analytics.Daily, Labels: map[string]string{
		"pageviews": translate(lang, "pageviews"), "uniqueVisitors": translate(lang, "unique_visitors"),
		"visitors": translate(lang, "visitors"), "unknown": translate(lang, "unknown"),
		"path": translate(lang, "path"), "share": translate(lang, "share"), "days": translate(lang, "days"),
	}, Paths: analytics.Paths, Browsers: analytics.Browsers, OS: analytics.OperatingSystems, Rules: analytics.DeduplicationRules}
	for _, point := range analytics.MapPoints {
		name := point.City
		if name == "" {
			name = point.CountryCode
		}
		payload.Points = append(payload.Points, chartPoint{Name: name, Lon: point.Longitude, Lat: point.Latitude, PV: point.Pageviews, UV: point.UniqueVisitors})
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return template.JS(data), nil
}

func (s *Server) analyticsRange(r *http.Request, site store.Site) (string, string, string, error) {
	rangeName := r.URL.Query().Get("range")
	location, err := time.LoadLocation(site.Timezone)
	if err != nil {
		return "", "", "", err
	}
	now := time.Now().In(location)
	end := now.Format(time.DateOnly)
	switch rangeName {
	case "", "30d":
		return now.AddDate(0, 0, -29).Format(time.DateOnly), end, "30d", nil
	case "today":
		return end, end, rangeName, nil
	case "7d":
		return now.AddDate(0, 0, -6).Format(time.DateOnly), end, rangeName, nil
	case "90d":
		return now.AddDate(0, 0, -89).Format(time.DateOnly), end, rangeName, nil
	case "all":
		start, finish, err := s.Store.AnalyticsBounds(r.Context(), site.ID)
		if err != nil {
			return "", "", "", err
		}
		if start == "" {
			start = end
		}
		if finish == "" {
			finish = end
		}
		return start, finish, rangeName, nil
	case "custom":
		return r.URL.Query().Get("start"), r.URL.Query().Get("end"), rangeName, nil
	default:
		return "", "", "", fmt.Errorf("unsupported analytics range")
	}
}

func (s *Server) redirectWithError(w http.ResponseWriter, r *http.Request, path, message string) {
	query := url.Values{"error": []string{message}}
	fragment := ""
	if index := strings.IndexByte(path, '#'); index >= 0 {
		fragment = path[index:]
		path = path[:index]
	}
	s.redirect(w, r, path+"?"+query.Encode()+fragment, http.StatusSeeOther)
}

func (s *Server) adminAssets(w http.ResponseWriter, r *http.Request) {
	data, err := pageAssets.ReadFile("assets/admin.css")
	if err != nil {
		http.Error(w, "asset unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
}

func (s *Server) analyticsAssets(w http.ResponseWriter, r *http.Request) {
	asset := "assets/analytics.js"
	encoding := ""
	accepted := r.Header.Get("Accept-Encoding")
	if strings.Contains(accepted, "br") {
		asset += ".br"
		encoding = "br"
	} else if strings.Contains(accepted, "gzip") {
		asset += ".gz"
		encoding = "gzip"
	}
	data, err := pageAssets.ReadFile(asset)
	if err != nil {
		http.Error(w, "asset unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Vary", "Accept-Encoding")
	if encoding != "" {
		w.Header().Set("Content-Encoding", encoding)
	}
	_, _ = w.Write(data)
}

func adminFlash(r *http.Request) string {
	if value := r.URL.Query().Get("error"); value != "" {
		return ""
	}
	key := ""
	switch r.URL.Query().Get("saved") {
	case "site":
		key = "flash_site"
	case "settings":
		key = "flash_settings"
	case "preset":
		key = "flash_preset"
	case "reset":
		key = "flash_reset"
	case "deleted":
		key = "flash_deleted"
	case "backup":
		key = "flash_backup"
	case "cleanup":
		key = "flash_cleanup"
	case "geoip":
		key = "flash_geoip"
	case "geoip-current":
		key = "flash_geoip_current"
	case "update-current":
		key = "flash_update_current"
	}
	if key == "" {
		return ""
	}
	return translate(adminLanguage(r), key)
}

func intFormValue(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.FormValue(key))
	if err != nil {
		return fallback
	}
	return value
}

func splitLines(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func geoLabel(value string) string {
	if value == "unknown" || value == "" {
		return "Unknown"
	}
	parts := strings.Split(value, "|")
	if len(parts) == 3 {
		return parts[2]
	}
	return value
}

func formatBytes(input any) string {
	var value uint64
	switch typed := input.(type) {
	case int64:
		if typed > 0 {
			value = uint64(typed)
		}
	case uint64:
		value = typed
	case int:
		if typed > 0 {
			value = uint64(typed)
		}
	}
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, exponent := uint64(unit), 0
	for amount := value / unit; amount >= unit && exponent < 3; amount /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGT"[exponent])
}

func formatDuration(value time.Duration, lang string) string {
	if value < time.Minute {
		if lang == "en" {
			return "< 1 minute"
		}
		return "< 1 分钟"
	}
	days := value / (24 * time.Hour)
	hours := value % (24 * time.Hour) / time.Hour
	minutes := value % time.Hour / time.Minute
	if days > 0 {
		if lang == "en" {
			return fmt.Sprintf("%d d %d h", days, hours)
		}
		return fmt.Sprintf("%d 天 %d 小时", days, hours)
	}
	if hours > 0 {
		if lang == "en" {
			return fmt.Sprintf("%d h %d min", hours, minutes)
		}
		return fmt.Sprintf("%d 小时 %d 分钟", hours, minutes)
	}
	if lang == "en" {
		return fmt.Sprintf("%d min", minutes)
	}
	return fmt.Sprintf("%d 分钟", minutes)
}

func formatUTCValue(input any) string {
	var value time.Time
	switch typed := input.(type) {
	case time.Time:
		value = typed
	case *time.Time:
		if typed != nil {
			value = *typed
		}
	}
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

func operationWarning(value, lang string) string {
	labels := map[string]string{
		"disk_low": "可用磁盘空间不足", "geoip_missing": "GeoIP 数据库不可用", "geoip_stale": "GeoIP 数据库超过 35 天未更新",
		"backup_missing": "尚未创建备份", "backup_stale": "最近备份超过 48 小时", "cleanup_stale": "自动清理超过 2 小时未成功完成",
		"backup_failed": "最近备份失败", "cleanup_failed": "最近清理失败", "geoip_update_failed": "最近 GeoIP 更新失败", "self_update_failed": "最近自更新失败",
	}
	if lang == "en" {
		labels = map[string]string{
			"disk_low": "Available disk space is low", "geoip_missing": "GeoIP database is unavailable", "geoip_stale": "GeoIP database is over 35 days old",
			"backup_missing": "No backup has been created", "backup_stale": "Latest backup is over 48 hours old", "cleanup_stale": "Cleanup has not succeeded for over 2 hours",
			"backup_failed": "Latest backup failed", "cleanup_failed": "Latest cleanup failed", "geoip_update_failed": "Latest GeoIP update failed", "self_update_failed": "Latest self-update failed",
		}
	}
	if label := labels[value]; label != "" {
		return label
	}
	return value
}

func operationLabel(value, lang string) string {
	labels := map[string]string{"backup": "备份", "cleanup": "维护清理", "geoip_update": "GeoIP 更新", "self_update": "版本更新"}
	if lang == "en" {
		labels = map[string]string{"backup": "Backup", "cleanup": "Maintenance cleanup", "geoip_update": "GeoIP update", "self_update": "Version update"}
	}
	if label := labels[value]; label != "" {
		return label
	}
	return value
}

func operationState(value, lang string) string {
	labels := map[string]string{"success": "成功", "failed": "失败", "running": "运行中"}
	if lang == "en" {
		labels = map[string]string{"success": "Success", "failed": "Failed", "running": "Running"}
	}
	if label := labels[value]; label != "" {
		return label
	}
	return value
}

func (s *Server) externalBaseURL(r *http.Request) string {
	if s.Config.BaseURL != "" {
		return strings.TrimSuffix(s.Config.BaseURL, "/")
	}
	return s.requestOrigin(r) + s.basePath
}

func (s *Server) requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || s.forwardedHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
