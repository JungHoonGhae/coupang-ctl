package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func TestPriceObservationsPersistExactOptionSeriesAndBoundHistory(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	observations := []core.ProductPriceObservation{
		priceObservation("101", "201", 42000, base),
		priceObservation("101", "202", 51000, base.Add(time.Hour)),
		priceObservation("101", "201", 39000, base.Add(2*time.Hour)),
		priceObservation("103", "201", 38000, base.Add(3*time.Hour)),
	}
	if err := ledger.RecordPriceObservations(ctx, observations); err != nil {
		t.Fatal(err)
	}
	// The unique key makes an identical read idempotent.
	if err := ledger.RecordPriceObservations(ctx, observations[:1]); err != nil {
		t.Fatal(err)
	}

	exact, truncated, err := ledger.ListPriceObservations(ctx, core.ProductPriceHistoryRequest{ProductID: "101", VendorItemID: "201", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(exact) != 3 || exact[0].CurrentAmount != 42000 || exact[2].CurrentAmount != 38000 || exact[0].Provenance != "observed" {
		t.Fatalf("unexpected exact option history: %#v truncated=%v", exact, truncated)
	}

	bounded, truncated, err := ledger.ListPriceObservations(ctx, core.ProductPriceHistoryRequest{ProductID: "101", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(bounded) != 2 {
		t.Fatalf("history limit was not explicit: %#v truncated=%v", bounded, truncated)
	}
	deleted, err := ledger.PurgePriceObservations(ctx)
	if err != nil || deleted != 4 {
		t.Fatalf("unexpected price observation purge: deleted=%d err=%v", deleted, err)
	}
	empty, _, err := ledger.ListPriceObservations(ctx, core.ProductPriceHistoryRequest{ProductID: "101", Limit: 10})
	if err != nil || len(empty) != 0 {
		t.Fatalf("price observations remained after purge: %#v err=%v", empty, err)
	}
}

func TestPriceObservationRejectsNonCoupangCanonicalURL(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	observation := priceObservation("101", "201", 42000, time.Now().UTC())
	observation.CanonicalURL = "https://example.test/vp/products/101"
	if err := ledger.RecordPriceObservations(ctx, []core.ProductPriceObservation{observation}); err == nil {
		t.Fatal("non-Coupang canonical URL was stored")
	}
}

func priceObservation(productID, vendorItemID string, amount int64, observedAt time.Time) core.ProductPriceObservation {
	return core.ProductPriceObservation{
		Reference: core.ProductReference{ProductID: productID, VendorItemID: vendorItemID},
		Name:      "Synthetic product", CanonicalURL: "https://www.coupang.com/vp/products/" + productID,
		CurrentAmount: amount, Currency: "KRW", ObservedAt: observedAt,
		Source: "coupang_product_search", Provenance: "observed",
	}
}
