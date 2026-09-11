package orders_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func TestSyncCheckpointFailureRollsBackPageAndProgress(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	ledger, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	// Persistent trigger affects the real service connection, not a mocked
	// repository. This database is created exclusively for this test.
	injector, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer injector.Close()
	if _, err := injector.ExecContext(ctx, `CREATE TRIGGER synthetic_checkpoint_failure BEFORE INSERT ON sync_checkpoint BEGIN SELECT RAISE(ABORT, 'synthetic checkpoint failure'); END`); err != nil {
		t.Fatal(err)
	}
	source := &fixturePageSource{page: core.OrderPage{
		Orders: []core.Order{{SourceRef: "synthetic-new", PurchasedAt: "2026-08-01", Currency: "KRW", Items: []core.OrderItem{{ProductID: "123", Name: "Synthetic", Quantity: 1}}}},
		Next:   &core.OrderCursor{Year: 2026, Page: 2},
	}}
	result, err := orders.NewWithPageSource(ledger, source).Sync(ctx, core.SyncRequest{MaxPages: 1})
	if err == nil {
		t.Fatal("injected checkpoint failure was ignored")
	}
	rows, err := ledger.ListOrders(ctx, core.OrderFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatal("page committed despite checkpoint failure")
	}
	cursor, err := ledger.LoadSyncCursor(ctx)
	if err != nil || cursor != nil || result.PagesProcessed != 0 || result.OrdersSeen != 0 {
		t.Fatal("failed page advanced checkpoint or progress")
	}
	status, err := ledger.LatestSyncStatus(ctx)
	if err != nil || status.State != core.SyncRunFailed || status.PagesProcessed != 0 || status.OrdersSeen != 0 {
		t.Fatal("failed page changed persisted progress")
	}
}

type cancelSyncPageSource struct {
	cancel context.CancelFunc
	calls  int
}

func (s *cancelSyncPageSource) FetchPage(ctx context.Context, _ *core.OrderCursor) (core.OrderPage, error) {
	s.calls++
	if s.calls == 2 {
		s.cancel()
		return core.OrderPage{}, ctx.Err()
	}
	return core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-first", PurchasedAt: "2026-08-01", Currency: "KRW"}}, Next: &core.OrderCursor{Year: 2026, Page: 2}}, nil
}

func TestSyncCancellationKeepsCommittedPageAndFinishesAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	source := &cancelSyncPageSource{cancel: cancel}
	result, err := orders.NewWithPageSource(ledger, source).Sync(ctx, core.SyncRequest{MaxPages: 5})
	if !errors.Is(err, context.Canceled) || source.calls != 2 || result.PagesProcessed != 1 {
		t.Fatal("cancellation lost its cause or started another page")
	}
	status, err := ledger.LatestSyncStatus(context.Background())
	if err != nil || status.State != core.SyncRunFailed || status.ErrorCode != "canceled" || status.PagesProcessed != 1 || status.OrdersSeen != 1 || status.HistoryComplete {
		t.Fatal("canceled attempt remained running or lost committed progress")
	}
	cursor, err := ledger.LoadSyncCursor(context.Background())
	if err != nil || cursor == nil || cursor.Page != 2 {
		t.Fatal("cancellation changed resumable checkpoint")
	}
	rows, err := ledger.ListOrders(context.Background(), core.OrderFilter{Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].SourceRef != "synthetic-first" {
		t.Fatal("cancellation lost the committed order")
	}
}

func TestSyncCancellationReportsCleanupFailureWithoutLosingCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	ledger, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	injector, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer injector.Close()
	if _, err := injector.ExecContext(ctx, `CREATE TRIGGER synthetic_finish_failure BEFORE UPDATE OF status ON sync_runs BEGIN SELECT RAISE(ABORT, 'synthetic finish failure'); END`); err != nil {
		t.Fatal(err)
	}
	source := &cancelSyncPageSource{cancel: cancel}
	result, err := orders.NewWithPageSource(ledger, source).Sync(ctx, core.SyncRequest{MaxPages: 5})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "finish sync run") {
		t.Fatal("cleanup failure hid cancellation or was silently ignored")
	}
	if source.calls != 2 || result.PagesProcessed != 1 {
		t.Fatal("cleanup performed extra acquisition or lost committed progress")
	}
	status, err := ledger.LatestSyncStatus(context.Background())
	if err != nil || status.State != core.SyncRunRunning || status.PagesProcessed != 1 || status.HistoryComplete {
		t.Fatal("failed cleanup claimed a persisted terminal state")
	}
}

func TestEmptySourceEndPreservesRetainedOrders(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	prior := core.Order{SourceRef: "synthetic-retained", PurchasedAt: "2026-08-01", Currency: "KRW", Items: []core.OrderItem{{ProductID: "123", Name: "Synthetic", Quantity: 2}}}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{prior}}); err != nil {
		t.Fatal(err)
	}
	result, err := orders.NewWithPageSource(ledger, &fixturePageSource{page: core.OrderPage{Orders: []core.Order{}}}).Sync(ctx, core.SyncRequest{MaxPages: 1})
	if err != nil || result.OrdersSeen != 0 || result.OrdersRemoved != 0 {
		t.Fatal("empty source result deleted retained data")
	}
	rows, err := ledger.ListOrders(ctx, core.OrderFilter{Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].FullyCanceled || len(rows[0].Items) != 1 || rows[0].Items[0].Quantity != 2 {
		t.Fatal("empty source result changed prior order or items")
	}
}
