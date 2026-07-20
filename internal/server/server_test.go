package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
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
