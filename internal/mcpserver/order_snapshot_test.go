package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/mcpserver"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type aggregateSourceProbe struct{ calls int }

func (s *aggregateSourceProbe) FetchPage(context.Context, *core.OrderCursor) (core.OrderPage, error) {
	s.calls++
	return core.OrderPage{}, nil
}

func TestLocalAggregateMCPEmitsSnapshotEvidenceWithoutSourceRead(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-private-order", PurchasedAt: "2026-08-01", Currency: "KRW", TotalAmount: 100}}}); err != nil {
		t.Fatal(err)
	}
	source := &aggregateSourceProbe{}
	server := mcpserver.NewWithProviders(mcpserver.Providers{Orders: orders.NewWithPageSource(ledger, source)}, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	toolList, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	rateFields := map[string][]string{
		"orders_stats":    {"fully_canceled_order_rate", "canceled_unit_rate", "returned_unit_rate", "returned_item_line_rate"},
		"orders_insights": {"night_fully_canceled_order_rate", "other_fully_canceled_order_rate", "night_returned_unit_rate", "other_returned_unit_rate", "night_order_rate", "late_evening_order_rate", "weekend_order_rate", "delivered_within_24_hours_rate", "delivered_within_48_hours_rate", "top_brand_share"},
	}
	schemasChecked := 0
	for _, tool := range toolList.Tools {
		fields, expected := rateFields[tool.Name]
		if !expected {
			continue
		}
		encoded, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Type any `json:"type"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatal(err)
		}
		for _, field := range fields {
			required := false
			for _, name := range schema.Required {
				required = required || name == field
			}
			if !required {
				t.Fatalf("MCP schema allows omitting rate %s instead of explicit null", field)
			}
			types, ok := schema.Properties[field].Type.([]any)
			hasNull, hasNumber := false, false
			for _, kind := range types {
				hasNull = hasNull || kind == "null"
				hasNumber = hasNumber || kind == "number"
			}
			if !ok || !hasNull || !hasNumber {
				t.Fatalf("MCP schema does not allow nullable rate %s: %s", field, encoded)
			}
		}
		schemasChecked++
	}
	if schemasChecked != len(rateFields) {
		t.Fatal("nullable aggregate schema missing")
	}
	for _, name := range []string{"orders_stats", "orders_spend", "orders_insights", "orders_product_insights"} {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name})
		if err != nil || result.IsError {
			t.Fatal("MCP local read failed", err)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			SchemaVersion int                    `json:"schema_version"`
			OrderCount    int                    `json:"order_count"`
			Evidence      core.OrderReadEvidence `json:"evidence"`
		}
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		wantVersion := core.OrderAggregateSchemaVersion
		if name == "orders_stats" {
			wantVersion = core.OrderStatsSchemaVersion
		}
		if name == "orders_insights" {
			wantVersion = core.ShoppingInsightsSchemaVersion
		}
		if got.SchemaVersion != wantVersion || (name != "orders_product_insights" && got.OrderCount != 1) || got.Evidence.Visibility != "private_local" || got.Evidence.Dataset != "retained_local_history" || got.Evidence.SnapshotCapturedAt.IsZero() || got.Evidence.LatestAttempt.State != core.SyncRunNeverRun || got.Evidence.LatestAttempt.HistoryComplete || len(got.Evidence.Limitations) == 0 {
			t.Fatal("MCP lost evidence or invented acquired coverage")
		}
		if bytes.Contains(encoded, []byte("synthetic-private-order")) {
			t.Fatal("aggregate exposed order identity")
		}
		if name == "orders_insights" {
			var fields map[string]any
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			for _, field := range rateFields[name] {
				value, exists := fields[field]
				if !exists || value != nil {
					t.Fatalf("MCP omitted or zero-filled unobserved cohort %s", field)
				}
			}
		}
		if name == "orders_stats" {
			var fields map[string]any
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"canceled_unit_rate", "returned_unit_rate", "returned_item_line_rate"} {
				value, exists := fields[field]
				if !exists || value != nil {
					t.Fatalf("MCP omitted or zero-filled undefined %s", field)
				}
			}
			if fields["fully_canceled_order_rate"] != float64(0) {
				t.Fatal("MCP lost computed zero over one order")
			}
		}
	}
	if source.calls != 0 {
		t.Fatal("local aggregate contacted source")
	}
}
