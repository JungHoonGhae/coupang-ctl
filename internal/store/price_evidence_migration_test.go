package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestLegacyPriceMigrationPreservesRowsAndDoesNotInventProvenance(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the pre-evidence price table only in this disposable synthetic DB.
	for _, statement := range []string{
		`DROP TABLE product_price_observations`,
		`CREATE TABLE product_price_observations(id INTEGER PRIMARY KEY AUTOINCREMENT, product_key TEXT NOT NULL,product_id TEXT NOT NULL,item_id TEXT NOT NULL DEFAULT '',vendor_item_id TEXT NOT NULL DEFAULT '',name TEXT NOT NULL,canonical_url TEXT NOT NULL DEFAULT '',current_amount INTEGER NOT NULL CHECK(current_amount>0),original_amount INTEGER NOT NULL DEFAULT 0,discount_rate INTEGER NOT NULL DEFAULT 0,currency TEXT NOT NULL,source TEXT NOT NULL,observed_at TEXT NOT NULL,UNIQUE(product_key,observed_at,source))`,
		`INSERT INTO product_price_observations(id,product_key,product_id,name,current_amount,currency,source,observed_at) VALUES(42,'product:101','101','Synthetic legacy',1200,'KRW','coupang_product_search','2026-09-01T00:00:00Z')`,
		`DELETE FROM schema_migrations WHERE version=14`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	legacy, _, err := s.ListPriceObservations(ctx, core.ProductPriceHistoryRequest{ProductID: "101", Limit: 10})
	if err != nil || len(legacy) != 1 || legacy[0].CurrentAmount != 1200 || legacy[0].Name != "Synthetic legacy" || legacy[0].Provenance != "unknown_legacy" || len(legacy[0].FieldEvidence) != 0 {
		t.Fatalf("legacy migration mismatch: rows=%d err=%v", len(legacy), err)
	}
	var id int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM product_price_observations WHERE product_id='101'`).Scan(&id); err != nil || id != 42 {
		t.Fatal("legacy row identity changed")
	}
	if _, changed, err := s.AddPriceWatch(ctx, core.ProductWatchRequest{ProductID: "101"}, time.Now()); err != nil || changed {
		t.Fatal("legacy row established watch eligibility")
	}
	stamp := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	o := core.ProductPriceObservation{Reference: core.ProductReference{ProductID: "102"}, Name: "Synthetic derived zero", Currency: "KRW", ObservedAt: stamp, Source: "coupang_product_search", Provenance: "derived", FieldEvidence: []core.ProductFieldEvidence{{Field: "price.current_amount", Source: "dom", Locator: "dom.price", Method: "numeric_parse", Provenance: "derived", Scope: "product", Reference: core.ProductReference{ProductID: "102"}, CapturedAt: stamp}}}
	if err := s.RecordPriceObservations(ctx, []core.ProductPriceObservation{o}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, _, err := s.ListPriceObservations(ctx, core.ProductPriceHistoryRequest{ProductID: "102", Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].CurrentAmount != 0 || rows[0].Provenance != "derived" || !rows[0].HasPriceEvidence() {
		t.Fatalf("reopened zero/provenance mismatch: rows=%d err=%v", len(rows), err)
	}
	if _, changed, err := s.AddPriceWatch(ctx, core.ProductWatchRequest{ProductID: "102"}, stamp); err != nil || !changed {
		t.Fatal("verified zero could not establish watch")
	}
	bad := o
	bad.FieldEvidence = nil
	if err := s.RecordPriceObservations(ctx, []core.ProductPriceObservation{bad}); err == nil {
		t.Fatal("unproven price accepted")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM schema_migrations WHERE version=14`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM product_price_observations`, 2)
	for _, id := range []string{"101", "102"} {
		if _, err := s.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-" + id, PurchasedAt: "2026-08-01", TotalAmount: 1200, Currency: "KRW", Items: []core.OrderItem{{ProductID: id, Name: "Synthetic", Quantity: 1, PaidPrice: 1200, UnitPrice: 1200, DeliveryStatus: "delivered"}}}}}); err != nil {
			t.Fatal(err)
		}
	}
	candidates, err := s.ReorderCandidates(ctx, core.OrderFilter{Limit: 10})
	if err != nil || len(candidates) != 2 {
		t.Fatalf("synthetic reorder: candidates=%d err=%v", len(candidates), err)
	}
	for _, c := range candidates {
		if c.ProductID == "101" && c.PriceComparison.Status != "unavailable_no_local_price_observation" {
			t.Fatal("legacy price entered reorder comparison")
		}
		if c.ProductID == "102" && (c.PriceComparison.Status != "available" || c.PriceComparison.LatestObservedAmountKRW != 0 || c.PriceComparison.PriceProvenance != "derived" || c.PriceComparison.DifferencePercent != -100) {
			t.Fatal("derived zero was lost in reorder comparison")
		}
	}
}

func TestPriceMigrationFailureLeavesOriginalData(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, statement := range []string{`DELETE FROM schema_migrations WHERE version=14`, `CREATE TABLE product_price_observations_v2(marker TEXT)`, `INSERT INTO product_price_observations(product_key,product_id,name,current_amount,currency,source,observed_at) VALUES('product:101','101','Synthetic',1200,'KRW','coupang_product_search','2026-09-07T00:00:00Z')`} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.migratePriceEvidence(ctx); err == nil {
		t.Fatal("expected injected migration failure")
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM product_price_observations`, 1)
	assertCount(t, s.db, `SELECT COUNT(*) FROM schema_migrations WHERE version=14`, 0)
}
