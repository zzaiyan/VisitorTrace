package store

import (
	"bytes"
	"context"
	"net/netip"
	"path/filepath"
	"testing"
	"time"
)

func TestRefreshPageviewGeoIPRebuildsRetainedGeography(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "GeoIP refresh", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	latitude := 30.5928
	longitude := 114.3055
	base := time.Now().UTC().Add(-time.Hour)
	records := []PageviewObservation{
		{OriginalIP: "192.0.2.1", VisitorDigest: bytes.Repeat([]byte{1}, 32), City: "Wuhan"},
		{OriginalIP: "192.0.2.1", VisitorDigest: bytes.Repeat([]byte{1}, 32), City: "Wuhan"},
		{OriginalIP: "192.0.2.2", VisitorDigest: bytes.Repeat([]byte{2}, 32), City: "Wuhan"},
		{OriginalIP: "not-an-ip", VisitorDigest: bytes.Repeat([]byte{3}, 32), RegionCode: "GD", City: "Shenzhen"},
	}
	localDate := ""
	for index := range records {
		records[index].SiteID = site.ID
		records[index].Hostname = "example.com"
		records[index].OccurredAt = base.Add(time.Duration(index) * time.Second)
		records[index].Path = "/geoip"
		records[index].CountryCode = "CN"
		records[index].Latitude = &latitude
		records[index].Longitude = &longitude
		records[index].OperatingSystem = "Linux"
		records[index].Browser = "Firefox"
		created, err := st.RecordPageview(ctx, records[index])
		if err != nil {
			t.Fatal(err)
		}
		localDate = created.LocalDate
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO daily_aggregates (site_id, local_date, dimension_kind, dimension_value, pageviews, unique_visitors)
		VALUES (?, '2000-01-01', 'country', 'LEGACY', 7, 5)
	`, site.ID); err != nil {
		t.Fatal(err)
	}

	sanFranciscoLatitude := 37.7749
	sanFranciscoLongitude := -122.4194
	result, err := st.RefreshPageviewGeoIP(ctx, site.ID, func(address netip.Addr) PageviewGeography {
		switch address.String() {
		case "192.0.2.1":
			return PageviewGeography{
				CountryCode: "US", RegionCode: "CA", City: "San Francisco",
				Latitude: &sanFranciscoLatitude, Longitude: &sanFranciscoLongitude,
			}
		case "192.0.2.2":
			return PageviewGeography{}
		default:
			t.Fatalf("unexpected GeoIP lookup for %s", address)
			return PageviewGeography{}
		}
	})
	if err != nil {
		t.Fatalf("RefreshPageviewGeoIP() error = %v", err)
	}
	if result.Processed != 3 || result.Changed != 3 || result.Located != 2 || result.Unmatched != 1 || result.InvalidIP != 1 || result.AggregateDates != 1 {
		t.Fatalf("RefreshPageviewGeoIP() = %#v", result)
	}

	page, err := st.PageviewRecords(ctx, PageviewFilters{SiteID: site.ID}, nil, "older", 10)
	if err != nil {
		t.Fatal(err)
	}
	byIP := make(map[string][]PageviewRecord)
	for _, record := range page.Records {
		byIP[record.OriginalIP] = append(byIP[record.OriginalIP], record)
	}
	for _, record := range byIP["192.0.2.1"] {
		if record.CountryCode != "US" || record.RegionCode != "CA" || record.City != "San Francisco" || !record.Latitude.Valid || record.Latitude.Float64 != sanFranciscoLatitude {
			t.Fatalf("located record = %#v", record)
		}
	}
	unmatched := byIP["192.0.2.2"][0]
	if unmatched.CountryCode != "" || unmatched.RegionCode != "" || unmatched.City != "" || unmatched.Latitude.Valid || unmatched.Longitude.Valid {
		t.Fatalf("unmatched record retained stale geography: %#v", unmatched)
	}
	invalid := byIP["not-an-ip"][0]
	if invalid.CountryCode != "CN" || invalid.RegionCode != "GD" || invalid.City != "Shenzhen" || !invalid.Latitude.Valid {
		t.Fatalf("invalid-IP record changed: %#v", invalid)
	}

	assertAggregate := func(date, kind, value string, wantPageviews, wantVisitors int64) {
		t.Helper()
		var pageviews, visitors int64
		err := st.DB.QueryRowContext(ctx, `
			SELECT pageviews, unique_visitors FROM daily_aggregates
			WHERE site_id = ? AND local_date = ? AND dimension_kind = ? AND dimension_value = ?
		`, site.ID, date, kind, value).Scan(&pageviews, &visitors)
		if err != nil || pageviews != wantPageviews || visitors != wantVisitors {
			t.Fatalf("aggregate %s/%s/%s = PV %d UV %d, %v; want PV %d UV %d", date, kind, value, pageviews, visitors, err, wantPageviews, wantVisitors)
		}
	}
	assertAggregate(localDate, "overall", "*", 4, 3)
	assertAggregate(localDate, "country", "US", 2, 1)
	assertAggregate(localDate, "country", "unknown", 1, 1)
	assertAggregate(localDate, "country", "CN", 1, 1)
	assertAggregate(localDate, "city", "US|CA|San Francisco", 2, 1)
	assertAggregate(localDate, "city", "CN|GD|Shenzhen", 1, 1)
	assertAggregate("2000-01-01", "country", "LEGACY", 7, 5)
	var staleWuhan int
	if err := st.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM daily_aggregates
		WHERE site_id = ? AND local_date = ? AND dimension_kind = 'city' AND dimension_value = 'CN||Wuhan'
	`, site.ID, localDate).Scan(&staleWuhan); err != nil || staleWuhan != 0 {
		t.Fatalf("stale Wuhan aggregates = %d, %v", staleWuhan, err)
	}
	mapData, err := st.PublicMapData(ctx, site.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundSanFrancisco := false
	for _, point := range mapData.Points {
		if point.City == "Wuhan" {
			t.Fatalf("stale Wuhan map point = %#v", point)
		}
		if point.City == "San Francisco" && point.Pageviews == 2 && point.UniqueVisitors == 1 {
			foundSanFrancisco = true
		}
	}
	if !foundSanFrancisco {
		t.Fatalf("rebuilt map points = %#v", mapData.Points)
	}
	var registration int
	if err := st.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM visitor_registrations
		WHERE site_id = ? AND dimension_kind = 'country' AND dimension_value = ? AND visitor_digest = ?
	`, site.ID, scopedVisitorDimension("example.com", "US"), bytes.Repeat([]byte{1}, 32)).Scan(&registration); err != nil || registration != 1 {
		t.Fatalf("rebuilt visitor registration = %d, %v", registration, err)
	}
}

func TestPageviewRecordFiltersAndCursorPagination(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "Records", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		_, err := st.RecordPageview(ctx, PageviewObservation{
			SiteID: site.ID, Hostname: []string{"one.example", "two.example"}[i%2], OccurredAt: base.Add(time.Duration(i) * time.Minute), Path: []string{"/", "/notes"}[i%2],
			CountryCode: "CN", RegionCode: "HB", City: "Wuhan", VisitorDigest: bytes.Repeat([]byte{byte(i + 1)}, 32),
			OriginalIP: []string{"192.0.2.1", "192.0.2.2"}[i%2], OperatingSystem: "Linux", Browser: []string{"Firefox", "Chrome"}[i%2],
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	filters := PageviewFilters{SiteID: site.ID}
	first, err := st.PageviewRecords(ctx, filters, nil, "older", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 2 || !first.More || !first.Records[0].OccurredAt.After(first.Records[1].OccurredAt) {
		t.Fatalf("first page = %#v", first)
	}
	boundary := first.Records[1]
	second, err := st.PageviewRecords(ctx, filters, &PageviewCursor{OccurredAt: boundary.OccurredAt, ID: boundary.ID}, "older", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 2 || !second.More || !second.Records[0].OccurredAt.Before(boundary.OccurredAt) {
		t.Fatalf("second page = %#v", second)
	}
	back, err := st.PageviewRecords(ctx, filters, &PageviewCursor{OccurredAt: second.Records[0].OccurredAt, ID: second.Records[0].ID}, "newer", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Records) != 2 || back.Records[0].ID != first.Records[0].ID || back.Records[1].ID != first.Records[1].ID {
		t.Fatalf("newer page = %#v, want %#v", back, first)
	}
	filtered, err := st.PageviewRecords(ctx, PageviewFilters{SiteID: site.ID, Path: "/notes", OriginalIP: "192.0.2.2", Browser: "Chrome"}, nil, "older", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Records) != 2 {
		t.Fatalf("filtered records = %d, want 2", len(filtered.Records))
	}
	hostFiltered, err := st.PageviewRecords(ctx, PageviewFilters{SiteID: site.ID, Hostname: "two.example"}, nil, "older", 100)
	if err != nil || len(hostFiltered.Records) != 2 {
		t.Fatalf("hostname-filtered records = %#v, %v", hostFiltered, err)
	}
	if hostFiltered.Records[0].Hostname != "two.example" {
		t.Fatalf("hostname-filtered record = %#v", hostFiltered.Records[0])
	}
}

func TestPageviewAndAggregateExports(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "Export", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RecordPageview(ctx, PageviewObservation{
		SiteID: site.ID, OccurredAt: time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC), Path: "/export",
		CountryCode: "CN", VisitorDigest: bytes.Repeat([]byte{1}, 32), OriginalIP: "192.0.2.1", OperatingSystem: "Linux", Browser: "Firefox",
	}); err != nil {
		t.Fatal(err)
	}
	var records []PageviewRecord
	if err := st.ExportPageviewRecords(ctx, PageviewFilters{SiteID: site.ID, CountryCode: "CN"}, func(record PageviewRecord) error {
		records = append(records, record)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].SiteTimezone != site.Timezone {
		t.Fatalf("exported records = %#v", records)
	}
	var aggregates []AggregateExportRow
	if err := st.ExportAggregates(ctx, site.ID, "2026-07-22", "2026-07-22", "overall", func(row AggregateExportRow) error {
		aggregates = append(aggregates, row)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(aggregates) != 1 || aggregates[0].Pageviews != 1 || aggregates[0].UniqueVisitors != 1 {
		t.Fatalf("exported Aggregates = %#v", aggregates)
	}
}
