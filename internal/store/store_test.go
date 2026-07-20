package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
