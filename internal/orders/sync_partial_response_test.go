package orders_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangorders "github.com/JungHoonGhae/coupang-ctl/internal/coupang/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

type parsedPartialSource struct{ *fixtureSource }

func (p parsedPartialSource) FetchPage(ctx context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	document, err := p.Fetch(ctx, cursor)
	if err != nil {
		return core.OrderPage{}, err
	}
	return coupangorders.ParseOrderDocument(document)
}

func TestSyncPartialPagePreservesCheckpointUntilVerifiedResumption(t *testing.T) {
	for _, pageSource := range []bool{false, true} {
		t.Run(map[bool]string{false: "document", true: "typed_page"}[pageSource], func(t *testing.T) {
			ctx := context.Background()
			ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			source := &fixtureSource{documents: map[string][]byte{
				"initial": syntheticPage("synthetic-first", "2026-08-01", 100, true),
				"2026/2":  []byte(`{"orderList":[{"orderId":"synthetic-must-not-commit","orderDate":"2026-08-02","totalPrice":200}],"hasNext":false,"partial":true}`),
			}}
			service := orders.New(ledger, source)
			if pageSource {
				service = orders.NewWithPageSource(ledger, parsedPartialSource{source})
			}
			result, err := service.Sync(ctx, core.SyncRequest{MaxPages: 3})
			if !errors.Is(err, core.ErrPartialOrderData) || result.PagesProcessed != 1 || result.OrdersSeen != 1 || result.CursorExhausted || result.Complete {
				t.Fatalf("partial response committed or advanced: err=%v pages=%d", err, result.PagesProcessed)
			}
			status, err := ledger.LatestSyncStatus(ctx)
			if err != nil || status.State != core.SyncRunFailed || status.ErrorCode != "partial_order_data" || status.HistoryComplete {
				t.Fatal("partial response did not preserve its typed failure", err)
			}
			cursor, err := ledger.LoadSyncCursor(ctx)
			if err != nil || cursor == nil || cursor.Year != 2026 || cursor.Page != 2 {
				t.Fatal("partial response cleared the pending cursor", err)
			}
			stats, err := ledger.Stats(ctx, core.OrderFilter{})
			if err != nil || stats.OrderCount != 1 {
				t.Fatal("partial response contaminated retained history", err)
			}
			source.documents["2026/2"] = syntheticPage("synthetic-second", "2026-08-02", 200, false)
			resumed, err := service.Sync(ctx, core.SyncRequest{MaxPages: 3})
			if err != nil || resumed.PagesProcessed != 1 || !resumed.CursorExhausted || resumed.CoverageStatus != core.SyncCoverageUnverified || len(source.seen) != 3 || source.seen[2] != "2026/2" {
				t.Fatal("verified response did not resume the pending page", err)
			}
			stats, err = ledger.Stats(ctx, core.OrderFilter{})
			if err != nil || stats.OrderCount != 2 {
				t.Fatal("resumption lost a verified order or retained a partial one", err)
			}
		})
	}
}
