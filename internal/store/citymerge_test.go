package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestCityStatisticsAndMapsMergeHistoricalVariants(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "Cities", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	date := ""
	items := []struct {
		country, region, city string
		latitude, longitude   float64
	}{
		{"CN", "HB", "Wuhan", 30.5928, 114.3055},
		{"CN", "", "武汉市", 30.5833, 114.2668},
		{"CN", "SH", "Shanghai", 31.2243, 121.4689},
		{"CN", "", "香港特别行政区", 22.2855, 114.1577},
		{"HK", "", "Hong Kong", 22.2860, 114.1580},
		{"FR", "IDF", "Paris", 48.8566, 2.3522},
		{"US", "TX", "Paris", 33.6609, -95.5555},
	}
	for index, item := range items {
		digest := byte(index + 1)
		if index == 1 {
			digest = 1 // Same visitor, different region label for Wuhan.
		}
		created, err := st.RecordPageview(ctx, PageviewObservation{
			SiteID: site.ID, Hostname: "example.com", Path: "/", OccurredAt: time.Now().UTC(),
			CountryCode: item.country, RegionCode: item.region, City: item.city,
			Latitude: &item.latitude, Longitude: &item.longitude,
			VisitorDigest: bytes.Repeat([]byte{digest}, 32), OriginalIP: "192.0.2.1",
		})
		if err != nil {
			t.Fatal(err)
		}
		date = created.LocalDate
	}
	// Simulate retained historical aggregates under older region-based keys.
	// Both coordinates are close enough to their canonical city markers.
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO daily_aggregates (site_id, local_date, dimension_kind, dimension_value, pageviews, unique_visitors)
		VALUES (?, ?, 'city', 'CN||上海', 2, 2)
	`, site.ID, date); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO geo_locations (site_id, dimension_kind, dimension_value, country_code, region_code, city, latitude, longitude, updated_at)
		VALUES (?, 'city', 'CN||上海', 'CN', '', '上海', 31.23, 121.47, ?)
	`, site.ID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO daily_aggregates (site_id, local_date, dimension_kind, dimension_value, pageviews, unique_visitors)
		VALUES (?, ?, 'city', 'CN|Hubei|武汉市', 3, 2)
	`, site.ID, date); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO geo_locations (site_id, dimension_kind, dimension_value, country_code, region_code, city, latitude, longitude, updated_at)
		VALUES (?, 'city', 'CN|Hubei|武汉市', 'CN', 'Hubei', '武汉市', 30.6, 114.3, ?)
	`, site.ID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	assertPoints := func(points []MapPoint) {
		t.Helper()
		counts := make(map[string]int)
		for _, point := range points {
			counts[point.City]++
			switch point.City {
			case "Wuhan":
				if point.Pageviews != 5 || point.UniqueVisitors != 3 || point.Latitude != 30.6 {
					t.Fatalf("merged Wuhan = %#v", point)
				}
			case "Hong Kong":
				if point.Pageviews != 2 || point.UniqueVisitors != 2 {
					t.Fatalf("merged %s = %#v", point.City, point)
				}
			case "Shanghai":
				if point.Pageviews != 3 || point.UniqueVisitors != 3 || point.Latitude == 0 {
					t.Fatalf("merged Shanghai = %#v", point)
				}
			}
		}
		if len(points) != 5 || counts["Wuhan"] != 1 || counts["Shanghai"] != 1 || counts["Hong Kong"] != 1 || counts["Paris"] != 2 {
			t.Fatalf("city map points = %#v", points)
		}
	}
	all, err := st.AdminMapData(ctx, site.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertPoints(all.Points)
	ranged, err := st.PublicMapDataRange(ctx, site.ID, date, date)
	if err != nil {
		t.Fatal(err)
	}
	assertPoints(ranged.Points)
	analytics, err := st.AdminAnalytics(ctx, site.ID, date, date)
	if err != nil {
		t.Fatal(err)
	}
	assertPoints(analytics.MapPoints)
	counts := make(map[string]int)
	for _, metric := range analytics.Cities {
		name := geoLabelFromCityValue(metric.Value)
		counts[name]++
		if name == "Wuhan" && (metric.Pageviews != 5 || metric.UniqueVisitors != 3) {
			t.Fatalf("merged Wuhan city metric = %#v", metric)
		}
		if name == "Shanghai" && (metric.Pageviews != 3 || metric.UniqueVisitors != 3) {
			t.Fatalf("merged city metric = %#v", metric)
		}
	}
	if len(analytics.Cities) != 5 || counts["Wuhan"] != 1 || counts["Shanghai"] != 1 || counts["Hong Kong"] != 1 || counts["Paris"] != 2 {
		t.Fatalf("city metrics = %#v", analytics.Cities)
	}
}

func geoLabelFromCityValue(value string) string {
	for index := len(value) - 1; index >= 0; index-- {
		if value[index] == '|' {
			return value[index+1:]
		}
	}
	return value
}

func TestCityMergeLimitsEveryPairTo80Kilometers(t *testing.T) {
	for _, test := range []struct {
		name       string
		baseLat    float64
		latitude   float64
		longitude  float64
		wantMerged bool
	}{
		{"within 80 km", 0, 0, 0.71, true},
		{"beyond 80 km", 0, 0, 0.73, false},
		{"diagonal below one degree on both axes", 0, 0.6, 0.6, false},
		{"longitude above one degree at high latitude", 60, 60, 1.1, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := []cityObservation{
				{MapPoint: MapPoint{CountryCode: "CN", RegionCode: "HB", City: "Wuhan", Latitude: test.baseLat, Longitude: 0, Pageviews: 1}, hasCoordinate: true},
				{MapPoint: MapPoint{CountryCode: "CN", RegionCode: "HB", City: "武汉市", Latitude: test.latitude, Longitude: test.longitude, Pageviews: 1}, hasCoordinate: true},
			}
			want := 2
			if test.wantMerged {
				want = 1
			}
			if got := len(mergeCities(rows, true)); got != want {
				t.Fatalf("map groups = %d, want %d", got, want)
			}
			if got := len(mergeCities(rows, false)); got != want {
				t.Fatalf("metric groups = %d, want %d", got, want)
			}
		})
	}
	bridge := []cityObservation{
		{MapPoint: MapPoint{CountryCode: "CN", City: "Wuhan", Latitude: 0, Longitude: 0, Pageviews: 1}, hasCoordinate: true},
		{MapPoint: MapPoint{CountryCode: "CN", City: "Wuhan", Latitude: 0, Longitude: 0.6, Pageviews: 1}, hasCoordinate: true},
		{MapPoint: MapPoint{CountryCode: "CN", City: "Wuhan", Latitude: 0, Longitude: -0.6, Pageviews: 1}, hasCoordinate: true},
	}
	if got := len(mergeCities(bridge, true)); got != 2 {
		t.Fatalf("bridge merged distant endpoints into %d groups", got)
	}
	unknown := []cityObservation{
		{MapPoint: MapPoint{CountryCode: "CN", RegionCode: "HB", City: "Wuhan", Pageviews: 1}},
		{MapPoint: MapPoint{CountryCode: "CN", RegionCode: "HB", City: "Wuhan", Latitude: 30, Longitude: 114, Pageviews: 1}, hasCoordinate: true},
	}
	if got := len(mergeCities(unknown, false)); got != 2 {
		t.Fatalf("unknown coordinate should not bypass distance check: %d groups", got)
	}
}

func TestDistantSameNameWithinOneRegionStaysDistinct(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "Distant namesakes", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	date := ""
	for index, latitude := range []float64{30, 31} {
		longitude := 114.0
		created, err := st.RecordPageview(ctx, PageviewObservation{
			SiteID: site.ID, Hostname: "example.com", Path: "/", OccurredAt: time.Now().UTC(),
			CountryCode: "CN", RegionCode: "HB", City: "Wuhan", Latitude: &latitude, Longitude: &longitude,
			VisitorDigest: bytes.Repeat([]byte{byte(index + 1)}, 32), OriginalIP: "192.0.2.1",
		})
		if err != nil {
			t.Fatal(err)
		}
		date = created.LocalDate
	}
	var aggregateRows int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM daily_aggregates WHERE site_id = ? AND dimension_kind = 'city'`, site.ID).Scan(&aggregateRows); err != nil {
		t.Fatal(err)
	}
	if aggregateRows != 2 {
		t.Fatalf("distant city aggregate rows = %d, want 2", aggregateRows)
	}
	mapData, err := st.PublicMapDataRange(ctx, site.ID, date, date)
	if err != nil {
		t.Fatal(err)
	}
	if len(mapData.Points) != 2 {
		t.Fatalf("distant city map points = %#v", mapData.Points)
	}
	analytics, err := st.AdminAnalytics(ctx, site.ID, date, date)
	if err != nil {
		t.Fatal(err)
	}
	if len(analytics.Cities) != 2 {
		t.Fatalf("distant city metrics = %#v", analytics.Cities)
	}
	cellSite, err := st.CreateSite(ctx, CreateSiteParams{Name: "Distant points in old cell", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	for index, coordinate := range [][2]float64{{0.1, 0.1}, {0.9, 0.9}} {
		latitude, longitude := coordinate[0], coordinate[1]
		if _, err := st.RecordPageview(ctx, PageviewObservation{
			SiteID: cellSite.ID, Hostname: "example.com", Path: "/", OccurredAt: time.Now().UTC(),
			CountryCode: "CN", RegionCode: "HB", City: "Wuhan", Latitude: &latitude, Longitude: &longitude,
			VisitorDigest: bytes.Repeat([]byte{byte(index + 1)}, 32), OriginalIP: "192.0.2.1",
		}); err != nil {
			t.Fatal(err)
		}
	}
	cellMap, err := st.AdminMapData(ctx, cellSite.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cellMap.Points) != 2 {
		t.Fatalf("distant points in one-degree cell = %#v", cellMap.Points)
	}
	nearSite, err := st.CreateSite(ctx, CreateSiteParams{Name: "Near cell boundary", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	for index, latitude := range []float64{30.99, 31.01} {
		longitude := 114.0
		if _, err := st.RecordPageview(ctx, PageviewObservation{
			SiteID: nearSite.ID, Hostname: "example.com", Path: "/", OccurredAt: time.Now().UTC(),
			CountryCode: "CN", RegionCode: "HB", City: "Wuhan", Latitude: &latitude, Longitude: &longitude,
			VisitorDigest: bytes.Repeat([]byte{byte(index + 1)}, 32), OriginalIP: "192.0.2.1",
		}); err != nil {
			t.Fatal(err)
		}
	}
	nearMap, err := st.AdminMapData(ctx, nearSite.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(nearMap.Points) != 1 || nearMap.Points[0].Pageviews != 2 {
		t.Fatalf("near city across coordinate cells = %#v", nearMap.Points)
	}
}
