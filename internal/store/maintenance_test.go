package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupBatchExpiresRecordsButPreservesAggregates(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{
		Name: "Cleanup", AllowedOrigins: []string{"https://example.com"}, RetentionDays: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	for i, occurredAt := range []time.Time{now.Add(-25 * time.Hour), now.Add(-23 * time.Hour)} {
		_, err := st.RecordPageview(ctx, PageviewObservation{
			SiteID: site.ID, OccurredAt: occurredAt, Path: "/", VisitorDigest: bytes.Repeat([]byte{byte(i + 1)}, 32),
			OriginalIP: "192.0.2.1", OperatingSystem: "Linux", Browser: "Firefox",
		})
		if err != nil {
			t.Fatalf("RecordPageview(%d) error = %v", i, err)
		}
	}
	if err := st.CreateAdministratorSession(ctx, bytes.Repeat([]byte{3}, 32), bytes.Repeat([]byte{4}, 32), now.Add(-24*time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	result, err := st.CleanupBatch(ctx, now, 100)
	if err != nil {
		t.Fatalf("CleanupBatch() error = %v", err)
	}
	if result.PageviewRecords != 1 || result.VisitorRegistrations == 0 || result.AdministratorSessions != 1 {
		t.Fatalf("CleanupBatch() = %#v", result)
	}
	var records, aggregatePageviews int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pageviews WHERE site_id = ?`, site.ID).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRowContext(ctx, `SELECT SUM(pageviews) FROM daily_aggregates WHERE site_id = ? AND dimension_kind = 'overall'`, site.ID).Scan(&aggregatePageviews); err != nil {
		t.Fatal(err)
	}
	if records != 1 || aggregatePageviews != 2 {
		t.Fatalf("records = %d, aggregate Pageviews = %d", records, aggregatePageviews)
	}
}

func TestCleanupBatchIsBounded(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	site, err := st.CreateSite(ctx, CreateSiteParams{Name: "Bounded", AllowedOrigins: []string{"https://example.com"}, RetentionDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		_, err := st.RecordPageview(ctx, PageviewObservation{
			SiteID: site.ID, OccurredAt: now.Add(-48 * time.Hour), Path: "/", VisitorDigest: bytes.Repeat([]byte{byte(i + 1)}, 32),
			OriginalIP: "192.0.2.1", OperatingSystem: "Linux", Browser: "Firefox",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := st.CleanupBatch(ctx, now, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.PageviewRecords != 2 {
		t.Fatalf("deleted Pageview Records = %d, want 2", result.PageviewRecords)
	}
}

func TestOperationStatusLifecycle(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	started := time.Date(2026, time.July, 22, 3, 0, 0, 0, time.UTC)
	if err := st.StartOperation(ctx, "cleanup", started); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishOperation(ctx, "cleanup", started.Add(time.Second), true, "pageviews=2"); err != nil {
		t.Fatal(err)
	}
	statuses, err := st.OperationStatuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].Succeeded == nil || !*statuses[0].Succeeded || statuses[0].Summary != "pageviews=2" {
		t.Fatalf("OperationStatuses() = %#v", statuses)
	}
}
