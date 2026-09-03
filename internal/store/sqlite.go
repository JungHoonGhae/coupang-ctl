package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite"
)

type SQLite struct {
	db  *sql.DB
	now func() time.Time
}

func Open(ctx context.Context, path string) (*SQLite, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil && runtime.GOOS != "windows" {
		return nil, fmt.Errorf("secure state directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create sqlite state file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close sqlite state file: %w", err)
	}
	if err := secureSQLiteFile(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &SQLite{db: db, now: time.Now}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	for _, sidecar := range []string{path + "-wal", path + "-shm", path + "-journal"} {
		if err := secureSQLiteFile(sidecar); err != nil {
			db.Close()
			return nil, err
		}
	}
	return store, nil
}

func secureSQLiteFile(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := os.Chmod(path, 0o600); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("secure sqlite state file: %w", err)
	}
	return nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}
	return nil
}

func (s *SQLite) migrate(ctx context.Context) error {
	statements := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sync_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			started_at TEXT NOT NULL,
			completed_at TEXT,
			status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
			pages_processed INTEGER NOT NULL DEFAULT 0,
			records_upserted INTEGER NOT NULL DEFAULT 0,
			error_code TEXT
		)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		 VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
		`CREATE TABLE IF NOT EXISTS orders (
			source_ref TEXT PRIMARY KEY,
			purchased_at TEXT NOT NULL,
			total_amount INTEGER NOT NULL CHECK (total_amount >= 0),
			discount_amount INTEGER NOT NULL CHECK (discount_amount >= 0),
			shipping_fee INTEGER NOT NULL CHECK (shipping_fee >= 0),
			currency TEXT NOT NULL CHECK (currency = 'KRW'),
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS order_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_ref TEXT NOT NULL REFERENCES orders(source_ref) ON DELETE CASCADE,
			position INTEGER NOT NULL,
			product_id TEXT,
			vendor_item_id TEXT,
			name TEXT NOT NULL,
			quantity INTEGER NOT NULL CHECK (quantity > 0),
			unit_price INTEGER NOT NULL CHECK (unit_price >= 0),
			paid_price INTEGER NOT NULL CHECK (paid_price >= 0),
			seller_name TEXT,
			delivery_status TEXT,
			delivered_at TEXT,
			UNIQUE(order_ref, position)
		)`,
		`CREATE INDEX IF NOT EXISTS orders_purchased_at_idx ON orders(purchased_at DESC)`,
		`CREATE INDEX IF NOT EXISTS order_items_product_idx ON order_items(vendor_item_id, product_id)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		 VALUES (2, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
		`CREATE TABLE IF NOT EXISTS sync_checkpoint (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			next_year INTEGER NOT NULL,
			next_page INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		 VALUES (3, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}
	columns := []struct {
		table      string
		name       string
		definition string
	}{
		{"orders", "fully_canceled", "INTEGER NOT NULL DEFAULT 0 CHECK (fully_canceled IN (0, 1))"},
		{"orders", "receipt_available", "INTEGER NOT NULL DEFAULT 0 CHECK (receipt_available IN (0, 1))"},
		{"order_items", "cancelled_quantity", "INTEGER NOT NULL DEFAULT 0 CHECK (cancelled_quantity >= 0)"},
		{"order_items", "returned_quantity", "INTEGER NOT NULL DEFAULT 0 CHECK (returned_quantity >= 0)"},
		{"orders", "purchased_at_time", "TEXT"},
		{"order_items", "brand_name", "TEXT"},
		{"order_items", "product_type", "TEXT"},
		{"order_items", "division_type", "TEXT"},
		{"order_items", "commerce_kind", "TEXT NOT NULL DEFAULT 'product_purchase'"},
		{"sync_runs", "history_complete", "INTEGER NOT NULL DEFAULT 0 CHECK (history_complete IN (0, 1))"},
	}
	for _, column := range columns {
		if err := s.ensureColumn(ctx, column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE order_items SET commerce_kind = 'membership_fee'
		WHERE UPPER(COALESCE(product_type, '')) IN ('MEMBERSHIP', 'WOW_MEMBERSHIP', 'MEMBERSHIP_FEE', 'SUBSCRIPTION')
			OR UPPER(COALESCE(division_type, '')) IN ('MEMBERSHIP', 'WOW_MEMBERSHIP', 'MEMBERSHIP_FEE', 'SUBSCRIPTION')`); err != nil {
		return fmt.Errorf("classify normalized membership items: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		VALUES (4, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`); err != nil {
		return fmt.Errorf("record sqlite migration: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		VALUES (5, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`); err != nil {
		return fmt.Errorf("record sqlite migration: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS product_categories (
		product_key TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		top_category TEXT NOT NULL,
		second_category TEXT NOT NULL,
		leaf_category TEXT NOT NULL,
		leaf_category_id TEXT NOT NULL DEFAULT '',
		breadcrumb_json TEXT NOT NULL DEFAULT '[]',
		fetched_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create product category cache: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		VALUES (6, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`); err != nil {
		return fmt.Errorf("record product category migration: %w", err)
	}
	categoryColumns := []struct {
		name       string
		definition string
	}{
		{"leaf_category_id", "TEXT NOT NULL DEFAULT ''"},
		{"breadcrumb_json", "TEXT NOT NULL DEFAULT '[]'"},
	}
	for _, column := range categoryColumns {
		if err := s.ensureColumn(ctx, "product_categories", column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		VALUES (7, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`); err != nil {
		return fmt.Errorf("record product category path migration: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		VALUES (8, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`); err != nil {
		return fmt.Errorf("record commerce classification migration: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS product_price_observations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		product_key TEXT NOT NULL,
		product_id TEXT NOT NULL,
		item_id TEXT NOT NULL DEFAULT '',
		vendor_item_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		canonical_url TEXT NOT NULL DEFAULT '',
		current_amount INTEGER NOT NULL CHECK (current_amount > 0),
		original_amount INTEGER NOT NULL DEFAULT 0 CHECK (original_amount >= 0),
		discount_rate INTEGER NOT NULL DEFAULT 0 CHECK (discount_rate BETWEEN 0 AND 100),
		currency TEXT NOT NULL CHECK (currency = 'KRW'),
		source TEXT NOT NULL CHECK (source IN ('coupang_product_search', 'coupang_product_inspection')),
		observed_at TEXT NOT NULL,
		UNIQUE(product_key, observed_at, source)
	)`); err != nil {
		return fmt.Errorf("create product price observations: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS product_price_observations_lookup_idx
		ON product_price_observations(product_id, vendor_item_id, observed_at DESC)`); err != nil {
		return fmt.Errorf("index product price observations: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		VALUES (9, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`); err != nil {
		return fmt.Errorf("record product price observation migration: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS product_price_watchlist (
		product_key TEXT PRIMARY KEY,
		product_id TEXT NOT NULL,
		item_id TEXT NOT NULL DEFAULT '',
		vendor_item_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		canonical_url TEXT NOT NULL DEFAULT '',
		added_at TEXT NOT NULL,
		last_checked_at TEXT,
		last_status TEXT NOT NULL CHECK (last_status IN ('pending', 'observed', 'unavailable', 'failed'))
	)`); err != nil {
		return fmt.Errorf("create product price watchlist: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS product_price_watchlist_due_idx
		ON product_price_watchlist(last_checked_at, added_at)`); err != nil {
		return fmt.Errorf("index product price watchlist: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		VALUES (10, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`); err != nil {
		return fmt.Errorf("record product price watchlist migration: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		VALUES (11, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`); err != nil {
		return fmt.Errorf("record complete-history evidence migration: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS product_category_observations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		product_key TEXT NOT NULL,
		source TEXT NOT NULL,
		breadcrumb_json TEXT NOT NULL DEFAULT '[]',
		observed_at TEXT NOT NULL,
		UNIQUE(product_key, source, observed_at)
	)`); err != nil {
		return fmt.Errorf("create product category observations: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS product_category_observations_lookup_idx
		ON product_category_observations(product_key, observed_at)`); err != nil {
		return fmt.Errorf("index product category observations: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO product_category_observations(
		product_key, source, breadcrumb_json, observed_at
	) SELECT product_key, source, breadcrumb_json, fetched_at FROM product_categories`); err != nil {
		return fmt.Errorf("seed product category observations: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		VALUES (12, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`); err != nil {
		return fmt.Errorf("record product category observation migration: %w", err)
	}
	return nil
}

func (s *SQLite) ensureColumn(ctx context.Context, table, column, definition string) error {
	if table != "orders" && table != "order_items" && table != "product_categories" && table != "sync_runs" {
		return fmt.Errorf("migrate sqlite: unsupported table")
	}
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return fmt.Errorf("inspect sqlite columns: %w", err)
	}
	present := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("scan sqlite columns: %w", err)
		}
		if name == column {
			present = true
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close sqlite column inspection: %w", err)
	}
	if present {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition); err != nil {
		return fmt.Errorf("migrate sqlite column: %w", err)
	}
	return nil
}
