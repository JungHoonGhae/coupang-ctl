package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func TestLedgerUpsertIsIdempotentAndSupportsConsumerQueries(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	deliveredAt := time.Date(2026, 8, 30, 5, 30, 0, 0, time.UTC)
	page := core.OrderPage{Orders: []core.Order{{
		SourceRef:      "synthetic-hash-a",
		PurchasedAt:    "2026-08-29",
		TotalAmount:    25900,
		DiscountAmount: 3100,
		Currency:       "KRW",
		Items: []core.OrderItem{{
			ProductID:      "101",
			VendorItemID:   "202",
			Name:           "Synthetic refill pack",
			Quantity:       2,
			UnitPrice:      14500,
			PaidPrice:      25900,
			SellerName:     "Synthetic seller",
			DeliveryStatus: "delivered",
			DeliveredAt:    &deliveredAt,
		}},
	}}}

	for attempt := 0; attempt < 2; attempt++ {
		result, err := ledger.UpsertOrderPage(ctx, page)
		if err != nil {
			t.Fatal(err)
		}
		if result.OrdersSeen != 1 {
			t.Fatalf("orders seen = %d, want 1", result.OrdersSeen)
		}
	}

	orders, err := ledger.ListOrders(ctx, core.OrderFilter{From: "2026-08-01", To: "2026-08-31", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || len(orders[0].Items) != 1 {
		t.Fatalf("idempotent list result = %#v", orders)
	}

	spend, err := ledger.Spend(ctx, core.OrderFilter{From: "2026-08-01", To: "2026-08-31"})
	if err != nil {
		t.Fatal(err)
	}
	if spend.OrderCount != 1 || spend.TotalAmount != 25900 || spend.DiscountAmount != 3100 {
		t.Fatalf("unexpected spend summary: %#v", spend)
	}
	if err := ledger.RecordPriceObservations(ctx, []core.ProductPriceObservation{
		priceObservation("101", "202", 11900, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
	}); err != nil {
		t.Fatal(err)
	}

	candidates, err := ledger.ReorderCandidates(ctx, core.OrderFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].PurchaseCount != 1 || candidates[0].TotalQuantity != 2 ||
		candidates[0].PriceComparison.Status != "available" || candidates[0].PriceComparison.LastPaidUnitAmountKRW != 12950 ||
		candidates[0].PriceComparison.LatestObservedAmountKRW != 11900 || candidates[0].PriceComparison.DifferenceKRW != -1050 ||
		candidates[0].PriceComparison.Direction != "lower" {
		t.Fatalf("unexpected reorder candidates: %#v", candidates)
	}

	exported, err := ledger.Export(ctx, core.OrderFilter{Limit: 20}, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if exported.SchemaVersion != 1 || len(exported.Orders) != 1 {
		t.Fatalf("unexpected export: %#v", exported)
	}

	purged, err := ledger.Purge(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if purged.OrdersDeleted != 1 || purged.ItemsDeleted != 1 {
		t.Fatalf("unexpected purge result: %#v", purged)
	}
	orders, err = ledger.ListOrders(ctx, core.OrderFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 0 {
		t.Fatalf("orders remain after purge: %#v", orders)
	}
}

func TestLedgerRejectsInvalidFilters(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if _, err := ledger.ListOrders(ctx, core.OrderFilter{From: "not-a-date"}); err == nil {
		t.Fatal("expected invalid date filter to be rejected")
	}
	if _, err := ledger.ListOrders(ctx, core.OrderFilter{Limit: 1001}); err == nil {
		t.Fatal("expected excessive limit to be rejected")
	}
}

func TestLedgerPersistsReceiptCancellationAndAdjustedSpendMetadata(t *testing.T) {
	ctx := context.Background()
	purchasedAt := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	canceledPurchasedAt := time.Date(2026, 8, 29, 16, 0, 0, 0, time.UTC)
	deliveredAt := purchasedAt.Add(48 * time.Hour)
	path := filepath.Join(t.TempDir(), "coupangctl.sqlite3")
	ledger, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	page := core.OrderPage{Orders: []core.Order{
		{
			SourceRef:        "synthetic-active",
			PurchasedAt:      "2026-08-29",
			PurchasedAtTime:  &purchasedAt,
			TotalAmount:      10000,
			ShippingFee:      3000,
			Currency:         "KRW",
			ReceiptAvailable: true,
			Items: []core.OrderItem{{
				Name: "Synthetic active item", Quantity: 2, CancelledQuantity: 1,
				UnitPrice: 5000, PaidPrice: 10000, BrandName: "Synthetic brand",
				DeliveryStatus: "delivered", DeliveredAt: &deliveredAt,
			}},
		},
		{
			SourceRef:       "synthetic-cancelled",
			PurchasedAt:     "2026-08-30",
			PurchasedAtTime: &canceledPurchasedAt,
			TotalAmount:     5000,
			ShippingFee:     3000,
			Currency:        "KRW",
			FullyCanceled:   true,
			Items: []core.OrderItem{{
				Name: "Synthetic cancelled item", Quantity: 1, CancelledQuantity: 1,
				UnitPrice: 5000, PaidPrice: 5000, DeliveryStatus: "cancelled",
			}},
		},
	}}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
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

	orders, err := ledger.ListOrders(ctx, core.OrderFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 || !orders[0].FullyCanceled || !orders[1].ReceiptAvailable || orders[1].PurchasedAtTime == nil || orders[1].Items[0].CancelledQuantity != 1 || orders[1].Items[0].BrandName != "Synthetic brand" {
		t.Fatalf("metadata did not round trip: %#v", orders)
	}
	spend, err := ledger.Spend(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if spend.OrderCount != 2 || spend.TotalAmount != 15000 || spend.FullyCanceledOrderCount != 1 || spend.FullyCanceledAmount != 5000 || spend.NonCanceledTotalAmount != 10000 || spend.NonCanceledShippingFee != 3000 {
		t.Fatalf("unexpected cancellation-aware spend: %#v", spend)
	}
	stats, err := ledger.Stats(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.OrderCount != 2 || stats.ItemLineCount != 2 || stats.OrderedUnits != 3 || stats.CanceledItemLineCount != 2 || stats.CanceledUnits != 2 || stats.ReturnedUnits != 0 || stats.CanceledUnitRate != 0.666667 {
		t.Fatalf("unexpected order stats: %#v", stats)
	}
	if len(stats.PurchaseHours) != 1 || stats.PurchaseHours[0].Key != "10" || stats.PurchaseHours[0].Count != 1 {
		t.Fatalf("unexpected purchase hours: %#v", stats.PurchaseHours)
	}
	if stats.DeliveryDuration.ShipmentCount != 1 || stats.DeliveryDuration.AverageHours != 48 {
		t.Fatalf("unexpected delivery duration: %#v", stats.DeliveryDuration)
	}
	if len(stats.TopBrands) != 1 || stats.TopBrands[0].Key != "Synthetic brand" {
		t.Fatalf("unexpected brand stats: %#v", stats.TopBrands)
	}
	insights, err := ledger.Insights(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if insights.FirstOrderDate != "2026-08-29" || insights.LastOrderDate != "2026-08-29" || insights.DistinctOrderDays != 1 || insights.Samples.TimedOrders != 1 || insights.Samples.NightWindowOrders != 0 {
		t.Fatalf("fully canceled order leaked into retained shopping behavior: %#v", insights)
	}
}

func TestLedgerReconcilesOnlyOrdersMissingFromACompleteHistory(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	page := core.OrderPage{Orders: []core.Order{
		{SourceRef: "synthetic-current", PurchasedAt: "2026-08-29", TotalAmount: 1000, Currency: "KRW"},
		{SourceRef: "synthetic-stale", PurchasedAt: "2025-08-29", TotalAmount: 2000, Currency: "KRW"},
	}}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		t.Fatal(err)
	}
	removed, err := ledger.ReconcileOrders(ctx, []string{"synthetic-current"})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	orders, err := ledger.ListOrders(ctx, core.OrderFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].SourceRef != "synthetic-current" {
		t.Fatalf("unexpected reconciled orders: %#v", orders)
	}
}

func TestDeliveryTrendComparesEarliestAndLatestPurchaseYears(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	earlyPurchase := time.Date(2021, 8, 5, 1, 0, 0, 0, time.UTC)
	earlyDelivery := earlyPurchase.Add(24 * time.Hour)
	latePurchase := time.Date(2026, 8, 5, 1, 0, 0, 0, time.UTC)
	lateDelivery := latePurchase.Add(48 * time.Hour)
	page := core.OrderPage{Orders: []core.Order{
		{SourceRef: "synthetic-early", PurchasedAt: "2021-08-05", PurchasedAtTime: &earlyPurchase, TotalAmount: 1000, Currency: "KRW", Items: []core.OrderItem{{VendorItemID: "synthetic-product", Name: "Synthetic early", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000, DeliveredAt: &earlyDelivery}}},
		{SourceRef: "synthetic-late", PurchasedAt: "2026-08-05", PurchasedAtTime: &latePurchase, TotalAmount: 1000, Currency: "KRW", Items: []core.OrderItem{{VendorItemID: "synthetic-product", Name: "Synthetic late", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000, DeliveredAt: &lateDelivery}}},
	}}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		t.Fatal(err)
	}
	stats, err := ledger.Stats(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.DeliveryTrend.BaselinePeriod != "2021" || stats.DeliveryTrend.LatestPeriod != "2026" || stats.DeliveryTrend.AverageHoursDelta != 24 || stats.DeliveryTrend.AverageHoursPercentChange != 1 || stats.DeliveryTrend.Direction != "slower" {
		t.Fatalf("unexpected delivery trend: %#v", stats.DeliveryTrend)
	}
	insights, err := ledger.Insights(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if insights.OrderCount != 2 || insights.DistinctOrderDays != 2 || insights.ActiveMonthCount != 2 || insights.LongestActiveMonthStreak != 1 || insights.DeliveredWithin24HoursRate != 0.5 || insights.DeliveredWithin48HoursRate != 1 || len(insights.DeliveryByYear) != 2 || insights.DeliveryTrend.Direction != "slower" {
		t.Fatalf("unexpected shopping insights: %#v", insights)
	}
	if insights.SchemaVersion != 1 || insights.RepeatPurchases.IdentifiedProductCount != 1 || insights.RepeatPurchases.RepeatProductCount != 1 || insights.RepeatPurchases.RepeatProductPurchaseOccasionRate != 1 || insights.RepeatPurchases.RepeatChoiceCount != 1 || insights.RepeatPurchases.RepeatChoiceRate != 0.5 || insights.RepeatPurchases.ProductIDCoverage != 1 {
		t.Fatalf("unexpected repeat-purchase insights: %#v", insights.RepeatPurchases)
	}
	if insights.Basket.RetainedOrderCount != 2 || insights.Basket.SingleItemOrderCount != 2 || insights.Basket.SingleItemOrderRate != 1 || insights.Basket.AverageItemLines != 1 || insights.Basket.RetainedItemAmount != 2000 || insights.Basket.SingleItemSpendRate != 1 || insights.Basket.MedianSingleItemOrderValue != 1000 || insights.Basket.MedianMultiItemOrderValue != 0 || insights.Basket.CompositionOrderCount != 2 || insights.Basket.SingleProductOrderCount != 2 || insights.Basket.SingleProductOrderRate != 1 || insights.Basket.AverageDistinctProducts != 1 {
		t.Fatalf("unexpected basket insights: %#v", insights.Basket)
	}
	if insights.PurchaseTiming.PurchaseDays != 2 || insights.PurchaseTiming.ObservationDays != 1827 || len(insights.PurchaseHours) != 1 || insights.PurchaseHours[0].Key != "10" || len(insights.PurchaseMonths) != 2 {
		t.Fatalf("unexpected purchase timing series: timing=%#v hours=%#v months=%#v", insights.PurchaseTiming, insights.PurchaseHours, insights.PurchaseMonths)
	}
}

func TestShoppingInsightsDoNotCountMembershipOnlyMonthsAsProductPurchaseActivity(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	membershipPurchasedAt := time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)
	membershipDeliveredAt := membershipPurchasedAt.Add(time.Hour)
	productPurchasedAt := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	productDeliveredAt := productPurchasedAt.Add(12 * time.Hour)
	page := core.OrderPage{Orders: []core.Order{
		{
			SourceRef: "synthetic-membership-charge", PurchasedAt: "2026-07-01", PurchasedAtTime: &membershipPurchasedAt, TotalAmount: 4990, Currency: "KRW",
			Items: []core.OrderItem{{Name: "Synthetic membership charge", Quantity: 1, UnitPrice: 4990, PaidPrice: 4990, ProductType: "MEMBERSHIP", BrandName: "Synthetic membership brand", DeliveredAt: &membershipDeliveredAt}},
		},
		{
			SourceRef: "synthetic-product-order", PurchasedAt: "2026-08-01", PurchasedAtTime: &productPurchasedAt, TotalAmount: 12000, Currency: "KRW",
			Items: []core.OrderItem{{VendorItemID: "synthetic-product", Name: "Synthetic product", Quantity: 1, UnitPrice: 12000, PaidPrice: 12000, ProductType: "PRODUCT", BrandName: "Synthetic product brand", DeliveredAt: &productDeliveredAt}},
		},
	}}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		t.Fatal(err)
	}

	got, err := ledger.Insights(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got.OrderCount != 1 || got.ActiveMonthCount != 1 || got.LongestActiveMonthStreak != 1 || got.Samples.TimedOrders != 1 || got.Samples.NightOrders != 0 || got.Samples.DeliveryEvents != 1 {
		t.Fatalf("product purchase activity includes a membership-only order: %#v", got)
	}
	if got.Basket.RetainedOrderCount != 1 || got.Basket.RetainedItemLineCount != 1 || got.RepeatPurchases.RetainedItemLineCount != 1 || got.TopBrand.Key != "Synthetic product brand" {
		t.Fatalf("product behavior aggregates include membership data: basket=%#v repeat=%#v top_brand=%#v", got.Basket, got.RepeatPurchases, got.TopBrand)
	}

	stats, err := ledger.Stats(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.OrderCount != 1 || stats.ItemLineCount != 1 || len(stats.PurchaseHours) != 1 || stats.PurchaseHours[0].Key != "10" {
		t.Fatalf("product stats include membership data: %#v", stats)
	}

	spend, err := ledger.Spend(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if spend.TotalAmount != 16990 || spend.Commerce.ProductPurchases.NonCanceledTotalAmount != 12000 || spend.Commerce.MembershipFees.NonCanceledTotalAmount != 4990 || spend.Commerce.Unclassified.OrderCount != 0 {
		t.Fatalf("commerce spend was not separated: %#v", spend)
	}

	products, err := ledger.ProductInsights(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if products.FirstPurchaseDate != "2026-08-01" || products.LastPurchaseDate != "2026-08-01" || products.TotalSpendAmount != 12000 || products.RetainedItemLineCount != 1 || products.HighestSpendDay.TotalAmount != 12000 {
		t.Fatalf("private product insights include membership data: %#v", products)
	}
}

func TestProductInsightsExposeIdentifiedProductLeadersAndSpendDayReceipts(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	page := core.OrderPage{Orders: []core.Order{
		{
			SourceRef: "synthetic-low-day-basket", PurchasedAt: "2026-08-01", TotalAmount: 2000, Currency: "KRW",
			Items: []core.OrderItem{
				{VendorItemID: "product-a", Name: "Synthetic oats", Quantity: 2, UnitPrice: 1000, PaidPrice: 1800, DeliveryStatus: "delivered"},
				{Name: "Synthetic unknown sample", Quantity: 1, UnitPrice: 200, PaidPrice: 200, DeliveryStatus: "delivered"},
			},
		},
		{
			SourceRef: "synthetic-high-day-basket", PurchasedAt: "2026-08-14", TotalAmount: 6200, Currency: "KRW",
			Items: []core.OrderItem{
				{VendorItemID: "product-a", Name: "Synthetic oats refreshed", Quantity: 1, UnitPrice: 1200, PaidPrice: 1200, DeliveryStatus: "delivered"},
				{ProductID: "product-b", Name: "Synthetic keyboard", Quantity: 1, UnitPrice: 5000, PaidPrice: 5000, DeliveryStatus: "delivered"},
			},
		},
		{
			SourceRef: "synthetic-minimum-day", PurchasedAt: "2026-08-20", TotalAmount: 200, Currency: "KRW",
			Items: []core.OrderItem{
				{VendorItemID: "product-c", Name: "Synthetic pencil", Quantity: 1, UnitPrice: 200, PaidPrice: 200, DeliveryStatus: "delivered"},
			},
		},
		{
			SourceRef: "synthetic-cancelled-day", PurchasedAt: "2026-08-21", TotalAmount: 9000, Currency: "KRW", FullyCanceled: true,
			Items: []core.OrderItem{
				{VendorItemID: "product-cancelled", Name: "Synthetic cancelled", Quantity: 1, UnitPrice: 9000, PaidPrice: 9000, DeliveryStatus: "cancelled"},
			},
		},
	}}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		t.Fatal(err)
	}

	got, err := ledger.ProductInsights(ctx, core.OrderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != 1 || got.Visibility != "private_local" || got.Currency != "KRW" {
		t.Fatalf("unexpected product insight envelope: %#v", got)
	}
	if got.FirstPurchaseDate != "2026-08-01" || got.LastPurchaseDate != "2026-08-20" || got.CalendarMonthCount != 1 || got.ActiveMonthCount != 1 || got.TotalSpendAmount != 8400 || got.AverageMonthlySpendAmount != 8400 {
		t.Fatalf("unexpected spending window: %#v", got)
	}
	if got.RetainedItemLineCount != 5 || got.IdentifiedItemLineCount != 4 || got.IdentifiedProductCount != 3 || got.RetainedUnitCount != 6 || got.ProductIDCoverage != 0.8 || got.SpendEligibleItemLineCount != 4 || got.SpendEligibleItemLineRate != 0.8 {
		t.Fatalf("unexpected product coverage: %#v", got)
	}
	if got.TopByUnits.Name != "Synthetic oats refreshed" || got.TopByUnits.UnitCount != 3 || got.TopByUnits.PurchaseCount != 2 || got.TopByUnits.TotalPaidAmount != 3000 {
		t.Fatalf("unexpected unit leader: %#v", got.TopByUnits)
	}
	if got.TopByOrders.Name != "Synthetic oats refreshed" || got.TopByOrders.PurchaseCount != 2 {
		t.Fatalf("unexpected order leader: %#v", got.TopByOrders)
	}
	if got.TopBySpend.Name != "Synthetic keyboard" || got.TopBySpend.TotalPaidAmount != 5000 {
		t.Fatalf("unexpected spend leader: %#v", got.TopBySpend)
	}
	if got.HighestPaidUnit.Name != "Synthetic keyboard" || got.HighestPaidUnit.PaidUnitAmount != 5000 || got.LowestPaidUnit.Name != "Synthetic pencil" || got.LowestPaidUnit.PaidUnitAmount != 200 {
		t.Fatalf("unexpected paid-unit extremes: high=%#v low=%#v", got.HighestPaidUnit, got.LowestPaidUnit)
	}
	if got.HighestSpendDay.Date != "2026-08-14" || got.HighestSpendDay.TotalAmount != 6200 || got.HighestSpendDay.OrderCount != 1 || len(got.HighestSpendDay.Products) != 2 || got.HighestSpendDay.Products[0].Name != "Synthetic keyboard" {
		t.Fatalf("unexpected highest spend day: %#v", got.HighestSpendDay)
	}
	if got.LowestSpendDay.Date != "2026-08-20" || got.LowestSpendDay.TotalAmount != 200 || len(got.LowestSpendDay.Products) != 1 || got.LowestSpendDay.Products[0].Name != "Synthetic pencil" {
		t.Fatalf("unexpected lowest spend day: %#v", got.LowestSpendDay)
	}
}
