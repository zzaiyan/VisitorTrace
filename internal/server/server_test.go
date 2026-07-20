package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
	if ready.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want %d", ready.Code, http.StatusOK)
	}
}
