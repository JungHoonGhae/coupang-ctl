package orders_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

type heldSyncSource struct{ entered chan struct{} }

func (s *heldSyncSource) FetchPage(ctx context.Context, _ *core.OrderCursor) (core.OrderPage, error) {
	close(s.entered)
	<-ctx.Done()
	return core.OrderPage{}, ctx.Err()
}

func TestConcurrentSyncStopsBeforeReadingSourceAndCanResumeAfterCancel(t *testing.T) {
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
	ownerCtx, cancel := context.WithCancel(ctx)
	owner := &heldSyncSource{entered: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := orders.NewWithPageSource(a, owner).Sync(ownerCtx, core.SyncRequest{MaxPages: 1})
		done <- err
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("owner did not terminate")
		}
	}()
	select {
	case <-owner.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("owner did not start")
	}
	second := &fixturePageSource{page: core.OrderPage{Orders: []core.Order{}}}
	_, err = orders.NewWithPageSource(b, second).Sync(ctx, core.SyncRequest{MaxPages: 1, RestartScan: true})
	if !errors.Is(err, core.ErrSyncInProgress) || len(second.seen) != 0 {
		t.Fatalf("competing sync reached source: calls=%d error=%v", len(second.seen), err)
	}
	status, err := b.LatestSyncStatus(ctx)
	if err != nil || status.State != core.SyncRunRunning {
		t.Fatal("rejected sync replaced active attempt status")
	}
	// Local statistics remain readable while acquisition owns the writer lease.
	if _, err := b.Stats(ctx, core.OrderFilter{}); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("owner lost cancellation")
		}
		done <- err
	case <-time.After(3 * time.Second):
		t.Fatal("owner did not release")
	}
	if _, err := orders.NewWithPageSource(b, second).Sync(ctx, core.SyncRequest{MaxPages: 1}); err != nil || len(second.seen) != 1 {
		t.Fatal("canceled owner stranded the sync writer")
	}
}
