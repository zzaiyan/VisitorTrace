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
