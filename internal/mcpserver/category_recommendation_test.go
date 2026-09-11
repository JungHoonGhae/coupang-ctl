package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPRecommendationRetainsCategoryAndValidatesBeforeWorkflow(t *testing.T) {
	ctx := context.Background()
	provider := &recommendationProvider{}
	server := NewWithProviders(Providers{Products: provider}, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-category", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	args := map[string]any{"category_id": "123456", "max_price": 2000000, "disable_affiliate": true,
		"answers": []map[string]string{{"question_id": "facet:Memory", "choice": "32 GB"}}}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: args})
	if err != nil || result.IsError {
		t.Fatal("MCP rejected category-only recommendation")
	}
	r := provider.request
	if provider.calls != 1 || r.CategoryID != "123456" || r.Query != "" || r.MaxPrice != 2000000 || !r.DisableAffiliate || len(r.Answers) != 1 {
		t.Fatal("MCP lost category scope or recommendation inputs")
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var wire core.ProductRecommendationResult
	if json.Unmarshal(data, &wire) != nil || wire.CategoryID != "123456" || wire.Query != "" {
		t.Fatal("MCP output lost category")
	}
	args["answers"] = []map[string]string{{"question_id": "facet:카테고리", "choice": "Parent"}, {"question_id": "facet:카테고리", "choice": "Child"}}
	result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: args})
	if err != nil || result.IsError || provider.calls != 2 || len(provider.request.Answers) != 2 || provider.request.Answers[0].Choice != "Parent" || provider.request.Answers[1].Choice != "Child" {
		t.Fatal("MCP dropped or reordered observed category navigation")
	}
	for _, invalid := range []map[string]any{{"category_id": "../search"}, {"category_id": "123456", "query": "synthetic"}, {}} {
		result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: invalid})
		if err != nil || !result.IsError || provider.calls != 2 {
			t.Fatal("invalid category reached workflow")
		}
	}
	delete(args, "category_id")
	args["query"] = "synthetic"
	result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: args})
	if err != nil || result.IsError || provider.calls != 3 || provider.request.Query != "synthetic" || provider.request.CategoryID != "" || len(provider.request.Answers) != 2 {
		t.Fatal("MCP rejected a query category trail")
	}
}
