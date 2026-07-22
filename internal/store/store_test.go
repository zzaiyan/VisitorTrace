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

func TestMigrateFromSchemaV8AddsHostname(t *testing.T) {
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
	for _, item := range migrations[:len(migrations)-1] {
		if err := st.applyMigration(ctx, item); err != nil {
			t.Fatalf("apply migration %d: %v", item.version, err)
		}
	}
	if version, err := st.SchemaVersion(ctx); err != nil || version != 8 {
		t.Fatalf("pre-migration schema = %d, %v", version, err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	var columns int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('pageviews') WHERE name = 'hostname'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 1 {
		t.Fatalf("hostname columns = %d, want 1", columns)
	}
	if err := st.SchemaReady(ctx); err != nil {
		t.Fatal(err)
	}
}
