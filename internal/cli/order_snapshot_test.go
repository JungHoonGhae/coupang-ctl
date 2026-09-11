package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

type aggregateSourceProbe struct{ calls int }

func (s *aggregateSourceProbe) FetchPage(context.Context, *core.OrderCursor) (core.OrderPage, error) {
	s.calls++
	return core.OrderPage{}, nil
}

type recapSnapshotWorkflow struct {
	orderWorkflow
	calls           int
	includeProducts bool
	filter          core.OrderFilter
	err             error
}

func (w *recapSnapshotWorkflow) ShoppingAnalysis(_ context.Context, filter core.OrderFilter, includeProducts bool) (core.ShoppingAnalysis, error) {
	w.calls++
	w.filter, w.includeProducts = filter, includeProducts
	result := core.ShoppingAnalysis{Insights: core.ShoppingInsights{SchemaVersion: core.ShoppingInsightsSchemaVersion}}
	if includeProducts {
		result.Products = &core.ProductInsights{SchemaVersion: core.OrderAggregateSchemaVersion, Visibility: "private_local"}
	}
	return result, w.err
}

func (*recapSnapshotWorkflow) Insights(context.Context, core.OrderFilter) (core.ShoppingInsights, error) {
	return core.ShoppingInsights{}, errors.New("recap must not acquire separate insight snapshot")
}

func (*recapSnapshotWorkflow) ProductInsights(context.Context, core.OrderFilter) (core.ProductInsights, error) {
	return core.ProductInsights{}, errors.New("recap must not acquire separate product snapshot")
}

func TestRecapUsesSingleCompositeReadWithExplicitProductScope(t *testing.T) {
	for _, includeProducts := range []bool{false, true} {
		workflow := &recapSnapshotWorkflow{}
		outputPath := filepath.Join(t.TempDir(), "synthetic.html")
		args := []string{"recap", "--output", outputPath, "--from", "2026-08-01", "--to", "2026-08-31"}
		if includeProducts {
			args = append(args, "--include-products")
		}
		var output bytes.Buffer
		if err := runOrders(context.Background(), args, &output, workflow); err != nil {
			t.Fatal(err)
		}
		if workflow.calls != 1 || workflow.includeProducts != includeProducts || workflow.filter.From != "2026-08-01" || workflow.filter.To != "2026-08-31" {
			t.Fatal("recap split snapshots or lost requested scope")
		}
		if _, err := os.Stat(outputPath); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecapSnapshotFailureDoesNotWritePartialOutput(t *testing.T) {
	want := errors.New("synthetic snapshot failure")
	workflow := &recapSnapshotWorkflow{err: want}
	outputPath := filepath.Join(t.TempDir(), "synthetic.html")
	var output bytes.Buffer
	err := runOrders(context.Background(), []string{"recap", "--output", outputPath, "--include-products"}, &output, workflow)
	if !errors.Is(err, want) || output.Len() != 0 || workflow.calls != 1 {
		t.Fatal("snapshot failure not propagated atomically")
	}
	if _, err := os.Stat(outputPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed snapshot created output", err)
	}
}

func TestLocalAggregateCLIEmitsSnapshotEvidenceWithoutSourceRead(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	page := core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-private-order", PurchasedAt: "2026-08-01", Currency: "KRW", TotalAmount: 100}}}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		t.Fatal(err)
	}
	source := &aggregateSourceProbe{}
	service := orders.NewWithPageSource(ledger, source)
	for _, command := range []string{"stats", "spend", "insights", "products"} {
		var output bytes.Buffer
		if err := runOrders(ctx, []string{command}, &output, service); err != nil {
			t.Fatal(err)
		}
		var result struct {
			SchemaVersion int                    `json:"schema_version"`
			OrderCount    int                    `json:"order_count"`
			Evidence      core.OrderReadEvidence `json:"evidence"`
		}
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		wantVersion := core.OrderAggregateSchemaVersion
		if command == "stats" {
			wantVersion = core.OrderStatsSchemaVersion
		}
		if command == "insights" {
			wantVersion = core.ShoppingInsightsSchemaVersion
		}
		if result.SchemaVersion != wantVersion || (command != "products" && result.OrderCount != 1) || result.Evidence.Visibility != "private_local" || result.Evidence.Dataset != "retained_local_history" || result.Evidence.SnapshotCapturedAt.IsZero() || result.Evidence.LatestAttempt.State != core.SyncRunNeverRun || result.Evidence.LatestAttempt.HistoryComplete || len(result.Evidence.Limitations) == 0 {
			t.Fatal("CLI lost local evidence or invented acquired coverage")
		}
		if bytes.Contains(output.Bytes(), []byte("synthetic-private-order")) {
			t.Fatal("aggregate exposed order identity")
		}
		if command == "insights" {
			var fields map[string]any
			if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"night_fully_canceled_order_rate", "other_fully_canceled_order_rate", "night_returned_unit_rate", "other_returned_unit_rate", "night_order_rate", "late_evening_order_rate", "weekend_order_rate", "delivered_within_24_hours_rate", "delivered_within_48_hours_rate", "top_brand_share"} {
				value, exists := fields[name]
				if !exists || value != nil {
					t.Fatalf("CLI omitted or zero-filled unobserved cohort %s", name)
				}
			}
		}
		if command == "stats" {
			var fields map[string]any
			if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"canceled_unit_rate", "returned_unit_rate", "returned_item_line_rate"} {
				value, exists := fields[name]
				if !exists || value != nil {
					t.Fatalf("CLI omitted or zero-filled undefined %s", name)
				}
			}
			if fields["fully_canceled_order_rate"] != float64(0) {
				t.Fatal("CLI lost computed zero over one order")
			}
		}
	}
	if source.calls != 0 {
		t.Fatal("local aggregate contacted source")
	}
}
