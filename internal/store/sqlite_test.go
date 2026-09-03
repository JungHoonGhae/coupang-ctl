package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestCategoryStabilityRequiresSameProductAcrossDistinctDays(t *testing.T) {
	ctx := context.Background()
	ledger, err := Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-multiday-order", PurchasedAt: "2026-08-29", TotalAmount: 2000, Currency: "KRW",
		Items: []core.OrderItem{
			{ProductID: "101", VendorItemID: "201", Name: "Synthetic private A", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000},
			{ProductID: "102", VendorItemID: "202", Name: "Synthetic private B", Quantity: 1, UnitPrice: 1000, PaidPrice: 1000},
		},
	}}}); err != nil {
		t.Fatal(err)
	}
	dayOne := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	dayTwo := dayOne.Add(24 * time.Hour)
	ledger.now = func() time.Time { return dayOne }
	category := core.ProductCategory{Source: core.CategorySourceProductJSONLDBreadcrumb, Path: []core.ProductCategoryNode{{ID: "200", Name: "Synthetic category", Position: 2}}}
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "201"}, category); err != nil {
		t.Fatal(err)
	}
	dayOne = dayOne.Add(time.Minute)
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "201"}, category); err != nil {
		t.Fatal(err)
	}
	ledger.now = func() time.Time { return dayTwo }
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "202"}, category); err != nil {
		t.Fatal(err)
	}

	report, err := ledger.CategoryStability(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.DistinctObservationDayCount != 2 || report.RecheckedProductCount != 1 || report.MultiDayRecheckedProductCount != 0 || report.Assessment != "insufficient_distinct_days" {
		t.Fatalf("unrelated observation days were treated as longitudinal evidence: %#v", report)
	}
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "201"}, category); err != nil {
		t.Fatal(err)
	}
	report, err = ledger.CategoryStability(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.MultiDayRecheckedProductCount != 1 || report.ChangedProductCount != 0 || report.Assessment != "stable_within_local_observation_window" {
		t.Fatalf("same-product multi-day evidence was not recognized: %#v", report)
	}
}

func TestOpenMigratesMetadataOnlySchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state", "coupangctl.sqlite3")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	assertCount(t, store.db, "SELECT COUNT(*) FROM schema_migrations", 12)
	assertCount(t, store.db, "SELECT COUNT(*) FROM pragma_table_info('sync_runs')", 8)
	assertCount(t, store.db, "SELECT COUNT(*) FROM pragma_table_info('product_category_observations')", 5)
}

func TestOpenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "coupangctl.sqlite3")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	assertCount(t, second.db, "SELECT COUNT(*) FROM schema_migrations", 12)
}

func TestOpenSecuresDatabaseAndSidecarFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not a Windows access-control boundary")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state", "coupangctl.sqlite3")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, "INSERT INTO sync_runs(started_at, status) VALUES ('2026-09-02T00:00:00Z', 'running')"); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode for %s = %o, want 600", filepath.Base(candidate), info.Mode().Perm())
		}
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}
