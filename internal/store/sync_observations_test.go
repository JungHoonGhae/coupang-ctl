package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestSyncObservationsAreAttemptScopedAndAtomic(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	page := core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-one", PurchasedAt: "2026-08-01", Currency: "KRW"}}, Next: &core.OrderCursor{Year: 2026, Page: 2}}
	run, err := s.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplySyncPage(ctx, run, nil, page); err != nil {
		t.Fatal(err)
	}
	// Re-observation in the same attempt is not another distinct order.
	last := page
	last.Next = nil
	if _, err := s.ApplySyncPage(ctx, run, page.Next, last); err != nil {
		t.Fatal(err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 1)
	if err := s.FinishSync(ctx, run, core.SyncResult{PagesProcessed: 2, OrdersSeen: 2}, ""); err != nil {
		t.Fatal(err)
	}
	nextRun, err := s.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplySyncPage(ctx, nextRun, nil, page); err != nil {
		t.Fatal(err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 2)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 2)
	// A checkpoint failure must roll back the new order AND its observation.
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER synthetic_failure BEFORE INSERT ON sync_checkpoint BEGIN SELECT RAISE(ABORT, 'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	failed := page
	failed.Orders = []core.Order{{SourceRef: "synthetic-failed", PurchasedAt: "2026-08-01", Currency: "KRW"}}
	failed.Next = &core.OrderCursor{Year: 2026, Page: 3}
	if _, err := s.ApplySyncPage(ctx, nextRun, page.Next, failed); err == nil {
		t.Fatal("checkpoint failure ignored")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 2)
	assertCount(t, s.db, `SELECT COUNT(*) FROM orders`, 1)
	// Explicit user purge still removes associated references; sync never purges.
	if _, err := s.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 0)
}

func TestSyncObservationMigrationPreservesLegacyWithoutInventingEvidence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-legacy", PurchasedAt: "2026-08-01", Currency: "KRW"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncCursor(ctx, &core.OrderCursor{Year: 2026, Page: 7}); err != nil {
		t.Fatal(err)
	}
	// Simulate the exact pre-observation schema in a temporary synthetic DB.
	for _, statement := range []string{`DROP TABLE sync_run_observed_orders`, `DELETE FROM schema_migrations WHERE version = 15`} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER synthetic_migration_failure BEFORE INSERT ON schema_migrations WHEN NEW.version = 15 BEGIN SELECT RAISE(ABORT, 'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateSyncObservations(ctx); err == nil {
		t.Fatal("injected migration failure ignored")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'sync_run_observed_orders'`, 0)
	assertCount(t, s.db, `SELECT COUNT(*) FROM schema_migrations WHERE version = 15`, 0)
	assertCount(t, s.db, `SELECT COUNT(*) FROM orders`, 1)
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER synthetic_migration_failure`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		s, err = Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		assertCount(t, s.db, `SELECT COUNT(*) FROM orders`, 1)
		assertCount(t, s.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 0)
		assertCount(t, s.db, `SELECT COUNT(*) FROM schema_migrations WHERE version = 15`, 1)
		cursor, err := s.LoadSyncCursor(ctx)
		if err != nil || cursor == nil || cursor.Page != 7 {
			t.Fatal("migration changed legacy checkpoint")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
