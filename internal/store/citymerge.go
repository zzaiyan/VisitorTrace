package store

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/zzaiyan/VisitorTrace/internal/geoip"
)

// cityObservation is a city aggregate with an optional map coordinate. Read
// paths merge these rows so historical aggregates need no database rewrite.
type cityObservation struct {
	MapPoint
	hasCoordinate bool
}

func scanCityObservations(rows *sql.Rows) ([]cityObservation, error) {
	var observations []cityObservation
	for rows.Next() {
		var value string
		var latitude, longitude sql.NullFloat64
		var item cityObservation
		if err := rows.Scan(&value, &latitude, &longitude, &item.Pageviews, &item.UniqueVisitors); err != nil {
			return nil, fmt.Errorf("scan city aggregate: %w", err)
		}
		country, region, city, ok := parseCityDimensionValue(value)
		if !ok {
			continue
		}
		item.CountryCode, item.RegionCode, item.City = country, region, city
		if latitude.Valid && longitude.Valid {
			item.Latitude, item.Longitude = latitude.Float64, longitude.Float64
			item.hasCoordinate = true
		}
		observations = append(observations, item)
	}
	return observations, rows.Err()
}

type mergedCity struct {
	MapPoint
	hasCoordinate  bool
	representative int64
	coordinates    []cityCoordinate
}

type cityCoordinate struct {
	latitude  float64
	longitude float64
}

const cityMergeDistanceKm = 80

func mergeCities(rows []cityObservation, requireCoordinates bool) []MapPoint {
	groups := make([]*mergedCity, 0, len(rows))
	byName := make(map[string][]*mergedCity)
	for _, row := range rows {
		city := geoip.NormalizeCityEN(row.City)
		if city == "" {
			continue
		}
		nameKey := strings.ToLower(city)
		country := strings.ToUpper(strings.TrimSpace(row.CountryCode))
		var group *mergedCity
		if row.hasCoordinate {
			for _, candidate := range byName[nameKey] {
				if candidate.canInclude(row) {
					group = candidate
					break
				}
			}
		}
		if group == nil {
			group = &mergedCity{MapPoint: MapPoint{CountryCode: country, RegionCode: row.RegionCode, City: city}}
			groups = append(groups, group)
			byName[nameKey] = append(byName[nameKey], group)
		}
		group.Pageviews += row.Pageviews
		group.UniqueVisitors += row.UniqueVisitors
		if row.hasCoordinate {
			group.coordinates = append(group.coordinates, cityCoordinate{row.Latitude, row.Longitude})
		}
		if row.hasCoordinate && (!group.hasCoordinate || row.Pageviews > group.representative) {
			group.Latitude = row.Latitude
			group.Longitude = row.Longitude
			group.CountryCode = country
			group.RegionCode = row.RegionCode
			group.hasCoordinate = true
			group.representative = row.Pageviews
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Pageviews != groups[j].Pageviews {
			return groups[i].Pageviews > groups[j].Pageviews
		}
		if groups[i].City != groups[j].City {
			return groups[i].City < groups[j].City
		}
		return groups[i].CountryCode < groups[j].CountryCode
	})
	result := make([]MapPoint, 0, len(groups))
	for _, group := range groups {
		if !requireCoordinates || group.hasCoordinate {
			result = append(result, group.MapPoint)
		}
	}
	return result
}

func (g *mergedCity) canInclude(row cityObservation) bool {
	if !g.hasCoordinate || !row.hasCoordinate {
		return false
	}
	for _, point := range g.coordinates {
		if greatCircleDistanceKm(point.latitude, point.longitude, row.Latitude, row.Longitude) > cityMergeDistanceKm {
			return false
		}
	}
	return true
}

func greatCircleDistanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0088
	lat1, lat2 = lat1*math.Pi/180, lat2*math.Pi/180
	dLat := lat2 - lat1
	dLon := (lon2 - lon1) * math.Pi / 180
	sinLat, sinLon := math.Sin(dLat/2), math.Sin(dLon/2)
	a := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLon*sinLon
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(math.Min(1, a)))
}

const cityCellSeparator = "\x1f"

// Half-degree cells are narrower than 80 km, keeping distant namesakes separate
// before read-time city merging. The prefix distinguishes them from older cells.
func cityDimensionValue(countryCode, regionCode, city string, latitude, longitude *float64) string {
	value := strings.ToUpper(strings.TrimSpace(countryCode)) + "|" + strings.TrimSpace(regionCode) + "|" + geoip.NormalizeCityEN(city)
	if latitude != nil && longitude != nil {
		value += cityCellSeparator + "h" + strconv.Itoa(int(math.Floor(*latitude*2))) + "," + strconv.Itoa(int(math.Floor(*longitude*2)))
	}
	return value
}

func parseCityDimensionValue(value string) (country, region, city string, ok bool) {
	value, _, _ = strings.Cut(value, cityCellSeparator)
	parts := strings.SplitN(value, "|", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
