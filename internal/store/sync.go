package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

func (s *SQLite) LoadSyncCursor(ctx context.Context) (*core.OrderCursor, error) {
	var cursor core.OrderCursor
	err := s.db.QueryRowContext(ctx, "SELECT next_year, next_page FROM sync_checkpoint WHERE id = 1").Scan(&cursor.Year, &cursor.Page)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load sync checkpoint: %w", err)
	}
	return &cursor, nil
}

func (s *SQLite) SaveSyncCursor(ctx context.Context, cursor *core.OrderCursor) error {
	if cursor == nil {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM sync_checkpoint WHERE id = 1"); err != nil {
			return fmt.Errorf("clear sync checkpoint: %w", err)
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sync_checkpoint(id, next_year, next_page, updated_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET next_year = excluded.next_year,
			next_page = excluded.next_page, updated_at = excluded.updated_at`,
		cursor.Year, cursor.Page, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save sync checkpoint: %w", err)
	}
	return nil
}

func (s *SQLite) BeginSync(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO sync_runs(started_at, status)
		VALUES (?, 'running')`, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("begin sync run: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("identify sync run: %w", err)
	}
	return id, nil
}

func (s *SQLite) FinishSync(ctx context.Context, runID int64, result core.SyncResult, errorCode string) error {
	status := "completed"
	if errorCode != "" {
		status = "failed"
	}
	databaseResult, err := s.db.ExecContext(ctx, `UPDATE sync_runs SET completed_at = ?, status = ?,
		pages_processed = ?, records_upserted = ?, error_code = ? WHERE id = ? AND status = 'running'`,
		time.Now().UTC().Format(time.RFC3339Nano), status, result.PagesProcessed,
		result.OrdersSeen, nullIfEmpty(errorCode), runID)
	if err != nil {
		return fmt.Errorf("finish sync run: %w", err)
	}
	rows, err := databaseResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm sync run: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("finish sync run: run is not active")
	}
	return nil
}
