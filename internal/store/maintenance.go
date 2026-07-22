package store

import (
	"context"
	"fmt"
	"time"
)

type CleanupResult struct {
	PageviewRecords       int64
	VisitorRegistrations  int64
	AdministratorSessions int64
}

func (r *CleanupResult) Add(other CleanupResult) {
	r.PageviewRecords += other.PageviewRecords
	r.VisitorRegistrations += other.VisitorRegistrations
	r.AdministratorSessions += other.AdministratorSessions
}

func (s *Store) CleanupBatch(ctx context.Context, now time.Time, batchSize int) (CleanupResult, error) {
	if batchSize < 1 || batchSize > 10_000 {
		return CleanupResult{}, fmt.Errorf("cleanup batch size must be between 1 and 10000")
	}
	sites, err := s.ListSites(ctx)
	if err != nil {
		return CleanupResult{}, err
	}
	var total CleanupResult
	for _, configuredSite := range sites {
		location, err := time.LoadLocation(configuredSite.Timezone)
		if err != nil {
			return total, fmt.Errorf("load Site %s timezone: %w", configuredSite.ID, err)
		}
		cutoff := now.UTC().AddDate(0, 0, -configuredSite.RetentionDays)
		localDate := now.In(location).Format(time.DateOnly)
		s.writeMu.Lock()
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			s.writeMu.Unlock()
			return total, fmt.Errorf("begin cleanup transaction: %w", err)
		}
		pageviews, err := tx.ExecContext(ctx, `
			DELETE FROM pageviews
			WHERE id IN (
				SELECT id FROM pageviews
				WHERE site_id = ? AND julianday(occurred_at) < julianday(?)
				ORDER BY occurred_at, id
				LIMIT ?
			)
		`, configuredSite.ID, cutoff.Format(time.RFC3339Nano), batchSize)
		if err != nil {
			_ = tx.Rollback()
			s.writeMu.Unlock()
			return total, fmt.Errorf("delete expired Pageview Records for Site %s: %w", configuredSite.ID, err)
		}
		registrations, err := tx.ExecContext(ctx, `
			DELETE FROM visitor_registrations
			WHERE site_id = ? AND (window_start, dimension_kind, dimension_value, visitor_digest) IN (
				SELECT window_start, dimension_kind, dimension_value, visitor_digest
				FROM visitor_registrations
				WHERE site_id = ? AND window_end <= ?
				ORDER BY window_end
				LIMIT ?
			)
		`, configuredSite.ID, configuredSite.ID, localDate, batchSize)
		if err != nil {
			_ = tx.Rollback()
			s.writeMu.Unlock()
			return total, fmt.Errorf("delete completed visitor registrations for Site %s: %w", configuredSite.ID, err)
		}
		if err := tx.Commit(); err != nil {
			s.writeMu.Unlock()
			return total, fmt.Errorf("commit cleanup transaction: %w", err)
		}
		s.writeMu.Unlock()
		count, _ := pageviews.RowsAffected()
		total.PageviewRecords += count
		count, _ = registrations.RowsAffected()
		total.VisitorRegistrations += count
	}

	s.writeMu.Lock()
	result, err := s.DB.ExecContext(ctx, `
		DELETE FROM administrator_sessions
		WHERE token_digest IN (
			SELECT token_digest FROM administrator_sessions
			WHERE julianday(expires_at) <= julianday(?) OR julianday(last_seen_at) <= julianday(?)
			LIMIT ?
		)
	`, now.UTC().Format(time.RFC3339Nano), now.UTC().Add(-12*time.Hour).Format(time.RFC3339Nano), batchSize)
	s.writeMu.Unlock()
	if err != nil {
		return total, fmt.Errorf("delete expired administrator sessions: %w", err)
	}
	total.AdministratorSessions, _ = result.RowsAffected()
	return total, nil
}
