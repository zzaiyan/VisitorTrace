package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordPageviewUpdatesRawAndAggregateData(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "Test", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}
	observation := PageviewObservation{
		SiteID:          site.ID,
		OccurredAt:      time.Date(2026, time.July, 20, 17, 0, 0, 0, time.UTC),
		Path:            "/notes/",
		CountryCode:     "CN",
		RegionCode:      "HB",
		City:            "Wuhan",
		VisitorDigest:   bytes.Repeat([]byte{1}, 32),
		OriginalIP:      "192.0.2.1",
		OperatingSystem: "Linux",
		Browser:         "Firefox",
	}
	latitude := 30.5928
	longitude := 114.3055
	observation.Latitude = &latitude
	observation.Longitude = &longitude
	first, err := st.RecordPageview(ctx, observation)
	if err != nil {
		t.Fatalf("RecordPageview() error = %v", err)
	}
	if first.LocalDate != "2026-07-21" || !first.NewOverallVisitor {
		t.Fatalf("first result = %#v", first)
	}
	second, err := st.RecordPageview(ctx, observation)
	if err != nil {
		t.Fatalf("RecordPageview() second error = %v", err)
	}
	if second.NewOverallVisitor {
		t.Fatal("repeat visitor was counted as new")
	}
	observation.VisitorDigest = bytes.Repeat([]byte{2}, 32)
	if _, err := st.RecordPageview(ctx, observation); err != nil {
		t.Fatalf("RecordPageview() third error = %v", err)
	}

	var records int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pageviews WHERE site_id = ?`, site.ID).Scan(&records); err != nil {
		t.Fatalf("count Pageview Records: %v", err)
	}
	if records != 3 {
		t.Fatalf("Pageview Record count = %d, want 3", records)
	}
	var pageviews, uniqueVisitors int
	err = st.DB.QueryRowContext(ctx, `
		SELECT pageviews, unique_visitors FROM daily_aggregates
		WHERE site_id = ? AND local_date = '2026-07-21' AND dimension_kind = 'overall' AND dimension_value = '*'
	`, site.ID).Scan(&pageviews, &uniqueVisitors)
	if err != nil {
		t.Fatalf("read overall Aggregate: %v", err)
	}
	if pageviews != 3 || uniqueVisitors != 2 {
		t.Fatalf("overall Aggregate = PV %d UV %d, want PV 3 UV 2", pageviews, uniqueVisitors)
	}
	updated, err := st.GetSite(ctx, site.ID)
	if err != nil {
		t.Fatalf("GetSite() error = %v", err)
	}
	if updated.FirstPageviewAt == nil {
		t.Fatal("Site timezone was not locked by the first Pageview")
	}
	mapData, err := st.PublicMapData(ctx, site.ID)
	if err != nil {
		t.Fatalf("PublicMapData() error = %v", err)
	}
	if mapData.Pageviews != 3 || len(mapData.Points) != 1 || mapData.Points[0].City != "Wuhan" {
		t.Fatalf("PublicMapData() = %#v", mapData)
	}
}

func TestRecordPageviewCombinesKnownCityAliases(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "City aliases", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	var localDate string
	for index, city := range []string{"香港特別行政區", "Hong Kong SAR, China"} {
		result, err := st.RecordPageview(ctx, PageviewObservation{
			SiteID: site.ID, Hostname: "example.com", Path: "/", OriginalIP: "8.8.8.8",
			CountryCode: "CN", City: city, VisitorDigest: bytes.Repeat([]byte{byte(index + 1)}, 32),
		})
		if err != nil {
			t.Fatal(err)
		}
		localDate = result.LocalDate
	}
	var cityRows, pageviews int
	if err := st.DB.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(pageviews), 0) FROM daily_aggregates
		WHERE site_id = ? AND local_date = ? AND dimension_kind = 'city'
	`, site.ID, localDate).Scan(&cityRows, &pageviews); err != nil || cityRows != 1 || pageviews != 2 {
		t.Fatalf("city aggregates = rows %d PV %d, %v", cityRows, pageviews, err)
	}
	var storedCity string
	if err := st.DB.QueryRowContext(ctx, `SELECT city FROM pageviews WHERE site_id = ? LIMIT 1`, site.ID).Scan(&storedCity); err != nil || storedCity != "Hong Kong" {
		t.Fatalf("stored city = %q, %v", storedCity, err)
	}
}

func TestRecordPageviewInfersProvinceFromGeneratedCity(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "Province", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := st.RecordPageview(ctx, PageviewObservation{
		SiteID: site.ID, Hostname: "example.com", Path: "/", OriginalIP: "8.8.8.8",
		CountryCode: "CN", City: "酒泉", VisitorDigest: bytes.Repeat([]byte{1}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	var regionCode, regionAggregate string
	if err := st.DB.QueryRowContext(ctx, `SELECT region_code FROM pageviews WHERE site_id = ?`, site.ID).Scan(&regionCode); err != nil || regionCode != "GS" {
		t.Fatalf("stored region code = %q, %v", regionCode, err)
	}
	if err := st.DB.QueryRowContext(ctx, `
		SELECT dimension_value FROM daily_aggregates
		WHERE site_id = ? AND local_date = ? AND dimension_kind = 'region'
	`, site.ID, result.LocalDate).Scan(&regionAggregate); err != nil || regionAggregate != "CN|GS" {
		t.Fatalf("region aggregate = %q, %v", regionAggregate, err)
	}
}

func TestNormalizedPageviewGeographyUsesGeneratedPlaceHierarchy(t *testing.T) {
	got := normalizedPageviewGeography(PageviewGeography{CountryCode: "CN", City: "梅州"})
	if got.CountryCode != "CN" || got.RegionCode != "GD" || got.City != "Meizhou" {
		t.Fatalf("normalized geography = %#v", got)
	}
}

func TestRecordPageviewRejectsInvalidDigestAtomically(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "Test", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}
	_, err = st.RecordPageview(ctx, PageviewObservation{SiteID: site.ID, Path: "/", VisitorDigest: []byte{1}})
	if err == nil {
		t.Fatal("RecordPageview() accepted an invalid Visitor Digest")
	}
	var records int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pageviews`).Scan(&records); err != nil {
		t.Fatalf("count Pageview Records: %v", err)
	}
	if records != 0 {
		t.Fatalf("invalid Pageview created %d records", records)
	}
}

func TestRecordPageviewScopesUniqueVisitorsByHostname(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "Domains", AllowedOrigins: []string{"https://one.example", "https://two.example"}})
	if err != nil {
		t.Fatal(err)
	}
	digest := bytes.Repeat([]byte{7}, 32)
	base := time.Date(2026, time.July, 22, 8, 0, 0, 0, time.UTC)
	for _, hostname := range []string{"one.example", "one.example", "two.example"} {
		result, err := st.RecordPageview(ctx, PageviewObservation{
			SiteID: site.ID, Hostname: hostname, OccurredAt: base, Path: "/", VisitorDigest: digest,
			OriginalIP: "192.0.2.7", OperatingSystem: "Linux", Browser: "Firefox",
		})
		if err != nil {
			t.Fatal(err)
		}
		if hostname == "one.example" && result.NewOverallVisitor != (result.ID == 1) {
			t.Fatalf("same-host visitor result = %#v", result)
		}
		if hostname == "two.example" && !result.NewOverallVisitor {
			t.Fatalf("cross-host visitor result = %#v", result)
		}
	}
	var pageviews, uniqueVisitors int
	if err := st.DB.QueryRowContext(ctx, `
		SELECT pageviews, unique_visitors FROM daily_aggregates
		WHERE site_id = ? AND dimension_kind = 'overall' AND dimension_value = '*'
	`, site.ID).Scan(&pageviews, &uniqueVisitors); err != nil {
		t.Fatal(err)
	}
	if pageviews != 3 || uniqueVisitors != 2 {
		t.Fatalf("overall aggregate = PV %d UV %d, want PV 3 UV 2", pageviews, uniqueVisitors)
	}
	var hostRows int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM daily_aggregates WHERE site_id = ? AND dimension_kind = 'hostname'`, site.ID).Scan(&hostRows); err != nil {
		t.Fatal(err)
	}
	if hostRows != 2 {
		t.Fatalf("hostname aggregate rows = %d, want 2", hostRows)
	}
}

func TestDeduplicationWindowChangeStartsAtNextLocalMidnight(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "Schedule", Timezone: "Asia/Shanghai", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation(site.Timezone)
	localNow := time.Now().In(location)
	todayNoon := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 12, 0, 0, 0, location)
	digest := bytes.Repeat([]byte{5}, 32)
	observation := PageviewObservation{SiteID: site.ID, OccurredAt: todayNoon, Path: "/", VisitorDigest: digest, OriginalIP: "192.0.2.5", OperatingSystem: "Linux", Browser: "Firefox"}
	first, err := st.RecordPageview(ctx, observation)
	if err != nil || !first.NewOverallVisitor {
		t.Fatalf("first Pageview = %#v, %v", first, err)
	}
	if _, err := st.UpdateSite(ctx, site.ID, UpdateSiteParams{
		Name: site.Name, Timezone: site.Timezone, AllowedOrigins: site.AllowedOrigins, AcceptPageviews: true, PublishPublic: true,
		PublicLanguage: "auto", DedupWindowDays: 7, RetentionDays: site.RetentionDays,
	}); err != nil {
		t.Fatal(err)
	}
	sameDay, err := st.RecordPageview(ctx, observation)
	if err != nil || sameDay.NewOverallVisitor || sameDay.DeduplicationWindow != first.DeduplicationWindow {
		t.Fatalf("same-day Pageview = %#v, %v; first = %#v", sameDay, err, first)
	}
	tomorrowDate := todayNoon.AddDate(0, 0, 1)
	observation.OccurredAt = tomorrowDate
	tomorrow, err := st.RecordPageview(ctx, observation)
	if err != nil || !tomorrow.NewOverallVisitor || tomorrow.DeduplicationWindow != tomorrowDate.Format(time.DateOnly) {
		t.Fatalf("next-day Pageview = %#v, %v", tomorrow, err)
	}
	observation.OccurredAt = tomorrowDate.AddDate(0, 0, 1)
	dayAfter, err := st.RecordPageview(ctx, observation)
	if err != nil || dayAfter.NewOverallVisitor || dayAfter.DeduplicationWindow != tomorrow.DeduplicationWindow {
		t.Fatalf("new seven-day window Pageview = %#v, %v", dayAfter, err)
	}
	var rules int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM site_deduplication_rules WHERE site_id = ?`, site.ID).Scan(&rules); err != nil || rules != 2 {
		t.Fatalf("deduplication rules = %d, %v", rules, err)
	}
	analytics, err := st.AdminAnalytics(ctx, site.ID, todayNoon.Format(time.DateOnly), tomorrowDate.AddDate(0, 0, 1).Format(time.DateOnly))
	if err != nil || len(analytics.DeduplicationRules) != 1 || analytics.DeduplicationRules[0].EffectiveDate != tomorrowDate.Format(time.DateOnly) || analytics.DeduplicationRules[0].WindowDays != 7 {
		t.Fatalf("AdminAnalytics deduplication rules = %#v, %v", analytics.DeduplicationRules, err)
	}
}
