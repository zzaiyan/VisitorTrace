package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"
)

type AdministratorSession struct {
	TokenDigest []byte
	CSRFToken   []byte
	CreatedAt   time.Time
	LastSeenAt  time.Time
	ExpiresAt   time.Time
}

func (s *Store) AdministratorPasswordHash(ctx context.Context) (string, error) {
	var hash string
	if err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM administrators WHERE id = 1`).Scan(&hash); err != nil {
		return "", fmt.Errorf("read administrator password: %w", err)
	}
	return hash, nil
}

func (s *Store) CreateAdministratorSession(ctx context.Context, tokenDigest, csrfToken []byte, createdAt, expiresAt time.Time) error {
	if len(tokenDigest) != sha256.Size || len(csrfToken) != sha256.Size {
		return fmt.Errorf("administrator session tokens must contain 32 bytes")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := createdAt.UTC().Format(time.RFC3339Nano)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO administrator_sessions (token_digest, csrf_token, created_at, last_seen_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, tokenDigest, csrfToken, now, now, expiresAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create administrator session: %w", err)
	}
	return nil
}

func (s *Store) FindAdministratorSession(ctx context.Context, tokenDigest []byte, now time.Time) (AdministratorSession, error) {
	if len(tokenDigest) != sha256.Size {
		return AdministratorSession{}, sql.ErrNoRows
	}
	var result AdministratorSession
	var created, lastSeen, expires string
	err := s.DB.QueryRowContext(ctx, `
		SELECT token_digest, csrf_token, created_at, last_seen_at, expires_at
		FROM administrator_sessions
		WHERE token_digest = ?
	`, tokenDigest).Scan(&result.TokenDigest, &result.CSRFToken, &created, &lastSeen, &expires)
	if err != nil {
		return AdministratorSession{}, err
	}
	var parseErr error
	result.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, created)
	if parseErr != nil {
		return AdministratorSession{}, fmt.Errorf("parse administrator session creation time: %w", parseErr)
	}
	result.LastSeenAt, parseErr = time.Parse(time.RFC3339Nano, lastSeen)
	if parseErr != nil {
		return AdministratorSession{}, fmt.Errorf("parse administrator session last-seen time: %w", parseErr)
	}
	result.ExpiresAt, parseErr = time.Parse(time.RFC3339Nano, expires)
	if parseErr != nil {
		return AdministratorSession{}, fmt.Errorf("parse administrator session expiry time: %w", parseErr)
	}
	if !now.UTC().Before(result.ExpiresAt) || now.UTC().Sub(result.LastSeenAt) >= 12*time.Hour {
		return AdministratorSession{}, sql.ErrNoRows
	}
	return result, nil
}

func (s *Store) TouchAdministratorSession(ctx context.Context, tokenDigest []byte, at time.Time) error {
	if len(tokenDigest) != sha256.Size {
		return fmt.Errorf("administrator session token must contain 32 bytes")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.DB.ExecContext(ctx, `UPDATE administrator_sessions SET last_seen_at = ? WHERE token_digest = ?`, at.UTC().Format(time.RFC3339Nano), tokenDigest)
	if err != nil {
		return fmt.Errorf("touch administrator session: %w", err)
	}
	return nil
}

func (s *Store) DeleteAdministratorSession(ctx context.Context, tokenDigest []byte) error {
	if len(tokenDigest) != sha256.Size {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.DB.ExecContext(ctx, `DELETE FROM administrator_sessions WHERE token_digest = ?`, tokenDigest)
	if err != nil {
		return fmt.Errorf("delete administrator session: %w", err)
	}
	return nil
}

func (s *Store) DeleteExpiredAdministratorSessions(ctx context.Context, now time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.DB.ExecContext(ctx, `DELETE FROM administrator_sessions WHERE expires_at <= ? OR last_seen_at <= ?`, now.UTC().Format(time.RFC3339Nano), now.UTC().Add(-12*time.Hour).Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("delete expired administrator sessions: %w", err)
	}
	return nil
}

func HashSessionToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
