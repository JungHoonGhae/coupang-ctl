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

type cyclingPageSource struct{ calls int }

func (s *cyclingPageSource) FetchPage(_ context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	s.calls++
	next := 2
	if cursor != nil && cursor.Page == 2 {
		next = 3
	}
	return core.OrderPage{Orders: []core.Order{}, Next: &core.OrderCursor{Year: 2026, Page: next}}, nil
}

func TestSyncCursorLoopSurvivesSeparateAttemptsAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	source := &cyclingPageSource{}
	for i := 0; i < 4; i++ {
		ledger, err := store.Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		result, syncErr := orders.NewWithPageSource(ledger, source).Sync(ctx, core.SyncRequest{MaxPages: 1})
		status, statusErr := ledger.LatestSyncStatus(ctx)
		ledger.Close()
		if i < 3 {
			if syncErr != nil || result.PagesProcessed != 1 {
				t.Fatalf("attempt %d: %v", i, syncErr)
			}
		} else if !errors.Is(syncErr, orders.ErrCursorLoop) || source.calls != 3 || result.PagesProcessed != 0 || statusErr != nil || status.State != core.SyncRunFailed || status.ErrorCode != "cursor_loop" {
			t.Fatalf("resumed loop was not stopped before source: calls=%d error=%v status=%s", source.calls, syncErr, status.ErrorCode)
		}
	}
}

func TestSyncFreshScanMayRevisitCursorsAfterSourceEnd(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	source := &fixturePageSource{page: core.OrderPage{Orders: []core.Order{}}}
	for i := 0; i < 2; i++ {
		result, err := orders.NewWithPageSource(ledger, source).Sync(ctx, core.SyncRequest{MaxPages: 1})
		if err != nil || result.PagesProcessed != 1 || !result.CursorExhausted || result.Complete {
			t.Fatal("new scan rejected or source end promoted to complete coverage", err)
		}
	}
	if len(source.seen) != 2 {
		t.Fatal("fresh scan skipped the source")
	}
}

func TestSyncExplicitRestartBeginsAtStartAndPreservesOrders(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	prior := core.Order{SourceRef: "synthetic-retained", PurchasedAt: "2026-08-01", Currency: "KRW"}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{prior}}); err != nil {
		t.Fatal(err)
	}
	cycling := &cyclingPageSource{}
	for i := 0; i < 3; i++ {
		if _, err := orders.NewWithPageSource(ledger, cycling).Sync(ctx, core.SyncRequest{MaxPages: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := orders.NewWithPageSource(ledger, cycling).Sync(ctx, core.SyncRequest{MaxPages: 1}); !errors.Is(err, orders.ErrCursorLoop) {
		t.Fatal("loop prerequisite missing")
	}
	fresh := &fixturePageSource{page: core.OrderPage{Orders: []core.Order{}}}
	result, err := orders.NewWithPageSource(ledger, fresh).Sync(ctx, core.SyncRequest{MaxPages: 1, RestartScan: true})
	if err != nil || len(fresh.seen) != 1 || fresh.seen[0] != nil || !result.CursorExhausted || result.Complete {
		t.Fatalf("explicit restart did not read source start: calls=%d err=%v", len(fresh.seen), err)
	}
	rows, err := ledger.ListOrders(ctx, core.OrderFilter{Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].SourceRef != prior.SourceRef {
		t.Fatal("restart deleted retained orders")
	}
}

func TestSyncRestartDoesNotDiscardCheckpointOnCanceledOrInvalidRequest(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	cycling := &cyclingPageSource{}
	service := orders.NewWithPageSource(ledger, cycling)
	if _, err := service.Sync(ctx, core.SyncRequest{MaxPages: 1}); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := service.Sync(canceled, core.SyncRequest{MaxPages: 1, RestartScan: true}); !errors.Is(err, context.Canceled) {
		t.Fatal("restart ignored cancellation")
	}
	if _, err := service.Sync(ctx, core.SyncRequest{MaxPages: 1001, RestartScan: true}); err == nil {
		t.Fatal("restart ignored request bounds")
	}
	cursor, err := ledger.LoadSyncCursor(ctx)
	if err != nil || cursor == nil || cursor.Page != 2 || cycling.calls != 1 {
		t.Fatal("rejected restart changed checkpoint or read source")
	}
}
