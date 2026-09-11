package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func TestSyncStatusExplainsResumedScanAndRetainedUnobservedOrders(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	order := func(ref string) core.Order {
		return core.Order{SourceRef: ref, PurchasedAt: "2026-08-01", Currency: "KRW"}
	}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{order("synthetic-retained")}}); err != nil {
		t.Fatal(err)
	}
	unlock, err := ledger.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	next := &core.OrderCursor{Year: 2026, Page: 2}
	first, err := ledger.BeginResumableSync(ctx, core.SyncSourceCamofox, core.SyncProvenanceObservedStructuredOrderDocument, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ApplySyncPage(ctx, first, nil, core.OrderPage{Orders: []core.Order{order("synthetic-a"), order("synthetic-b")}, Next: next}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.FinishSync(ctx, first, core.SyncResult{PagesProcessed: 1, OrdersSeen: 2}, ""); err != nil {
		t.Fatal(err)
	}
	second, err := ledger.BeginResumableSync(ctx, core.SyncSourceCamofox, core.SyncProvenanceObservedStructuredOrderDocument, next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ApplySyncPage(ctx, second, next, core.OrderPage{Orders: []core.Order{order("synthetic-a"), order("synthetic-c")}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.FinishSync(ctx, second, core.SyncResult{PagesProcessed: 1, OrdersSeen: 2}, ""); err != nil {
		t.Fatal(err)
	}
	status, err := ledger.LatestSyncStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	scan, ok := wire["scan"].(map[string]any)
	if !ok {
		t.Fatal("latest-attempt response omits cumulative scan evidence")
	}
	for field, want := range map[string]any{
		"state": "cursor_exhausted", "starts_from_beginning": true,
		"attempts": float64(2), "pages_processed": float64(2),
		"retained_orders_observed": float64(3), "retained_orders_not_observed": float64(1), "next": nil,
	} {
		got, present := scan[field]
		if !present || got != want {
			t.Fatalf("scan %s = %v, want %v", field, got, want)
		}
	}
	if wire["schema_version"] != float64(3) || status.PagesProcessed != 1 || status.OrdersSeen != 2 || status.HistoryComplete || status.CoverageStatus != core.SyncCoverageUnverified {
		t.Fatal("cumulative scan changed attempt counts or asserted whole-account coverage")
	}
	if bytes.Contains(encoded, []byte("synthetic-")) {
		t.Fatal("scan evidence exposed order references")
	}
}

func TestScanEvidencePreservesLegacyStartFailureAndExplicitRestart(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	unlock, err := ledger.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	status, err := ledger.LatestSyncStatus(ctx)
	if err != nil || status.Scan != nil {
		t.Fatal("never-run ledger invented a scan", err)
	}
	next := &core.OrderCursor{Year: 2026, Page: 9}
	if err := ledger.SaveSyncCursor(ctx, next); err != nil {
		t.Fatal(err)
	}
	first, err := ledger.BeginResumableSync(ctx, core.SyncSourceCamofox, core.SyncProvenanceObservedStructuredOrderDocument, next)
	if err != nil {
		t.Fatal(err)
	}
	status, err = ledger.LatestSyncStatus(ctx)
	if err != nil || status.Scan == nil || status.Scan.StartsFromBeginning || status.Scan.PagesProcessed != 0 || status.Scan.State != "active" || status.Scan.Next == nil || *status.Scan.Next != *next {
		t.Fatal("legacy checkpoint became a scan from the beginning", err)
	}
	later := &core.OrderCursor{Year: 2026, Page: 10}
	if _, err := ledger.ApplySyncPage(ctx, first, next, core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-history", PurchasedAt: "2026-08-01", Currency: "KRW"}}, Next: later}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.FinishSync(ctx, first, core.SyncResult{PagesProcessed: 1, OrdersSeen: 1}, ""); err != nil {
		t.Fatal(err)
	}
	second, err := ledger.BeginResumableSync(ctx, core.SyncSourceCamofox, core.SyncProvenanceObservedStructuredOrderDocument, later)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.FinishSync(ctx, second, core.SyncResult{}, "document_source"); err != nil {
		t.Fatal(err)
	}
	status, err = ledger.LatestSyncStatus(ctx)
	if err != nil || status.State != core.SyncRunFailed || status.PagesProcessed != 0 || status.Scan == nil || status.Scan.Attempts != 2 || status.Scan.PagesProcessed != 1 || status.Scan.RetainedOrdersObserved != 1 || status.Scan.State != "active" || status.Scan.StartsFromBeginning {
		t.Fatal("failed resumption lost prior committed progress", err)
	}
	if _, err := ledger.RestartSyncScan(ctx, core.SyncSourceCamofox, core.SyncProvenanceObservedStructuredOrderDocument, later); err != nil {
		t.Fatal(err)
	}
	status, err = ledger.LatestSyncStatus(ctx)
	if err != nil || status.Scan == nil || status.Scan.State != "active" || !status.Scan.StartsFromBeginning || status.Scan.Attempts != 1 || status.Scan.PagesProcessed != 0 || status.Scan.Next != nil || status.Scan.RetainedOrdersObserved != 0 || status.Scan.RetainedOrdersNotObserved != 1 {
		t.Fatal("restart reused prior scan evidence or removed retained history", err)
	}
	if status.CursorExhausted != nil || status.HistoryComplete {
		t.Fatal("unread restart became source-end proof")
	}
}
