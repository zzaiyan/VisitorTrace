package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"
)

type AdministratorSession struct {
	TokenDigest        []byte
	CSRFToken          []byte
	CreatedAt          time.Time
	LastSeenAt         time.Time
	ExpiresAt          time.Time
	PasswordVerifiedAt time.Time
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
		INSERT INTO administrator_sessions (token_digest, csrf_token, created_at, last_seen_at, expires_at, password_verified_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, tokenDigest, csrfToken, now, now, expiresAt.UTC().Format(time.RFC3339Nano), now)
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
	var created, lastSeen, expires, passwordVerified string
	err := s.DB.QueryRowContext(ctx, `
		SELECT token_digest, csrf_token, created_at, last_seen_at, expires_at, password_verified_at
		FROM administrator_sessions
		WHERE token_digest = ?
	`, tokenDigest).Scan(&result.TokenDigest, &result.CSRFToken, &created, &lastSeen, &expires, &passwordVerified)
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
	result.PasswordVerifiedAt, parseErr = time.Parse(time.RFC3339Nano, passwordVerified)
	if parseErr != nil {
		return AdministratorSession{}, fmt.Errorf("parse administrator password verification time: %w", parseErr)
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

func (s *Store) MarkAdministratorPasswordVerified(ctx context.Context, tokenDigest []byte, at time.Time) error {
	if len(tokenDigest) != sha256.Size {
		return fmt.Errorf("administrator session token must contain 32 bytes")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	result, err := s.DB.ExecContext(ctx, `UPDATE administrator_sessions SET password_verified_at = ? WHERE token_digest = ?`, at.UTC().Format(time.RFC3339Nano), tokenDigest)
	if err != nil {
		return fmt.Errorf("mark administrator password verification: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return fmt.Errorf("administrator session is unavailable")
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
	_, err := s.DB.ExecContext(ctx, `DELETE FROM administrator_sessions WHERE julianday(expires_at) <= julianday(?) OR julianday(last_seen_at) <= julianday(?)`, now.UTC().Format(time.RFC3339Nano), now.UTC().Add(-12*time.Hour).Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("delete expired administrator sessions: %w", err)
	}
	return nil
}

func (s *Store) RevokeAdministratorSessions(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM administrator_sessions`); err != nil {
		return fmt.Errorf("revoke administrator sessions: %w", err)
	}
	return nil
}

func (s *Store) UpdateAdministratorPassword(ctx context.Context, hash string) error {
	if hash == "" {
		return fmt.Errorf("administrator password hash is required")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin administrator password update: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE administrators SET password_hash = ? WHERE id = 1`, hash)
	if err != nil {
		return fmt.Errorf("update administrator password: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return fmt.Errorf("administrator credential is unavailable")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM administrator_sessions`); err != nil {
		return fmt.Errorf("revoke administrator sessions after password update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit administrator password update: %w", err)
	}
	return nil
}

func HashSessionToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
