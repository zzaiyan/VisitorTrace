package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/maprender"
	"github.com/zzaiyan/VisitorTrace/internal/password"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func TestHealthEndpoints(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default(dir)
	if err := os.WriteFile(cfg.GeoIPPath, []byte("placeholder"), 0o600); err != nil {
		t.Fatalf("write GeoIP fixture: %v", err)
	}
	st, err := store.Initialize(context.Background(), filepath.Join(dir, "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	defer st.Close()
	app := New(cfg, st)

	live := httptest.NewRecorder()
	app.Handler().ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if live.Code != http.StatusOK {
		t.Fatalf("live status = %d, want %d", live.Code, http.StatusOK)
	}

	ready := httptest.NewRecorder()
	app.Handler().ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status = %d, want %d", ready.Code, http.StatusServiceUnavailable)
	}
}

func TestPageviewCollection(t *testing.T) {
	app, st, site := testServer(t)
	handler := app.Handler()
	body := `{"path":"/about/","visitor_id":"00112233445566778899aabbccddeeff"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/pageviews", strings.NewReader(body))
	request.Header.Set("Origin", "https://example.com")
	request.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	request.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Firefox/128.0")
	request.RemoteAddr = "192.0.2.10:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("collection status = %d, body = %q", response.Code, response.Body.String())
	}
	var count int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM pageviews WHERE site_id = ? AND path = '/about/'`, site.ID).Scan(&count); err != nil {
		t.Fatalf("count Pageview Records: %v", err)
	}
	if count != 1 {
		t.Fatalf("Pageview Record count = %d, want 1", count)
	}
}

func TestPageviewCollectionRejectsDisallowedOrigin(t *testing.T) {
	app, st, site := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+site.ID+"/pageviews", strings.NewReader(`{"path":"/"}`))
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("User-Agent", "Mozilla/5.0 Firefox/128.0")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("collection status = %d, want %d", response.Code, http.StatusForbidden)
	}
	var count int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM pageviews`).Scan(&count); err != nil {
		t.Fatalf("count Pageview Records: %v", err)
	}
	if count != 0 {
		t.Fatalf("disallowed request created %d records", count)
	}
}

func TestTrackerScript(t *testing.T) {
	app, _, site := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/embed/tracker.js?site_id="+site.ID, nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("tracker status = %d, body = %q", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "window.VisitorTrace.track") {
		t.Fatal("tracker response does not expose the explicit track API")
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := io.Copy(writer, bytes.NewReader(response.Body.Bytes())); err != nil {
		t.Fatalf("compress tracker: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tracker compressor: %v", err)
	}
	if compressed.Len() > 2*1024 {
		t.Fatalf("tracker gzip size = %d bytes, want <= 2048", compressed.Len())
	}
}

func TestIntegratedWidgetScript(t *testing.T) {
	app, _, site := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/embed/widget.js?site_id="+site.ID+"&w=640", nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("widget status = %d, body = %q", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "visitortrace-widget") || !strings.Contains(response.Body.String(), "/map.svg") {
		t.Fatal("widget response does not include integrated map insertion")
	}
}

func TestPublicMap(t *testing.T) {
	app, st, site := testServer(t)
	latitude := 30.5928
	longitude := 114.3055
	_, err := st.RecordPageview(context.Background(), store.PageviewObservation{
		SiteID:          site.ID,
		OccurredAt:      time.Date(2026, time.July, 21, 0, 0, 0, 0, time.UTC),
		Path:            "/",
		CountryCode:     "CN",
		RegionCode:      "HB",
		City:            "Wuhan",
		Latitude:        &latitude,
		Longitude:       &longitude,
		VisitorDigest:   bytes.Repeat([]byte{1}, 32),
		OriginalIP:      "192.0.2.1",
		OperatingSystem: "Linux",
		Browser:         "Firefox",
	})
	if err != nil {
		t.Fatalf("RecordPageview() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/map.svg?w=640&h=360&metric=uv", nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("map status = %d, body = %q", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "image/svg+xml; charset=utf-8" || !strings.Contains(response.Body.String(), "<svg") || !strings.Contains(response.Body.String(), "Wuhan") {
		t.Fatal("map response is missing SVG content or marker data")
	}
	etag := response.Header().Get("ETag")
	if etag == "" {
		t.Fatal("map response is missing ETag")
	}
	conditional := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/map.svg?w=640&h=360&metric=uv", nil)
	conditional.Header.Set("If-None-Match", etag)
	conditionalResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(conditionalResponse, conditional)
	if conditionalResponse.Code != http.StatusNotModified {
		t.Fatalf("conditional map status = %d, want %d", conditionalResponse.Code, http.StatusNotModified)
	}
}

func TestPublicMapRejectsUnknownOption(t *testing.T) {
	app, _, site := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/map.svg?unknown=value", nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("map status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestAdminLoginAndDashboard(t *testing.T) {
	app, _, site := testAdminServer(t)
	login := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(url.Values{"password": {"correct horse"}}.Encode()))
	login.Host = "127.0.0.1:8790"
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loggedIn := httptest.NewRecorder()
	app.Handler().ServeHTTP(loggedIn, login)
	if loggedIn.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, body = %q", loggedIn.Code, loggedIn.Body.String())
	}
	cookies := loggedIn.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "visitortrace_admin" {
		t.Fatalf("login cookies = %#v", cookies)
	}
	dashboard := httptest.NewRequest(http.MethodGet, "/admin", nil)
	dashboard.Host = "127.0.0.1:8790"
	dashboard.AddCookie(cookies[0])
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, dashboard)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), site.Name) || !strings.Contains(response.Body.String(), "管理总览") {
		t.Fatalf("dashboard status = %d, body = %q", response.Code, response.Body.String())
	}
	csrfMatch := regexp.MustCompile(`name="csrf" value="([a-f0-9]{64})"`).FindStringSubmatch(response.Body.String())
	if len(csrfMatch) != 2 {
		t.Fatal("dashboard did not render a CSRF token")
	}
	presetForm := url.Values{
		"csrf": {csrfMatch[1]}, "w": {"640"}, "h": {"320"}, "title": {"Preview"},
		"pv_label": {"PV"}, "uv_label": {"UV"}, "fs": {"12"}, "bg_color": {"#f2f3f3"},
		"land": {"#6f808f"}, "border": {"#ffffff"}, "text": {"#54606a"}, "marker": {"#e34949"},
		"metric": {"pv"}, "show_title": {"on"}, "show_pv": {"on"}, "bg_transparent": {"on"},
	}
	initialMap := httptest.NewRecorder()
	app.Handler().ServeHTTP(initialMap, httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/map.svg", nil))
	if initialMap.Code != http.StatusOK || !strings.Contains(initialMap.Body.String(), `width="300"`) {
		t.Fatalf("initial cached map = status %d body %q", initialMap.Code, initialMap.Body.String())
	}
	presetRequest := httptest.NewRequest(http.MethodPost, "/admin/sites/"+site.ID+"/preset", strings.NewReader(presetForm.Encode()))
	presetRequest.Host = "127.0.0.1:8790"
	presetRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	presetRequest.AddCookie(cookies[0])
	presetResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(presetResponse, presetRequest)
	if presetResponse.Code != http.StatusSeeOther || presetResponse.Header().Get("Location") != "/admin/sites/"+site.ID+"?saved=preset#preset" {
		t.Fatalf("preset update status = %d, body = %q", presetResponse.Code, presetResponse.Body.String())
	}
	updatedMap := httptest.NewRecorder()
	app.Handler().ServeHTTP(updatedMap, httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/map.svg", nil))
	if updatedMap.Code != http.StatusOK || !strings.Contains(updatedMap.Body.String(), `width="640"`) {
		t.Fatalf("invalidated map = status %d body %q", updatedMap.Code, updatedMap.Body.String())
	}
	settingsForm := url.Values{
		"csrf": {csrfMatch[1]}, "name": {site.Name}, "timezone": {site.Timezone},
		"origins": {"https://example.com"}, "dedup_window_days": {"1"}, "retention_days": {"30"},
		"accept_pageviews": {"on"}, "publish_public": {"on"},
	}
	settingsRequest := httptest.NewRequest(http.MethodPost, "/admin/sites/"+site.ID+"/settings", strings.NewReader(settingsForm.Encode()))
	settingsRequest.Host = "127.0.0.1:8790"
	settingsRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	settingsRequest.AddCookie(cookies[0])
	settingsResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(settingsResponse, settingsRequest)
	if settingsResponse.Code != http.StatusSeeOther || settingsResponse.Header().Get("Location") != "/admin/sites/"+site.ID+"?saved=settings#settings" {
		t.Fatalf("settings update status = %d, body = %q", settingsResponse.Code, settingsResponse.Body.String())
	}
	previewRequest := httptest.NewRequest(http.MethodGet, "/admin/sites/"+site.ID+"/preset-preview.svg", nil)
	previewRequest.Host = "127.0.0.1:8790"
	previewRequest.AddCookie(cookies[0])
	previewResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(previewResponse, previewRequest)
	if previewResponse.Code != http.StatusOK || !strings.Contains(previewResponse.Body.String(), `width="640" height="320"`) || !strings.Contains(previewResponse.Body.String(), `fill="none"`) {
		t.Fatalf("admin preset preview = status %d body prefix %q", previewResponse.Code, previewResponse.Body.String()[:min(140, len(previewResponse.Body.String()))])
	}
	sitePage := httptest.NewRequest(http.MethodGet, "/admin/sites/"+site.ID, nil)
	sitePage.Host = "127.0.0.1:8790"
	sitePage.AddCookie(cookies[0])
	sitePageResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(sitePageResponse, sitePage)
	if sitePageResponse.Code != http.StatusOK || !strings.Contains(sitePageResponse.Body.String(), "地图预设") || !strings.Contains(sitePageResponse.Body.String(), "http://127.0.0.1:8790/embed/widget.js") || !strings.Contains(sitePageResponse.Body.String(), `data-auto-dimension="width"`) || !strings.Contains(sitePageResponse.Body.String(), `data-auto-dimension="height"`) || !strings.Contains(sitePageResponse.Body.String(), `data-map-aspect="2.4"`) || !strings.Contains(sitePageResponse.Body.String(), `name="bg_transparent"`) {
		t.Fatalf("admin Site page = status %d, body = %q", sitePageResponse.Code, sitePageResponse.Body.String())
	}
}

func TestAdminPasswordChangeRevokesSession(t *testing.T) {
	app, st, _ := testAdminServer(t)
	cookie, csrf := loginAdmin(t, app)
	form := url.Values{
		"csrf": {csrf}, "current_password": {"correct horse"},
		"new_password": {"new password"}, "confirm_password": {"new password"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/settings/password", strings.NewReader(form.Encode()))
	request.Host = "127.0.0.1:8790"
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/login?changed=1" {
		t.Fatalf("password change = status %d location %q body %q", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	hash, err := st.AdministratorPasswordHash(context.Background())
	if err != nil || !password.Verify([]byte("new password"), hash) {
		t.Fatalf("new password was not stored: %v", err)
	}
	dashboard := httptest.NewRequest(http.MethodGet, "/admin", nil)
	dashboard.Host = "127.0.0.1:8790"
	dashboard.AddCookie(cookie)
	dashboardResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(dashboardResponse, dashboard)
	if dashboardResponse.Code != http.StatusSeeOther {
		t.Fatalf("revoked session status = %d", dashboardResponse.Code)
	}
}

func TestAdminSiteResetAndDelete(t *testing.T) {
	app, st, site := testAdminServer(t)
	cookie, csrf := loginAdmin(t, app)
	if _, err := st.RecordPageview(context.Background(), store.PageviewObservation{
		SiteID: site.ID, Path: "/", VisitorDigest: bytes.Repeat([]byte{9}, 32), OriginalIP: "192.0.2.9", OperatingSystem: "Linux", Browser: "Firefox",
	}); err != nil {
		t.Fatal(err)
	}
	post := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		form := url.Values{"csrf": {csrf}, "site_id": {site.ID}, "password": {"correct horse"}}
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		request.Host = "127.0.0.1:8790"
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		return response
	}
	reset := post("/admin/sites/" + site.ID + "/reset")
	if reset.Code != http.StatusSeeOther || reset.Header().Get("Location") != "/admin/sites/"+site.ID+"?saved=reset#danger" {
		t.Fatalf("reset = status %d location %q", reset.Code, reset.Header().Get("Location"))
	}
	resetSite, err := st.GetSite(context.Background(), site.ID)
	if err != nil || resetSite.AcceptPageviews || resetSite.PublishPublic {
		t.Fatalf("reset Site = %#v, %v", resetSite, err)
	}
	deleted := post("/admin/sites/" + site.ID + "/delete")
	if deleted.Code != http.StatusSeeOther || deleted.Header().Get("Location") != "/admin?saved=deleted" {
		t.Fatalf("delete = status %d location %q", deleted.Code, deleted.Header().Get("Location"))
	}
	if _, err := st.GetSite(context.Background(), site.ID); err == nil {
		t.Fatal("Site remained after Admin deletion")
	}
}

func TestAdminRecordFilteringAndCSVExports(t *testing.T) {
	app, st, site := testAdminServer(t)
	cookie, _ := loginAdmin(t, app)
	_, err := st.RecordPageview(context.Background(), store.PageviewObservation{
		SiteID: site.ID, OccurredAt: time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC), Path: "/records",
		CountryCode: "CN", RegionCode: "HB", City: "Wuhan", VisitorDigest: bytes.Repeat([]byte{6}, 32),
		OriginalIP: "203.0.113.6", OperatingSystem: "Linux", Browser: "Firefox",
	})
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Host = "127.0.0.1:8790"
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		return response
	}
	page := get("/admin/records?site_id=" + site.ID + "&ip=203.0.113.6")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "203.0.113.6") || !strings.Contains(page.Body.String(), "/records") || !strings.Contains(page.Body.String(), "导出当前筛选 CSV") {
		t.Fatalf("record page = status %d body %q", page.Code, page.Body.String())
	}
	recordCSV := get("/admin/records.csv?site_id=" + site.ID + "&ip=203.0.113.6")
	if recordCSV.Code != http.StatusOK || recordCSV.Header().Get("Content-Type") != "text/csv; charset=utf-8" || !strings.Contains(recordCSV.Body.String(), "occurred_at_site_time") || !strings.Contains(recordCSV.Body.String(), "203.0.113.6") {
		t.Fatalf("record CSV = status %d body %q", recordCSV.Code, recordCSV.Body.String())
	}
	aggregateCSV := get("/admin/aggregates.csv?site_id=" + site.ID + "&dimension=overall&start=2026-07-22&end=2026-07-22")
	if aggregateCSV.Code != http.StatusOK || !strings.Contains(aggregateCSV.Body.String(), "dimension_kind") || !strings.Contains(aggregateCSV.Body.String(), "overall") {
		t.Fatalf("Aggregate CSV = status %d body %q", aggregateCSV.Code, aggregateCSV.Body.String())
	}
	invalid := get("/admin/records?digest=not-a-digest")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid filter status = %d", invalid.Code)
	}
}

func TestRecordCursorRejectsChangedFilters(t *testing.T) {
	record := store.PageviewRecord{ID: 12, OccurredAt: time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC)}
	first := store.PageviewFilters{SiteID: "one"}
	second := store.PageviewFilters{SiteID: "two"}
	token := encodeRecordCursor(record, recordFilterFingerprint(first, 100))
	if _, err := decodeRecordCursor(token, recordFilterFingerprint(second, 100)); err == nil {
		t.Fatal("cursor was accepted with changed filters")
	}
}

func TestAdminOperationalActions(t *testing.T) {
	app, st, _ := testAdminServer(t)
	cookie, csrf := loginAdmin(t, app)
	post := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		form := url.Values{"csrf": {csrf}}
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		request.Host = "127.0.0.1:8790"
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		return response
	}
	backupResponse := post("/admin/operations/backup")
	if backupResponse.Code != http.StatusSeeOther || backupResponse.Header().Get("Location") != "/admin?saved=backup" {
		t.Fatalf("backup action = status %d location %q body %q", backupResponse.Code, backupResponse.Header().Get("Location"), backupResponse.Body.String())
	}
	archives, err := filepath.Glob(filepath.Join(app.Config.BackupDir, "*.vtbackup"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("backup archives = %#v, %v", archives, err)
	}
	cleanupResponse := post("/admin/operations/cleanup")
	if cleanupResponse.Code != http.StatusSeeOther || cleanupResponse.Header().Get("Location") != "/admin?saved=cleanup" {
		t.Fatalf("cleanup action = status %d location %q", cleanupResponse.Code, cleanupResponse.Header().Get("Location"))
	}
	statuses, err := st.OperationStatuses(context.Background())
	if err != nil || len(statuses) < 2 {
		t.Fatalf("operation statuses = %#v, %v", statuses, err)
	}
	dashboard := httptest.NewRequest(http.MethodGet, "/admin", nil)
	dashboard.Host = "127.0.0.1:8790"
	dashboard.AddCookie(cookie)
	dashboardResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(dashboardResponse, dashboard)
	if dashboardResponse.Code != http.StatusOK || !strings.Contains(dashboardResponse.Body.String(), "运行状态") || !strings.Contains(dashboardResponse.Body.String(), "visitortrace-") {
		t.Fatalf("operations dashboard = status %d body %q", dashboardResponse.Code, dashboardResponse.Body.String())
	}
}

func TestAdminSelfUpdateRequiresEmbeddedKey(t *testing.T) {
	app, _, _ := testAdminServer(t)
	cookie, csrf := loginAdmin(t, app)
	settings := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	settings.Host = "127.0.0.1:8790"
	settings.AddCookie(cookie)
	settingsResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(settingsResponse, settings)
	if settingsResponse.Code != http.StatusOK || !strings.Contains(settingsResponse.Body.String(), "版本更新") || !strings.Contains(settingsResponse.Body.String(), "未配置") || !strings.Contains(settingsResponse.Body.String(), "disabled") {
		t.Fatalf("settings update section = status %d body %q", settingsResponse.Code, settingsResponse.Body.String())
	}
	form := url.Values{"csrf": {csrf}, "password": {"correct horse"}}
	request := httptest.NewRequest(http.MethodPost, "/admin/settings/update", strings.NewReader(form.Encode()))
	request.Host = "127.0.0.1:8790"
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || !strings.Contains(response.Header().Get("Location"), "error=") {
		t.Fatalf("self-update without key = status %d location %q", response.Code, response.Header().Get("Location"))
	}
}

func TestPublicAnalyticsHidesSensitiveFields(t *testing.T) {
	app, st, site := testServer(t)
	now := time.Now().UTC()
	latitude := 30.5928
	longitude := 114.3055
	_, err := st.RecordPageview(context.Background(), store.PageviewObservation{
		SiteID: site.ID, OccurredAt: now, Path: "/private-page-path", CountryCode: "CN", RegionCode: "HB", City: "Wuhan",
		Latitude: &latitude, Longitude: &longitude,
		VisitorDigest: bytes.Repeat([]byte{7}, 32), OriginalIP: "203.0.113.7", OperatingSystem: "Linux", Browser: "Firefox",
	})
	if err != nil {
		t.Fatalf("RecordPageview() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/public/"+site.ID+"/analytics?range=30d", nil)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Public Analytics status = %d, body = %q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, "203.0.113.7") {
		t.Fatal("Public Analytics exposed the original IP")
	}
	if strings.Contains(body, "0707070707070707070707070707070707070707070707070707070707070707") {
		t.Fatal("Public Analytics exposed the Visitor Digest")
	}
	if strings.Contains(body, "/private-page-path") {
		t.Fatal("Public Analytics exposed a page path")
	}
	if !strings.Contains(body, "Firefox") || !strings.Contains(body, "Wuhan") || !strings.Contains(body, "Countries or regions") {
		t.Fatalf("Public Analytics is missing aggregate data: %q", body)
	}
	if !strings.Contains(body, `/assets/analytics.js`) || !strings.Contains(body, `"date"`) || !strings.Contains(body, `analytics-map.svg?`) {
		t.Fatalf("Public Analytics enhancement or range map fallback is missing: %q", body)
	}
}

func TestAdminAnalyticsIncludesPathsForPrivateSite(t *testing.T) {
	app, st, site := testAdminServer(t)
	if _, err := st.DB.ExecContext(context.Background(), `UPDATE sites SET publish_public = 0 WHERE id = ?`, site.ID); err != nil {
		t.Fatal(err)
	}
	latitude, longitude := 31.2304, 121.4737
	if _, err := st.RecordPageview(context.Background(), store.PageviewObservation{
		SiteID: site.ID, OccurredAt: time.Now(), Path: "/admin-only-path", CountryCode: "CN", RegionCode: "SH", City: "Shanghai",
		Latitude: &latitude, Longitude: &longitude, VisitorDigest: bytes.Repeat([]byte{10}, 32), OriginalIP: "203.0.113.10", OperatingSystem: "Linux", Browser: "Firefox",
	}); err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation(site.Timezone)
	today := time.Now().In(location).Format(time.DateOnly)
	if _, err := st.DB.ExecContext(context.Background(), `
		INSERT INTO site_deduplication_rules (site_id, effective_date, window_days, created_at)
		VALUES (?, ?, 5, ?)
	`, site.ID, today, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	cookie, _ := loginAdmin(t, app)
	request := httptest.NewRequest(http.MethodGet, "/admin/sites/"+site.ID+"/analytics?range=today&lang=en", nil)
	request.Host = "127.0.0.1:8790"
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "/admin-only-path") || !strings.Contains(body, "Path performance") || !strings.Contains(body, `id="path-chart"`) || !strings.Contains(body, "Counting rule changes") || !strings.Contains(body, `"window_days":5`) {
		t.Fatalf("Admin Analytics = status %d body %q", response.Code, body)
	}
	public := httptest.NewRecorder()
	app.Handler().ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/public/"+site.ID+"/analytics", nil))
	if public.Code != http.StatusNotFound {
		t.Fatalf("private Public Analytics status = %d", public.Code)
	}
}

func TestPublicAnalyticsMapHonorsDateRange(t *testing.T) {
	app, st, site := testServer(t)
	recentLat, recentLon := 30.5928, 114.3055
	oldLat, oldLon := 48.8566, 2.3522
	now := time.Now().UTC()
	observations := []store.PageviewObservation{
		{SiteID: site.ID, OccurredAt: now, Path: "/recent", CountryCode: "CN", RegionCode: "HB", City: "Wuhan", Latitude: &recentLat, Longitude: &recentLon, VisitorDigest: bytes.Repeat([]byte{8}, 32), OriginalIP: "203.0.113.8", OperatingSystem: "Linux", Browser: "Firefox"},
		{SiteID: site.ID, OccurredAt: now.AddDate(0, 0, -40), Path: "/old", CountryCode: "FR", RegionCode: "IDF", City: "Paris", Latitude: &oldLat, Longitude: &oldLon, VisitorDigest: bytes.Repeat([]byte{9}, 32), OriginalIP: "203.0.113.9", OperatingSystem: "Linux", Browser: "Firefox"},
	}
	for _, observation := range observations {
		if _, err := st.RecordPageview(context.Background(), observation); err != nil {
			t.Fatal(err)
		}
	}
	location, _ := time.LoadLocation(site.Timezone)
	today := now.In(location).Format(time.DateOnly)
	request := httptest.NewRequest(http.MethodGet, "/public/"+site.ID+"/analytics-map.svg?start="+today+"&end="+today, nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Wuhan") || strings.Contains(response.Body.String(), "Paris") {
		t.Fatalf("date-range analytics map = status %d body %q", response.Code, response.Body.String())
	}
}

func TestAnalyticsAssetServesPrecompressedBundle(t *testing.T) {
	app, _, _ := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/assets/analytics.js", nil)
	request.Header.Set("Accept-Encoding", "br, gzip")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Encoding") != "br" || response.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("analytics asset headers = status %d encoding %q vary %q", response.Code, response.Header().Get("Content-Encoding"), response.Header().Get("Vary"))
	}
	if response.Body.Len() > 250*1024 {
		t.Fatalf("Brotli analytics asset = %d bytes, want <= 250 KiB", response.Body.Len())
	}
}

func TestMapPresetDefaultsAndOverrides(t *testing.T) {
	app, st, site := testServer(t)
	options := maprender.DefaultOptions()
	options.Width = 640
	options.Height = 320
	options.Show = map[string]bool{}
	preset, err := maprender.PresetJSON(options)
	if err != nil {
		t.Fatalf("PresetJSON() error = %v", err)
	}
	if err := st.UpdateMapPreset(context.Background(), site.ID, preset); err != nil {
		t.Fatalf("UpdateMapPreset() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/map.svg", nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `width="640" height="320"`) {
		t.Fatalf("preset map = status %d body prefix %q", response.Code, response.Body.String()[:min(120, len(response.Body.String()))])
	}
	override := httptest.NewRequest(http.MethodGet, "/api/v1/sites/"+site.ID+"/map.svg?w=300&h=168&show=title", nil)
	overrideResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(overrideResponse, override)
	if overrideResponse.Code != http.StatusOK || !strings.Contains(overrideResponse.Body.String(), `width="300" height="168"`) || strings.Contains(overrideResponse.Body.String(), "Total Pageviews") {
		t.Fatalf("preset override = status %d body prefix %q", overrideResponse.Code, overrideResponse.Body.String()[:min(160, len(overrideResponse.Body.String()))])
	}
}

func testServer(t *testing.T) (*Server, *store.Store, store.Site) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default(dir)
	if err := os.WriteFile(cfg.GeoIPPath, []byte("placeholder"), 0o600); err != nil {
		t.Fatalf("write GeoIP fixture: %v", err)
	}
	st, err := store.Initialize(context.Background(), filepath.Join(dir, "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	created, err := st.CreateSite(context.Background(), store.CreateSiteParams{Name: "Test", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}
	return New(cfg, st), st, created
}

func testAdminServer(t *testing.T) (*Server, *store.Store, store.Site) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default(dir)
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	passwordHash, err := password.Hash([]byte("correct horse"))
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	st, err := store.Initialize(context.Background(), filepath.Join(dir, "visitortrace.sqlite3"), passwordHash)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	site, err := st.CreateSite(context.Background(), store.CreateSiteParams{Name: "Admin Site", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}
	app := New(cfg, st)
	app.ConfigPath = configPath
	return app, st, site
}

func loginAdmin(t *testing.T, app *Server) (*http.Cookie, string) {
	t.Helper()
	login := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(url.Values{"password": {"correct horse"}}.Encode()))
	login.Host = "127.0.0.1:8790"
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loggedIn := httptest.NewRecorder()
	app.Handler().ServeHTTP(loggedIn, login)
	if loggedIn.Code != http.StatusSeeOther || len(loggedIn.Result().Cookies()) != 1 {
		t.Fatalf("login status = %d, body = %q", loggedIn.Code, loggedIn.Body.String())
	}
	cookie := loggedIn.Result().Cookies()[0]
	dashboard := httptest.NewRequest(http.MethodGet, "/admin", nil)
	dashboard.Host = "127.0.0.1:8790"
	dashboard.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, dashboard)
	match := regexp.MustCompile(`name="csrf" value="([a-f0-9]{64})"`).FindStringSubmatch(response.Body.String())
	if response.Code != http.StatusOK || len(match) != 2 {
		t.Fatalf("dashboard status = %d, body = %q", response.Code, response.Body.String())
	}
	return cookie, match[1]
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
