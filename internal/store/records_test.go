package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"
)

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
			SiteID: site.ID, OccurredAt: base.Add(time.Duration(i) * time.Minute), Path: []string{"/", "/notes"}[i%2],
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
