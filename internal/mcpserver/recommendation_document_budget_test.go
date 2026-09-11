package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/products"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"slices"
	"testing"
)

type budgetSource struct {
	conclusionSource
	reserved, searches, details int
}

func (s *budgetSource) Search(_ context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	if r.DocumentReadLimit != 1 {
		return nil, core.ProductCoverage{}, fmt.Errorf("search allowance must disable retries")
	}
	s.reserved += r.DocumentReadLimit
	s.searches++
	items := make([]core.ProductCard, 20)
	for i := range items {
		p := s.product()
		p.Reference.ProductID = fmt.Sprint(1000 + i)
		p.URL = "https://www.coupang.com/vp/products/" + p.Reference.ProductID
		for j := range p.FieldEvidence {
			p.FieldEvidence[j].Reference = p.Reference
		}
		items[i] = p
	}
	return items, core.ProductCoverage{}, nil
}

func (s *budgetSource) Inspect(_ context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	s.reserved += r.DocumentReadLimit
	s.details++
	if r.DocumentReadLimit != 1 || s.reserved > 40 {
		return core.ProductInspection{}, fmt.Errorf("unexpected detail allowance")
	}
	p := s.product()
	p.Reference.ProductID = r.ProductID
	p.URL = "https://www.coupang.com/vp/products/" + r.ProductID
	for j := range p.FieldEvidence {
		p.FieldEvidence[j].Reference = p.Reference
	}
	return core.ProductInspection{Product: p, Coverage: core.ProductCoverage{
		BudgetOmittedFields: []string{"quantity_info", "reviews"},
		UnavailableFields:   []string{"quantity_info", "reviews"},
	}}, nil
}

func assertBudgetResult(t *testing.T, result core.ProductRecommendationResult, source *budgetSource, display int) {
	t.Helper()
	if source.reserved != 40 || source.searches != 20 || source.details != 20 ||
		result.Audit.DocumentReadBudget != 40 || result.Audit.DocumentReadReservations != source.reserved ||
		!result.Audit.DocumentBudgetConstrained || result.Status != core.ProductRecommendationIncomplete {
		t.Fatal("reservation evidence changed across adapter")
	}
	if result.Audit.DiscoveryStopReason != "document_budget_reserved_for_inspection" ||
		result.Audit.InspectionStopReason != "auxiliary_document_budget_limited" ||
		len(result.Inspected) != 20 {
		t.Fatal("budget became source exhaustion or incomplete work disappeared")
	}
	want := display
	if want == 0 {
		want = 20
	}
	if len(result.Candidates) != want {
		t.Fatal("presentation count incorrect")
	}
	found := false
	for _, action := range result.NextActions {
		if action.Reason == "document_budget_omitted_fields" && len(action.References) == 20 &&
			slices.Contains(action.Fields, "reviews") && slices.Contains(action.Fields, "quantity_info") {
			found = true
		}
	}
	if !found {
		t.Fatal("hidden candidates lost budget followup")
	}
}

func TestMCPRecommendationDocumentBudgetWire(t *testing.T) {
	for _, display := range []int{0, 1} {
		t.Run(fmt.Sprint(display), func(t *testing.T) {
			ctx := context.Background()
			source := &budgetSource{conclusionSource: conclusionSource{known: true}}
			server := NewWithFeatures(fixedStatusProvider{}, nil, products.New(source), "test")
			ct, st := mcp.NewInMemoryTransports()
			ss, err := server.Connect(ctx, st, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ss.Close()
			client := mcp.NewClient(&mcp.Implementation{Name: "budget-test", Version: "test"}, nil)
			cs, err := client.Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			response, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: map[string]any{
				"query": "synthetic", "proceed": true, "disable_affiliate": true, "search_page_limit": 10, "max_items": display,
			}})
			if err != nil {
				t.Fatal(err)
			}
			if response.IsError {
				t.Fatal("MCP rejected budget result")
			}
			encoded, err := json.Marshal(response.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			var result core.ProductRecommendationResult
			if err := json.Unmarshal(encoded, &result); err != nil {
				t.Fatal(err)
			}
			assertBudgetResult(t, result, source, display)
		})
	}
}
