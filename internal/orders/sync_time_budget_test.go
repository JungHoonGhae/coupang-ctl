package orders

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

type timedPageSource func(context.Context, *core.OrderCursor) (core.OrderPage, error)

func (f timedPageSource) FetchPage(ctx context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	return f(ctx, cursor)
}

type timedDocumentSource func(context.Context, *core.OrderCursor) ([]byte, error)

func (f timedDocumentSource) Fetch(ctx context.Context, cursor *core.OrderCursor) ([]byte, error) {
	return f(ctx, cursor)
}

func budgetTestLedger(t *testing.T) *store.SQLite {
	t.Helper()
	ledger, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ledger.Close() })
	return ledger
}

func TestSyncPublicDefaultsBoundBothSourceKinds(t *testing.T) {
	if syncTimeBudget != 5*time.Minute || syncPageTimeout != time.Minute {
		t.Fatal("sync limits no longer match the accepted acquisition contract")
	}
	for _, typed := range []bool{false, true} {
		t.Run(map[bool]string{true: "typed", false: "document"}[typed], func(t *testing.T) {
			ledger := budgetTestLedger(t)
			stop := errors.New("synthetic stop")
			check := func(ctx context.Context) {
				t.Helper()
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > time.Minute {
					t.Fatal("source did not receive the bounded page deadline")
				}
			}
			service := New(ledger, timedDocumentSource(func(ctx context.Context, _ *core.OrderCursor) ([]byte, error) {
				check(ctx)
				return nil, stop
			}))
			if typed {
				service = NewWithPageSource(ledger, timedPageSource(func(ctx context.Context, _ *core.OrderCursor) (core.OrderPage, error) {
					check(ctx)
					return core.OrderPage{}, stop
				}))
			}
			_, err := service.Sync(context.Background(), core.SyncRequest{MaxPages: 1})
			if !errors.Is(err, stop) || !errors.Is(err, ErrDocumentSource) {
				t.Fatal("bounded acquisition lost the source error")
			}
		})
	}
}

func TestSyncTimeoutPreservesCommittedPageAndCanResume(t *testing.T) {
	for _, tc := range []struct {
		name        string
		total, page time.Duration
		want        error
		code        string
	}{
		{"attempt", 100 * time.Millisecond, time.Second, ErrSyncTimeBudget, "time_budget_exhausted"},
		{"page", time.Second, 100 * time.Millisecond, ErrSyncPageDeadline, "page_deadline_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := budgetTestLedger(t)
			calls := 0
			var firstDeadline time.Time
			service := NewWithPageSource(ledger, timedPageSource(func(ctx context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
				calls++
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("missing page deadline")
				}
				if calls == 1 {
					firstDeadline = deadline
					return core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-committed", PurchasedAt: "2026-08-01", Currency: "KRW"}}, Next: &core.OrderCursor{Year: 2026, Page: 2}}, nil
				}
				if cursor == nil || cursor.Page != 2 {
					t.Fatal("wrong resume cursor")
				}
				if tc.name == "attempt" && !deadline.Equal(firstDeadline) {
					t.Fatal("second page extended the whole attempt budget")
				}
				<-ctx.Done()
				// Even an adapter returning a seemingly valid terminal page after
				// cancellation must not commit it or claim cursor exhaustion.
				return core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-too-late", PurchasedAt: "2026-08-01", Currency: "KRW"}}}, nil
			}))
			result, err := service.syncWithTimeLimits(context.Background(), core.SyncRequest{MaxPages: 10}, tc.total, tc.page)
			if !errors.Is(err, tc.want) || !errors.Is(err, context.DeadlineExceeded) || calls != 2 || result.PagesProcessed != 1 || result.CursorExhausted || result.Complete {
				t.Fatalf("incorrect timeout outcome: calls=%d pages=%d err=%v", calls, result.PagesProcessed, err)
			}
			status, err := ledger.LatestSyncStatus(context.Background())
			if err != nil || status.State != core.SyncRunFailed || status.ErrorCode != tc.code || status.PagesProcessed != 1 || status.OrdersSeen != 1 {
				t.Fatalf("timeout bookkeeping lost committed progress: %+v %v", status, err)
			}
			rows, err := ledger.ListOrders(context.Background(), core.OrderFilter{Limit: 10})
			if err != nil || len(rows) != 1 || rows[0].SourceRef != "synthetic-committed" {
				t.Fatal("late page was committed")
			}
			resume := NewWithPageSource(ledger, timedPageSource(func(_ context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
				if cursor == nil || cursor.Page != 2 {
					t.Fatal("timeout lost the resumable checkpoint")
				}
				return core.OrderPage{Orders: []core.Order{}}, nil
			}))
			result, err = resume.Sync(context.Background(), core.SyncRequest{MaxPages: 1})
			if err != nil || result.PagesProcessed != 1 || !result.CursorExhausted || result.Complete {
				t.Fatal("failed to resume after timeout")
			}
		})
	}
}

func TestSyncCallerDeadlineRemainsAuthoritative(t *testing.T) {
	ledger := budgetTestLedger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	wantDeadline, _ := ctx.Deadline()
	service := New(ledger, timedDocumentSource(func(ctx context.Context, _ *core.OrderCursor) ([]byte, error) {
		deadline, _ := ctx.Deadline()
		if !deadline.Equal(wantDeadline) {
			t.Fatal("caller deadline was extended")
		}
		<-ctx.Done()
		return []byte("not parsed after deadline"), nil
	}))
	_, err := service.Sync(ctx, core.SyncRequest{MaxPages: 1})
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrSyncTimeBudget) || errors.Is(err, ErrSyncPageDeadline) {
		t.Fatalf("caller deadline was misclassified: %v", err)
	}
	status, err := ledger.LatestSyncStatus(context.Background())
	if err != nil || status.ErrorCode != "deadline_exceeded" || status.PagesProcessed != 0 {
		t.Fatal("caller deadline was not persisted correctly")
	}
}
