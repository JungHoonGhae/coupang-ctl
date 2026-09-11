package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestLegacyCompletionIsNotVerifiedAccountCoverage(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO sync_runs(started_at,completed_at,status,history_complete) VALUES ('2026-09-01','2026-09-01','completed',1)`); err != nil {
		t.Fatal(err)
	}
	status, err := s.LatestSyncStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.HistoryComplete {
		t.Fatal("legacy cursor completion promoted to verified account coverage")
	}
	context, err := s.RecommendationPurchaseContext(ctx, nil)
	if err != nil || context.Status == "available" {
		t.Fatal("unverified history presented as complete recommendation context")
	}
	costs, err := s.MembershipCosts(ctx)
	if err != nil || costs.CompleteHistorySync || costs.Status == "complete_available_history" {
		t.Fatal("unverified history presented as complete membership history")
	}
}

func TestCursorObservationIsNullableAtomicAndNotCoverage(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	status, err := s.LatestSyncStatus(ctx)
	if err != nil || status.CursorExhausted != nil || status.CoverageStatus != core.SyncCoverageNotAssessed {
		t.Fatal("never-run state invented cursor evidence")
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if value, present := wire["cursor_exhausted"]; !present || value != nil {
		t.Fatal("unknown cursor evidence must serialize as explicit null")
	}
	run, err := s.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
	if err != nil {
		t.Fatal(err)
	}
	page := core.OrderPage{Next: &core.OrderCursor{Year: 2026, Page: 2}}
	if _, err := s.ApplySyncPage(ctx, run, nil, page); err != nil {
		t.Fatal(err)
	}
	status, err = s.LatestSyncStatus(ctx)
	if err != nil || status.CursorExhausted == nil || *status.CursorExhausted {
		t.Fatal("continuation page cursor observation incorrect")
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER synthetic_checkpoint_failure BEFORE DELETE ON sync_checkpoint BEGIN SELECT RAISE(ABORT,'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplySyncPage(ctx, run, page.Next, core.OrderPage{}); err == nil {
		t.Fatal("checkpoint failure ignored")
	}
	status, err = s.LatestSyncStatus(ctx)
	if err != nil || status.CursorExhausted == nil || *status.CursorExhausted || status.PagesProcessed != 1 {
		t.Fatal("failed page changed cursor observation")
	}
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER synthetic_checkpoint_failure`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplySyncPage(ctx, run, page.Next, core.OrderPage{}); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSync(ctx, run, core.SyncResult{Complete: true, CursorExhausted: true, PagesProcessed: 2}, ""); err != nil {
		t.Fatal(err)
	}
	status, err = s.LatestSyncStatus(ctx)
	if err != nil || status.CursorExhausted == nil || !*status.CursorExhausted || status.HistoryComplete || status.CoverageStatus != core.SyncCoverageUnverified {
		t.Fatal("cursor exhaustion or caller boolean was promoted to verified coverage")
	}
}

func TestSyncCoverageMigrationPreservesLegacyFactsAndRollsBack(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, statement := range []string{
		`ALTER TABLE sync_runs DROP COLUMN cursor_exhausted`,
		`ALTER TABLE sync_runs DROP COLUMN coverage_status`,
		`DELETE FROM schema_migrations WHERE version=16`,
		`INSERT INTO sync_runs(started_at,status,history_complete) VALUES ('2026-09-01','completed',1)`,
		`CREATE TRIGGER synthetic_migration_failure BEFORE INSERT ON schema_migrations WHEN NEW.version=16 BEGIN SELECT RAISE(ABORT,'synthetic'); END`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.migrateSyncCoverage(ctx); err == nil {
		t.Fatal("migration failure ignored")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM pragma_table_info('sync_runs') WHERE name IN ('cursor_exhausted','coverage_status')`, 0)
	assertCount(t, s.db, `SELECT COUNT(*) FROM schema_migrations WHERE version=16`, 0)
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER synthetic_migration_failure`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.migrateSyncCoverage(ctx); err != nil {
			t.Fatal(err)
		}
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM sync_runs WHERE history_complete=1`, 1)
	status, err := s.LatestSyncStatus(ctx)
	if err != nil || status.CursorExhausted != nil || status.CoverageStatus != core.SyncCoverageUnknownLegacy || status.HistoryComplete {
		t.Fatal("migration invented account or cursor proof")
	}
}
