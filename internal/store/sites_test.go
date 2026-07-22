package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateGetAndListSite(t *testing.T) {
	st, err := Initialize(context.Background(), filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	defer st.Close()

	want, err := st.CreateSite(context.Background(), CreateSiteParams{
		Name:           "Academic homepage",
		AllowedOrigins: []string{"HTTPS://Example.com/", "https://blog.example.com"},
	})
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}
	if len(want.ID) != 16 || len(want.HMACKey) != 32 {
		t.Fatalf("generated Site credentials have unexpected sizes: id=%d key=%d", len(want.ID), len(want.HMACKey))
	}
	if !want.AllowsOrigin("https://example.com") || want.AllowsOrigin("https://example.com.evil.test") {
		t.Fatal("Allowed Origin matching is incorrect")
	}

	got, err := st.GetSite(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("GetSite() error = %v", err)
	}
	if got.Name != want.Name || got.Timezone != "Asia/Shanghai" || got.DedupWindowDays != 1 || got.RetentionDays != 30 {
		t.Fatalf("GetSite() = %#v", got)
	}
	if len(got.HMACKey) != 32 {
		t.Fatalf("GetSite() returned an invalid HMAC key length: %d", len(got.HMACKey))
	}

	items, err := st.ListSites(context.Background())
	if err != nil {
		t.Fatalf("ListSites() error = %v", err)
	}
	if len(items) != 1 || len(items[0].HMACKey) != 0 {
		t.Fatalf("ListSites() = %#v", items)
	}
}

func TestResetAndDeleteSite(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	created, err := st.CreateSite(ctx, CreateSiteParams{Name: "Reset", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	originalKey := append([]byte(nil), created.HMACKey...)
	if _, err := st.RecordPageview(ctx, PageviewObservation{
		SiteID: created.ID, OccurredAt: time.Now(), Path: "/", VisitorDigest: bytes.Repeat([]byte{1}, 32),
		OriginalIP: "192.0.2.1", OperatingSystem: "Linux", Browser: "Firefox",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.ResetSiteData(ctx, created.ID); err != nil {
		t.Fatalf("ResetSiteData() error = %v", err)
	}
	reset, err := st.GetSite(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reset.AcceptPageviews || reset.PublishPublic || reset.FirstPageviewAt != nil || bytes.Equal(reset.HMACKey, originalKey) {
		t.Fatalf("reset Site = %#v", reset)
	}
	for _, table := range []string{"pageviews", "visitor_registrations", "daily_aggregates", "geo_locations"} {
		var count int
		if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE site_id = ?`, created.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d rows", table, count)
		}
	}
	if _, err := st.UpdateSite(ctx, created.ID, UpdateSiteParams{
		Name: "Reset", Timezone: "UTC", AllowedOrigins: []string{"https://example.com"}, DedupWindowDays: 1, RetentionDays: 30,
	}); err != nil {
		t.Fatalf("timezone remained locked after reset: %v", err)
	}
	if err := st.DeleteSite(ctx, created.ID); err != nil {
		t.Fatalf("DeleteSite() error = %v", err)
	}
	if _, err := st.GetSite(ctx, created.ID); err == nil {
		t.Fatal("deleted Site is still available")
	}
}
