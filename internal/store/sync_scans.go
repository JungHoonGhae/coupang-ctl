package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

var ErrSyncScanMismatch = errors.New("sync scan source or checkpoint does not match")

func (s *SQLite) migrateSyncScans(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(17,strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		`CREATE TABLE IF NOT EXISTS sync_scans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sync_source TEXT NOT NULL, sync_provenance TEXT NOT NULL,
			started_at TEXT NOT NULL, ended_at TEXT,
			starts_from_beginning INTEGER NOT NULL CHECK(starts_from_beginning IN (0,1)),
			state TEXT NOT NULL CHECK(state IN ('active','exhausted')),
			next_year INTEGER, next_page INTEGER,
			CHECK((next_year IS NULL) = (next_page IS NULL))
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS sync_one_active_scan ON sync_scans(state) WHERE state='active'`,
		`CREATE TABLE IF NOT EXISTS sync_scan_attempts (
			run_id INTEGER PRIMARY KEY REFERENCES sync_runs(id) ON DELETE CASCADE,
			scan_id INTEGER NOT NULL REFERENCES sync_scans(id)
		)`,
		`CREATE TABLE IF NOT EXISTS sync_scan_pages (
			scan_id INTEGER NOT NULL REFERENCES sync_scans(id),
			cursor_key TEXT NOT NULL,
			run_id INTEGER NOT NULL REFERENCES sync_runs(id),
			PRIMARY KEY(scan_id,cursor_key)
		)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate sync scans: %w", err)
		}
	}
	return tx.Commit()
}

// BeginResumableSync requires the caller to hold AcquireSyncWriter until the
// attempt finishes. A scan groups bounded attempts; it does not verify source
// account identity, adapter semantics, requested coverage, or completeness.
// Legacy checkpoints start a partial scan without inventing past page visits.
func (s *SQLite) BeginResumableSync(ctx context.Context, source core.SyncSource, provenance string, expected *core.OrderCursor) (int64, error) {
	return s.beginSyncScan(ctx, source, provenance, expected, false)
}

// RestartSyncScan requires the same whole-attempt writer ownership as resume.
// It supersedes a pending scan without deleting its evidence or retained orders.
// Explicit restart is not evidence that the source's previous failure is fixed.
func (s *SQLite) RestartSyncScan(ctx context.Context, source core.SyncSource, provenance string, expected *core.OrderCursor) (int64, error) {
	return s.beginSyncScan(ctx, source, provenance, expected, true)
}

func (s *SQLite) beginSyncScan(ctx context.Context, source core.SyncSource, provenance string, expected *core.OrderCursor, restart bool) (int64, error) {
	if !source.ValidForAcquisition() || provenance != core.SyncProvenanceObservedStructuredOrderDocument {
		return 0, errors.New("begin resumable sync: invalid acquisition evidence")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// Obtain SQLite's write lock before reads; the whole-attempt OS lock only
	// coordinates cooperative acquisition, not direct database writers.
	if _, err := tx.ExecContext(ctx, `UPDATE sync_scans SET state=state WHERE state='active'`); err != nil {
		return 0, err
	}
	actual, err := loadSyncCursor(ctx, tx)
	if err != nil {
		return 0, err
	}
	if !sameSyncCursor(actual, expected) {
		return 0, ErrSyncCheckpointChanged
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if restart {
		if _, err := tx.ExecContext(ctx, `UPDATE sync_scans SET superseded_at=? WHERE state='active' AND superseded_at IS NULL`, now); err != nil {
			return 0, err
		}
		if err := saveSyncCursor(ctx, tx, nil); err != nil {
			return 0, err
		}
		expected = nil
	}
	var scanID int64
	var scanSource, scanProvenance string
	var year, page sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT id,sync_source,sync_provenance,next_year,next_page FROM sync_scans WHERE state='active' AND superseded_at IS NULL`).Scan(&scanID, &scanSource, &scanProvenance, &year, &page)
	if err == sql.ErrNoRows {
		y, p := syncCursorValues(expected)
		created, err := tx.ExecContext(ctx, `INSERT INTO sync_scans(sync_source,sync_provenance,started_at,starts_from_beginning,state,next_year,next_page) VALUES(?,?,?,?,'active',?,?)`, source, provenance, now, expected == nil, y, p)
		if err != nil {
			return 0, err
		}
		scanID, err = created.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	} else if scanSource != string(source) || scanProvenance != provenance || !nullableCursorMatches(year, page, expected) {
		return 0, ErrSyncScanMismatch
	}
	// The OS lease has no previous live owner. Preserve committed progress and
	// mark only managed, abandoned attempts as interrupted, never completed.
	if _, err := tx.ExecContext(ctx, `UPDATE sync_runs SET status='failed',completed_at=?,error_code='interrupted',history_complete=0
		WHERE status='running' AND id IN (SELECT run_id FROM sync_scan_attempts)`, now); err != nil {
		return 0, err
	}
	created, err := tx.ExecContext(ctx, `INSERT INTO sync_runs(started_at,status,sync_source,sync_provenance,coverage_status) VALUES(?,'running',?,?,?)`, now, source, provenance, core.SyncCoverageUnverified)
	if err != nil {
		return 0, err
	}
	runID, err := created.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sync_scan_attempts(run_id,scan_id) VALUES(?,?)`, runID, scanID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return runID, nil
}

type syncCursorReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func syncCursorKey(cursor *core.OrderCursor) string {
	if cursor == nil {
		return "start"
	}
	return fmt.Sprintf("%d:%d", cursor.Year, cursor.Page)
}

func sameSyncCursor(a, b *core.OrderCursor) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func syncCursorValues(cursor *core.OrderCursor) (any, any) {
	if cursor == nil {
		return nil, nil
	}
	return cursor.Year, cursor.Page
}

func nullableCursorMatches(year, page sql.NullInt64, cursor *core.OrderCursor) bool {
	if cursor == nil {
		return !year.Valid && !page.Valid
	}
	return year.Valid && page.Valid && year.Int64 == int64(cursor.Year) && page.Int64 == int64(cursor.Page)
}

// CheckSyncCursor rejects a committed page revisit before another source read.
// Failed/uncommitted reads are retryable because visits commit with page data.
func (s *SQLite) CheckSyncCursor(ctx context.Context, runID int64, cursor *core.OrderCursor) error {
	return checkSyncCursor(ctx, s.db, runID, cursor)
}

func checkSyncCursor(ctx context.Context, reader syncCursorReader, runID int64, cursor *core.OrderCursor) error {
	var visited bool
	err := reader.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sync_scan_attempts a JOIN sync_scan_pages p ON p.scan_id=a.scan_id WHERE a.run_id=? AND p.cursor_key=?)`, runID, syncCursorKey(cursor)).Scan(&visited)
	if err != nil {
		return err
	}
	if visited {
		return core.ErrSyncCursorLoop
	}
	return nil
}

func recordSyncScanPage(ctx context.Context, tx *sql.Tx, runID int64, expected, next *core.OrderCursor) error {
	var scanID int64
	err := tx.QueryRowContext(ctx, `SELECT scan_id FROM sync_scan_attempts WHERE run_id=?`, runID).Scan(&scanID)
	if err == sql.ErrNoRows {
		return nil
	} // Legacy low-level acquisition API.
	if err != nil {
		return err
	}
	if err := checkSyncCursor(ctx, tx, runID, expected); err != nil {
		return err
	}
	var state string
	var year, page sql.NullInt64
	var superseded sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT state,next_year,next_page,superseded_at FROM sync_scans WHERE id=?`, scanID).Scan(&state, &year, &page, &superseded); err != nil {
		return err
	}
	if state != "active" || superseded.Valid || !nullableCursorMatches(year, page, expected) {
		return ErrSyncScanMismatch
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sync_scan_pages(scan_id,cursor_key,run_id) VALUES(?,?,?)`, scanID, syncCursorKey(expected), runID); err != nil {
		return err
	}
	y, p := syncCursorValues(next)
	var ended any
	if next == nil {
		state = "exhausted"
		ended = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err = tx.ExecContext(ctx, `UPDATE sync_scans SET state=?,next_year=?,next_page=?,ended_at=? WHERE id=?`, state, y, p, ended, scanID)
	return err
}
