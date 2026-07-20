package store

import (
	"context"
	"path/filepath"
	"testing"
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
