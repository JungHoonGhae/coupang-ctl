package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type categorySearchProvider struct {
	fixedProductProvider
	request core.ProductSearchRequest
}

func (p *categorySearchProvider) Search(ctx context.Context, r core.ProductSearchRequest) (core.ProductSearchResult, error) {
	p.request = r
	result, err := p.fixedProductProvider.Search(ctx, r)
	result.AppliedFilters = r
	return result, err
}

func TestMCPCategoryAndSidebarReachTypedWorkflow(t *testing.T) {
	ctx := context.Background()
	provider := &categorySearchProvider{}
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
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_search", Arguments: map[string]any{
		"category_id": "123456", "max_price": 2000000, "disable_affiliate": true,
		"facet_selections": []map[string]string{{"name": "Memory", "label": "32 GB"}},
	}})
	if err != nil || result.IsError {
		t.Fatal("category-only input rejected by MCP")
	}
	r := provider.request
	if r.CategoryID != "123456" || r.Query != "" || r.MaxPrice != 2000000 || !r.DisableAffiliate || len(r.FacetSelections) != 1 || r.FacetSelections[0] != (core.ProductFacetSelection{Name: "Memory", Label: "32 GB"}) {
		t.Fatal("category input changed before typed workflow")
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var wire core.ProductSearchResult
	if json.Unmarshal(data, &wire) != nil || wire.AppliedFilters.CategoryID != "123456" || len(wire.AppliedFilters.FacetSelections) != 1 {
		t.Fatal("category request lost in MCP output")
	}
	result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_search", Arguments: map[string]any{
		"query": "synthetic", "category_trail": []string{"Parent"},
		"facet_selections": []map[string]string{{"name": "카테고리", "label": "Child"}},
	}})
	if err != nil || result.IsError || provider.request.Query != "synthetic" || len(provider.request.CategoryTrail) != 1 || provider.request.CategoryTrail[0] != "Parent" {
		t.Fatal("MCP lost navigation trail")
	}
}
