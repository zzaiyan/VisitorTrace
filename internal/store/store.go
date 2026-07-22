package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

const schemaVersion = 5

const MinimumSQLiteVersion = "3.51.3"

type Store struct {
	DB      *sql.DB
	Path    string
	writeMu sync.Mutex
}

func Initialize(ctx context.Context, path, passwordHash string) (*Store, error) {
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("database already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("check database: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	store, err := open(ctx, path)
	if err != nil {
		return nil, err
	}
	initialized := false
	defer func() {
		if !initialized {
			_ = store.Close()
			removeDatabaseFiles(path)
		}
	}()
	if err := store.initializeBaseSchema(ctx, passwordHash); err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("protect database: %w", err)
	}
	initialized = true
	return store, nil
}

func OpenExisting(ctx context.Context, path string) (*Store, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("database is unavailable: %w", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("database permissions %o are too broad; want 600", info.Mode().Perm())
	}
	return open(ctx, path)
}

func open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	store := &Store{DB: db, Path: path}
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure sqlite: %w", err)
		}
	}
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable sqlite WAL: %w", err)
	}
	if journalMode != "wal" && journalMode != "WAL" {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite did not enable WAL mode: %s", journalMode)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return store, nil
}

func (s *Store) SchemaReady(ctx context.Context) error {
	var version string
	if err := s.DB.QueryRowContext(ctx, "SELECT value FROM service_meta WHERE key = 'schema_version'").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version != fmt.Sprint(schemaVersion) {
		return fmt.Errorf("unsupported schema version %s", version)
	}
	return nil
}

func (s *Store) SQLiteVersion(ctx context.Context) (string, error) {
	var version string
	if err := s.DB.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		return "", fmt.Errorf("read sqlite version: %w", err)
	}
	return version, nil
}

func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	return s.currentSchemaVersion(ctx)
}

// OnlineBackup creates a transactionally consistent SQLite snapshot while the
// source database remains available to readers and writers.
func (s *Store) OnlineBackup(ctx context.Context, destination string) error {
	if destination == "" {
		return fmt.Errorf("backup destination is required")
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("backup destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check backup destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	// VACUUM INTO produces a compact, point-in-time copy without modifying the
	// source database or requiring the service to stop ingestion.
	if _, err := s.DB.ExecContext(ctx, `VACUUM INTO ?`, destination); err != nil {
		return fmt.Errorf("create SQLite snapshot: %w", err)
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		return fmt.Errorf("protect SQLite snapshot: %w", err)
	}
	return nil
}

func IntegrityCheckFile(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open database for integrity check: %w", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("run database integrity check: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("read database integrity result: %w", err)
		}
		if result != "ok" {
			return fmt.Errorf("database integrity check failed: %s", result)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate database integrity results: %w", err)
	}
	return nil
}

func SQLiteVersionAtLeast(actual, minimum string) bool {
	a, ok := parseVersion(actual)
	if !ok {
		return false
	}
	m, ok := parseVersion(minimum)
	if !ok {
		return false
	}
	for i := range a {
		if a[i] != m[i] {
			return a[i] > m[i]
		}
	}
	return true
}

func parseVersion(value string) ([3]int, bool) {
	var result [3]int
	parts := strings.Split(value, ".")
	if len(parts) != len(result) {
		return result, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return result, false
		}
		result[i] = n
	}
	return result, true
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func removeDatabaseFiles(path string) {
	_ = os.Remove(path)
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
}
