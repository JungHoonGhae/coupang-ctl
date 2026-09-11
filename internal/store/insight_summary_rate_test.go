package store_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func TestInsightSummaryRatesDistinguishMissingEvidenceFromZero(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	check := func(want map[string]any) {
		t.Helper()
		got, err := ledger.Insights(ctx, core.OrderFilter{})
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		for name, expected := range want {
			value, present := fields[name]
			if !present || value != expected {
				t.Fatalf("%s = %v (present=%v), want %v", name, value, present, expected)
			}
		}
	}
	check(map[string]any{"night_order_rate": nil, "late_evening_order_rate": nil, "weekend_order_rate": nil, "delivered_within_24_hours_rate": nil, "delivered_within_48_hours_rate": nil, "top_brand_share": nil})
	at := time.Date(2026, 8, 3, 3, 0, 0, 0, time.UTC) // Monday noon KST
	delivered := at.Add(72 * time.Hour)
	order := core.Order{SourceRef: "synthetic", PurchasedAt: "2026-08-03", PurchasedAtTime: &at, Currency: "KRW", Items: []core.OrderItem{{ProductID: "123", Name: "Synthetic", Quantity: 1, CommerceKind: core.CommerceKindProductPurchase, DeliveredAt: &delivered}}}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{order}}); err != nil {
		t.Fatal(err)
	}
	check(map[string]any{"night_order_rate": float64(0), "late_evening_order_rate": float64(0), "weekend_order_rate": float64(0), "delivered_within_24_hours_rate": float64(0), "delivered_within_48_hours_rate": float64(0), "top_brand_share": nil})
	order.Items[0].BrandName = "Synthetic brand"
	for _, tc := range []struct {
		hours                  int
		within24, within48     any
		events, fast24, fast48 int
	}{
		{24, float64(1), float64(1), 1, 1, 1},
		{48, float64(0), float64(1), 1, 0, 1},
		{-1, nil, nil, 0, 0, 0}, // unusable negative duration is not slow delivery
	} {
		delivered = at.Add(time.Duration(tc.hours) * time.Hour)
		if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{order}}); err != nil {
			t.Fatal(err)
		}
		check(map[string]any{"top_brand_share": float64(1), "delivered_within_24_hours_rate": tc.within24, "delivered_within_48_hours_rate": tc.within48})
		got, err := ledger.Insights(ctx, core.OrderFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if got.Samples.DeliveryEvents != tc.events || got.Samples.DeliveredWithin24Hours != tc.fast24 || got.Samples.DeliveredWithin48Hours != tc.fast48 || got.Samples.BrandedRetainedItemLines != 1 || got.TopBrand.Count != 1 || got.Samples.WeekendOrders != 0 || got.Samples.TimedOrders != 1 {
			t.Fatal("rate numerator/denominator evidence disagrees with synthetic observations")
		}
	}
}
