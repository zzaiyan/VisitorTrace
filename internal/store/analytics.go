package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type AnalyticsMetric struct {
	Value          string `json:"value"`
	Pageviews      int64  `json:"pageviews"`
	UniqueVisitors int64  `json:"unique_visitors"`
}

type DailyMetric struct {
	Date           string `json:"date"`
	Pageviews      int64  `json:"pageviews"`
	UniqueVisitors int64  `json:"unique_visitors"`
}

type PublicAnalyticsData struct {
	SiteID           string
	SiteName         string
	StartDate        string
	EndDate          string
	Pageviews        int64
	UniqueVisitors   int64
	Daily            []DailyMetric
	Countries        []AnalyticsMetric
	Cities           []AnalyticsMetric
	Browsers         []AnalyticsMetric
	OperatingSystems []AnalyticsMetric
	Paths            []AnalyticsMetric
	MapPoints        []MapPoint
}

type SiteOverview struct {
	Pageviews       int64
	UniqueVisitors  int64
	PageviewRecords int64
}

func (s *Store) SiteOverview(ctx context.Context, siteID string) (SiteOverview, error) {
	var result SiteOverview
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(pageviews), 0), COALESCE(SUM(unique_visitors), 0)
		FROM daily_aggregates
		WHERE site_id = ? AND dimension_kind = 'overall' AND dimension_value = '*'
	`, siteID).Scan(&result.Pageviews, &result.UniqueVisitors); err != nil {
		return SiteOverview{}, fmt.Errorf("read Site overview: %w", err)
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pageviews WHERE site_id = ?`, siteID).Scan(&result.PageviewRecords); err != nil {
		return SiteOverview{}, fmt.Errorf("count Pageview Records: %w", err)
	}
	return result, nil
}

func (s *Store) AnalyticsBounds(ctx context.Context, siteID string) (string, string, error) {
	var start, end sql.NullString
	if err := s.DB.QueryRowContext(ctx, `
		SELECT MIN(local_date), MAX(local_date)
		FROM daily_aggregates
		WHERE site_id = ? AND dimension_kind = 'overall' AND dimension_value = '*'
	`, siteID).Scan(&start, &end); err != nil {
		return "", "", fmt.Errorf("read Analytics date bounds: %w", err)
	}
	if !start.Valid || !end.Valid {
		return "", "", nil
	}
	return start.String, end.String, nil
}

func (s *Store) PublicAnalytics(ctx context.Context, siteID, startDate, endDate string) (PublicAnalyticsData, error) {
	site, err := s.GetSite(ctx, siteID)
	if err != nil {
		return PublicAnalyticsData{}, err
	}
	if !site.PublishPublic {
		return PublicAnalyticsData{}, ErrPublicationDisabled
	}
	return s.analyticsData(ctx, site, startDate, endDate, false)
}

func (s *Store) AdminAnalytics(ctx context.Context, siteID, startDate, endDate string) (PublicAnalyticsData, error) {
	site, err := s.GetSite(ctx, siteID)
	if err != nil {
		return PublicAnalyticsData{}, err
	}
	return s.analyticsData(ctx, site, startDate, endDate, true)
}

func (s *Store) analyticsData(ctx context.Context, site Site, startDate, endDate string, includePaths bool) (PublicAnalyticsData, error) {
	siteID := site.ID
	var err error
	startDate, endDate, err = analyticsDates(site.Timezone, startDate, endDate)
	if err != nil {
		return PublicAnalyticsData{}, err
	}
	result := PublicAnalyticsData{SiteID: site.ID, SiteName: site.Name, StartDate: startDate, EndDate: endDate}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(pageviews), 0), COALESCE(SUM(unique_visitors), 0)
		FROM daily_aggregates
		WHERE site_id = ? AND local_date BETWEEN ? AND ?
		  AND dimension_kind = 'overall' AND dimension_value = '*'
	`, siteID, startDate, endDate).Scan(&result.Pageviews, &result.UniqueVisitors); err != nil {
		return PublicAnalyticsData{}, fmt.Errorf("read Public Analytics totals: %w", err)
	}
	result.Daily, err = s.readDailyMetrics(ctx, siteID, startDate, endDate)
	if err != nil {
		return PublicAnalyticsData{}, err
	}
	for _, target := range []struct {
		kind string
		out  *[]AnalyticsMetric
	}{
		{kind: "country", out: &result.Countries},
		{kind: "browser", out: &result.Browsers},
		{kind: "os", out: &result.OperatingSystems},
	} {
		items, err := s.readDimensionMetrics(ctx, siteID, startDate, endDate, target.kind)
		if err != nil {
			return PublicAnalyticsData{}, err
		}
		*target.out = items
	}
	result.Cities, err = s.readCityMetrics(ctx, siteID, startDate, endDate)
	if err != nil {
		return PublicAnalyticsData{}, err
	}
	result.MapPoints, err = s.readRangeMapPoints(ctx, siteID, startDate, endDate)
	if err != nil {
		return PublicAnalyticsData{}, err
	}
	if includePaths {
		result.Paths, err = s.readDimensionMetrics(ctx, siteID, startDate, endDate, "path")
		if err != nil {
			return PublicAnalyticsData{}, err
		}
	}
	return result, nil
}

func (s *Store) readRangeMapPoints(ctx context.Context, siteID, startDate, endDate string) ([]MapPoint, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT g.country_code, g.region_code, g.city, g.latitude, g.longitude,
		       SUM(a.pageviews), SUM(a.unique_visitors)
		FROM daily_aggregates AS a
		JOIN geo_locations AS g
		  ON g.site_id = a.site_id
		 AND g.dimension_kind = a.dimension_kind
		 AND g.dimension_value = a.dimension_value
		WHERE a.site_id = ? AND a.local_date BETWEEN ? AND ? AND a.dimension_kind = 'city'
		GROUP BY g.dimension_value, g.country_code, g.region_code, g.city, g.latitude, g.longitude
		ORDER BY SUM(a.pageviews) DESC, g.dimension_value
	`, siteID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("read date-range map points: %w", err)
	}
	defer rows.Close()
	var result []MapPoint
	for rows.Next() {
		var point MapPoint
		if err := rows.Scan(&point.CountryCode, &point.RegionCode, &point.City, &point.Latitude, &point.Longitude, &point.Pageviews, &point.UniqueVisitors); err != nil {
			return nil, fmt.Errorf("scan date-range map point: %w", err)
		}
		result = append(result, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate date-range map points: %w", err)
	}
	return result, nil
}

func analyticsDates(timezone, startDate, endDate string) (string, string, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", "", fmt.Errorf("load Site timezone: %w", err)
	}
	now := time.Now().In(location)
	if startDate == "" && endDate == "" {
		return now.AddDate(0, 0, -29).Format(time.DateOnly), now.Format(time.DateOnly), nil
	}
	if startDate == "" || endDate == "" {
		return "", "", fmt.Errorf("analytics start and end dates must be provided together")
	}
	if _, err := time.Parse(time.DateOnly, startDate); err != nil {
		return "", "", fmt.Errorf("invalid analytics start date")
	}
	if _, err := time.Parse(time.DateOnly, endDate); err != nil {
		return "", "", fmt.Errorf("invalid analytics end date")
	}
	if startDate > endDate {
		return "", "", fmt.Errorf("analytics start date must not be after end date")
	}
	return startDate, endDate, nil
}

func (s *Store) readDailyMetrics(ctx context.Context, siteID, startDate, endDate string) ([]DailyMetric, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT local_date, pageviews, unique_visitors
		FROM daily_aggregates
		WHERE site_id = ? AND local_date BETWEEN ? AND ?
		  AND dimension_kind = 'overall' AND dimension_value = '*'
		ORDER BY local_date
	`, siteID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("read daily Analytics metrics: %w", err)
	}
	defer rows.Close()
	result := make([]DailyMetric, 0)
	for rows.Next() {
		var item DailyMetric
		if err := rows.Scan(&item.Date, &item.Pageviews, &item.UniqueVisitors); err != nil {
			return nil, fmt.Errorf("scan daily Analytics metric: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily Analytics metrics: %w", err)
	}
	return result, nil
}

func (s *Store) readDimensionMetrics(ctx context.Context, siteID, startDate, endDate, kind string) ([]AnalyticsMetric, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT dimension_value, SUM(pageviews), SUM(unique_visitors)
		FROM daily_aggregates
		WHERE site_id = ? AND local_date BETWEEN ? AND ? AND dimension_kind = ?
		GROUP BY dimension_value
		ORDER BY SUM(pageviews) DESC, dimension_value
		LIMIT 12
	`, siteID, startDate, endDate, kind)
	if err != nil {
		return nil, fmt.Errorf("read %s Analytics metrics: %w", kind, err)
	}
	defer rows.Close()
	result := make([]AnalyticsMetric, 0)
	for rows.Next() {
		var item AnalyticsMetric
		if err := rows.Scan(&item.Value, &item.Pageviews, &item.UniqueVisitors); err != nil {
			return nil, fmt.Errorf("scan %s Analytics metric: %w", kind, err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s Analytics metrics: %w", kind, err)
	}
	return result, nil
}

func (s *Store) readCityMetrics(ctx context.Context, siteID, startDate, endDate string) ([]AnalyticsMetric, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT a.dimension_value, SUM(a.pageviews), SUM(a.unique_visitors)
		FROM daily_aggregates AS a
		WHERE a.site_id = ? AND a.local_date BETWEEN ? AND ? AND a.dimension_kind = 'city'
		GROUP BY a.dimension_value
		ORDER BY SUM(a.pageviews) DESC, a.dimension_value
		LIMIT 12
	`, siteID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("read city Analytics metrics: %w", err)
	}
	defer rows.Close()
	result := make([]AnalyticsMetric, 0)
	for rows.Next() {
		var item AnalyticsMetric
		if err := rows.Scan(&item.Value, &item.Pageviews, &item.UniqueVisitors); err != nil {
			return nil, fmt.Errorf("scan city Analytics metric: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate city Analytics metrics: %w", err)
	}
	return result, nil
}

type PageviewRecord struct {
	ID              int64
	SiteID          string
	SiteName        string
	SiteTimezone    string
	OccurredAt      time.Time
	LocalDate       string
	Path            string
	CountryCode     string
	RegionCode      string
	City            string
	Latitude        sql.NullFloat64
	Longitude       sql.NullFloat64
	VisitorDigest   string
	OriginalIP      string
	OperatingSystem string
	Browser         string
}

func (s *Store) RecentPageviewRecords(ctx context.Context, siteID string, limit int) ([]PageviewRecord, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	page, err := s.PageviewRecords(ctx, PageviewFilters{SiteID: siteID}, nil, "older", limit)
	if err != nil {
		return nil, err
	}
	return page.Records, nil
}
