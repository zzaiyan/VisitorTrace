package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/clientip"
	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/maprender"
	"github.com/zzaiyan/VisitorTrace/internal/pageview"
	"github.com/zzaiyan/VisitorTrace/internal/ratelimit"
	visitorsite "github.com/zzaiyan/VisitorTrace/internal/site"
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
	Hostname  string `json:"hostname"`
}

type Server struct {
	Config        config.Config
	ConfigPath    string
	Store         *store.Store
	Started       time.Time
	clientIP      *clientip.Resolver
	ipLimit       *ratelimit.Limiter
	siteLimit     *ratelimit.Limiter
	logger        *slog.Logger
	geoMu         sync.RWMutex
	geoIP         *geoip.Resolver
	mapCache      *mapCache
	loginLimit    *ratelimit.Limiter
	recordGeoIPMu sync.Mutex
	basePath      string
	restartOnce   sync.Once
	restart       chan struct{}
}

func New(cfg config.Config, st *store.Store, loggers ...*slog.Logger) *Server {
	resolver, _ := clientip.New(cfg.TrustedProxies)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &Server{
		Config:     cfg,
		Store:      st,
		Started:    time.Now().UTC(),
		clientIP:   resolver,
		ipLimit:    ratelimit.New(120, 30),
		siteLimit:  ratelimit.New(3000, 500),
		logger:     logger,
		mapCache:   newMapCache(),
		loginLimit: ratelimit.New(10, 5),
		basePath:   config.BasePath(cfg.BaseURL),
		restart:    make(chan struct{}),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", s.live)
	mux.HandleFunc("GET /health/ready", s.ready)
	mux.HandleFunc("/admin/login", s.adminLogin)
	mux.HandleFunc("/admin/logout", s.adminLogout)
	mux.HandleFunc("GET /admin/assets/style.css", s.adminAssets)
	mux.HandleFunc("GET /assets/analytics.js", s.analyticsAssets)
	mux.HandleFunc("GET /admin", s.adminDashboard)
	mux.HandleFunc("GET /admin/settings", s.adminSettings)
	mux.HandleFunc("POST /admin/settings/configuration", s.adminUpdateConfiguration)
	mux.HandleFunc("POST /admin/settings/geoip/update", s.adminRunGeoIPUpdateFromSettings)
	mux.HandleFunc("POST /admin/settings/password", s.adminChangePassword)
	mux.HandleFunc("POST /admin/settings/update", s.adminRunSelfUpdate)
	mux.HandleFunc("POST /admin/settings/update/local", s.adminRunLocalSelfUpdate)
	mux.HandleFunc("GET /admin/records", s.adminRecords)
	mux.HandleFunc("GET /admin/records.csv", s.adminRecordsCSV)
	mux.HandleFunc("GET /admin/aggregates.csv", s.adminAggregatesCSV)
	mux.HandleFunc("POST /admin/operations/backup", s.adminRunBackup)
	mux.HandleFunc("POST /admin/operations/cleanup", s.adminRunCleanup)
	mux.HandleFunc("POST /admin/operations/geoip", s.adminRunGeoIPUpdate)
	mux.HandleFunc("GET /admin/sites/new", s.adminNewSite)
	mux.HandleFunc("POST /admin/sites/new", s.adminCreateSite)
	mux.HandleFunc("GET /admin/sites/{siteID}", s.adminSite)
	mux.HandleFunc("GET /admin/sites/{siteID}/analytics", s.adminSiteAnalytics)
	mux.HandleFunc("POST /admin/sites/{siteID}/settings", s.adminUpdateSite)
	mux.HandleFunc("POST /admin/sites/{siteID}/preset", s.adminUpdatePreset)
	mux.HandleFunc("POST /admin/sites/{siteID}/records/geoip", s.adminRefreshSiteRecordGeoIP)
	mux.HandleFunc("POST /admin/sites/{siteID}/reset", s.adminResetSite)
	mux.HandleFunc("POST /admin/sites/{siteID}/delete", s.adminDeleteSite)
	mux.HandleFunc("GET /admin/sites/{siteID}/preset-preview.svg", s.adminPresetPreview)
	mux.HandleFunc("GET /public/{siteID}/analytics", s.publicAnalytics)
	mux.HandleFunc("GET /public/{siteID}/analytics-map.svg", s.publicAnalyticsMap)
	mux.HandleFunc("POST /api/v1/sites/{siteID}/pageviews", s.collectPageview)
	mux.HandleFunc("GET /embed/tracker.js", s.trackerScript)
	mux.HandleFunc("GET /embed/widget.js", s.widgetScript)
	mux.HandleFunc("GET /api/v1/sites/{siteID}/map.svg", s.mapSVG)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		s.redirect(w, r, "/admin", http.StatusSeeOther)
	})
	if s.basePath == "" {
		return mux
	}
	outer := http.NewServeMux()
	outer.Handle(s.basePath+"/", http.StripPrefix(s.basePath, mux))
	outer.HandleFunc(s.basePath, func(w http.ResponseWriter, r *http.Request) {
		target := s.basePath + "/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
	return outer
}

func (s *Server) appPath(value string) string {
	if value == "" {
		value = "/"
	} else if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return s.basePath + value
}

func (s *Server) cookiePath() string {
	if s.basePath == "" {
		return "/"
	}
	return s.basePath + "/"
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, target string, status int) {
	http.Redirect(w, r, s.appPath(target), status)
}

func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr:              s.Config.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func (s *Server) SetGeoIP(resolver *geoip.Resolver) {
	s.geoMu.Lock()
	defer s.geoMu.Unlock()
	previous := s.geoIP
	s.geoIP = resolver
	if previous != nil && previous != resolver {
		_ = previous.Close()
	}
}

func (s *Server) CloseGeoIP() {
	s.SetGeoIP(nil)
}

func (s *Server) Ready(ctx context.Context) bool {
	if err := s.Store.DB.PingContext(ctx); err != nil {
		return false
	}
	if err := s.Store.SchemaReady(ctx); err != nil {
		return false
	}
	s.geoMu.RLock()
	defer s.geoMu.RUnlock()
	return s.geoIP != nil
}

func (s *Server) RequestRestart() {
	s.restartOnce.Do(func() { close(s.restart) })
}

func (s *Server) RestartRequested() <-chan struct{} {
	return s.restart
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
	s.geoMu.RLock()
	geoAvailable := s.geoIP != nil
	s.geoMu.RUnlock()
	if geoAvailable {
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
	hostname, err := visitorsite.HostnameFromOrigin(origin)
	if err != nil {
		http.Error(w, "origin hostname is invalid", http.StatusBadRequest)
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
	if payload.Hostname != "" && normalizeReportedHostname(payload.Hostname) != hostname {
		http.Error(w, "reported hostname does not match origin", http.StatusBadRequest)
		return
	}
	digest, err := visitor.Digest(configuredSite.HMACKey, payload.VisitorID, address.String(), userAgentValue)
	if err != nil {
		http.Error(w, "invalid visitor identity", http.StatusBadRequest)
		return
	}
	s.geoMu.RLock()
	location := geoip.Location{}
	if s.geoIP != nil {
		location = s.geoIP.Lookup(address)
	}
	s.geoMu.RUnlock()
	_, err = s.Store.RecordPageview(r.Context(), store.PageviewObservation{
		SiteID:          siteID,
		Hostname:        hostname,
		Path:            path,
		CountryCode:     location.CountryCode,
		RegionCode:      location.RegionCode,
		City:            location.City,
		Latitude:        location.Latitude,
		Longitude:       location.Longitude,
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

func normalizeReportedHostname(value string) string {
	value = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = value[1 : len(value)-1]
	}
	return value
}

func (s *Server) trackerScript(w http.ResponseWriter, r *http.Request) {
	s.serveEmbedScript(w, r)
}

func (s *Server) widgetScript(w http.ResponseWriter, r *http.Request) {
	s.serveEmbedScript(w, r)
}

func (s *Server) serveEmbedScript(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) mapSVG(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("siteID")
	configuredSite, err := s.Store.GetSite(r.Context(), siteID)
	if err != nil || !configuredSite.PublishPublic {
		http.Error(w, "unknown Site", http.StatusNotFound)
		return
	}
	preset, err := maprender.ParsePresetJSON(configuredSite.MapPresetJSON)
	if err != nil {
		s.logger.Error("load Map Preset failed", "site_id", siteID, "error", err)
		http.Error(w, "could not load Map Preset", http.StatusInternalServerError)
		return
	}
	options, err := maprender.ParseOptionsWithDefaults(r.URL.Query(), preset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := siteID + "|" + options.CacheKey()
	cached, err := s.mapCache.getOrRender(r.Context(), key, siteID, time.Now(), func() (mapCacheItem, error) {
		data, err := s.Store.PublicMapData(r.Context(), siteID)
		if err != nil {
			return mapCacheItem{}, err
		}
		body, err := maprender.Render(data, options)
		if err != nil {
			return mapCacheItem{}, err
		}
		sum := sha256.Sum256(body)
		return mapCacheItem{
			body: body, etag: fmt.Sprintf("\"%x\"", sum[:16]), expiresAt: time.Now().Add(mapCacheTTL),
		}, nil
	})
	if errors.Is(err, store.ErrPublicationDisabled) {
		http.Error(w, "public views are disabled", http.StatusNotFound)
		return
	}
	if err != nil {
		s.logger.Error("render Public Map failed", "site_id", siteID, "error", err)
		http.Error(w, "could not render Public Map", http.StatusInternalServerError)
		return
	}
	s.writeMapResponse(w, r, cached)
}

func (s *Server) writeMapResponse(w http.ResponseWriter, r *http.Request, cached mapCacheItem) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("ETag", cached.etag)
	if r.Header.Get("If-None-Match") == cached.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = w.Write(cached.body)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
