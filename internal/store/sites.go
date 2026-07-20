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
		       dedup_window_days, retention_days, first_pageview_at, hmac_key, created_at, updated_at
		FROM sites WHERE id = ?
	`, id).Scan(&result.ID, &result.Name, &result.Timezone, &originsJSON, &accept, &publish, &dedup, &retention, &firstPageview, &key, &created, &updated)
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
