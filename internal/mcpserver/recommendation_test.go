package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type recommendationProvider struct {
	fixedProductProvider
	request core.ProductRecommendationRequest
	result  *core.ProductRecommendationResult
	calls   int
}

func (p *recommendationProvider) Recommend(_ context.Context, request core.ProductRecommendationRequest) (core.ProductRecommendationResult, error) {
	p.calls++
	p.request = request
	if p.result != nil {
		return *p.result, nil
	}
	return core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Query: request.Query, CategoryID: request.CategoryID}, nil
}

func TestRecommendationMCPExplicitConditionsAndAssessmentWire(t *testing.T) {
	ctx := context.Background()
	wanted := false
	condition := core.ProductRecommendationCondition{ID: "delivery", Field: "rocket", Operator: "eq", Boolean: &wanted}
	provider := &recommendationProvider{result: &core.ProductRecommendationResult{
		SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete,
		Inspected: []core.ProductRecommendationCandidate{{Conditions: []core.ProductConditionAssessment{{Condition: condition, Status: core.ProductConditionUnknown, MissingEvidence: []string{"rocket.provenance"}}}, ExclusionReason: "required_condition_unverified"}},
	}}
	server := NewWithFeatures(fixedStatusProvider{}, nil, provider, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	args := map[string]any{"query": "synthetic", "proceed": true, "required_conditions": []any{map[string]any{"id": "delivery", "field": "rocket", "operator": "eq", "boolean": false}}, "comparison_axes": []any{map[string]any{"field": "price.current_amount", "direction": "minimize"}}}
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError || provider.calls != 1 || len(provider.request.RequiredConditions) != 1 || provider.request.RequiredConditions[0].Boolean == nil || *provider.request.RequiredConditions[0].Boolean {
		t.Fatal("MCP lost explicit false condition")
	}
	if len(provider.request.ComparisonAxes) != 1 || provider.request.ComparisonAxes[0].Direction != "minimize" {
		t.Fatal("MCP lost explicit comparison axes")
	}
	data, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var result core.ProductRecommendationResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Inspected) != 1 || len(result.Inspected[0].Conditions) != 1 || result.Inspected[0].Conditions[0].Status != core.ProductConditionUnknown || result.Inspected[0].Conditions[0].ObservedBoolean != nil {
		t.Fatal("MCP promoted unknown to false/met")
	}
	args["required_conditions"] = []any{map[string]any{"id": "unsupported", "field": "allergen_free", "operator": "eq", "boolean": true}}
	r, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || provider.calls != 1 {
		t.Fatal("unsupported condition reached provider")
	}
}

func TestRecommendationMCPReviewCountAvailability(t *testing.T) {
	for _, known := range []bool{false, true} {
		var count *int
		if known {
			count = new(int)
		}
		provider := &recommendationProvider{result: &core.ProductRecommendationResult{
			SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete,
			Candidates: []core.ProductRecommendationCandidate{{ReviewsAvailable: count}},
			Audit:      core.ProductRecommendationAudit{ReviewsAvailable: count, TimeBudgetExhausted: known},
		}}
		ctx := context.Background()
		server := NewWithFeatures(fixedStatusProvider{}, nil, provider, "test")
		ct, st := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, st, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ss.Close()
		client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
		cs, err := client.Connect(ctx, ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cs.Close()
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: map[string]any{"query": "synthetic"}})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatalf("recommendation output schema rejected availability: %#v", result.Content)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var output map[string]any
		if err := json.Unmarshal(data, &output); err != nil {
			t.Fatal(err)
		}
		if output["schema_version"] != float64(core.ProductRecommendationSchemaVersion) {
			t.Fatal("unexpected recommendation version")
		}
		if output["audit"].(map[string]any)["time_budget_exhausted"] != known {
			t.Fatal("MCP lost explicit time-budget audit flag")
		}
		for _, row := range []map[string]any{output["candidates"].([]any)[0].(map[string]any), output["audit"].(map[string]any)} {
			value, present := row["reviews_available"]
			if present != known || present && value != float64(0) {
				t.Fatal("recommendation review availability changed on MCP wire")
			}
		}
	}
}

func TestRecommendationMCPExposesTypedInputAndIncompleteStatus(t *testing.T) {
	ctx := context.Background()
	provider := &recommendationProvider{}
	server := NewWithFeatures(fixedStatusProvider{}, nil, provider, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: map[string]any{"query": "synthetic bowl", "max_price": 2000, "proceed": true, "use_purchase_history": true}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool failed: %#v", result.Content)
	}
	if provider.request.Query != "synthetic bowl" || provider.request.MaxPrice != 2000 || !provider.request.Proceed || !provider.request.UsePurchaseHistory || provider.request.MaxItems != 0 {
		t.Fatal("typed recommendation input was not forwarded")
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	if response["status"] != "incomplete" {
		t.Fatalf("status=%v", response["status"])
	}
	provider.result = &core.ProductRecommendationResult{
		SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete,
		Inspected: []core.ProductRecommendationCandidate{{Status: core.ProductRecommendationNeedsVerification, ExclusionReason: "budget_condition_unverified"}},
		Audit:     core.ProductRecommendationAudit{InspectionAttempts: 1, DetailsInspected: 1, InspectionStopReason: "inspection_budget_reached", UninspectedModelFamilies: 4},
	}
	result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: map[string]any{"query": "synthetic", "max_items": 6, "proceed": true}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || provider.request.MaxItems != 6 {
		t.Fatal("MCP retained arbitrary five-item cap")
	}
	encoded, err = json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var decoded core.ProductRecommendationResult
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Inspected) != 1 || decoded.Inspected[0].ExclusionReason != "budget_condition_unverified" || decoded.Audit.InspectionStopReason != "inspection_budget_reached" || decoded.Audit.UninspectedModelFamilies != 4 {
		t.Fatal("MCP lost investigated evidence or budget state")
	}
}
