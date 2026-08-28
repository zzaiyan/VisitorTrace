package store

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitializeCreatesProtectedSQLiteStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "visitortrace.sqlite3")
	ctx := context.Background()
	st, err := Initialize(ctx, path, "test-hash")
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	defer st.Close()
	if err := st.SchemaReady(ctx); err != nil {
		t.Fatalf("SchemaReady() error = %v", err)
	}
	version, err := st.SQLiteVersion(ctx)
	if err != nil {
		t.Fatalf("SQLiteVersion() error = %v", err)
	}
	if version == "" {
		t.Fatal("SQLiteVersion() returned an empty version")
	}
	if !SQLiteVersionAtLeast(version, MinimumSQLiteVersion) {
		t.Fatalf("SQLite version %s is older than %s", version, MinimumSQLiteVersion)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("stat database: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions = %o, want 600", info.Mode().Perm())
	}
	if _, err := Initialize(ctx, path, "test-hash"); err == nil {
		t.Fatal("Initialize() allowed overwriting an existing database")
	}
}

func TestUpdateAdministratorPasswordRevokesSessions(t *testing.T) {
	ctx := context.Background()
	st, err := Initialize(ctx, filepath.Join(t.TempDir(), "visitortrace.sqlite3"), "old-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.CreateAdministratorSession(ctx, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32), time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateAdministratorPassword(ctx, "new-hash"); err != nil {
		t.Fatal(err)
	}
	hash, err := st.AdministratorPasswordHash(ctx)
	if err != nil || hash != "new-hash" {
		t.Fatalf("password hash = %q, error = %v", hash, err)
	}
	var sessions int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM administrator_sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("sessions = %d, want 0", sessions)
	}
}

func TestSQLiteVersionAtLeast(t *testing.T) {
	tests := []struct {
		actual  string
		minimum string
		want    bool
	}{
		{"3.51.3", "3.51.3", true},
		{"3.53.3", "3.51.3", true},
		{"3.51.2", "3.51.3", false},
		{"invalid", "3.51.3", false},
	}
	for _, test := range tests {
		if got := SQLiteVersionAtLeast(test.actual, test.minimum); got != test.want {
			t.Errorf("SQLiteVersionAtLeast(%q, %q) = %v, want %v", test.actual, test.minimum, got, test.want)
		}
	}
}

func TestMigrateFromSchemaV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "visitortrace.sqlite3")
	ctx := context.Background()
	st, err := open(ctx, path)
	if err != nil {
		t.Fatalf("open() error = %v", err)
	}
	defer st.Close()
	if err := st.initializeBaseSchema(ctx, "test-hash"); err != nil {
		t.Fatalf("initializeBaseSchema() error = %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := st.SchemaReady(ctx); err != nil {
		t.Fatalf("SchemaReady() error = %v", err)
	}
	var table string
	if err := st.DB.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'sites'`).Scan(&table); err != nil {
		t.Fatalf("sites table is unavailable: %v", err)
	}
}

func TestMigrateFromSchemaV10AddsUnlimitedRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "visitortrace.sqlite3")
	ctx := context.Background()
	st, err := open(ctx, path)
	if err != nil {
		t.Fatalf("open() error = %v", err)
	}
	defer st.Close()
	if err := st.initializeBaseSchema(ctx, "test-hash"); err != nil {
		t.Fatal(err)
	}
	for _, item := range migrations {
		if item.version > 10 {
			break
		}
		if err := st.applyMigration(ctx, item); err != nil {
			t.Fatalf("apply migration %d: %v", item.version, err)
		}
	}
	if version, err := st.SchemaVersion(ctx); err != nil || version != 10 {
		t.Fatalf("pre-migration schema = %d, %v", version, err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO sites (id, name, timezone, allowed_origins, hmac_key, created_at, updated_at)
		VALUES ('migration-site', 'Migration', 'UTC', '["https://example.com"]', ?, '2026-07-26T00:00:00Z', '2026-07-26T00:00:00Z')
	`, bytes.Repeat([]byte{1}, 32)); err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	var columns int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('sites') WHERE name = 'retention_unlimited'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 1 {
		t.Fatalf("retention_unlimited columns = %d, want 1", columns)
	}
	var unlimited int
	if err := st.DB.QueryRowContext(ctx, `SELECT retention_unlimited FROM sites WHERE id = 'migration-site'`).Scan(&unlimited); err != nil {
		t.Fatal(err)
	}
	if unlimited != 0 {
		t.Fatalf("migrated retention_unlimited = %d, want 0", unlimited)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE sites SET public_language = 'ja' WHERE id = 'migration-site'`); err != nil {
		t.Fatalf("Japanese public language is not accepted after migration: %v", err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO pageviews (
			site_id, occurred_at, local_date, hostname, path, visitor_digest,
			original_ip, operating_system, browser
		) VALUES (?, '2026-07-26T00:00:00Z', '2026-07-26', 'example.com', '/', ?, '192.0.2.1', 'Linux', 'Firefox')
	`, "migration-site", bytes.Repeat([]byte{2}, 32)); err != nil {
		t.Fatal(err)
	}
	var method string
	if err := st.DB.QueryRowContext(ctx, `SELECT collection_method FROM pageviews WHERE site_id = ?`, "migration-site").Scan(&method); err != nil {
		t.Fatal(err)
	}
	if method != CollectionMethodJS {
		t.Fatalf("default collection method = %q, want %q", method, CollectionMethodJS)
	}
	if err := st.SchemaReady(ctx); err != nil {
		t.Fatal(err)
	}
}
