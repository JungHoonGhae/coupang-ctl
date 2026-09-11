package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type selectedAttributeProvider struct{ fixedProductProvider }

func (selectedAttributeProvider) Inspect(_ context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	ref := core.ProductReference{ProductID: r.ProductID, ItemID: r.ItemID, VendorItemID: r.VendorItemID}
	return core.ProductInspection{
		SchemaVersion:      core.ProductSchemaVersion,
		Product:            core.ProductCard{Reference: ref, Name: "Synthetic", ComputerSpecs: &core.ComputerSpecifications{MemoryGB: 32, Provenance: "inferred", Method: "capacity_and_model_regex", Source: "product_title_heuristic"}},
		SelectedAttributes: []core.ProductSelectedAttribute{{Name: "RAM용량 × 저장용량", Value: "32GB × 1TB"}},
		FieldEvidence:      []core.ProductFieldEvidence{{Field: "selected_attributes", Source: "product_options", Locator: "options.optionRows.selectedAttribute", Scope: "selected_option", Reference: ref, Provenance: "observed", Method: "native_field", CapturedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}},
		Coverage:           core.ProductCoverage{ObservedFields: []string{"selected_attributes"}},
	}, nil
}

func TestMCPSelectedAttributesAndHeuristicProvenanceRoundTrip(t *testing.T) {
	ctx := context.Background()
	server := NewWithFeatures(fixedStatusProvider{}, fixedOrderProvider{}, selectedAttributeProvider{}, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "selected-attributes-test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "product_inspect", Arguments: map[string]any{"product_id": "101", "item_id": "201", "vendor_item_id": "301"}})
	if err != nil || result.IsError {
		t.Fatal("MCP selected attribute contract rejected", err)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var got core.ProductInspection
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if e, ok := got.SelectedAttributesEvidence(); !ok || e.Reference.VendorItemID != "301" {
		t.Fatal("exact option evidence lost")
	}
	if len(got.SelectedAttributes) != 1 || got.SelectedAttributes[0].Value != "32GB × 1TB" || got.Product.ComputerSpecs.Provenance != "inferred" {
		t.Fatal("source tuple conflated with inferred specs")
	}
}
