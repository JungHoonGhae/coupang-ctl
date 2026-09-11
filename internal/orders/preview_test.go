package orders_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
)

type previewSource struct {
	page   core.OrderPage
	err    error
	calls  int
	cancel context.CancelFunc
}

func (s *previewSource) FetchPage(ctx context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	s.calls++
	if cursor != nil {
		return core.OrderPage{}, errors.New("preview resumed a stored cursor")
	}
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 60*time.Second {
		return core.OrderPage{}, errors.New("unbounded read")
	}
	if s.cancel != nil {
		s.cancel()
	}
	return s.page, s.err
}

func previewPage() core.OrderPage {
	return core.OrderPage{Orders: []core.Order{{SourceRef: core.OrderSourceReference("synthetic-preview"), PurchasedAt: "2026-09-01", Currency: "KRW", TotalAmount: 1000, Items: []core.OrderItem{{ProductID: "101", Name: "Synthetic product", Quantity: 1, PaidPrice: 1000, CommerceKind: core.CommerceKindProductPurchase}}}}, Next: &core.OrderCursor{Year: 2026, Page: 1}}
}

func TestPreviewReadsOnePageWithoutAnyLedger(t *testing.T) {
	source := &previewSource{page: previewPage()}
	// A nil ledger makes accidental persistence, cursor reads or local fallback fail.
	service := orders.NewWithPageSourceAndSyncSource(nil, source, core.SyncSourceCamofox)
	result, err := service.Preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if source.calls != 1 || result.OrderCount != 1 || !result.HasNextPage || result.HistoryComplete || result.Persisted || result.AccountIdentityVerified {
		t.Fatal("preview promoted one page to account history or persisted it")
	}
	if result.Visibility != "private_local" || result.Scope != "source_entry_page" || result.CapturedAt.IsZero() || result.Source != core.SyncSourceCamofox || len(result.Limitations) == 0 {
		t.Fatal("missing preview evidence")
	}
	if result.Orders[0].Items[0].Name != "Synthetic product" {
		t.Fatal("preview lost normalized evidence")
	}
	source.page = core.OrderPage{Orders: []core.Order{}}
	result, err = service.Preview(context.Background())
	if err != nil || result.OrderCount != 0 || result.Orders == nil || result.HistoryComplete || result.HasNextPage {
		t.Fatal("empty page became whole-account zero history")
	}
}

func TestPreviewRejectsMissingMalformedAndPartialEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*previewSource)
	}{
		{"missing_orders", func(s *previewSource) { s.page.Orders = nil }},
		{"raw_reference", func(s *previewSource) { s.page.Orders[0].SourceRef = "synthetic-private-raw-id" }},
		{"duplicate", func(s *previewSource) { s.page.Orders = append(s.page.Orders, s.page.Orders[0]) }},
		{"negative_amount", func(s *previewSource) { s.page.Orders[0].TotalAmount = -1 }},
		{"bad_cursor", func(s *previewSource) { s.page.Next.Year = 0 }},
		{"too_many_orders", func(s *previewSource) {
			for len(s.page.Orders) < 6 {
				s.page.Orders = append(s.page.Orders, s.page.Orders[0])
			}
		}},
		{"partial", func(s *previewSource) { s.err = core.ErrPartialOrderData }},
		{"authentication", func(s *previewSource) { s.err = core.ErrAuthenticationRequired }},
		{"blocked", func(s *previewSource) { s.err = core.ErrBrowserAccessDenied }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &previewSource{page: previewPage()}
			tc.change(source)
			result, err := orders.NewWithPageSourceAndSyncSource(nil, source, core.SyncSourceCamofox).Preview(context.Background())
			if err == nil || result.Orders != nil || !result.CapturedAt.IsZero() || source.calls != 1 {
				t.Fatal("invalid preview exposed a success/partial result")
			}
			if source.err != nil && !errors.Is(err, source.err) {
				t.Fatal("source failure category lost")
			}
		})
	}
}

func TestPreviewHonorsCancellationBeforeAndAfterSource(t *testing.T) {
	for _, before := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		source := &previewSource{page: previewPage()}
		if before {
			cancel()
		} else {
			source.cancel = cancel
		}
		result, err := orders.NewWithPageSourceAndSyncSource(nil, source, core.SyncSourceCamofox).Preview(ctx)
		cancel()
		if !errors.Is(err, context.Canceled) || result.Orders != nil {
			t.Fatal("cancelled preview returned evidence")
		}
		if before && source.calls != 0 || !before && source.calls != 1 {
			t.Fatal("cancellation did not bound the read")
		}
	}
}
