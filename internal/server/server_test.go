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
	presetRequest := httptest.NewRequest(http.MethodPost, "/admin/sites/"+site.ID+"/preset", strings.NewReader(presetForm.Encode()))
	presetRequest.Host = "127.0.0.1:8790"
	presetRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	presetRequest.AddCookie(cookies[0])
	presetResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(presetResponse, presetRequest)
	if presetResponse.Code != http.StatusSeeOther || presetResponse.Header().Get("Location") != "/admin/sites/"+site.ID+"?saved=preset" {
		t.Fatalf("preset update status = %d, body = %q", presetResponse.Code, presetResponse.Body.String())
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
	if sitePageResponse.Code != http.StatusOK || !strings.Contains(sitePageResponse.Body.String(), "地图预设") || !strings.Contains(sitePageResponse.Body.String(), "http://127.0.0.1:8790/embed/widget.js") {
		t.Fatalf("admin Site page = status %d, body = %q", sitePageResponse.Code, sitePageResponse.Body.String())
	}
}

func TestPublicAnalyticsHidesSensitiveFields(t *testing.T) {
	app, st, site := testServer(t)
	now := time.Now().UTC()
	_, err := st.RecordPageview(context.Background(), store.PageviewObservation{
		SiteID: site.ID, OccurredAt: now, Path: "/private-page-path", CountryCode: "CN", RegionCode: "HB", City: "Wuhan",
		VisitorDigest: bytes.Repeat([]byte{7}, 32), OriginalIP: "203.0.113.7", OperatingSystem: "Linux", Browser: "Firefox",
	})
	if err != nil {
		t.Fatalf("RecordPageview() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/public/"+site.ID+"/analytics?range=30d", nil)
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
	if !strings.Contains(body, "Firefox") || !strings.Contains(body, "Wuhan") {
		t.Fatalf("Public Analytics is missing aggregate data: %q", body)
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
	return New(cfg, st), st, site
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
