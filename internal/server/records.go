package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/pageview"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

var recordFilterKeys = []string{"site_id", "from", "to", "path", "ip", "digest", "country", "region", "city", "browser", "os"}

type recordFilterValues struct {
	SiteID  string
	From    string
	To      string
	Path    string
	IP      string
	Digest  string
	Country string
	Region  string
	City    string
	Browser string
	OS      string
	Limit   int
}

type recordsPageData struct {
	pageLayout
	Sites        []store.Site
	Records      []store.PageviewRecord
	Filters      recordFilterValues
	OlderURL     string
	NewerURL     string
	RecordCSVURL string
}

type recordCursorEnvelope struct {
	OccurredAt string `json:"occurred_at"`
	ID         int64  `json:"id"`
	Filter     string `json:"filter"`
}

func (s *Server) adminRecords(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	sites, err := s.Store.ListSites(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法读取 Site。")
		return
	}
	filters, values, err := parseRecordFilters(r.URL.Query())
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if values.SiteID != "" && !siteInList(sites, values.SiteID) {
		s.renderError(w, r, http.StatusBadRequest, "Site 不存在。")
		return
	}
	fingerprint := recordFilterFingerprint(filters, values.Limit)
	var cursor *store.PageviewCursor
	direction := r.URL.Query().Get("direction")
	if direction == "" {
		direction = "older"
	}
	if direction != "older" && direction != "newer" {
		s.renderError(w, r, http.StatusBadRequest, "分页方向无效。")
		return
	}
	if token := r.URL.Query().Get("cursor"); token != "" {
		cursor, err = decodeRecordCursor(token, fingerprint)
		if err != nil {
			s.renderError(w, r, http.StatusBadRequest, "分页游标无效或与当前筛选不匹配。")
			return
		}
	}
	page, err := s.Store.PageviewRecords(r.Context(), filters, cursor, direction, values.Limit)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法读取 Pageview Record。")
		return
	}
	csvQuery := recordFilterQuery(values, false).Encode()
	recordCSVURL := "/admin/records.csv"
	if csvQuery != "" {
		recordCSVURL += "?" + csvQuery
	}
	recordCSVURL = s.appPath(recordCSVURL)
	data := recordsPageData{
		pageLayout: s.adminLayout(r, session, "Pageview Records", "records"),
		Sites:      sites, Records: page.Records, Filters: values,
		RecordCSVURL: recordCSVURL,
	}
	if len(page.Records) > 0 {
		first := page.Records[0]
		last := page.Records[len(page.Records)-1]
		if direction == "older" {
			if page.More {
				data.OlderURL = s.appPath(recordPageURL(values, encodeRecordCursor(last, fingerprint), "older"))
			}
			if cursor != nil {
				data.NewerURL = s.appPath(recordPageURL(values, encodeRecordCursor(first, fingerprint), "newer"))
			}
		} else {
			if page.More {
				data.NewerURL = s.appPath(recordPageURL(values, encodeRecordCursor(first, fingerprint), "newer"))
			}
			if cursor != nil {
				data.OlderURL = s.appPath(recordPageURL(values, encodeRecordCursor(last, fingerprint), "older"))
			}
		}
	}
	s.renderPage(w, r, "records", data)
}

func (s *Server) adminRecordsCSV(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	filters, _, err := parseRecordFilters(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="visitortrace-pageviews.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{
		"id", "site_id", "site_name", "occurred_at_utc", "occurred_at_site_time", "local_date", "path",
		"country_code", "region_code", "city", "latitude", "longitude", "visitor_digest", "original_ip", "operating_system", "browser",
	})
	err = s.Store.ExportPageviewRecords(r.Context(), filters, func(record store.PageviewRecord) error {
		return writer.Write(pageviewCSVRow(record))
	})
	writer.Flush()
	if err == nil {
		err = writer.Error()
	}
	if err != nil {
		s.logger.Error("stream Pageview Record CSV failed", "error", err)
	}
}

func (s *Server) adminAggregatesCSV(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	siteID := strings.TrimSpace(r.URL.Query().Get("site_id"))
	dimension := strings.TrimSpace(r.URL.Query().Get("dimension"))
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	if err := store.ValidateAggregateDates(start, end); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !store.ValidAggregateDimension(dimension) {
		http.Error(w, "unsupported Aggregate dimension", http.StatusBadRequest)
		return
	}
	if _, err := s.Store.GetSite(r.Context(), siteID); err != nil {
		http.Error(w, "unknown Site", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="visitortrace-aggregates.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"site_id", "site_name", "local_date", "dimension_kind", "dimension_value", "pageviews", "unique_visitors"})
	err := s.Store.ExportAggregates(r.Context(), siteID, start, end, dimension, func(row store.AggregateExportRow) error {
		return writer.Write([]string{
			csvSafe(row.SiteID), csvSafe(row.SiteName), row.LocalDate, row.DimensionKind, csvSafe(row.DimensionValue),
			strconv.FormatInt(row.Pageviews, 10), strconv.FormatInt(row.UniqueVisitors, 10),
		})
	})
	writer.Flush()
	if err == nil {
		err = writer.Error()
	}
	if err != nil {
		s.logger.Error("stream Aggregate CSV failed", "error", err)
	}
}

func parseRecordFilters(query url.Values) (store.PageviewFilters, recordFilterValues, error) {
	limit := 100
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || (parsed != 50 && parsed != 100 && parsed != 200) {
			return store.PageviewFilters{}, recordFilterValues{}, fmt.Errorf("每页数量必须是 50、100 或 200")
		}
		limit = parsed
	}
	values := recordFilterValues{
		SiteID: strings.TrimSpace(query.Get("site_id")), From: strings.TrimSpace(query.Get("from")), To: strings.TrimSpace(query.Get("to")),
		Path: strings.TrimSpace(query.Get("path")), IP: strings.TrimSpace(query.Get("ip")), Digest: strings.TrimSpace(query.Get("digest")),
		Country: strings.ToUpper(strings.TrimSpace(query.Get("country"))), Region: strings.TrimSpace(query.Get("region")),
		City: strings.TrimSpace(query.Get("city")), Browser: strings.TrimSpace(query.Get("browser")), OS: strings.TrimSpace(query.Get("os")), Limit: limit,
	}
	filters := store.PageviewFilters{
		SiteID: values.SiteID, CountryCode: values.Country, RegionCode: values.Region, City: values.City,
		Browser: values.Browser, OperatingSystem: values.OS,
	}
	if values.From != "" {
		parsed, err := parseUTCFilterTime(values.From)
		if err != nil {
			return filters, values, fmt.Errorf("起始时间无效")
		}
		filters.OccurredFrom = &parsed
	}
	if values.To != "" {
		parsed, err := parseUTCFilterTime(values.To)
		if err != nil {
			return filters, values, fmt.Errorf("结束时间无效")
		}
		filters.OccurredTo = &parsed
	}
	if filters.OccurredFrom != nil && filters.OccurredTo != nil && filters.OccurredFrom.After(*filters.OccurredTo) {
		return filters, values, fmt.Errorf("起始时间不能晚于结束时间")
	}
	if values.Path != "" {
		normalized, err := pageview.NormalizePath(values.Path)
		if err != nil {
			return filters, values, fmt.Errorf("路径筛选无效")
		}
		filters.Path = normalized
		values.Path = normalized
	}
	if values.IP != "" {
		address, err := netip.ParseAddr(values.IP)
		if err != nil {
			return filters, values, fmt.Errorf("IP 筛选无效")
		}
		filters.OriginalIP = address.String()
		values.IP = address.String()
	}
	if values.Digest != "" {
		digest, err := hex.DecodeString(values.Digest)
		if err != nil || len(digest) != sha256.Size {
			return filters, values, fmt.Errorf("Visitor Digest 必须是 64 位十六进制值")
		}
		filters.VisitorDigest = digest
		values.Digest = strings.ToLower(values.Digest)
	}
	for name, value := range map[string]string{"Site ID": values.SiteID, "国家": values.Country, "地区": values.Region, "城市": values.City, "浏览器": values.Browser, "操作系统": values.OS} {
		if len(value) > 200 {
			return filters, values, fmt.Errorf("%s筛选过长", name)
		}
	}
	return filters, values, nil
}

func parseUTCFilterTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", time.DateOnly} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid UTC time")
}

func recordFilterFingerprint(filters store.PageviewFilters, limit int) string {
	payload, _ := json.Marshal(struct {
		Filters store.PageviewFilters `json:"filters"`
		Limit   int                   `json:"limit"`
	}{filters, limit})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:8])
}

func encodeRecordCursor(record store.PageviewRecord, fingerprint string) string {
	payload, _ := json.Marshal(recordCursorEnvelope{
		OccurredAt: record.OccurredAt.UTC().Format(time.RFC3339Nano), ID: record.ID, Filter: fingerprint,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeRecordCursor(token, fingerprint string) (*store.PageviewCursor, error) {
	if len(token) > 2048 {
		return nil, fmt.Errorf("invalid cursor")
	}
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(payload) > 1024 {
		return nil, fmt.Errorf("invalid cursor")
	}
	var envelope recordCursorEnvelope
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("invalid cursor content")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, envelope.OccurredAt)
	if err != nil || envelope.ID < 1 || envelope.Filter != fingerprint {
		return nil, fmt.Errorf("invalid cursor fields")
	}
	return &store.PageviewCursor{OccurredAt: occurredAt, ID: envelope.ID}, nil
}

func recordFilterQuery(values recordFilterValues, includeLimit bool) url.Values {
	query := url.Values{}
	input := map[string]string{
		"site_id": values.SiteID, "from": values.From, "to": values.To, "path": values.Path, "ip": values.IP,
		"digest": values.Digest, "country": values.Country, "region": values.Region, "city": values.City,
		"browser": values.Browser, "os": values.OS,
	}
	for _, key := range recordFilterKeys {
		if input[key] != "" {
			query.Set(key, input[key])
		}
	}
	if includeLimit && values.Limit != 100 {
		query.Set("limit", strconv.Itoa(values.Limit))
	}
	return query
}

func recordPageURL(values recordFilterValues, cursor, direction string) string {
	query := recordFilterQuery(values, true)
	query.Set("cursor", cursor)
	query.Set("direction", direction)
	return "/admin/records?" + query.Encode()
}

func siteInList(sites []store.Site, siteID string) bool {
	for _, item := range sites {
		if item.ID == siteID {
			return true
		}
	}
	return false
}

func pageviewCSVRow(record store.PageviewRecord) []string {
	latitude, longitude := "", ""
	if record.Latitude.Valid {
		latitude = strconv.FormatFloat(record.Latitude.Float64, 'f', 6, 64)
	}
	if record.Longitude.Valid {
		longitude = strconv.FormatFloat(record.Longitude.Float64, 'f', 6, 64)
	}
	localTime := record.OccurredAt.UTC()
	if location, err := time.LoadLocation(record.SiteTimezone); err == nil {
		localTime = record.OccurredAt.In(location)
	}
	return []string{
		strconv.FormatInt(record.ID, 10), csvSafe(record.SiteID), csvSafe(record.SiteName), record.OccurredAt.UTC().Format(time.RFC3339Nano),
		localTime.Format(time.RFC3339Nano), record.LocalDate, csvSafe(record.Path), csvSafe(record.CountryCode), csvSafe(record.RegionCode),
		csvSafe(record.City), latitude, longitude, record.VisitorDigest, csvSafe(record.OriginalIP), csvSafe(record.OperatingSystem), csvSafe(record.Browser),
	}
}

func csvSafe(value string) string {
	if value != "" && strings.ContainsRune("=+-@", rune(value[0])) {
		return "'" + value
	}
	return value
}

func formatRecordTime(record store.PageviewRecord) string {
	location, err := time.LoadLocation(record.SiteTimezone)
	if err != nil {
		return record.OccurredAt.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	return record.OccurredAt.In(location).Format("2006-01-02 15:04:05 MST")
}
