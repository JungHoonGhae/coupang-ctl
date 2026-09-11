package store_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func TestRecommendationPurchaseContextIdentityCoverageAndExclusions(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	item := func(vendor string, quantity int) core.OrderItem {
		return core.OrderItem{ProductID: "123", VendorItemID: vendor, Name: "Synthetic bowl", Quantity: quantity, PaidPrice: 1000, DeliveryStatus: "delivered"}
	}
	partial := item("789", 5)
	partial.CancelledQuantity = 1
	partial.ReturnedQuantity = 2
	cancelled := item("789", 20)
	cancelled.DeliveryStatus = "cancelled"
	returned := item("789", 20)
	returned.DeliveryStatus = "returned"
	zero := item("789", 1)
	zero.ReturnedQuantity = 1
	service := item("789", 20)
	service.CommerceKind = core.CommerceKindMembershipFee
	otherProduct := item("789", 20)
	otherProduct.ProductID = "999"
	page := core.OrderPage{Orders: []core.Order{
		{SourceRef: "synthetic-a", PurchasedAt: "2026-08-01", Currency: "KRW", Items: []core.OrderItem{item("789", 2), partial, item("888", 7), cancelled, returned, zero, service, otherProduct}},
		{SourceRef: "synthetic-b", PurchasedAt: "2026-09-01", Currency: "KRW", Items: []core.OrderItem{item("789", 1)}},
		{SourceRef: "synthetic-c", PurchasedAt: "2026-09-02", Currency: "KRW", FullyCanceled: true, Items: []core.OrderItem{item("789", 20)}},
	}}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		t.Fatal(err)
	}
	refs := []core.ProductReference{{ProductID: "123", VendorItemID: "789"}, {ProductID: "123"}, {ProductID: "123", VendorItemID: "000"}, {ProductID: "123", VendorItemID: "789"}}
	result, err := ledger.RecommendationPurchaseContext(ctx, refs)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || result.Sync.State != core.SyncRunNeverRun || result.Visibility != "private_local" || len(result.Matches) != 2 {
		t.Fatalf("unexpected coverage: %#v", result)
	}
	exact, product := result.Matches[0], result.Matches[1]
	if exact.IdentityScope != "product_and_vendor_item" || exact.OrderCount != 2 || exact.ItemLineCount != 3 || exact.RetainedUnits != 5 || exact.FirstPurchaseMonth != "2026-08" || exact.LatestPurchaseMonth != "2026-09" {
		t.Fatalf("exact aggregate: %#v", exact)
	}
	if product.IdentityScope != "product_only" || product.RetainedUnits != 12 || product.OrderCount != 2 || product.ItemLineCount != 4 {
		t.Fatalf("product aggregate: %#v", product)
	}
	run, err := ledger.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.FinishSync(ctx, run, core.SyncResult{Complete: true, OrdersSeen: 3, PagesProcessed: 1}, ""); err != nil {
		t.Fatal(err)
	}
	result, err = ledger.RecommendationPurchaseContext(ctx, refs[:1])
	if err != nil || result.Status != "partial" || result.Sync.HistoryComplete {
		t.Fatalf("completed coverage: %#v %v", result, err)
	}
	if _, err := ledger.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument); err != nil {
		t.Fatal(err)
	}
	result, err = ledger.RecommendationPurchaseContext(ctx, refs[:1])
	if err != nil || result.Status != "partial" || result.Sync.State != core.SyncRunRunning {
		t.Fatalf("running coverage: %#v %v", result, err)
	}
}

func TestRecommendationPurchaseContextEmptyAndInvalid(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	result, err := ledger.RecommendationPurchaseContext(ctx, []core.ProductReference{{ProductID: "123"}})
	if err != nil || result.Status != "partial" || len(result.Matches) != 0 {
		t.Fatalf("empty ledger: %#v %v", result, err)
	}
	for _, refs := range [][]core.ProductReference{{{}}, {{ProductID: "123", VendorItemID: "not-an-id"}}, make([]core.ProductReference, 6)} {
		if _, err := ledger.RecommendationPurchaseContext(ctx, refs); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}

func TestRecommendationPurchaseContextBoundsRequestsWithoutFixedShortlist(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	refs := make([]core.ProductReference, 201)
	for i := range refs {
		refs[i].ProductID = fmt.Sprint(1000 + i)
	}
	for _, size := range []int{0, 6, 20, 200} {
		result, err := ledger.RecommendationPurchaseContext(ctx, refs[:size])
		if err != nil || result.Status != "partial" || len(result.Matches) != 0 {
			t.Fatalf("valid bounded batch of %d references failed: %v", size, err)
		}
	}
	if _, err := ledger.RecommendationPurchaseContext(ctx, refs); err == nil {
		t.Fatal("unbounded purchase-context batch accepted")
	}
}
