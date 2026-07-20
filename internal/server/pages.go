package server

import (
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/maprender"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

//go:embed templates/*.html assets/admin.css
var pageAssets embed.FS

type pageLayout struct {
	Title       string
	Admin       bool
	CSRF        string
	Flash       string
	Error       string
	Active      string
	CurrentPath string
}

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
	Sites []siteSummary
}

type adminSiteData struct {
	pageLayout
	Site          store.Site
	Overview      store.SiteOverview
	Preset        maprender.Options
	Recent        []store.PageviewRecord
	OriginsText   string
	MapPreviewURL string
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

type publicAnalyticsData struct {
	pageLayout
	Site      store.Site
	Analytics store.PublicAnalyticsData
	MapURL    string
	Range     string
}

func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, page string, data any) {
	templates, err := template.New("layout.html").Funcs(template.FuncMap{
		"formatCount": func(value int64) string { return strconv.FormatInt(value, 10) },
		"formatTime":  func(value time.Time) string { return value.Local().Format("2006-01-02 15:04:05") },
		"formatUTC":   func(value time.Time) string { return value.UTC().Format(time.RFC3339) },
		"geoLabel":    geoLabel,
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
		Active: active, CurrentPath: r.URL.Path,
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
		result.Sites = append(result.Sites, siteSummary{Site: site, Overview: overview, Preset: preset, MapPreviewURL: "/admin/sites/" + site.ID + "/preset-preview.svg"})
	}
	result.Flash = adminFlash(r)
	s.renderPage(w, r, "dashboard", result)
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
	http.Redirect(w, r, "/admin/sites/"+created.ID+"?saved=site", http.StatusSeeOther)
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
		MapPreviewURL: "/admin/sites/" + site.ID + "/preset-preview.svg", BaseURL: s.externalBaseURL(r), Saved: adminFlash(r),
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
		AcceptPageviews: r.FormValue("accept_pageviews") == "on", PublishPublic: r.FormValue("publish_public") == "on",
		DedupWindowDays: intFormValue(r, "dedup_window_days", 1), RetentionDays: intFormValue(r, "retention_days", 30),
	})
	if err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+siteID, err.Error())
		return
	}
	http.Redirect(w, r, "/admin/sites/"+siteID+"?saved=settings", http.StatusSeeOther)
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
	for _, key := range []string{"w", "h", "title", "pv_label", "uv_label", "fs", "bg", "land", "border", "text", "marker", "metric"} {
		values.Set(key, r.FormValue(key))
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
	http.Redirect(w, r, "/admin/sites/"+r.PathValue("siteID")+"?saved=preset", http.StatusSeeOther)
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
	data := publicAnalyticsData{
		pageLayout: pageLayout{Title: site.Name + " · Public Analytics", CurrentPath: r.URL.Path},
		Site:       site, Analytics: analytics, Range: rangeName,
		MapURL: "/api/v1/sites/" + site.ID + "/map.svg?w=720&h=400",
	}
	s.renderPage(w, r, "public-analytics", data)
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
	http.Redirect(w, r, path+"?"+query.Encode(), http.StatusSeeOther)
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

func adminFlash(r *http.Request) string {
	if value := r.URL.Query().Get("error"); value != "" {
		return ""
	}
	switch r.URL.Query().Get("saved") {
	case "site":
		return "Site 已创建。"
	case "settings":
		return "站点设置已保存。"
	case "preset":
		return "Map Preset 已保存。"
	default:
		return ""
	}
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

func (s *Server) externalBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || s.forwardedHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
