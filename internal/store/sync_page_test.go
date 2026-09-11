package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func syncPage(ref string, quantity, next int) core.OrderPage {
	return core.OrderPage{Orders: []core.Order{{SourceRef: ref, PurchasedAt: "2026-08-01", Currency: "KRW",
		Items: []core.OrderItem{{ProductID: "123", Name: "Synthetic", Quantity: quantity}},
	}}, Next: &core.OrderCursor{Year: 2026, Page: next}}
}

func TestSyncPageFailurePreservesPreviousRowsCursorAndProgress(t *testing.T) {
	for _, trigger := range []string{
		`CREATE TRIGGER synthetic_failure BEFORE INSERT ON sync_checkpoint BEGIN SELECT RAISE(ABORT, 'synthetic'); END`,
		`CREATE TRIGGER synthetic_failure BEFORE INSERT ON order_items BEGIN SELECT RAISE(ABORT, 'synthetic'); END`,
		`CREATE TRIGGER synthetic_failure BEFORE UPDATE OF pages_processed ON sync_runs BEGIN SELECT RAISE(ABORT, 'synthetic'); END`,
		`CREATE TRIGGER synthetic_failure BEFORE INSERT ON sync_run_observed_orders BEGIN SELECT RAISE(ABORT, 'synthetic'); END`,
	} {
		ctx := context.Background()
		path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
		ledger, err := store.Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		defer ledger.Close()
		previous := syncPage("synthetic-order", 3, 4)
		if _, err := ledger.UpsertOrderPage(ctx, previous); err != nil {
			t.Fatal(err)
		}
		if err := ledger.SaveSyncCursor(ctx, previous.Next); err != nil {
			t.Fatal(err)
		}
		run, err := ledger.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
		if err != nil {
			t.Fatal(err)
		}
		injector, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := injector.ExecContext(ctx, trigger); err != nil {
			t.Fatal(err)
		}
		injector.Close()
		if _, err := ledger.ApplySyncPage(ctx, run, previous.Next, syncPage("synthetic-order", 7, 5)); err == nil {
			t.Fatal("failure injection ignored")
		}
		rows, err := ledger.ListOrders(ctx, core.OrderFilter{Limit: 10})
		if err != nil || len(rows) != 1 || len(rows[0].Items) != 1 || rows[0].Items[0].Quantity != 3 {
			t.Fatal("failed replacement changed prior rows")
		}
		cursor, err := ledger.LoadSyncCursor(ctx)
		if err != nil || cursor == nil || *cursor != *previous.Next {
			t.Fatal("failed transaction changed checkpoint")
		}
		status, err := ledger.LatestSyncStatus(ctx)
		if err != nil || status.PagesProcessed != 0 || status.OrdersSeen != 0 {
			t.Fatal("failed transaction changed progress")
		}
	}
}

func TestSyncPageCommitSurvivesReopenAndRejectsStaleWriter(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	ledger, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	run, err := ledger.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ApplySyncPage(ctx, run, nil, syncPage("synthetic-first", 1, 2)); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	ledger, err = store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	status, err := ledger.LatestSyncStatus(ctx)
	if err != nil || status.State != core.SyncRunRunning || status.PagesProcessed != 1 || status.OrdersSeen != 1 {
		t.Fatal("committed progress lost before attempt finish")
	}
	if _, err := ledger.ApplySyncPage(ctx, run, nil, syncPage("synthetic-stale", 9, 3)); !errors.Is(err, store.ErrSyncCheckpointChanged) {
		t.Fatal("stale expected cursor accepted")
	}
	rows, err := ledger.ListOrders(ctx, core.OrderFilter{Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].SourceRef != "synthetic-first" {
		t.Fatal("stale writer changed rows")
	}
	if err := ledger.FinishSync(ctx, run, core.SyncResult{PagesProcessed: 1, OrdersSeen: 1}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ApplySyncPage(ctx, run, &core.OrderCursor{Year: 2026, Page: 2}, syncPage("synthetic-terminal", 1, 3)); !errors.Is(err, store.ErrSyncRunInactive) {
		t.Fatal("finished run accepted another page")
	}
}

func TestSyncPageConcurrentWritersDoNotBothAdvanceSameCursor(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	a, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	run, err := a.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, db := range []*store.SQLite{a, b} {
		wg.Add(1)
		go func(db *store.SQLite) {
			defer wg.Done()
			<-start
			_, err := db.ApplySyncPage(ctx, run, nil, syncPage("synthetic-race", 1, 2))
			errs <- err
		}(db)
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		} else if !errors.Is(err, store.ErrSyncCheckpointChanged) {
			t.Fatal(err)
		}
	}
	status, err := a.LatestSyncStatus(ctx)
	if err != nil || successes != 1 || status.PagesProcessed != 1 || status.OrdersSeen != 1 {
		t.Fatal("concurrent page commit was counted twice")
	}
}
