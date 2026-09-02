package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
	"github.com/JungHoonGhae/oss-coupangctl/internal/store"
)

func TestProductCategoryCacheProducesSourceNativeBreakdown(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	page := core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-order", PurchasedAt: "2026-08-29", TotalAmount: 3000, Currency: "KRW",
		Items: []core.OrderItem{
			{ProductID: "101", VendorItemID: "201", Name: "Synthetic private A", Quantity: 2, UnitPrice: 1000, PaidPrice: 2000},
			{ProductID: "102", VendorItemID: "202", Name: "Synthetic private B", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000},
		},
	}}}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		t.Fatal(err)
	}
	pending, err := ledger.PendingCategoryProducts(ctx, 10)
	if err != nil || len(pending) != 2 {
		t.Fatalf("unexpected pending categories: %#v, %v", pending, err)
	}
	if err := ledger.SaveProductCategory(ctx, pending[0], core.ProductCategory{
		Source: core.CategorySourceProductJSONLDBreadcrumb,
		Path: []core.ProductCategoryNode{
			{ID: "100", Name: "Synthetic broad", Position: 2},
			{ID: "200", Name: "Synthetic leaf", Position: 3},
		},
	}); err != nil {
		t.Fatal(err)
	}
	remaining, err := ledger.RemainingCategoryProducts(ctx)
	if err != nil || remaining != 1 {
		t.Fatalf("remaining = %d, err = %v", remaining, err)
	}
	breakdown, err := ledger.CategoryBreakdown(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if breakdown.Method != core.CategorySourceProductJSONLDBreadcrumb || breakdown.Grouping != "breadcrumb_leaf" || breakdown.TotalItemLines != 2 || breakdown.ClassifiedItemLines != 1 || breakdown.RetainedUnits != 3 || len(breakdown.Buckets) != 1 || breakdown.Buckets[0].CategoryID != "200" || breakdown.Buckets[0].Key != "Synthetic leaf" {
		t.Fatalf("unexpected category breakdown: %#v", breakdown)
	}
}
