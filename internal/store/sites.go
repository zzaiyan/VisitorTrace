package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/site"
)

type Site struct {
	ID              string
	Name            string
	Timezone        string
	AllowedOrigins  []string
	AcceptPageviews bool
	PublishPublic   bool
	DedupWindowDays int
	RetentionDays   int
	FirstPageviewAt *time.Time
	HMACKey         []byte
	MapPresetJSON   string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CreateSiteParams struct {
	Name            string
	Timezone        string
	AllowedOrigins  []string
	DedupWindowDays int
	RetentionDays   int
}

func (s *Store) CreateSite(ctx context.Context, params CreateSiteParams) (Site, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	name := strings.TrimSpace(params.Name)
	if name == "" || len(name) > 100 {
		return Site{}, fmt.Errorf("site name must contain between 1 and 100 bytes")
	}
	timezone := params.Timezone
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return Site{}, fmt.Errorf("invalid site timezone: %w", err)
	}
	origins, err := site.NormalizeOrigins(params.AllowedOrigins)
	if err != nil {
		return Site{}, err
	}
	dedupWindow := params.DedupWindowDays
	if dedupWindow == 0 {
		dedupWindow = 1
	}
	retention := params.RetentionDays
	if retention == 0 {
		retention = 30
	}
	if dedupWindow < 1 || dedupWindow > 30 {
		return Site{}, fmt.Errorf("deduplication window must be between 1 and 30 days")
	}
	if retention < 1 || retention > 90 {
		return Site{}, fmt.Errorf("retention period must be between 1 and 90 days")
	}

	id, err := newSiteID()
	if err != nil {
		return Site{}, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return Site{}, fmt.Errorf("generate Site HMAC key: %w", err)
	}
	originJSON, err := json.Marshal(origins)
	if err != nil {
		return Site{}, fmt.Errorf("encode allowed origins: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO sites (
			id, name, timezone, allowed_origins, accept_pageviews, publish_public,
			dedup_window_days, retention_days, hmac_key, created_at, updated_at
		) VALUES (?, ?, ?, ?, 1, 1, ?, ?, ?, ?, ?)
	`, id, name, timezone, string(originJSON), dedupWindow, retention, key, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return Site{}, fmt.Errorf("create Site: %w", err)
	}
	return Site{
		ID:              id,
		Name:            name,
		Timezone:        timezone,
		AllowedOrigins:  origins,
		AcceptPageviews: true,
		PublishPublic:   true,
		DedupWindowDays: dedupWindow,
		RetentionDays:   retention,
		HMACKey:         key,
		MapPresetJSON:   "{}",
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func (s *Store) GetSite(ctx context.Context, id string) (Site, error) {
	var result Site
	var originsJSON string
	var accept, publish, dedup, retention int
	var firstPageview sql.NullString
	var created, updated string
	var key []byte
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, name, timezone, allowed_origins, accept_pageviews, publish_public,
		       dedup_window_days, retention_days, first_pageview_at, hmac_key, map_preset, created_at, updated_at
		FROM sites WHERE id = ?
	`, id).Scan(&result.ID, &result.Name, &result.Timezone, &originsJSON, &accept, &publish, &dedup, &retention, &firstPageview, &key, &result.MapPresetJSON, &created, &updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return Site{}, fmt.Errorf("Site %q not found", id)
		}
		return Site{}, fmt.Errorf("read Site: %w", err)
	}
	if err := json.Unmarshal([]byte(originsJSON), &result.AllowedOrigins); err != nil {
		return Site{}, fmt.Errorf("decode Site origins: %w", err)
	}
	result.AcceptPageviews = accept == 1
	result.PublishPublic = publish == 1
	result.DedupWindowDays = dedup
	result.RetentionDays = retention
	result.HMACKey = append([]byte(nil), key...)
	if firstPageview.Valid {
		value, err := time.Parse(time.RFC3339Nano, firstPageview.String)
		if err != nil {
			return Site{}, fmt.Errorf("parse first Pageview timestamp: %w", err)
		}
		result.FirstPageviewAt = &value
	}
	result.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Site{}, fmt.Errorf("parse Site created timestamp: %w", err)
	}
	result.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return Site{}, fmt.Errorf("parse Site updated timestamp: %w", err)
	}
	return result, nil
}

type UpdateSiteParams struct {
	Name            string
	Timezone        string
	AllowedOrigins  []string
	AcceptPageviews bool
	PublishPublic   bool
	DedupWindowDays int
	RetentionDays   int
}

func (s *Store) UpdateSite(ctx context.Context, id string, params UpdateSiteParams) (Site, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	name := strings.TrimSpace(params.Name)
	if name == "" || len(name) > 100 {
		return Site{}, fmt.Errorf("site name must contain between 1 and 100 bytes")
	}
	if _, err := time.LoadLocation(params.Timezone); err != nil {
		return Site{}, fmt.Errorf("invalid site timezone: %w", err)
	}
	origins, err := site.NormalizeOrigins(params.AllowedOrigins)
	if err != nil {
		return Site{}, err
	}
	if params.DedupWindowDays < 1 || params.DedupWindowDays > 30 {
		return Site{}, fmt.Errorf("deduplication window must be between 1 and 30 days")
	}
	if params.RetentionDays < 1 || params.RetentionDays > 90 {
		return Site{}, fmt.Errorf("retention period must be between 1 and 90 days")
	}
	originJSON, err := json.Marshal(origins)
	if err != nil {
		return Site{}, fmt.Errorf("encode allowed origins: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Site{}, fmt.Errorf("begin update Site transaction: %w", err)
	}
	defer tx.Rollback()
	var firstPageview sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT first_pageview_at FROM sites WHERE id = ?`, id).Scan(&firstPageview); err != nil {
		if err == sql.ErrNoRows {
			return Site{}, fmt.Errorf("Site %q not found", id)
		}
		return Site{}, fmt.Errorf("read Site state: %w", err)
	}
	if firstPageview.Valid {
		var currentTimezone string
		if err := tx.QueryRowContext(ctx, `SELECT timezone FROM sites WHERE id = ?`, id).Scan(&currentTimezone); err != nil {
			return Site{}, fmt.Errorf("read Site timezone: %w", err)
		}
		if params.Timezone != currentTimezone {
			return Site{}, fmt.Errorf("Site timezone is locked after the first Pageview")
		}
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE sites
		SET name = ?, timezone = ?, allowed_origins = ?, accept_pageviews = ?, publish_public = ?,
		    dedup_window_days = ?, retention_days = ?, updated_at = ?
		WHERE id = ?
	`, name, params.Timezone, string(originJSON), boolInt(params.AcceptPageviews), boolInt(params.PublishPublic), params.DedupWindowDays, params.RetentionDays, now, id)
	if err != nil {
		return Site{}, fmt.Errorf("update Site: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Site{}, fmt.Errorf("commit Site update: %w", err)
	}
	return s.GetSite(ctx, id)
}

func (s *Store) UpdateMapPreset(ctx context.Context, id, presetJSON string) error {
	if strings.TrimSpace(presetJSON) == "" {
		presetJSON = "{}"
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	result, err := s.DB.ExecContext(ctx, `UPDATE sites SET map_preset = ?, updated_at = ? WHERE id = ?`, presetJSON, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("update Map Preset: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("Site %q not found", id)
	}
	return nil
}

func (s *Store) ResetSiteData(ctx context.Context, id string) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("regenerate Site HMAC key: %w", err)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Site data reset: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE sites
		SET accept_pageviews = 0, publish_public = 0, first_pageview_at = NULL,
		    hmac_key = ?, updated_at = ?
		WHERE id = ?
	`, key, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("disable Site before data reset: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return fmt.Errorf("Site %q not found", id)
	}
	for _, statement := range []string{
		`DELETE FROM pageviews WHERE site_id = ?`,
		`DELETE FROM visitor_registrations WHERE site_id = ?`,
		`DELETE FROM daily_aggregates WHERE site_id = ?`,
		`DELETE FROM geo_locations WHERE site_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, id); err != nil {
			return fmt.Errorf("clear Site data: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Site data reset: %w", err)
	}
	return nil
}

func (s *Store) DeleteSite(ctx context.Context, id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Site deletion: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE sites SET accept_pageviews = 0, publish_public = 0 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("disable Site before deletion: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return fmt.Errorf("Site %q not found", id)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sites WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete Site: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Site deletion: %w", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Store) ListSites(ctx context.Context) ([]Site, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM sites ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list Sites: %w", err)
	}
	defer rows.Close()
	var result []Site
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan Site: %w", err)
		}
		item, err := s.GetSite(ctx, id)
		if err != nil {
			return nil, err
		}
		item.HMACKey = nil
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Sites: %w", err)
	}
	return result, nil
}

func (s Site) AllowsOrigin(origin string) bool {
	normalized, err := site.NormalizeOrigin(origin)
	if err != nil {
		return false
	}
	for _, allowed := range s.AllowedOrigins {
		if normalized == allowed {
			return true
		}
	}
	return false
}

func newSiteID() (string, error) {
	value := make([]byte, 10)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate Site ID: %w", err)
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value)), nil
}
