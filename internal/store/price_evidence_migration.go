package store

import (
	"context"
	"fmt"
)

// Rebuild in one transaction to relax the old positive-only CHECK. No legacy
// source class is invented; all original rows and IDs survive the copy.
func (s *SQLite) migratePriceEvidence(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var done bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = 14)").Scan(&done); err != nil {
		return err
	}
	if done {
		return tx.Commit()
	}
	for _, statement := range []string{
		`CREATE TABLE product_price_observations_v2 (
		id INTEGER PRIMARY KEY AUTOINCREMENT, product_key TEXT NOT NULL, product_id TEXT NOT NULL,
		item_id TEXT NOT NULL DEFAULT '', vendor_item_id TEXT NOT NULL DEFAULT '', name TEXT NOT NULL,
		canonical_url TEXT NOT NULL DEFAULT '', current_amount INTEGER NOT NULL CHECK(current_amount >= 0),
		original_amount INTEGER NOT NULL DEFAULT 0 CHECK(original_amount >= 0),
		discount_rate INTEGER NOT NULL DEFAULT 0 CHECK(discount_rate BETWEEN 0 AND 100),
		currency TEXT NOT NULL CHECK(currency = 'KRW'),
		source TEXT NOT NULL CHECK(source IN ('coupang_product_search','coupang_product_inspection')),
		observed_at TEXT NOT NULL, provenance TEXT NOT NULL DEFAULT 'unknown_legacy',
		field_evidence_json TEXT NOT NULL DEFAULT '[]', UNIQUE(product_key,observed_at,source))`,
		`INSERT INTO product_price_observations_v2(id,product_key,product_id,item_id,vendor_item_id,name,canonical_url,current_amount,original_amount,discount_rate,currency,source,observed_at)
		SELECT id,product_key,product_id,item_id,vendor_item_id,name,canonical_url,current_amount,original_amount,discount_rate,currency,source,observed_at FROM product_price_observations`,
		`DROP TABLE product_price_observations`,
		`ALTER TABLE product_price_observations_v2 RENAME TO product_price_observations`,
		`CREATE INDEX product_price_observations_lookup_idx ON product_price_observations(product_id,vendor_item_id,observed_at DESC)`,
		`INSERT INTO schema_migrations(version,applied_at) VALUES(14,strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate price evidence: %w", err)
		}
	}
	return tx.Commit()
}
