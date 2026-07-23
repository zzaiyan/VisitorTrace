package server

import (
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"

	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func (s *Server) adminRefreshSiteRecordGeoIP(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	siteID := r.PathValue("siteID")
	if _, err := s.Store.GetSite(r.Context(), siteID); err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#records", "Site 不存在。")
		return
	}
	if !s.recordGeoIPMu.TryLock() {
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#records", "另一项 Pageview 地理信息刷新正在运行。")
		return
	}
	defer s.recordGeoIPMu.Unlock()

	s.geoMu.RLock()
	defer s.geoMu.RUnlock()
	if s.geoIP == nil {
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#records", "GeoIP 数据库当前不可用。")
		return
	}
	result, err := s.Store.RefreshPageviewGeoIP(r.Context(), siteID, func(address netip.Addr) store.PageviewGeography {
		location := s.geoIP.Lookup(address)
		return store.PageviewGeography{
			CountryCode: location.CountryCode, RegionCode: location.RegionCode, City: location.City,
			Latitude: location.Latitude, Longitude: location.Longitude,
		}
	})
	if err != nil {
		s.redirectWithError(w, r, "/admin/sites/"+siteID+"#records", "无法刷新 Pageview 地理信息："+err.Error())
		return
	}
	s.mapCache.deleteSite(siteID)
	query := url.Values{
		"saved":     {"record-geoip"},
		"processed": {strconv.FormatInt(result.Processed, 10)},
		"changed":   {strconv.FormatInt(result.Changed, 10)},
		"located":   {strconv.FormatInt(result.Located, 10)},
		"unmatched": {strconv.FormatInt(result.Unmatched, 10)},
		"invalid":   {strconv.FormatInt(result.InvalidIP, 10)},
		"dates":     {strconv.FormatInt(result.AggregateDates, 10)},
	}
	s.redirect(w, r, "/admin/sites/"+siteID+"?"+query.Encode()+"#records", http.StatusSeeOther)
}

func (s *Server) geoIPAvailable() bool {
	s.geoMu.RLock()
	defer s.geoMu.RUnlock()
	return s.geoIP != nil
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
