package orders_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

type fixtureSource struct {
	documents        map[string][]byte
	failures         map[string]error
	seen             []string
	categoryDocument []byte
	categoryError    error
}

func (f *fixtureSource) Fetch(_ context.Context, cursor *core.OrderCursor) ([]byte, error) {
	key := cursorKey(cursor)
	f.seen = append(f.seen, key)
	if err := f.failures[key]; err != nil {
		return nil, err
	}
	document, ok := f.documents[key]
	if !ok {
		return nil, errors.New("fixture page missing")
	}
	return document, nil
}

func (f *fixtureSource) FetchProductCategory(_ context.Context, _ core.ProductReference) ([]byte, error) {
	if f.categoryError != nil {
		return nil, f.categoryError
	}
	if len(f.categoryDocument) == 0 {
		return nil, errors.New("synthetic category document missing")
	}
	return f.categoryDocument, nil
}

func TestCategoryEnrichmentRecordsUnavailableWithoutInventingACategory(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-unavailable-order", PurchasedAt: "2026-08-29", TotalAmount: 1000, Currency: "KRW",
		Items: []core.OrderItem{{ProductID: "101", VendorItemID: "201", Name: "Synthetic private item", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000}},
	}}}); err != nil {
		t.Fatal(err)
	}
	workflow := orders.New(ledger, &fixtureSource{categoryError: core.ErrProductCategoryUnavailable})
	result, err := workflow.EnrichCategories(ctx, core.CategoryEnrichmentRequest{MaxProducts: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProductsProcessed != 1 || result.CategoriesUnavailable != 1 || !result.Complete {
		t.Fatalf("unexpected unavailable result: %#v", result)
	}
	insight, err := workflow.Insights(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if insight.Categories.ClassifiedItemLines != 0 || len(insight.Categories.Buckets) != 0 {
		t.Fatalf("unavailable product was classified: %#v", insight.Categories)
	}
}

func TestCategoryEnrichmentCachesOnlySourceNativeBreadcrumbs(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-category-order", PurchasedAt: "2026-08-29", TotalAmount: 1000, Currency: "KRW",
		Items: []core.OrderItem{{ProductID: "101", VendorItemID: "201", Name: "Synthetic private item", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000}},
	}}}); err != nil {
		t.Fatal(err)
	}
	source := &fixtureSource{categoryDocument: []byte(`{"json_ld":[{"@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"Home","item":"https://www.coupang.com/"},{"@type":"ListItem","position":2,"name":"Synthetic broad","item":"https://www.coupang.com/np/categories/100"},{"@type":"ListItem","position":3,"name":"Synthetic specific","item":"https://www.coupang.com/np/categories/200"}]}]}`)}
	workflow := orders.New(ledger, source)
	result, err := workflow.EnrichCategories(ctx, core.CategoryEnrichmentRequest{MaxProducts: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProductsProcessed != 1 || result.CategoriesStored != 1 || !result.Complete {
		t.Fatalf("unexpected category enrichment: %#v", result)
	}
	insight, err := workflow.Insights(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(insight.Categories.Buckets) != 1 || insight.Categories.Buckets[0].Key != "Synthetic specific" {
		t.Fatalf("unexpected source-native category breakdown: %#v", insight.Categories)
	}
}

func TestSyncResumesAfterFailureWithoutDuplicates(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	source := &fixtureSource{
		documents: map[string][]byte{
			"initial": syntheticPage("source-a", "2026-08-29", 1000, true),
			"2026/2":  syntheticPage("source-b", "2026-08-28", 2000, false),
		},
		failures: map[string]error{"2026/2": errors.New("synthetic private upstream failure")},
	}
	workflow := orders.New(ledger, source)

	if _, err := workflow.Sync(ctx, core.SyncRequest{MaxPages: 10}); !errors.Is(err, orders.ErrDocumentSource) {
		t.Fatalf("first sync error = %v, want ErrDocumentSource", err)
	}
	delete(source.failures, "2026/2")
	result, err := workflow.Sync(ctx, core.SyncRequest{MaxPages: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.PagesProcessed != 1 || result.OrdersSeen != 1 {
		t.Fatalf("unexpected resumed sync result: %#v", result)
	}
	if len(source.seen) != 3 || source.seen[2] != "2026/2" {
		t.Fatalf("sync did not resume at checkpoint: %#v", source.seen)
	}
	stored, err := workflow.List(ctx, core.OrderFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored orders = %d, want 2: %#v", len(stored), stored)
	}
}

func TestSyncStopsAtPageBudgetAndContinuesLater(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	source := &fixtureSource{documents: map[string][]byte{
		"initial": syntheticPage("source-a", "2026-08-29", 1000, true),
		"2026/2":  syntheticPage("source-b", "2026-08-28", 2000, false),
	}}
	workflow := orders.New(ledger, source)

	first, err := workflow.Sync(ctx, core.SyncRequest{MaxPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.Complete || first.Next == nil || first.Next.Page != 2 {
		t.Fatalf("unexpected budgeted sync result: %#v", first)
	}
	second, err := workflow.Sync(ctx, core.SyncRequest{MaxPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Complete || second.PagesProcessed != 1 {
		t.Fatalf("unexpected continuation result: %#v", second)
	}
}

func TestCompleteFullHistorySyncRemovesStaleNormalizedOrders(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-stale", PurchasedAt: "2025-08-29", TotalAmount: 2000, Currency: "KRW",
	}}}); err != nil {
		t.Fatal(err)
	}
	source := &fixtureSource{documents: map[string][]byte{
		"initial": syntheticPage("source-current", "2026-08-29", 1000, false),
	}}
	workflow := orders.New(ledger, source)
	result, err := workflow.Sync(ctx, core.SyncRequest{MaxPages: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.OrdersRemoved != 1 {
		t.Fatalf("unexpected reconciliation result: %#v", result)
	}
	stored, err := workflow.List(ctx, core.OrderFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].PurchasedAt != "2026-08-29" {
		t.Fatalf("unexpected reconciled history: %#v", stored)
	}
}

func TestNormalizedExportCanBeImportedWithoutBrowserState(t *testing.T) {
	ctx := context.Background()
	sourceLedger, err := store.Open(ctx, filepath.Join(t.TempDir(), "source.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	sourceWorkflow := orders.New(sourceLedger, nil)
	if _, err := sourceLedger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-hash", PurchasedAt: "2026-08-29", TotalAmount: 3200,
		Currency: "KRW", Items: []core.OrderItem{},
	}}}); err != nil {
		t.Fatal(err)
	}
	exported, err := sourceWorkflow.Export(ctx, core.OrderFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	sourceLedger.Close()

	destinationLedger, err := store.Open(ctx, filepath.Join(t.TempDir(), "destination.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer destinationLedger.Close()
	destinationWorkflow := orders.New(destinationLedger, nil)
	for attempt := 0; attempt < 2; attempt++ {
		result, err := destinationWorkflow.Import(ctx, exported)
		if err != nil {
			t.Fatal(err)
		}
		if result.OrdersSeen != 1 {
			t.Fatalf("orders seen = %d, want 1", result.OrdersSeen)
		}
	}
	stored, err := destinationWorkflow.List(ctx, core.OrderFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].TotalAmount != 3200 {
		t.Fatalf("unexpected imported orders: %#v", stored)
	}
}

func cursorKey(cursor *core.OrderCursor) string {
	if cursor == nil {
		return "initial"
	}
	return fmt.Sprintf("%d/%d", cursor.Year, cursor.Page)
}

func syntheticPage(sourceRef, date string, total int64, hasNext bool) []byte {
	next := `"hasNext":false`
	if hasNext {
		next = `"hasNext":true,"nextYear":2026,"nextPageIndex":2`
	}
	return []byte(fmt.Sprintf(`{"props":{"pageProps":{"domains":{"desktopOrder":{"orderList":[{"orderId":%q,"orderDate":%q,"totalPrice":%d}],"orderPagination":{%s}}}}}}`, sourceRef, date, total, next))
}
