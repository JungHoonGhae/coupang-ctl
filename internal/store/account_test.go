package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestMembershipCostsUsesOnlyExplicitMembershipOnlyOrders(t *testing.T) {
	ctx := context.Background()
	ledger, err := Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	orders := []core.Order{
		{SourceRef: "synthetic-fee-paid", PurchasedAt: "2025-08-01", TotalAmount: 7_890, Currency: "KRW", Items: []core.OrderItem{{Name: "Synthetic membership", Quantity: 1, UnitPrice: 7_890, PaidPrice: 7_890, ProductType: "MEMBERSHIP"}}},
		{SourceRef: "synthetic-fee-cancelled", PurchasedAt: "2025-09-01", TotalAmount: 7_890, Currency: "KRW", FullyCanceled: true, Items: []core.OrderItem{{Name: "Synthetic membership", Quantity: 1, UnitPrice: 7_890, PaidPrice: 7_890, DivisionType: "SUBSCRIPTION"}}},
		{SourceRef: "synthetic-product", PurchasedAt: "2025-09-02", TotalAmount: 7_890, Currency: "KRW", Items: []core.OrderItem{{Name: "Synthetic product", Quantity: 1, UnitPrice: 7_890, PaidPrice: 7_890}}},
		{SourceRef: "synthetic-mixed", PurchasedAt: "2025-09-03", TotalAmount: 15_780, Currency: "KRW", Items: []core.OrderItem{{Name: "Synthetic membership", Quantity: 1, UnitPrice: 7_890, PaidPrice: 7_890, ProductType: "MEMBERSHIP"}, {Name: "Synthetic product", Quantity: 1, UnitPrice: 7_890, PaidPrice: 7_890}}},
	}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: orders}); err != nil {
		t.Fatal(err)
	}
	runID, err := ledger.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.FinishSync(ctx, runID, core.SyncResult{Complete: true, OrdersSeen: len(orders)}, ""); err != nil {
		t.Fatal(err)
	}

	got, err := ledger.MembershipCosts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "complete_available_history" || !got.CompleteHistorySync || got.ObservedPaymentCount != 2 || got.ObservedGrossAmountKRW != 15_780 {
		t.Fatalf("unexpected membership cost coverage: %#v", got)
	}
	if got.ObservedNonCanceledPaymentCount != 1 || got.ObservedPaidAmountKRW != 7_890 || got.FirstObservedPaymentDate != "2025-08-01" || got.LastObservedPaymentDate != "2025-09-01" {
		t.Fatalf("unexpected membership costs: %#v", got)
	}
	if _, err := time.Parse(time.RFC3339Nano, got.LastCompleteHistorySyncAt); err != nil {
		t.Fatalf("invalid complete sync time %q: %v", got.LastCompleteHistorySyncAt, err)
	}
}

func TestMembershipCostsKeepsImportedOrIncompleteHistoryPartial(t *testing.T) {
	ctx := context.Background()
	ledger, err := Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-imported-fee", PurchasedAt: "2026-08-01", TotalAmount: 7_890, Currency: "KRW",
		Items: []core.OrderItem{{Name: "Synthetic membership", Quantity: 1, UnitPrice: 7_890, PaidPrice: 7_890, ProductType: "MEMBERSHIP"}},
	}}}); err != nil {
		t.Fatal(err)
	}
	got, err := ledger.MembershipCosts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "partial_history" || got.CompleteHistorySync || got.ObservedPaidAmountKRW != 7_890 {
		t.Fatalf("imported evidence should remain partial: %#v", got)
	}
}
