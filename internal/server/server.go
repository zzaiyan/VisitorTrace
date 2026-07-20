package server

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/clientip"
	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/pageview"
	"github.com/zzaiyan/VisitorTrace/internal/ratelimit"
	"github.com/zzaiyan/VisitorTrace/internal/store"
	"github.com/zzaiyan/VisitorTrace/internal/useragent"
	"github.com/zzaiyan/VisitorTrace/internal/visitor"
)

//go:embed assets/tracker.js
var embeddedAssets embed.FS

const maxIngestionBody = 2 * 1024

type pageviewPayload struct {
	Path      string `json:"path"`
	VisitorID string `json:"visitor_id"`
}

type Server struct {
	Config    config.Config
	Store     *store.Store
	Started   time.Time
	clientIP  *clientip.Resolver
	ipLimit   *ratelimit.Limiter
	siteLimit *ratelimit.Limiter
	logger    *slog.Logger
}

func New(cfg config.Config, st *store.Store, loggers ...*slog.Logger) *Server {
	resolver, _ := clientip.New(cfg.TrustedProxies)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &Server{
		Config:    cfg,
		Store:     st,
		Started:   time.Now().UTC(),
		clientIP:  resolver,
		ipLimit:   ratelimit.New(120, 30),
		siteLimit: ratelimit.New(3000, 500),
		logger:    logger,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", s.live)
	mux.HandleFunc("GET /health/ready", s.ready)
	mux.HandleFunc("POST /api/v1/sites/{siteID}/pageviews", s.collectPageview)
	mux.HandleFunc("GET /embed/tracker.js", s.trackerScript)
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

func (s *Server) collectPageview(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("siteID")
	configuredSite, err := s.Store.GetSite(r.Context(), siteID)
	if err != nil {
		http.Error(w, "unknown Site", http.StatusNotFound)
		return
	}
	origin := r.Header.Get("Origin")
	if origin == "" || !configuredSite.AllowsOrigin(origin) {
		http.Error(w, "origin is not allowed", http.StatusForbidden)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Add("Vary", "Origin")

	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "text/plain" {
		http.Error(w, "content type must be text/plain", http.StatusUnsupportedMediaType)
		return
	}
	if s.clientIP == nil {
		http.Error(w, "client IP resolver is unavailable", http.StatusInternalServerError)
		return
	}
	address, err := s.clientIP.Resolve(r)
	if err != nil {
		http.Error(w, "client IP is invalid", http.StatusBadRequest)
		return
	}
	if !s.ipLimit.Allow(siteID+"|"+address.String()) || !s.siteLimit.Allow(siteID) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	userAgentValue := r.UserAgent()
	if userAgentValue == "" {
		http.Error(w, "User-Agent is required", http.StatusBadRequest)
		return
	}
	classification := useragent.Classify(userAgentValue)
	if classification.Bot {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var payload pageviewPayload
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIngestionBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}
	path, err := pageview.NormalizePath(payload.Path)
	if err != nil {
		http.Error(w, "invalid page path", http.StatusBadRequest)
		return
	}
	digest, err := visitor.Digest(configuredSite.HMACKey, payload.VisitorID, address.String(), userAgentValue)
	if err != nil {
		http.Error(w, "invalid visitor identity", http.StatusBadRequest)
		return
	}
	_, err = s.Store.RecordPageview(r.Context(), store.PageviewObservation{
		SiteID:          siteID,
		Path:            path,
		VisitorDigest:   digest,
		OriginalIP:      address.String(),
		OperatingSystem: classification.OperatingSystem,
		Browser:         classification.Browser,
	})
	if errors.Is(err, store.ErrCollectionDisabled) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		s.logger.Error("record Pageview failed", "site_id", siteID, "error", err)
		http.Error(w, "could not record Pageview", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) trackerScript(w http.ResponseWriter, r *http.Request) {
	siteID := strings.TrimSpace(r.URL.Query().Get("site_id"))
	if siteID == "" {
		http.Error(w, "site_id is required", http.StatusBadRequest)
		return
	}
	if _, err := s.Store.GetSite(r.Context(), siteID); err != nil {
		http.Error(w, "unknown Site", http.StatusNotFound)
		return
	}
	data, err := embeddedAssets.ReadFile("assets/tracker.js")
	if err != nil {
		http.Error(w, "tracker is unavailable", http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(data)
	etag := fmt.Sprintf("\"%x\"", sum[:16])
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("ETag", etag)
	_, _ = io.Copy(w, bytes.NewReader(data))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
