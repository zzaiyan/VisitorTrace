package server

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

type Server struct {
	Config  config.Config
	Store   *store.Store
	Started time.Time
}

func New(cfg config.Config, st *store.Store) *Server {
	return &Server{Config: cfg, Store: st, Started: time.Now().UTC()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", s.live)
	mux.HandleFunc("GET /health/ready", s.ready)
	return mux
}

func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr:              s.Config.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func (s *Server) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	checks := map[string]bool{"sqlite": false, "schema": false, "geoip": false}
	if err := s.Store.DB.PingContext(r.Context()); err == nil {
		checks["sqlite"] = true
	}
	if err := s.Store.SchemaReady(r.Context()); err == nil {
		checks["schema"] = true
	}
	if info, err := os.Stat(s.Config.GeoIPPath); err == nil && !info.IsDir() && info.Size() > 0 {
		checks["geoip"] = true
	}
	status := http.StatusOK
	state := "ready"
	for _, ok := range checks {
		if !ok {
			status = http.StatusServiceUnavailable
			state = "not_ready"
			break
		}
	}
	writeJSON(w, status, map[string]any{"status": state, "checks": checks})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
