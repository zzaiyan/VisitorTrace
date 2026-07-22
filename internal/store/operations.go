package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type OperationStatus struct {
	Operation   string
	StartedAt   time.Time
	CompletedAt *time.Time
	Succeeded   *bool
	Summary     string
}

func (s *Store) StartOperation(ctx context.Context, operation string, at time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO operation_status (operation, started_at, completed_at, succeeded, summary)
		VALUES (?, ?, NULL, NULL, '')
		ON CONFLICT (operation) DO UPDATE SET
			started_at = excluded.started_at,
			completed_at = NULL,
			succeeded = NULL,
			summary = ''
	`, operation, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("start %s operation: %w", operation, err)
	}
	return nil
}

func (s *Store) FinishOperation(ctx context.Context, operation string, at time.Time, succeeded bool, summary string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	success := 0
	if succeeded {
		success = 1
	}
	result, err := s.DB.ExecContext(ctx, `
		UPDATE operation_status
		SET completed_at = ?, succeeded = ?, summary = ?
		WHERE operation = ?
	`, at.UTC().Format(time.RFC3339Nano), success, summary, operation)
	if err != nil {
		return fmt.Errorf("finish %s operation: %w", operation, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read %s operation update result: %w", operation, err)
	}
	if rows != 1 {
		return fmt.Errorf("finish %s operation: operation was not started", operation)
	}
	return nil
}

func (s *Store) OperationStatuses(ctx context.Context) ([]OperationStatus, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT operation, started_at, completed_at, succeeded, summary
		FROM operation_status
		ORDER BY operation
	`)
	if err != nil {
		return nil, fmt.Errorf("read operation statuses: %w", err)
	}
	defer rows.Close()
	var result []OperationStatus
	for rows.Next() {
		var item OperationStatus
		var started string
		var completed sql.NullString
		var succeeded sql.NullInt64
		if err := rows.Scan(&item.Operation, &started, &completed, &succeeded, &item.Summary); err != nil {
			return nil, fmt.Errorf("scan operation status: %w", err)
		}
		parsed, err := time.Parse(time.RFC3339Nano, started)
		if err != nil {
			return nil, fmt.Errorf("parse operation start time: %w", err)
		}
		item.StartedAt = parsed
		if completed.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, completed.String)
			if err != nil {
				return nil, fmt.Errorf("parse operation completion time: %w", err)
			}
			item.CompletedAt = &parsed
		}
		if succeeded.Valid {
			value := succeeded.Int64 == 1
			item.Succeeded = &value
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operation statuses: %w", err)
	}
	return result, nil
}
