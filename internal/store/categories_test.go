package store_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
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

func TestCategoryCatalogFindsOnlyObservedLabelsWithExplicitCoverage(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-catalog-order", PurchasedAt: "2026-08-29", TotalAmount: 4000, Currency: "KRW",
		Items: []core.OrderItem{
			{ProductID: "101", VendorItemID: "201", Name: "Synthetic private product A", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000},
			{ProductID: "102", VendorItemID: "202", Name: "Synthetic private product B", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000},
			{ProductID: "103", VendorItemID: "203", Name: "Name must never become a category", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000},
			{ProductID: "104", VendorItemID: "204", Name: "Synthetic private product D", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000},
		},
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "201"}, core.ProductCategory{
		Source: core.CategorySourceProductJSONLDBreadcrumb,
		Path: []core.ProductCategoryNode{
			{ID: "100", Name: "Synthetic broad", Position: 2},
			{ID: "200", Name: "Synthetic first leaf", Position: 3},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "202"}, core.ProductCategory{
		Source: core.CategorySourceProductJSONLDBreadcrumb,
		Path: []core.ProductCategoryNode{
			{ID: "100", Name: "Synthetic broad", Position: 2},
			{ID: "300", Name: "Synthetic second leaf", Position: 3},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.SaveUnavailableProductCategory(ctx, core.ProductReference{VendorItemID: "203"}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "204"}, core.ProductCategory{
		Source: core.CategorySourceProductJSONLDBreadcrumb,
		Path:   []core.ProductCategoryNode{{ID: "100", Name: "Synthetic broad", Position: 2}},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := ledger.CategoryCatalog(ctx, core.CategoryCatalogRequest{Query: "synthetic broad", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != core.CategoryCatalogSchemaVersion || got.Visibility != "private_local" || got.Source != core.CategorySourceProductJSONLDBreadcrumb {
		t.Fatalf("unexpected catalog envelope: %#v", got)
	}
	if got.Coverage.EligibleProductCount != 4 || got.Coverage.ClassifiedProductCount != 3 || got.Coverage.UnclassifiedProductCount != 1 || got.Coverage.ClassifiedProductRate != 0.75 {
		t.Fatalf("unexpected catalog coverage: %#v", got.Coverage)
	}
	if got.TotalCategoryCount != 3 || got.MatchedCategoryCount != 1 || got.ReturnedCategoryCount != 1 || got.Truncated {
		t.Fatalf("unexpected catalog counts: %#v", got)
	}
	entry := got.Categories[0]
	if entry.CategoryID != "100" || entry.Name != "Synthetic broad" || entry.ObservedProductCount != 3 || entry.ObservedLeafProductCount != 1 || entry.ObservedAncestorProductCount != 2 || entry.Role != "sometimes_leaf" || entry.MatchKind != "exact_label" || len(entry.Path) != 1 {
		t.Fatalf("unexpected catalog match: %#v", entry)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "Name must never become a category") {
		t.Fatalf("catalog inferred a category from a product name: %s", encoded)
	}

	limited, err := ledger.CategoryCatalog(ctx, core.CategoryCatalogRequest{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if limited.MatchedCategoryCount != 3 || limited.ReturnedCategoryCount != 2 || !limited.Truncated || limited.Categories[0].CategoryID != "100" {
		t.Fatalf("unexpected limited catalog: %#v", limited)
	}
}

func TestOrderPurgeRemovesDerivedCategoryEvidence(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	page := core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-purge-order", PurchasedAt: "2026-08-29", TotalAmount: 1000, Currency: "KRW",
		Items: []core.OrderItem{{ProductID: "101", VendorItemID: "201", Name: "Synthetic private product", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000}},
	}}}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		t.Fatal(err)
	}
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "201"}, core.ProductCategory{
		Source: core.CategorySourceProductJSONLDBreadcrumb,
		Path:   []core.ProductCategoryNode{{ID: "200", Name: "Synthetic category", Position: 2}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		t.Fatal(err)
	}
	pending, err := ledger.PendingCategoryProducts(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].VendorItemID != "201" {
		t.Fatalf("purged category evidence was reused: %#v", pending)
	}
	report, err := ledger.CategoryStability(ctx)
	if err != nil || report.ObservationCount != 0 || report.Assessment != "unavailable_no_observed_breadcrumbs" {
		t.Fatalf("purged category observation remained visible: %#v, %v", report, err)
	}
}
