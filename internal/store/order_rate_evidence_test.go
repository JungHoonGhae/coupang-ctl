package store_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func TestOrderStatsUndefinedRatesSerializeAsNullNotZero(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	assertRates := func(filter core.OrderFilter, want map[string]any) {
		t.Helper()
		stats, err := ledger.Stats(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		if stats.SchemaVersion != 3 {
			t.Fatalf("nullable rate contract schema_version = %d, want 3", stats.SchemaVersion)
		}
		encoded, err := json.Marshal(stats)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		for field, expected := range want {
			value, exists := got[field]
			if !exists || value != expected {
				t.Fatalf("%s = %v (present=%v), want %v", field, value, exists, expected)
			}
		}
	}
	none := map[string]any{"fully_canceled_order_rate": nil, "canceled_unit_rate": nil, "returned_item_line_rate": nil, "returned_unit_rate": nil}
	assertRates(core.OrderFilter{}, none)
	// One retained order can establish a zero fully-canceled-order fraction,
	// but no item rows cannot establish a unit- or item-based rate.
	order := core.Order{SourceRef: "synthetic", PurchasedAt: "2026-08-01", Currency: "KRW", TotalAmount: 100}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{order}}); err != nil {
		t.Fatal(err)
	}
	assertRates(core.OrderFilter{}, map[string]any{"fully_canceled_order_rate": float64(0), "canceled_unit_rate": nil, "returned_item_line_rate": nil, "returned_unit_rate": nil})
	order.Items = []core.OrderItem{{ProductID: "123", Name: "Synthetic", Quantity: 2, ReturnedQuantity: 1, CommerceKind: core.CommerceKindProductPurchase}}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{order}}); err != nil {
		t.Fatal(err)
	}
	assertRates(core.OrderFilter{}, map[string]any{"fully_canceled_order_rate": float64(0), "canceled_unit_rate": float64(0), "returned_item_line_rate": float64(1), "returned_unit_rate": 0.5})
	assertRates(core.OrderFilter{From: "2026-09-01"}, none)
}
