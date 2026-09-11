package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

var ErrSyncCheckpointChanged = errors.New("sync checkpoint changed before page commit")
var ErrSyncRunInactive = errors.New("sync run is not active")

// ApplySyncPage commits normalized rows, attempt observations, managed scan
// visits, the next cursor, and progress together. expected binds the page to the cursor actually requested. This
// protects page commits, but is not a scan-wide writer lease or account proof.
func (s *SQLite) ApplySyncPage(ctx context.Context, runID int64, expected *core.OrderCursor, page core.OrderPage) (core.UpsertResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.UpsertResult{}, fmt.Errorf("begin sync page: %w", err)
	}
	defer tx.Rollback()
	// Acquire SQLite's write lock before checking the cursor; a deferred
	// read-then-write transaction could otherwise race another writer.
	update, err := tx.ExecContext(ctx, `UPDATE sync_runs SET pages_processed = pages_processed + 1,
		records_upserted = records_upserted + ?, cursor_exhausted = ? WHERE id = ? AND status = 'running'`, len(page.Orders), page.Next == nil, runID)
	if err != nil {
		return core.UpsertResult{}, fmt.Errorf("record sync page progress: %w", err)
	}
	changed, err := update.RowsAffected()
	if err != nil {
		return core.UpsertResult{}, fmt.Errorf("verify sync run update: %w", err)
	}
	if changed != 1 {
		return core.UpsertResult{}, ErrSyncRunInactive
	}
	actual, err := loadSyncCursor(ctx, tx)
	if err != nil {
		return core.UpsertResult{}, err
	}
	if (actual == nil) != (expected == nil) || actual != nil && *actual != *expected {
		return core.UpsertResult{}, ErrSyncCheckpointChanged
	}
	if err := recordSyncScanPage(ctx, tx, runID, expected, page.Next); err != nil {
		return core.UpsertResult{}, err
	}
	result, err := upsertOrderPageTx(ctx, tx, page)
	if err != nil {
		return core.UpsertResult{}, err
	}
	for _, order := range page.Orders {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO sync_run_observed_orders(run_id, source_ref) VALUES (?, ?)`, runID, order.SourceRef); err != nil {
			return core.UpsertResult{}, fmt.Errorf("record sync order observation: %w", err)
		}
	}
	if err := saveSyncCursor(ctx, tx, page.Next); err != nil {
		return core.UpsertResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return core.UpsertResult{}, fmt.Errorf("commit sync page: %w", err)
	}
	return result, nil
}
