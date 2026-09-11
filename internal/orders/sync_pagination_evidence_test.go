package orders_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func TestSyncMissingEndEvidencePreservesCheckpointAndRejectsPage(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	source := &fixtureSource{documents: map[string][]byte{
		"initial": syntheticPage("synthetic-first", "2026-08-01", 100, true),
		"2026/2":  []byte(`{"orderList":[{"orderId":"synthetic-must-not-commit","orderDate":"2026-08-02","totalPrice":200}]}`),
	}}
	service := orders.New(ledger, source)
	got, err := service.Sync(ctx, core.SyncRequest{MaxPages: 3})
	if !errors.Is(err, core.ErrInvalidOrderData) || got.PagesProcessed != 1 || got.OrdersSeen != 1 || got.CursorExhausted || got.CoverageStatus != core.SyncCoverageUnverified {
		t.Fatalf("missing end evidence accepted: error=%v pages=%d exhausted=%v", err, got.PagesProcessed, got.CursorExhausted)
	}
	status, err := ledger.LatestSyncStatus(ctx)
	if err != nil || status.State != core.SyncRunFailed || status.ErrorCode != "invalid_document" || status.PagesProcessed != 1 || status.HistoryComplete {
		t.Fatal("invalid pagination did not persist a partial failed attempt", err)
	}
	cursor, err := ledger.LoadSyncCursor(ctx)
	if err != nil || cursor == nil || cursor.Year != 2026 || cursor.Page != 2 {
		t.Fatal("ambiguous page cleared resumable checkpoint", err)
	}
	stats, err := ledger.Stats(ctx, core.OrderFilter{})
	if err != nil || stats.OrderCount != 1 {
		t.Fatal("ambiguous page committed an order", err)
	}
	// Corrected source evidence resumes the rejected page, not the root, and
	// source exhaustion still does not assert verified account/range coverage.
	source.documents["2026/2"] = syntheticPage("synthetic-second", "2026-08-02", 200, false)
	resumed, err := service.Sync(ctx, core.SyncRequest{MaxPages: 3})
	if err != nil || resumed.PagesProcessed != 1 || !resumed.CursorExhausted || resumed.CoverageStatus != core.SyncCoverageUnverified || len(source.seen) != 3 || source.seen[2] != "2026/2" {
		t.Fatal("valid page could not resume after ambiguous pagination", err)
	}
}
