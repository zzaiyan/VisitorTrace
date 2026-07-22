package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type PageviewFilters struct {
	SiteID          string
	OccurredFrom    *time.Time
	OccurredTo      *time.Time
	Hostname        string
	Path            string
	OriginalIP      string
	VisitorDigest   []byte
	CountryCode     string
	RegionCode      string
	City            string
	Browser         string
	OperatingSystem string
}

type PageviewCursor struct {
	OccurredAt time.Time
	ID         int64
}

type PageviewRecordPage struct {
	Records []PageviewRecord
	More    bool
}

func (s *Store) PageviewRecords(ctx context.Context, filters PageviewFilters, cursor *PageviewCursor, direction string, limit int) (PageviewRecordPage, error) {
	if limit < 1 || limit > 200 {
		return PageviewRecordPage{}, fmt.Errorf("Pageview Record limit must be between 1 and 200")
	}
	if direction == "" {
		direction = "older"
	}
	if direction != "older" && direction != "newer" {
		return PageviewRecordPage{}, fmt.Errorf("invalid Pageview Record cursor direction")
	}
	query, args := pageviewRecordQuery(filters)
	if cursor != nil {
		operator := "<"
		if direction == "newer" {
			operator = ">"
		}
		query += fmt.Sprintf(` AND (julianday(p.occurred_at) %s julianday(?) OR (p.occurred_at = ? AND p.id %s ?))`, operator, operator)
		stamp := cursor.OccurredAt.UTC().Format(time.RFC3339Nano)
		args = append(args, stamp, stamp, cursor.ID)
	}
	order := "DESC"
	if direction == "newer" {
		order = "ASC"
	}
	query += ` ORDER BY julianday(p.occurred_at) ` + order + `, p.id ` + order + ` LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return PageviewRecordPage{}, fmt.Errorf("query Pageview Records: %w", err)
	}
	defer rows.Close()
	records, err := scanPageviewRecords(rows, limit+1)
	if err != nil {
		return PageviewRecordPage{}, err
	}
	more := len(records) > limit
	if more {
		records = records[:limit]
	}
	if direction == "newer" {
		reversePageviewRecords(records)
	}
	return PageviewRecordPage{Records: records, More: more}, nil
}

func (s *Store) ExportPageviewRecords(ctx context.Context, filters PageviewFilters, yield func(PageviewRecord) error) error {
	query, args := pageviewRecordQuery(filters)
	query += ` ORDER BY julianday(p.occurred_at) DESC, p.id DESC`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query Pageview Record export: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanPageviewRecord(rows)
		if err != nil {
			return err
		}
		if err := yield(item); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate Pageview Record export: %w", err)
	}
	return nil
}

func pageviewRecordQuery(filters PageviewFilters) (string, []any) {
	query := `
		SELECT p.id, p.site_id, s.name, s.timezone, p.occurred_at, p.local_date, p.hostname, p.path,
		       p.country_code, p.region_code, p.city, p.latitude, p.longitude,
		       p.visitor_digest, p.original_ip, p.operating_system, p.browser
		FROM pageviews AS p
		JOIN sites AS s ON s.id = p.site_id
		WHERE 1 = 1`
	args := make([]any, 0, 12)
	add := func(condition string, value any) {
		query += " AND " + condition
		args = append(args, value)
	}
	if filters.SiteID != "" {
		add("p.site_id = ?", filters.SiteID)
	}
	if filters.OccurredFrom != nil {
		add("julianday(p.occurred_at) >= julianday(?)", filters.OccurredFrom.UTC().Format(time.RFC3339Nano))
	}
	if filters.OccurredTo != nil {
		add("julianday(p.occurred_at) <= julianday(?)", filters.OccurredTo.UTC().Format(time.RFC3339Nano))
	}
	for _, filter := range []struct {
		value     string
		condition string
	}{
		{filters.Hostname, "p.hostname = ?"},
		{filters.Path, "p.path = ?"},
		{filters.OriginalIP, "p.original_ip = ?"},
		{filters.CountryCode, "p.country_code = ?"},
		{filters.RegionCode, "p.region_code = ?"},
		{filters.City, "p.city = ?"},
		{filters.Browser, "p.browser = ?"},
		{filters.OperatingSystem, "p.operating_system = ?"},
	} {
		if filter.value != "" {
			add(filter.condition, filter.value)
		}
	}
	if len(filters.VisitorDigest) > 0 {
		add("p.visitor_digest = ?", filters.VisitorDigest)
	}
	return query, args
}

type rowScanner interface {
	Scan(...any) error
}

func scanPageviewRecord(scanner rowScanner) (PageviewRecord, error) {
	var item PageviewRecord
	var occurred string
	var digest []byte
	if err := scanner.Scan(
		&item.ID, &item.SiteID, &item.SiteName, &item.SiteTimezone, &occurred, &item.LocalDate, &item.Hostname, &item.Path,
		&item.CountryCode, &item.RegionCode, &item.City, &item.Latitude, &item.Longitude,
		&digest, &item.OriginalIP, &item.OperatingSystem, &item.Browser,
	); err != nil {
		return PageviewRecord{}, fmt.Errorf("scan Pageview Record: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, occurred)
	if err != nil {
		return PageviewRecord{}, fmt.Errorf("parse Pageview timestamp: %w", err)
	}
	item.OccurredAt = parsed
	item.VisitorDigest = fmt.Sprintf("%x", digest)
	return item, nil
}

func scanPageviewRecords(rows *sql.Rows, capacity int) ([]PageviewRecord, error) {
	result := make([]PageviewRecord, 0, capacity)
	for rows.Next() {
		item, err := scanPageviewRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Pageview Records: %w", err)
	}
	return result, nil
}

func reversePageviewRecords(records []PageviewRecord) {
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
}

type AggregateExportRow struct {
	SiteID         string
	SiteName       string
	LocalDate      string
	DimensionKind  string
	DimensionValue string
	Pageviews      int64
	UniqueVisitors int64
}

func (s *Store) ExportAggregates(ctx context.Context, siteID, startDate, endDate, dimension string, yield func(AggregateExportRow) error) error {
	if siteID == "" {
		return fmt.Errorf("Site ID is required for Aggregate export")
	}
	if !ValidAggregateDimension(dimension) {
		return fmt.Errorf("unsupported Aggregate dimension %q", dimension)
	}
	query := `
		SELECT a.site_id, s.name, a.local_date, a.dimension_kind, a.dimension_value,
		       a.pageviews, a.unique_visitors
		FROM daily_aggregates AS a
		JOIN sites AS s ON s.id = a.site_id
		WHERE a.site_id = ? AND a.dimension_kind = ?`
	args := []any{siteID, dimension}
	if startDate != "" {
		query += ` AND a.local_date >= ?`
		args = append(args, startDate)
	}
	if endDate != "" {
		query += ` AND a.local_date <= ?`
		args = append(args, endDate)
	}
	query += ` ORDER BY a.local_date, a.dimension_value`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query Aggregate export: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item AggregateExportRow
		if err := rows.Scan(&item.SiteID, &item.SiteName, &item.LocalDate, &item.DimensionKind, &item.DimensionValue, &item.Pageviews, &item.UniqueVisitors); err != nil {
			return fmt.Errorf("scan Aggregate export: %w", err)
		}
		if err := yield(item); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate Aggregate export: %w", err)
	}
	return nil
}

func ValidAggregateDimension(value string) bool {
	return map[string]bool{"overall": true, "hostname": true, "path": true, "country": true, "region": true, "city": true, "browser": true, "os": true}[value]
}

func ValidateAggregateDates(startDate, endDate string) error {
	for _, value := range []string{startDate, endDate} {
		if value == "" {
			continue
		}
		if _, err := time.Parse(time.DateOnly, value); err != nil {
			return fmt.Errorf("Aggregate dates must use YYYY-MM-DD")
		}
	}
	if startDate != "" && endDate != "" && strings.Compare(startDate, endDate) > 0 {
		return fmt.Errorf("Aggregate start date must not be after end date")
	}
	return nil
}
