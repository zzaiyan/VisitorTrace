package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"
)

const pageviewGeoIPBatchSize = 500

type PageviewGeography struct {
	CountryCode string
	RegionCode  string
	City        string
	Latitude    *float64
	Longitude   *float64
}

type PageviewGeoIPRefreshResult struct {
	Processed      int64
	Changed        int64
	Located        int64
	Unmatched      int64
	InvalidIP      int64
	AggregateDates int64
}

type geoRefreshRule struct {
	effectiveDate string
	windowDays    int
}

type geoRefreshRecord struct {
	id            int64
	localDate     string
	hostname      string
	old           PageviewGeography
	latitude      sql.NullFloat64
	longitude     sql.NullFloat64
	visitorDigest []byte
	originalIP    string
	occurredAt    string
}

type geoRefreshAggregateKey struct {
	localDate string
	kind      string
	value     string
}

type geoRefreshAggregate struct {
	pageviews      int64
	uniqueVisitors int64
}

type geoRefreshVisitorKey struct {
	windowStart string
	kind        string
	value       string
	digest      string
}

type geoRefreshRegistration struct {
	windowStart string
	kind        string
	value       string
	digest      []byte
	createdAt   string
	windowEnd   string
}

// RefreshPageviewGeoIP replaces retained Pageview geography and rebuilds the
// geographic aggregates for dates that still have detailed records.
func (s *Store) RefreshPageviewGeoIP(ctx context.Context, siteID string, lookup func(netip.Addr) PageviewGeography) (PageviewGeoIPRefreshResult, error) {
	if strings.TrimSpace(siteID) == "" {
		return PageviewGeoIPRefreshResult{}, fmt.Errorf("Site ID is required")
	}
	if lookup == nil {
		return PageviewGeoIPRefreshResult{}, fmt.Errorf("GeoIP lookup is required")
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return PageviewGeoIPRefreshResult{}, fmt.Errorf("begin Pageview GeoIP refresh: %w", err)
	}
	defer tx.Rollback()

	var timezone string
	if err := tx.QueryRowContext(ctx, `SELECT timezone FROM sites WHERE id = ?`, siteID).Scan(&timezone); err != nil {
		if err == sql.ErrNoRows {
			return PageviewGeoIPRefreshResult{}, fmt.Errorf("Site %q not found", siteID)
		}
		return PageviewGeoIPRefreshResult{}, fmt.Errorf("read Pageview GeoIP Site: %w", err)
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return PageviewGeoIPRefreshResult{}, fmt.Errorf("load Pageview GeoIP Site timezone: %w", err)
	}
	currentLocalDate := time.Now().In(location).Format(time.DateOnly)
	rules, err := readGeoRefreshRules(ctx, tx, siteID)
	if err != nil {
		return PageviewGeoIPRefreshResult{}, err
	}

	aggregates := make(map[geoRefreshAggregateKey]*geoRefreshAggregate)
	visitors := make(map[geoRefreshVisitorKey]struct{})
	registrations := make(map[geoRefreshVisitorKey]geoRefreshRegistration)
	locations := make(map[string]PageviewGeography)
	coveredDates := make(map[string]struct{})
	result := PageviewGeoIPRefreshResult{}
	lastID := int64(0)

	for {
		records, err := readGeoRefreshBatch(ctx, tx, siteID, lastID)
		if err != nil {
			return PageviewGeoIPRefreshResult{}, err
		}
		if len(records) == 0 {
			break
		}
		for _, record := range records {
			lastID = record.id
			geography := record.old
			address, parseErr := netip.ParseAddr(strings.TrimSpace(record.originalIP))
			if parseErr != nil {
				result.InvalidIP++
			} else {
				geography = normalizedPageviewGeography(lookup(address.Unmap()))
				result.Processed++
				if pageviewGeographyLocated(geography) {
					result.Located++
				} else {
					result.Unmatched++
				}
				if !samePageviewGeography(record, geography) {
					result.Changed++
				}
				if _, err := tx.ExecContext(ctx, `
					UPDATE pageviews
					SET country_code = ?, region_code = ?, city = ?, latitude = ?, longitude = ?
					WHERE site_id = ? AND id = ?
				`, geography.CountryCode, geography.RegionCode, geography.City, geography.Latitude, geography.Longitude, siteID, record.id); err != nil {
					return PageviewGeoIPRefreshResult{}, fmt.Errorf("update Pageview GeoIP record %d: %w", record.id, err)
				}
			}

			coveredDates[record.localDate] = struct{}{}
			windowStart, windowEnd, err := geoRefreshWindow(record.localDate, rules)
			if err != nil {
				return PageviewGeoIPRefreshResult{}, err
			}
			for _, dimension := range geographicAggregateDimensions(geography) {
				aggregateKey := geoRefreshAggregateKey{localDate: record.localDate, kind: dimension.kind, value: dimension.value}
				counter := aggregates[aggregateKey]
				if counter == nil {
					counter = &geoRefreshAggregate{}
					aggregates[aggregateKey] = counter
				}
				counter.pageviews++
				visitorKey := geoRefreshVisitorKey{
					windowStart: windowStart,
					kind:        dimension.kind,
					value:       scopedVisitorDimension(record.hostname, dimension.value),
					digest:      string(record.visitorDigest),
				}
				if _, seen := visitors[visitorKey]; !seen {
					visitors[visitorKey] = struct{}{}
					counter.uniqueVisitors++
					registrations[visitorKey] = geoRefreshRegistration{
						windowStart: windowStart, kind: dimension.kind, value: visitorKey.value,
						digest: append([]byte(nil), record.visitorDigest...), createdAt: record.occurredAt, windowEnd: windowEnd,
					}
				}
			}
			if geography.City != "" && geography.Latitude != nil && geography.Longitude != nil {
				value := geography.CountryCode + "|" + geography.RegionCode + "|" + geography.City
				locations[value] = geography
			}
		}
	}

	if len(coveredDates) > 0 {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM daily_aggregates
			WHERE site_id = ? AND dimension_kind IN ('country', 'region', 'city')
			  AND local_date IN (SELECT DISTINCT local_date FROM pageviews WHERE site_id = ?)
		`, siteID, siteID); err != nil {
			return PageviewGeoIPRefreshResult{}, fmt.Errorf("remove stale geographic aggregates: %w", err)
		}
		if err := insertGeoRefreshAggregates(ctx, tx, siteID, aggregates); err != nil {
			return PageviewGeoIPRefreshResult{}, err
		}
		if err := upsertGeoRefreshLocations(ctx, tx, siteID, locations); err != nil {
			return PageviewGeoIPRefreshResult{}, err
		}
		if err := upsertGeoRefreshRegistrations(ctx, tx, siteID, currentLocalDate, registrations); err != nil {
			return PageviewGeoIPRefreshResult{}, err
		}
	}
	result.AggregateDates = int64(len(coveredDates))
	if err := tx.Commit(); err != nil {
		return PageviewGeoIPRefreshResult{}, fmt.Errorf("commit Pageview GeoIP refresh: %w", err)
	}
	return result, nil
}

func readGeoRefreshRules(ctx context.Context, tx *sql.Tx, siteID string) ([]geoRefreshRule, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT effective_date, window_days
		FROM site_deduplication_rules
		WHERE site_id = ?
		ORDER BY effective_date
	`, siteID)
	if err != nil {
		return nil, fmt.Errorf("read GeoIP refresh deduplication rules: %w", err)
	}
	defer rows.Close()
	var rules []geoRefreshRule
	for rows.Next() {
		var rule geoRefreshRule
		if err := rows.Scan(&rule.effectiveDate, &rule.windowDays); err != nil {
			return nil, fmt.Errorf("scan GeoIP refresh deduplication rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate GeoIP refresh deduplication rules: %w", err)
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("Site %q has no deduplication rules", siteID)
	}
	return rules, nil
}

func readGeoRefreshBatch(ctx context.Context, tx *sql.Tx, siteID string, afterID int64) ([]geoRefreshRecord, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, local_date, hostname, country_code, region_code, city, latitude, longitude,
		       visitor_digest, original_ip, occurred_at
		FROM pageviews
		WHERE site_id = ? AND id > ?
		ORDER BY id
		LIMIT ?
	`, siteID, afterID, pageviewGeoIPBatchSize)
	if err != nil {
		return nil, fmt.Errorf("read Pageview GeoIP batch: %w", err)
	}
	defer rows.Close()
	records := make([]geoRefreshRecord, 0, pageviewGeoIPBatchSize)
	for rows.Next() {
		var record geoRefreshRecord
		if err := rows.Scan(
			&record.id, &record.localDate, &record.hostname,
			&record.old.CountryCode, &record.old.RegionCode, &record.old.City, &record.latitude, &record.longitude,
			&record.visitorDigest, &record.originalIP, &record.occurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan Pageview GeoIP batch: %w", err)
		}
		if record.latitude.Valid {
			value := record.latitude.Float64
			record.old.Latitude = &value
		}
		if record.longitude.Valid {
			value := record.longitude.Float64
			record.old.Longitude = &value
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Pageview GeoIP batch: %w", err)
	}
	return records, nil
}

func geoRefreshWindow(localDate string, rules []geoRefreshRule) (string, string, error) {
	index := sort.Search(len(rules), func(index int) bool { return rules[index].effectiveDate > localDate }) - 1
	if index < 0 {
		return "", "", fmt.Errorf("no deduplication rule applies to Pageview date %s", localDate)
	}
	local, err := time.Parse(time.DateOnly, localDate)
	if err != nil {
		return "", "", fmt.Errorf("parse Pageview local date %q: %w", localDate, err)
	}
	anchor, err := time.Parse(time.DateOnly, rules[index].effectiveDate)
	if err != nil {
		return "", "", fmt.Errorf("parse deduplication rule date %q: %w", rules[index].effectiveDate, err)
	}
	start := deduplicationWindowStart(local, anchor, rules[index].windowDays)
	return start.Format(time.DateOnly), start.AddDate(0, 0, rules[index].windowDays).Format(time.DateOnly), nil
}

func geographicAggregateDimensions(geography PageviewGeography) []aggregateDimension {
	result := []aggregateDimension{{kind: "country", value: fallbackDimension(geography.CountryCode)}}
	if geography.RegionCode != "" {
		result = append(result, aggregateDimension{kind: "region", value: geography.CountryCode + "|" + geography.RegionCode})
	}
	if geography.City != "" {
		result = append(result, aggregateDimension{kind: "city", value: geography.CountryCode + "|" + geography.RegionCode + "|" + geography.City})
	}
	return result
}

func normalizedPageviewGeography(value PageviewGeography) PageviewGeography {
	value.CountryCode = strings.TrimSpace(value.CountryCode)
	value.RegionCode = strings.TrimSpace(value.RegionCode)
	value.City = strings.TrimSpace(value.City)
	return value
}

func pageviewGeographyLocated(value PageviewGeography) bool {
	return value.CountryCode != "" || value.RegionCode != "" || value.City != "" || value.Latitude != nil || value.Longitude != nil
}

func samePageviewGeography(record geoRefreshRecord, value PageviewGeography) bool {
	return record.old.CountryCode == value.CountryCode && record.old.RegionCode == value.RegionCode && record.old.City == value.City &&
		sameNullableFloat(record.latitude, value.Latitude) && sameNullableFloat(record.longitude, value.Longitude)
}

func sameNullableFloat(current sql.NullFloat64, updated *float64) bool {
	if !current.Valid || updated == nil {
		return !current.Valid && updated == nil
	}
	return current.Float64 == *updated
}

func insertGeoRefreshAggregates(ctx context.Context, tx *sql.Tx, siteID string, aggregates map[geoRefreshAggregateKey]*geoRefreshAggregate) error {
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO daily_aggregates (site_id, local_date, dimension_kind, dimension_value, pageviews, unique_visitors)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare geographic aggregate rebuild: %w", err)
	}
	defer statement.Close()
	for key, counter := range aggregates {
		if _, err := statement.ExecContext(ctx, siteID, key.localDate, key.kind, key.value, counter.pageviews, counter.uniqueVisitors); err != nil {
			return fmt.Errorf("insert rebuilt geographic aggregate: %w", err)
		}
	}
	return nil
}

func upsertGeoRefreshLocations(ctx context.Context, tx *sql.Tx, siteID string, locations map[string]PageviewGeography) error {
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO geo_locations (
			site_id, dimension_kind, dimension_value, country_code, region_code, city, latitude, longitude, updated_at
		) VALUES (?, 'city', ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (site_id, dimension_kind, dimension_value)
		DO UPDATE SET latitude = excluded.latitude, longitude = excluded.longitude, updated_at = excluded.updated_at
	`)
	if err != nil {
		return fmt.Errorf("prepare GeoIP location rebuild: %w", err)
	}
	defer statement.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for value, location := range locations {
		if _, err := statement.ExecContext(ctx, siteID, value, location.CountryCode, location.RegionCode, location.City, location.Latitude, location.Longitude, now); err != nil {
			return fmt.Errorf("upsert rebuilt GeoIP location: %w", err)
		}
	}
	return nil
}

func upsertGeoRefreshRegistrations(ctx context.Context, tx *sql.Tx, siteID, currentLocalDate string, registrations map[geoRefreshVisitorKey]geoRefreshRegistration) error {
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO visitor_registrations (
			site_id, window_start, dimension_kind, dimension_value, visitor_digest, created_at, window_end
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("prepare geographic visitor registrations: %w", err)
	}
	defer statement.Close()
	for _, registration := range registrations {
		if registration.windowEnd <= currentLocalDate {
			continue
		}
		if _, err := statement.ExecContext(ctx, siteID, registration.windowStart, registration.kind, registration.value, registration.digest, registration.createdAt, registration.windowEnd); err != nil {
			return fmt.Errorf("upsert geographic visitor registration: %w", err)
		}
	}
	return nil
}
