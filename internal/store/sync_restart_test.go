package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestSyncRestartPreservesEvidenceAndRollsBackOnFailure(t *testing.T) {
	ctx := context.Background()
	s := openWriterLedger(t, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	release, err := s.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	run := startSyntheticScan(t, s, nil)
	next := &core.OrderCursor{Year: 2026, Page: 2}
	page := core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-retained", PurchasedAt: "2026-08-01", Currency: "KRW"}}, Next: next}
	if _, err := s.ApplySyncPage(ctx, run, nil, page); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSync(ctx, run, core.SyncResult{PagesProcessed: 1, OrdersSeen: 1}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER synthetic_fail BEFORE INSERT ON sync_runs BEGIN SELECT RAISE(ABORT,'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestartSyncScan(ctx, core.SyncSourceOrdinaryBrowser, core.SyncProvenanceObservedStructuredOrderDocument, next); err == nil {
		t.Fatal("restart failure ignored")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scans WHERE state='active' AND superseded_at IS NULL`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scans`, 1)
	cursor, err := s.LoadSyncCursor(ctx)
	if err != nil || !sameSyncCursor(cursor, next) {
		t.Fatal("failed restart discarded checkpoint")
	}
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER synthetic_fail`); err != nil {
		t.Fatal(err)
	}
	newRun, err := s.RestartSyncScan(ctx, core.SyncSourceOrdinaryBrowser, core.SyncProvenanceObservedStructuredOrderDocument, next)
	if err != nil || newRun == run {
		t.Fatal("restart failed", err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scans WHERE superseded_at IS NOT NULL AND state='active' AND ended_at IS NULL AND next_page=2`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scans WHERE superseded_at IS NULL AND starts_from_beginning=1 AND next_page IS NULL`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scan_pages`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM orders`, 1)
	if err := s.CheckSyncCursor(ctx, newRun, nil); err != nil {
		t.Fatal("old visit contaminated new scan", err)
	}
	if cursor, err := s.LoadSyncCursor(ctx); err != nil || cursor != nil {
		t.Fatal("restart did not reset checkpoint")
	}
}

func TestSyncRestartMigrationPreservesExistingScanAndIsAtomic(t *testing.T) {
	ctx := context.Background()
	s := openWriterLedger(t, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	// Recreate the v17 shape in this synthetic DB only.
	for _, statement := range []string{
		`DROP INDEX sync_one_active_scan`,
		`ALTER TABLE sync_scans DROP COLUMN superseded_at`,
		`CREATE UNIQUE INDEX sync_one_active_scan ON sync_scans(state) WHERE state='active'`,
		`DELETE FROM schema_migrations WHERE version=18`,
		`INSERT INTO sync_scans(sync_source,sync_provenance,started_at,starts_from_beginning,state,next_year,next_page) VALUES('ordinary_browser','synthetic','2026-08-01',0,'active',2026,2)`,
		`CREATE TRIGGER synthetic_fail BEFORE INSERT ON schema_migrations WHEN NEW.version=18 BEGIN SELECT RAISE(ABORT,'synthetic'); END`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.migrateSyncScanSupersession(ctx); err == nil {
		t.Fatal("migration failure ignored")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM schema_migrations WHERE version=18`, 0)
	assertCount(t, s.db, `SELECT COUNT(*) FROM pragma_table_info('sync_scans') WHERE name='superseded_at'`, 0)
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scans WHERE next_page=2`, 1)
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER synthetic_fail`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.migrateSyncScanSupersession(ctx); err != nil {
			t.Fatal(err)
		}
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_scans WHERE superseded_at IS NULL AND next_page=2`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM schema_migrations WHERE version=18`, 1)
}
