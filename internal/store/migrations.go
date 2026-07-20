package store

import (
	"context"
	"database/sql"
	"fmt"
)

type migration struct {
	version    int
	statements []string
}

var migrations = []migration{
	{
		version: 2,
		statements: []string{
			`CREATE TABLE sites (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				timezone TEXT NOT NULL,
				allowed_origins TEXT NOT NULL,
				accept_pageviews INTEGER NOT NULL DEFAULT 1 CHECK (accept_pageviews IN (0, 1)),
				publish_public INTEGER NOT NULL DEFAULT 1 CHECK (publish_public IN (0, 1)),
				dedup_window_days INTEGER NOT NULL DEFAULT 1 CHECK (dedup_window_days BETWEEN 1 AND 30),
				retention_days INTEGER NOT NULL DEFAULT 30 CHECK (retention_days BETWEEN 1 AND 90),
				first_pageview_at TEXT,
				hmac_key BLOB NOT NULL CHECK (length(hmac_key) = 32),
				map_preset TEXT NOT NULL DEFAULT '{}',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE pageviews (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
				occurred_at TEXT NOT NULL,
				local_date TEXT NOT NULL,
				path TEXT NOT NULL,
				country_code TEXT NOT NULL DEFAULT '',
				region_code TEXT NOT NULL DEFAULT '',
				city TEXT NOT NULL DEFAULT '',
				latitude REAL,
				longitude REAL,
				visitor_digest BLOB NOT NULL CHECK (length(visitor_digest) = 32),
				original_ip TEXT NOT NULL,
				operating_system TEXT NOT NULL,
				browser TEXT NOT NULL
			)`,
			`CREATE INDEX pageviews_site_time ON pageviews (site_id, occurred_at DESC, id DESC)`,
			`CREATE INDEX pageviews_site_ip ON pageviews (site_id, original_ip, occurred_at DESC)`,
			`CREATE INDEX pageviews_site_digest ON pageviews (site_id, visitor_digest, occurred_at DESC)`,
			`CREATE TABLE visitor_registrations (
				site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
				window_start TEXT NOT NULL,
				dimension_kind TEXT NOT NULL,
				dimension_value TEXT NOT NULL,
				visitor_digest BLOB NOT NULL CHECK (length(visitor_digest) = 32),
				created_at TEXT NOT NULL,
				PRIMARY KEY (site_id, window_start, dimension_kind, dimension_value, visitor_digest)
			) WITHOUT ROWID`,
			`CREATE INDEX visitor_registrations_window ON visitor_registrations (site_id, window_start)`,
			`CREATE TABLE daily_aggregates (
				site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
				local_date TEXT NOT NULL,
				dimension_kind TEXT NOT NULL,
				dimension_value TEXT NOT NULL,
				pageviews INTEGER NOT NULL DEFAULT 0 CHECK (pageviews >= 0),
				unique_visitors INTEGER NOT NULL DEFAULT 0 CHECK (unique_visitors >= 0),
				PRIMARY KEY (site_id, local_date, dimension_kind, dimension_value)
			) WITHOUT ROWID`,
		},
	},
}

func (s *Store) initializeBaseSchema(ctx context.Context, passwordHash string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin base schema transaction: %w", err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`, nil},
		{`CREATE TABLE service_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`, nil},
		{`CREATE TABLE administrators (id INTEGER PRIMARY KEY CHECK (id = 1), password_hash TEXT NOT NULL, created_at TEXT NOT NULL)`, nil},
		{`INSERT INTO service_meta (key, value) VALUES ('schema_version', '1')`, nil},
		{`INSERT INTO schema_migrations (version, applied_at) VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`, nil},
		{`INSERT INTO administrators (id, password_hash, created_at) VALUES (1, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`, []any{passwordHash}},
	}
	for i, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply base schema statement %d: %w", i+1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit base schema: %w", err)
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	current, err := s.currentSchemaVersion(ctx)
	if err != nil {
		return err
	}
	if current > schemaVersion {
		return fmt.Errorf("database schema %d is newer than supported schema %d", current, schemaVersion)
	}
	for _, item := range migrations {
		if item.version <= current {
			continue
		}
		if item.version != current+1 {
			return fmt.Errorf("missing migration from schema %d to %d", current, item.version)
		}
		if err := s.applyMigration(ctx, item); err != nil {
			return err
		}
		current = item.version
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, item migration) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", item.version, err)
	}
	for i, statement := range item.statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d statement %d: %w", item.version, i+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`, item.version); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration %d: %w", item.version, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE service_meta SET value = ? WHERE key = 'schema_version'`, fmt.Sprint(item.version)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("update schema version %d: %w", item.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", item.version, err)
	}
	return nil
}

func (s *Store) currentSchemaVersion(ctx context.Context) (int, error) {
	var version int
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("read current schema version: %w", err)
	}
	return version, nil
}
