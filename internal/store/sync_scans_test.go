package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func startSyntheticScan(t *testing.T, s *SQLite, cursor *core.OrderCursor) int64 {
	t.Helper()
	id, err := s.BeginResumableSync(context.Background(), core.SyncSourceOrdinaryBrowser, core.SyncProvenanceObservedStructuredOrderDocument, cursor)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSyncScanPageFailureRollsBackVisitsAndCanRetry(t *testing.T) {
	for _, trigger := range []string{
		`CREATE TRIGGER synthetic_fail BEFORE INSERT ON sync_scan_pages BEGIN SELECT RAISE(ABORT,'synthetic'); END`,
		`CREATE TRIGGER synthetic_fail BEFORE UPDATE OF next_page ON sync_scans BEGIN SELECT RAISE(ABORT,'synthetic'); END`,
		`CREATE TRIGGER synthetic_fail BEFORE INSERT ON sync_checkpoint BEGIN SELECT RAISE(ABORT,'synthetic'); END`,
	} {
		ctx := context.Background()
		s := openWriterLedger(t, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
		release, err := s.AcquireSyncWriter(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { release() })
		run := startSyntheticScan(t, s, nil)
		if _, err := s.db.ExecContext(ctx, trigger); err != nil {
			t.Fatal(err)
		}
		page := core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-order", PurchasedAt: "2026-08-01", Currency: "KRW"}}, Next: &core.OrderCursor{Year: 2026, Page: 2}}
		if _, err := s.ApplySyncPage(ctx, run, nil, page); err == nil {
			t.Fatal("injected page failure ignored")
		}
		assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scan_pages`, 0)
		assertCount(t, s.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 0)
		assertCount(t, s.db, `SELECT COUNT(*) FROM orders`, 0)
		assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scans WHERE next_page IS NULL AND state='active'`, 1)
		if err := s.CheckSyncCursor(ctx, run, nil); err != nil {
			t.Fatal("uncommitted read became visited", err)
		}
		if _, err := s.db.ExecContext(ctx, `DROP TRIGGER synthetic_fail`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ApplySyncPage(ctx, run, nil, page); err != nil {
			t.Fatal("retry rejected", err)
		}
		assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scan_pages`, 1)
		assertCount(t, s.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 1)
		assertCount(t, s.db, `SELECT pages_processed FROM sync_runs WHERE id=1`, 1)
		if err := s.CheckSyncCursor(ctx, run, nil); !errors.Is(err, core.ErrSyncCursorLoop) {
			t.Fatal("committed revisit accepted")
		}
	}
}

func TestSyncScanRecoversInterruptedAttemptAndRetainsProgress(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	s := openWriterLedger(t, path)
	release, err := s.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first := startSyntheticScan(t, s, nil)
	next := &core.OrderCursor{Year: 2026, Page: 2}
	if _, err := s.ApplySyncPage(ctx, first, nil, core.OrderPage{Orders: []core.Order{}, Next: next}); err != nil {
		t.Fatal(err)
	}
	// Abandon bookkeeping without FinishSync, then reopen like a new caller.
	if err := release(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	b := openWriterLedger(t, path)
	release, err = b.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	second := startSyntheticScan(t, b, next)
	if second == first {
		t.Fatal("attempt identity reused")
	}
	assertCount(t, b.db, `SELECT COUNT(*) FROM sync_scans`, 1)
	assertCount(t, b.db, `SELECT COUNT(*) FROM sync_scan_attempts`, 2)
	assertCount(t, b.db, `SELECT COUNT(*) FROM sync_runs WHERE status='failed' AND error_code='interrupted' AND pages_processed=1 AND history_complete=0`, 1)
	if err := b.CheckSyncCursor(ctx, second, nil); !errors.Is(err, core.ErrSyncCursorLoop) {
		t.Fatal("recovery forgot committed visits")
	}
	if err := b.CheckSyncCursor(ctx, second, next); err != nil {
		t.Fatal("recovery rejected unvisited next page")
	}
}

func TestSyncScanLegacyCheckpointHasNoInventedVisitsAndSourceCannotChange(t *testing.T) {
	ctx := context.Background()
	s := openWriterLedger(t, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	release, err := s.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	next := &core.OrderCursor{Year: 2026, Page: 9}
	if err := s.SaveSyncCursor(ctx, next); err != nil {
		t.Fatal(err)
	}
	run := startSyntheticScan(t, s, next)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scans WHERE starts_from_beginning=0`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scan_pages`, 0)
	if err := s.CheckSyncCursor(ctx, run, next); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginResumableSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument, next); !errors.Is(err, ErrSyncScanMismatch) {
		t.Fatal("source mismatch silently resumed")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_runs WHERE status='running'`, 1)
	if err := s.SaveSyncCursor(ctx, &core.OrderCursor{Year: 2026, Page: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginResumableSync(ctx, core.SyncSourceOrdinaryBrowser, core.SyncProvenanceObservedStructuredOrderDocument, &core.OrderCursor{Year: 2026, Page: 10}); !errors.Is(err, ErrSyncScanMismatch) {
		t.Fatal("external checkpoint edit silently resumed")
	}
}

func TestSyncScanMigrationIsAtomicAndIdempotent(t *testing.T) {
	ctx := context.Background()
	s := openWriterLedger(t, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	for _, sql := range []string{`DROP TABLE sync_scan_pages`, `DROP TABLE sync_scan_attempts`, `DROP TABLE sync_scans`, `DELETE FROM schema_migrations WHERE version=17`,
		`CREATE TRIGGER synthetic_fail BEFORE INSERT ON schema_migrations WHEN NEW.version=17 BEGIN SELECT RAISE(ABORT,'synthetic'); END`} {
		if _, err := s.db.ExecContext(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.migrateSyncScans(ctx); err == nil {
		t.Fatal("migration failure ignored")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM schema_migrations WHERE version=17`, 0)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sync_scans'`, 0)
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER synthetic_fail`); err != nil {
		t.Fatal(err)
	}
	// Force a later DDL failure after the version row and scan table were
	// tentatively created. Neither may survive the failed migration.
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE sync_one_active_scan(synthetic INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateSyncScans(ctx); err == nil {
		t.Fatal("late migration failure ignored")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM schema_migrations WHERE version=17`, 0)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sync_scans'`, 0)
	if _, err := s.db.ExecContext(ctx, `DROP TABLE sync_one_active_scan`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.migrateSyncScans(ctx); err != nil {
			t.Fatal(err)
		}
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM schema_migrations WHERE version=17`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scans`, 0)
}

func TestSyncScanRecoveryAndNewAttemptAreAtomic(t *testing.T) {
	ctx := context.Background()
	s := openWriterLedger(t, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	release, err := s.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	startSyntheticScan(t, s, nil)
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER synthetic_fail BEFORE INSERT ON sync_runs BEGIN SELECT RAISE(ABORT,'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginResumableSync(ctx, core.SyncSourceOrdinaryBrowser, core.SyncProvenanceObservedStructuredOrderDocument, nil); err == nil {
		t.Fatal("failed new attempt ignored")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_runs WHERE status='running' AND error_code IS NULL`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scan_attempts`, 1)
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER synthetic_fail`); err != nil {
		t.Fatal(err)
	}
	startSyntheticScan(t, s, nil)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_runs WHERE status='failed' AND error_code='interrupted'`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_runs WHERE status='running'`, 1)
}
