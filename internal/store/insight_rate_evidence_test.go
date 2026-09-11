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

func TestInsightTimeCohortRatesRequireTheirOwnDenominators(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	check := func(wantRates, wantSamples map[string]any) {
		t.Helper()
		insights, err := ledger.Insights(ctx, core.OrderFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if insights.SchemaVersion != 3 {
			t.Fatalf("nullable cohort contract schema_version = %d, want 3", insights.SchemaVersion)
		}
		encoded, err := json.Marshal(insights)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		for name, want := range wantRates {
			value, present := got[name]
			if !present || value != want {
				t.Fatalf("%s = %v (present=%v), want %v", name, value, present, want)
			}
		}
		samples := got["samples"].(map[string]any)
		for name, want := range wantSamples {
			value, present := samples[name]
			if !present || value != want {
				t.Fatalf("samples.%s = %v (present=%v), want %v", name, value, present, want)
			}
		}
	}
	check(map[string]any{"night_fully_canceled_order_rate": nil, "other_fully_canceled_order_rate": nil, "night_returned_unit_rate": nil, "other_returned_unit_rate": nil}, nil)
	// A date without a timestamp is not midnight and belongs to neither cohort.
	untimed := core.Order{SourceRef: "synthetic-untimed", PurchasedAt: "2026-08-01", Currency: "KRW", TotalAmount: 100,
		Items: []core.OrderItem{{ProductID: "789", Name: "Synthetic", Quantity: 2, ReturnedQuantity: 1, CommerceKind: core.CommerceKindProductPurchase}}}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{untimed}}); err != nil {
		t.Fatal(err)
	}
	check(map[string]any{"night_fully_canceled_order_rate": nil, "other_fully_canceled_order_rate": nil, "night_returned_unit_rate": nil, "other_returned_unit_rate": nil},
		map[string]any{"night_all_timed_orders": float64(0), "other_all_timed_orders": float64(0), "night_ordered_units": float64(0), "other_ordered_units": float64(0)})
	// A daytime purchase cannot establish a zero rate for an unobserved night cohort.
	day := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC) // noon KST
	order := core.Order{SourceRef: "synthetic-day", PurchasedAt: "2026-08-01", PurchasedAtTime: &day, Currency: "KRW", TotalAmount: 100,
		Items: []core.OrderItem{{ProductID: "123", Name: "Synthetic", Quantity: 3, ReturnedQuantity: 1, CommerceKind: core.CommerceKindProductPurchase}}}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{order}}); err != nil {
		t.Fatal(err)
	}
	check(map[string]any{"night_fully_canceled_order_rate": nil, "other_fully_canceled_order_rate": float64(0), "night_returned_unit_rate": nil, "other_returned_unit_rate": 0.333333},
		map[string]any{"night_all_timed_orders": float64(0), "other_all_timed_orders": float64(1), "other_fully_canceled_orders": float64(0), "other_ordered_units": float64(3), "other_returned_units": float64(1)})
	// Fully canceled night orders belong to the cancellation denominator even
	// though they are excluded from the retained-purchase time-of-day samples.
	night := time.Date(2026, 8, 1, 16, 0, 0, 0, time.UTC) // 01:00 KST next day
	order = core.Order{SourceRef: "synthetic-night", PurchasedAt: "2026-08-02", PurchasedAtTime: &night, Currency: "KRW", FullyCanceled: true, TotalAmount: 100,
		Items: []core.OrderItem{{ProductID: "456", Name: "Synthetic", Quantity: 2, CancelledQuantity: 2, CommerceKind: core.CommerceKindProductPurchase}}}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{order}}); err != nil {
		t.Fatal(err)
	}
	check(map[string]any{"night_fully_canceled_order_rate": float64(1), "other_fully_canceled_order_rate": float64(0), "night_returned_unit_rate": float64(0), "other_returned_unit_rate": 0.333333},
		map[string]any{"night_all_timed_orders": float64(1), "night_fully_canceled_orders": float64(1), "night_orders": float64(0), "night_ordered_units": float64(2), "night_returned_units": float64(0)})
}

func TestInsightCancellationCohortUsesKSTBoundary(t *testing.T) {
	for _, tc := range []struct {
		utc   string
		night bool
	}{
		{"2026-08-01T14:59:59Z", false}, // 23:59:59 KST
		{"2026-08-01T15:00:00Z", true},  // 00:00:00 KST
		{"2026-08-01T20:59:59Z", true},  // 05:59:59 KST
		{"2026-08-01T21:00:00Z", false}, // 06:00:00 KST
	} {
		t.Run(tc.utc, func(t *testing.T) {
			ctx := context.Background()
			ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			at, err := time.Parse(time.RFC3339, tc.utc)
			if err != nil {
				t.Fatal(err)
			}
			order := core.Order{SourceRef: "synthetic-boundary", PurchasedAt: at.Add(9 * time.Hour).Format(time.DateOnly), PurchasedAtTime: &at, FullyCanceled: true, Currency: "KRW"}
			if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{order}}); err != nil {
				t.Fatal(err)
			}
			got, err := ledger.Insights(ctx, core.OrderFilter{})
			if err != nil {
				t.Fatal(err)
			}
			observed, absent := got.OtherFullyCanceledOrderRate, got.NightFullyCanceledOrderRate
			observedCount, absentCount := got.Samples.OtherAllTimedOrders, got.Samples.NightAllTimedOrders
			if tc.night {
				observed, absent = got.NightFullyCanceledOrderRate, got.OtherFullyCanceledOrderRate
				observedCount, absentCount = got.Samples.NightAllTimedOrders, got.Samples.OtherAllTimedOrders
			}
			if observed == nil || *observed != 1 || absent != nil || observedCount != 1 || absentCount != 0 {
				t.Fatal("cancellation rate assigned to wrong KST cohort or empty cohort became zero")
			}
		})
	}
}
