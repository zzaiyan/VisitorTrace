package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrCollectionDisabled = errors.New("Pageview collection is disabled for this Site")

type PageviewObservation struct {
	SiteID          string
	OccurredAt      time.Time
	Path            string
	CountryCode     string
	RegionCode      string
	City            string
	Latitude        *float64
	Longitude       *float64
	VisitorDigest   []byte
	OriginalIP      string
	OperatingSystem string
	Browser         string
}

type RecordPageviewResult struct {
	ID                  int64
	LocalDate           string
	DeduplicationWindow string
	NewOverallVisitor   bool
}

type aggregateDimension struct {
	kind  string
	value string
}

func (s *Store) RecordPageview(ctx context.Context, observation PageviewObservation) (RecordPageviewResult, error) {
	if len(observation.VisitorDigest) != 32 {
		return RecordPageviewResult{}, fmt.Errorf("Visitor Digest must contain 32 bytes")
	}
	if observation.OccurredAt.IsZero() {
		observation.OccurredAt = time.Now().UTC()
	} else {
		observation.OccurredAt = observation.OccurredAt.UTC()
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RecordPageviewResult{}, fmt.Errorf("begin Pageview transaction: %w", err)
	}
	defer tx.Rollback()

	var timezone string
	var collectionEnabled int
	var windowDays int
	err = tx.QueryRowContext(ctx, `SELECT timezone, accept_pageviews, dedup_window_days FROM sites WHERE id = ?`, observation.SiteID).Scan(&timezone, &collectionEnabled, &windowDays)
	if err != nil {
		if err == sql.ErrNoRows {
			return RecordPageviewResult{}, fmt.Errorf("Site %q not found", observation.SiteID)
		}
		return RecordPageviewResult{}, fmt.Errorf("read Pageview Site: %w", err)
	}
	if collectionEnabled != 1 {
		return RecordPageviewResult{}, ErrCollectionDisabled
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return RecordPageviewResult{}, fmt.Errorf("load Site timezone: %w", err)
	}
	local := observation.OccurredAt.In(location)
	localDate := local.Format(time.DateOnly)
	windowStartTime := deduplicationWindowStart(local, windowDays)
	windowStart := windowStartTime.Format(time.DateOnly)
	windowEnd := windowStartTime.AddDate(0, 0, windowDays).Format(time.DateOnly)

	result, err := tx.ExecContext(ctx, `
		INSERT INTO pageviews (
			site_id, occurred_at, local_date, path, country_code, region_code, city,
			latitude, longitude, visitor_digest, original_ip, operating_system, browser
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, observation.SiteID, observation.OccurredAt.Format(time.RFC3339Nano), localDate, observation.Path,
		observation.CountryCode, observation.RegionCode, observation.City, observation.Latitude, observation.Longitude,
		observation.VisitorDigest, observation.OriginalIP, observation.OperatingSystem, observation.Browser)
	if err != nil {
		return RecordPageviewResult{}, fmt.Errorf("insert Pageview Record: %w", err)
	}
	pageviewID, err := result.LastInsertId()
	if err != nil {
		return RecordPageviewResult{}, fmt.Errorf("read Pageview Record ID: %w", err)
	}

	dimensions := aggregateDimensions(observation)
	if observation.City != "" && observation.Latitude != nil && observation.Longitude != nil {
		cityValue := observation.CountryCode + "|" + observation.RegionCode + "|" + observation.City
		_, err := tx.ExecContext(ctx, `
			INSERT INTO geo_locations (
				site_id, dimension_kind, dimension_value, country_code, region_code, city,
				latitude, longitude, updated_at
			) VALUES (?, 'city', ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (site_id, dimension_kind, dimension_value)
			DO UPDATE SET
				latitude = excluded.latitude,
				longitude = excluded.longitude,
				updated_at = excluded.updated_at
		`, observation.SiteID, cityValue, observation.CountryCode, observation.RegionCode, observation.City,
			*observation.Latitude, *observation.Longitude, observation.OccurredAt.Format(time.RFC3339Nano))
		if err != nil {
			return RecordPageviewResult{}, fmt.Errorf("update durable geographic location: %w", err)
		}
	}
	newOverall := false
	for _, dimension := range dimensions {
		registration, err := tx.ExecContext(ctx, `
			INSERT INTO visitor_registrations (
				site_id, window_start, dimension_kind, dimension_value, visitor_digest, created_at, window_end
			) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING
		`, observation.SiteID, windowStart, dimension.kind, dimension.value, observation.VisitorDigest, observation.OccurredAt.Format(time.RFC3339Nano), windowEnd)
		if err != nil {
			return RecordPageviewResult{}, fmt.Errorf("register Unique Visitor: %w", err)
		}
		rows, err := registration.RowsAffected()
		if err != nil {
			return RecordPageviewResult{}, fmt.Errorf("read Unique Visitor registration result: %w", err)
		}
		uniqueIncrement := int64(0)
		if rows == 1 {
			uniqueIncrement = 1
			if dimension.kind == "overall" {
				newOverall = true
			}
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO daily_aggregates (
				site_id, local_date, dimension_kind, dimension_value, pageviews, unique_visitors
			) VALUES (?, ?, ?, ?, 1, ?)
			ON CONFLICT (site_id, local_date, dimension_kind, dimension_value)
			DO UPDATE SET
				pageviews = pageviews + 1,
				unique_visitors = unique_visitors + excluded.unique_visitors
		`, observation.SiteID, localDate, dimension.kind, dimension.value, uniqueIncrement)
		if err != nil {
			return RecordPageviewResult{}, fmt.Errorf("update daily Aggregate: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE sites
		SET first_pageview_at = COALESCE(first_pageview_at, ?), updated_at = ?
		WHERE id = ?
	`, observation.OccurredAt.Format(time.RFC3339Nano), observation.OccurredAt.Format(time.RFC3339Nano), observation.SiteID)
	if err != nil {
		return RecordPageviewResult{}, fmt.Errorf("lock Site timezone: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RecordPageviewResult{}, fmt.Errorf("commit Pageview transaction: %w", err)
	}
	return RecordPageviewResult{ID: pageviewID, LocalDate: localDate, DeduplicationWindow: windowStart, NewOverallVisitor: newOverall}, nil
}

func aggregateDimensions(observation PageviewObservation) []aggregateDimension {
	result := []aggregateDimension{
		{kind: "overall", value: "*"},
		{kind: "path", value: observation.Path},
		{kind: "browser", value: fallbackDimension(observation.Browser)},
		{kind: "os", value: fallbackDimension(observation.OperatingSystem)},
		{kind: "country", value: fallbackDimension(observation.CountryCode)},
	}
	if observation.RegionCode != "" {
		result = append(result, aggregateDimension{kind: "region", value: observation.CountryCode + "|" + observation.RegionCode})
	}
	if observation.City != "" {
		result = append(result, aggregateDimension{kind: "city", value: observation.CountryCode + "|" + observation.RegionCode + "|" + observation.City})
	}
	return result
}

func fallbackDimension(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func deduplicationWindowStart(local time.Time, days int) time.Time {
	dateUTC := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	anchor := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
	daysSinceAnchor := int(dateUTC.Sub(anchor) / (24 * time.Hour))
	windowDay := daysSinceAnchor - daysSinceAnchor%days
	startUTC := anchor.AddDate(0, 0, windowDay)
	return time.Date(startUTC.Year(), startUTC.Month(), startUTC.Day(), 0, 0, 0, 0, local.Location())
}
