package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRecommendationMCPCapacitySchemaAndWire(t *testing.T) {
	value := int64(1000)
	p := &recommendationProvider{result: &core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Inspected: []core.ProductRecommendationCandidate{{Conditions: []core.ProductConditionAssessment{{Status: core.ProductConditionMet, DerivedInteger: &value, Derivation: &core.ProductCapacityDerivation{Provenance: "derived", Method: "standalone_capacity_decimal_gb", Unit: "GB", Input: core.ProductSelectedAttribute{Name: "저장용량", Value: "1TB"}}}}}}}}
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := NewWithFeatures(fixedStatusProvider{}, nil, p, "test").Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	args := map[string]any{"query": "synthetic", "proceed": true, "required_conditions": []any{map[string]any{"id": "storage", "field": "specifications.storage_gb", "operator": "gte", "integer": 1000}}, "comparison_axes": []any{map[string]any{"field": "specifications.memory_gb", "direction": "maximize"}}}
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError || p.calls != 1 || *p.request.RequiredConditions[0].Integer != 1000 || p.request.ComparisonAxes[0].Field != "specifications.memory_gb" {
		t.Fatal("MCP capacity input lost")
	}
	data, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var result core.ProductRecommendationResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	c := result.Inspected[0].Conditions[0]
	if c.DerivedInteger == nil || *c.DerivedInteger != 1000 || c.ObservedInteger != nil || c.Derivation.Input.Value != "1TB" {
		t.Fatal("derived capacity lost on MCP wire")
	}
	args["required_conditions"] = []any{map[string]any{"id": "storage", "field": "specifications.storage_gb", "operator": "gte", "number": 1000}}
	r, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || p.calls != 1 {
		t.Fatal("wrong numeric type reached provider")
	}
}
