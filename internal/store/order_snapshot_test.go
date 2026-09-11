package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// Interpose only at the actual database/sql boundary. The hook runs after the
// first aggregate query closes its rows, then commits a second connection's
// change before the next query. No production hook or timing race is needed.
type snapshotConnector struct {
	driver         driver.Driver
	path           string
	once           sync.Once
	afterAggregate func() error
	match          func(string) bool
}

func (c *snapshotConnector) Driver() driver.Driver { return c.driver }
func (c *snapshotConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.path)
	if err != nil {
		return nil, err
	}
	return &snapshotConn{Conn: conn, owner: c}, nil
}

type snapshotConn struct {
	driver.Conn
	owner *snapshotConnector
}

func (c *snapshotConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}
func (c *snapshotConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
	if err != nil {
		return nil, err
	}
	matched := strings.HasPrefix(q, "SELECT COUNT(*)") && strings.Contains(q, "FROM orders WHERE")
	if c.owner.match != nil {
		matched = c.owner.match(q)
	}
	if matched {
		return &snapshotRows{Rows: rows, owner: c.owner}, nil
	}
	return rows, nil
}

func TestHigherInsightsDoNotMixConcurrentCommits(t *testing.T) {
	for _, operation := range []string{"insights", "products", "analysis", "analysis_without_products"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
			reader := openWriterLedger(t, path)
			writer := openWriterLedger(t, path)
			first := core.Order{SourceRef: "synthetic-first", PurchasedAt: "2026-08-01", Currency: "KRW", TotalAmount: 100,
				Items: []core.OrderItem{{ProductID: "123", VendorItemID: "456", Name: "Synthetic", Quantity: 1, PaidPrice: 100, CommerceKind: core.CommerceKindProductPurchase}}}
			if _, err := writer.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{first}}); err != nil {
				t.Fatal(err)
			}
			fired := false
			connector := &snapshotConnector{driver: reader.db.Driver(), path: path, afterAggregate: func() error {
				fired = true
				second := first
				second.SourceRef = "synthetic-second"
				_, err := writer.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{second}})
				return err
			}}
			productWindowReads := 0
			connector.match = func(q string) bool {
				productWindow := strings.HasPrefix(q, "SELECT COALESCE(MIN(purchased_at)") && strings.Contains(q, "COUNT(DISTINCT substr(purchased_at, 1, 7))")
				if productWindow {
					productWindowReads++
				}
				if operation == "products" {
					return productWindow
				}
				return strings.HasPrefix(q, "SELECT COUNT(*)") && strings.Contains(q, "FROM orders WHERE")
			}
			reader.db.Close()
			reader.db = sql.OpenDB(connector)
			reader.db.SetMaxOpenConns(1)
			var evidence core.OrderReadEvidence
			if operation == "insights" || strings.HasPrefix(operation, "analysis") {
				var products *core.ProductInsights
				var got core.ShoppingInsights
				var err error
				if strings.HasPrefix(operation, "analysis") {
					analysis, readErr := reader.ShoppingAnalysis(ctx, core.OrderFilter{}, operation == "analysis")
					got, products, err = analysis.Insights, analysis.Products, readErr
				} else {
					got, err = reader.Insights(ctx, core.OrderFilter{})
				}
				if err != nil {
					t.Fatal(err)
				}
				evidence = got.Evidence
				if got.SchemaVersion != core.ShoppingInsightsSchemaVersion || got.Categories.TotalItemLines != 1 {
					t.Fatal("insight version or category snapshot differs")
				}
				if got.OrderCount != 1 || got.RepeatPurchases.PurchaseOccasionCount != 1 || len(got.PurchaseMonths) != 1 || got.PurchaseMonths[0].OrderCount != 1 {
					t.Fatalf("mixed insights: orders=%d repeat occasions=%d months=%v", got.OrderCount, got.RepeatPurchases.PurchaseOccasionCount, got.PurchaseMonths)
				}
				if operation == "analysis" {
					if products == nil || products.SchemaVersion != core.OrderAggregateSchemaVersion || products.TotalSpendAmount != 100 || products.RetainedUnitCount != 1 || products.HighestSpendDay.TotalAmount != 100 || !products.Evidence.SnapshotCapturedAt.Equal(got.Evidence.SnapshotCapturedAt) {
						t.Fatal("combined summary and private products did not share one snapshot")
					}
				} else if products != nil {
					t.Fatal("private product details returned without explicit scope")
				}
			} else {
				got, err := reader.ProductInsights(ctx, core.OrderFilter{})
				if err != nil {
					t.Fatal(err)
				}
				evidence = got.Evidence
				if got.SchemaVersion != core.OrderAggregateSchemaVersion {
					t.Fatal("product insight version missing")
				}
				if got.TotalSpendAmount != 100 || got.RetainedUnitCount != 1 || got.TopByUnits.UnitCount != 1 || got.HighestSpendDay.TotalAmount != 100 {
					t.Fatalf("mixed products: total=%d units=%d top=%d day=%d", got.TotalSpendAmount, got.RetainedUnitCount, got.TopByUnits.UnitCount, got.HighestSpendDay.TotalAmount)
				}
			}
			if !fired {
				t.Fatal("interleave did not execute")
			}
			wantProductReads := 0
			if operation == "analysis" || operation == "products" {
				wantProductReads = 1
			}
			if productWindowReads != wantProductReads {
				t.Fatalf("product queries = %d, want %d for explicit scope", productWindowReads, wantProductReads)
			}
			if evidence.Visibility != "private_local" || evidence.Dataset != "retained_local_history" || evidence.SnapshotCapturedAt.IsZero() || evidence.LatestAttempt.State != core.SyncRunNeverRun {
				t.Fatal("higher insights lost local snapshot evidence")
			}
			latest, err := writer.Stats(ctx, core.OrderFilter{})
			if err != nil || latest.OrderCount != 2 {
				t.Fatal("concurrent write was not committed", err)
			}
		})
	}
}

type snapshotRows struct {
	driver.Rows
	owner *snapshotConnector
}

func TestShoppingAnalysisReadFailureReturnsNoPartialEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	reader := openWriterLedger(t, path)
	want := errors.New("synthetic aggregate read failure")
	connector := &snapshotConnector{driver: reader.db.Driver(), path: path, afterAggregate: func() error { return want }}
	reader.db.Close()
	reader.db = sql.OpenDB(connector)
	reader.db.SetMaxOpenConns(1)
	got, err := reader.ShoppingAnalysis(ctx, core.OrderFilter{}, true)
	if !errors.Is(err, want) || !reflect.DeepEqual(got, core.ShoppingAnalysis{}) {
		t.Fatal("failed composite read returned partial statistics or snapshot evidence", err)
	}
	// Rollback must release the transaction/connection even with a single slot.
	if _, err := reader.ShoppingAnalysis(ctx, core.OrderFilter{}, true); err != nil {
		t.Fatal("failed read left transaction open", err)
	}
}

func (r *snapshotRows) Close() error {
	if err := r.Rows.Close(); err != nil {
		return err
	}
	var err error
	r.owner.once.Do(func() { err = r.owner.afterAggregate() })
	return err
}

func TestOrderAggregatesDoNotMixConcurrentCommits(t *testing.T) {
	for _, operation := range []string{"stats", "spend"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
			reader := openWriterLedger(t, path)
			writer := openWriterLedger(t, path)
			first := core.Order{SourceRef: "synthetic-first", PurchasedAt: "2026-08-01", Currency: "KRW", TotalAmount: 100,
				Items: []core.OrderItem{{ProductID: "123", Name: "Synthetic", Quantity: 1, CommerceKind: core.CommerceKindProductPurchase}}}
			if _, err := writer.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{first}}); err != nil {
				t.Fatal(err)
			}
			run, err := writer.BeginSync(ctx, core.SyncSourceOrdinaryBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
			if err != nil {
				t.Fatal(err)
			}
			capturedAt := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
			reader.now = func() time.Time { return capturedAt }
			fired := false
			connector := &snapshotConnector{driver: reader.db.Driver(), path: path, afterAggregate: func() error {
				fired = true
				second := first
				second.SourceRef = "synthetic-second"
				if _, err := writer.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{second}}); err != nil {
					return err
				}
				return writer.FinishSync(ctx, run, core.SyncResult{PagesProcessed: 1, OrdersSeen: 1}, "synthetic_interruption")
			}}
			reader.db.Close()
			reader.db = sql.OpenDB(connector)
			reader.db.SetMaxOpenConns(1)
			var evidence core.OrderReadEvidence
			if operation == "stats" {
				got, err := reader.Stats(ctx, core.OrderFilter{})
				if err != nil {
					t.Fatal(err)
				}
				if got.SchemaVersion != core.OrderStatsSchemaVersion {
					t.Fatal("stats version missing")
				}
				evidence = got.Evidence
				if got.OrderCount != 1 || got.ItemLineCount != 1 || got.OrderedUnits != 1 || len(got.PurchaseMonths) != 1 || got.PurchaseMonths[0].OrderCount != 1 {
					t.Fatalf("mixed snapshots: orders=%d lines=%d units=%d months=%v", got.OrderCount, got.ItemLineCount, got.OrderedUnits, got.PurchaseMonths)
				}
			} else {
				got, err := reader.Spend(ctx, core.OrderFilter{})
				if err != nil {
					t.Fatal(err)
				}
				if got.SchemaVersion != core.OrderAggregateSchemaVersion {
					t.Fatal("spend version missing")
				}
				evidence = got.Evidence
				if got.OrderCount != 1 || got.TotalAmount != 100 || got.Commerce.ProductPurchases.OrderCount != 1 || got.Commerce.ProductPurchases.GrossAmount != 100 {
					t.Fatalf("mixed snapshots: orders=%d amount=%d bucket=%v", got.OrderCount, got.TotalAmount, got.Commerce.ProductPurchases)
				}
			}
			if !fired {
				t.Fatal("writer interleave did not execute")
			}
			if evidence.LatestAttempt.State != core.SyncRunRunning || evidence.LatestAttempt.HistoryComplete || evidence.Dataset != "retained_local_history" || evidence.Visibility != "private_local" || !evidence.SnapshotCapturedAt.Equal(capturedAt) {
				t.Fatal("aggregate and acquisition evidence did not share the original snapshot")
			}
			latest, err := writer.Spend(ctx, core.OrderFilter{})
			if err != nil || latest.OrderCount != 2 || latest.Evidence.LatestAttempt.State != core.SyncRunFailed {
				t.Fatal("concurrent commit did not persist", err)
			}
		})
	}
}
