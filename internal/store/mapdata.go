package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrPublicationDisabled = errors.New("public views are disabled for this Site")

type MapPoint struct {
	CountryCode    string
	RegionCode     string
	City           string
	Latitude       float64
	Longitude      float64
	Pageviews      int64
	UniqueVisitors int64
}

type PublicMapData struct {
	SiteID         string
	SiteName       string
	Pageviews      int64
	UniqueVisitors int64
	Points         []MapPoint
}

func (s *Store) PublicMapData(ctx context.Context, siteID string) (PublicMapData, error) {
	return s.mapData(ctx, siteID, true)
}

func (s *Store) AdminMapData(ctx context.Context, siteID string) (PublicMapData, error) {
	return s.mapData(ctx, siteID, false)
}

func (s *Store) mapData(ctx context.Context, siteID string, requirePublished bool) (PublicMapData, error) {
	var result PublicMapData
	var published int
	err := s.DB.QueryRowContext(ctx, `SELECT id, name, publish_public FROM sites WHERE id = ?`, siteID).Scan(&result.SiteID, &result.SiteName, &published)
	if err != nil {
		if err == sql.ErrNoRows {
			return PublicMapData{}, fmt.Errorf("Site %q not found", siteID)
		}
		return PublicMapData{}, fmt.Errorf("read Public Map Site: %w", err)
	}
	if requirePublished && published != 1 {
		return PublicMapData{}, ErrPublicationDisabled
	}
	err = s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(pageviews), 0), COALESCE(SUM(unique_visitors), 0)
		FROM daily_aggregates
		WHERE site_id = ? AND dimension_kind = 'overall' AND dimension_value = '*'
	`, siteID).Scan(&result.Pageviews, &result.UniqueVisitors)
	if err != nil {
		return PublicMapData{}, fmt.Errorf("read Public Map totals: %w", err)
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT g.country_code, g.region_code, g.city, g.latitude, g.longitude,
		       SUM(a.pageviews), SUM(a.unique_visitors)
		FROM daily_aggregates AS a
		JOIN geo_locations AS g
		  ON g.site_id = a.site_id
		 AND g.dimension_kind = a.dimension_kind
		 AND g.dimension_value = a.dimension_value
		WHERE a.site_id = ? AND a.dimension_kind = 'city'
		GROUP BY g.dimension_value, g.country_code, g.region_code, g.city, g.latitude, g.longitude
		ORDER BY SUM(a.pageviews) DESC, g.dimension_value
	`, siteID)
	if err != nil {
		return PublicMapData{}, fmt.Errorf("read Public Map points: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var point MapPoint
		if err := rows.Scan(&point.CountryCode, &point.RegionCode, &point.City, &point.Latitude, &point.Longitude, &point.Pageviews, &point.UniqueVisitors); err != nil {
			return PublicMapData{}, fmt.Errorf("scan Public Map point: %w", err)
		}
		result.Points = append(result.Points, point)
	}
	if err := rows.Err(); err != nil {
		return PublicMapData{}, fmt.Errorf("iterate Public Map points: %w", err)
	}
	return result, nil
}
