package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/store"
)

type recordGeoIPTask struct {
	SiteID   string                             `json:"-"`
	State    string                             `json:"state"`
	Progress store.PageviewGeoIPRefreshProgress `json:"progress"`
	Result   store.PageviewGeoIPRefreshResult   `json:"result"`
	Error    string                             `json:"error,omitempty"`
	Redirect string                             `json:"redirect,omitempty"`
}

func (s *Server) recordGeoIPTaskForSite(siteID string) *recordGeoIPTask {
	s.recordGeoIPTaskMu.RLock()
	defer s.recordGeoIPTaskMu.RUnlock()
	task := s.recordGeoIPTasks[siteID]
	if task == nil {
		return nil
	}
	copy := *task
	return &copy
}

func (s *Server) adminRefreshSiteRecordGeoIP(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, translate(adminLanguage(r), "err_csrf"))
		return
	}
	siteID := r.PathValue("siteID")
	if _, err := s.Store.GetSite(r.Context(), siteID); err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#records", translate(adminLanguage(r), "err_site_not_found"))
		return
	}
	if !s.recordGeoIPMu.TryLock() {
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#records", translate(adminLanguage(r), "err_geoip_refresh_busy"))
		return
	}

	if !s.geoIPAvailable() {
		s.recordGeoIPMu.Unlock()
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#records", translate(adminLanguage(r), "err_geoip_unavailable"))
		return
	}
	s.recordGeoIPTaskMu.Lock()
	s.recordGeoIPTasks[siteID] = &recordGeoIPTask{SiteID: siteID, State: "running", Progress: store.PageviewGeoIPRefreshProgress{Stage: "preparing"}}
	s.recordGeoIPTaskMu.Unlock()
	go s.runRecordGeoIPRefresh(siteID)
	s.redirect(w, r, "/admin/sites/"+siteID+"#records", http.StatusSeeOther)
}

func (s *Server) runRecordGeoIPRefresh(siteID string) {
	defer s.recordGeoIPMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	result, err := s.Store.RefreshPageviewGeoIPWithProgress(ctx, siteID, func(address netip.Addr) store.PageviewGeography {
		location := s.locate(ctx, address)
		return store.PageviewGeography{
			CountryCode: location.CountryCode, RegionCode: location.RegionCode, City: location.City,
			Latitude: location.Latitude, Longitude: location.Longitude,
		}
	}, func(progress store.PageviewGeoIPRefreshProgress) {
		s.recordGeoIPTaskMu.Lock()
		s.recordGeoIPTasks[siteID].Progress = progress
		s.recordGeoIPTaskMu.Unlock()
	})
	s.recordGeoIPTaskMu.Lock()
	defer s.recordGeoIPTaskMu.Unlock()
	task := s.recordGeoIPTasks[siteID]
	if err != nil {
		task.State = "failed"
		task.Error = err.Error()
		s.logger.Error("refresh Pageview geography failed", "site_id", siteID, "error", err)
		return
	}
	s.mapCache.deleteSite(siteID)
	task.State = "success"
	task.Result = result
	task.Redirect = s.appPath("/admin/sites/"+siteID) + "?" + recordGeoIPResultQuery(result).Encode() + "#records"
}

func (s *Server) adminRecordGeoIPStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	task := s.recordGeoIPTaskForSite(r.PathValue("siteID"))
	if task == nil {
		task = &recordGeoIPTask{State: "idle"}
	}
	_ = json.NewEncoder(w).Encode(task)
}

func recordGeoIPResultQuery(result store.PageviewGeoIPRefreshResult) url.Values {
	query := url.Values{
		"saved":     {"record-geoip"},
		"processed": {strconv.FormatInt(result.Processed, 10)},
		"changed":   {strconv.FormatInt(result.Changed, 10)},
		"located":   {strconv.FormatInt(result.Located, 10)},
		"unmatched": {strconv.FormatInt(result.Unmatched, 10)},
		"invalid":   {strconv.FormatInt(result.InvalidIP, 10)},
		"dates":     {strconv.FormatInt(result.AggregateDates, 10)},
	}
	return query
}

func (s *Server) geoIPAvailable() bool {
	s.geoMu.RLock()
	defer s.geoMu.RUnlock()
	return s.geoIPAvailableLocked()
}

func (s *Server) geoIPAvailableLocked() bool {
	return s.geoIP != nil || s.geoSingleOnline != nil || s.geoChain.Available()
}

func recordGeoIPFlash(r *http.Request, lang string) string {
	if r.URL.Query().Get("saved") != "record-geoip" {
		return ""
	}
	value := func(key string) int64 {
		parsed, err := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
		if err != nil || parsed < 0 {
			return 0
		}
		return parsed
	}
	return fmt.Sprintf(
		translate(lang, "flash_record_geoip"),
		value("processed"), value("changed"), value("located"), value("unmatched"), value("invalid"), value("dates"),
	)
}
