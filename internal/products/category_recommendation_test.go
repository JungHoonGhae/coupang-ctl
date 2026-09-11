package products

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangproducts "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
)

func TestRecommendRetainsCategoryAcrossRefinementAndSorts(t *testing.T) {
	groups := []core.ProductFacet{{Name: "Memory", Options: []core.ProductFacetOption{{Label: "32 GB"}}}}
	source := newRefinementSource(func(r core.ProductSearchRequest) []core.ProductFacet { return markedCatalog(groups, r.FacetSelections) })
	var request core.ProductRecommendationRequest
	if err := json.Unmarshal([]byte(`{"category_id":"123456","proceed":true,"max_price":2000000,"disable_affiliate":true,"answers":[{"question_id":"facet:Memory","choice":"32 GB"}]}`), &request); err != nil {
		t.Fatal(err)
	}
	result, err := New(source).Recommend(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.requests) != 6 || len(source.detailLimits) != 1 || result.Refinement.Issue != nil || len(result.Refinement.AppliedSelections) != 1 || len(result.Inspected) != 1 {
		t.Fatal("category recommendation did not refine then inspect")
	}
	for i, r := range source.requests {
		if r.CategoryID != "123456" || r.Query != "" || r.Page != 1 || !r.DisableAffiliate || len(r.FacetSelections) != min(i, 1) {
			t.Fatalf("category/selection lost at search %d", i)
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if json.Unmarshal(encoded, &wire) != nil || wire["category_id"] != "123456" || result.Query != "" {
		t.Fatal("recommendation output erased source category")
	}
}

type categoryPaginationSource struct{ *refinementSource }

func (s categoryPaginationSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	if r.Page > 1 {
		s.requests = append(s.requests, r)
		return coupangproducts.ParseSearchDocument([]byte(`{"items":[],"no_results":true}`))
	}
	return s.refinementSource.Search(ctx, r)
}

func TestRecommendCategoryPaginationDoesNotFallBackToQuery(t *testing.T) {
	source := categoryPaginationSource{newRefinementSource(func(core.ProductSearchRequest) []core.ProductFacet { return nil })}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{CategoryID: "123456", Proceed: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(source.requests) != 10 || len(source.detailLimits) != 1 || result.CategoryID != "123456" || result.Audit.DiscoveryStopReason != "source_results_exhausted" {
		t.Fatal("category pagination did not reach verified terminal pages and inspection")
	}
	for i, r := range source.requests {
		if r.CategoryID != "123456" || r.Query != "" || r.Page != i/5+1 {
			t.Fatalf("source scope lost at page request %d", i)
		}
	}
	if findRecommendationAction(result, "clarify_requirements", "category_scope_is_not_verified_required_conditions") == nil {
		t.Fatal("category membership was treated as verified requirements")
	}
}

func TestRecommendCategoryRetainedWhenRefinementNeedsInputOrFails(t *testing.T) {
	for _, mode := range []string{"question", "stale_choice", "denied"} {
		t.Run(mode, func(t *testing.T) {
			source := newRefinementSource(func(core.ProductSearchRequest) []core.ProductFacet {
				return []core.ProductFacet{{Name: "Memory", Options: []core.ProductFacetOption{{Label: "32 GB"}}}}
			})
			r := core.ProductRecommendationRequest{CategoryID: "123456"}
			wantCalls, wantStatus := 1, core.ProductRecommendationNeedsInput
			switch mode {
			case "stale_choice":
				r.Answers = []core.ProductRecommendationAnswer{{QuestionID: "facet:Memory", Choice: "missing"}}
			case "denied":
				r.Answers = []core.ProductRecommendationAnswer{{QuestionID: "facet:Memory", Choice: "32 GB"}}
				source.failAt, source.failure = 2, core.ErrBrowserAccessDenied
				wantCalls, wantStatus = 2, core.ProductRecommendationIncomplete
			}
			result, err := New(source).Recommend(context.Background(), r)
			if err != nil || result.CategoryID != "123456" || result.Query != "" || result.Status != wantStatus || len(source.requests) != wantCalls || len(source.detailLimits) != 0 || len(result.Discovered) != 1 {
				t.Fatal("partial category state lost or continued to details")
			}
			if len(result.Refinement.AppliedSelections) != 0 {
				t.Fatal("unverified category choice was applied")
			}
		})
	}
}

type categoryTransitionSource struct {
	*refinementSource
	driftAt int
}

type categoryPathSource struct{ *refinementSource }

func (s categoryPathSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	items, coverage, err := s.refinementSource.Search(ctx, r)
	for _, selection := range r.FacetSelections {
		if selection.Name != "카테고리" {
			continue
		}
		switch selection.Label {
		case "Parent":
			coverage.AppliedCategoryID = "234567"
		case "Child":
			coverage.AppliedCategoryID = "345678"
		}
	}
	return items, coverage, err
}

func TestRecommendCategoryPathRetainsOtherVerifiedFilters(t *testing.T) {
	groups := []core.ProductFacet{{Name: "카테고리", Options: []core.ProductFacetOption{{Label: "Parent"}, {Label: "Child"}}}, {Name: "Memory", Options: []core.ProductFacetOption{{Label: "32 GB"}}}}
	source := categoryPathSource{newRefinementSource(func(r core.ProductSearchRequest) []core.ProductFacet { return markedCatalog(groups, r.FacetSelections) })}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{CategoryID: "123456", Proceed: true, Answers: []core.ProductRecommendationAnswer{{QuestionID: "facet:카테고리", Choice: "Parent"}, {QuestionID: "facet:Memory", Choice: "32 GB"}, {QuestionID: "facet:카테고리", Choice: "Child"}}})
	if err != nil || result.Refinement.Issue != nil || len(result.Refinement.AppliedSelections) != 2 || len(result.Refinement.Steps) != 4 || len(source.requests) != 8 || len(source.detailLimits) != 1 {
		t.Fatal("category replacement lost constraints or did not reach research")
	}
	if result.AppliedCategoryID != "345678" || result.Refinement.AppliedSelections[0].Label != "Child" || result.Refinement.AppliedSelections[1].Name != "Memory" || result.Refinement.AppliedSelections[1].Label != "32 GB" {
		t.Fatal("active selections did not preserve memory alongside the final category")
	}
	for i, request := range source.requests {
		if i >= 3 && (len(request.FacetSelections) != 2 || request.FacetSelections[0].Label != "Child" || request.FacetSelections[1].Label != "32 GB") {
			t.Fatal("later source read did not replay the complete verified set")
		}
		if i >= 4 && request.CategoryID != "345678" {
			t.Fatal("comparison returned to an ancestor category")
		}
	}
	if result.Refinement.Steps[1].Selection.Label != "Parent" || result.Refinement.Steps[2].Selection.Name != "Memory" {
		t.Fatal("category replacement rewrote the earlier choice history")
	}
}

func TestRecommendDescendsObservedCategoryPath(t *testing.T) {
	for _, mode := range []string{"questions", "research", "missing_child", "denied_child"} {
		t.Run(mode, func(t *testing.T) {
			source := categoryPathSource{newRefinementSource(func(r core.ProductSearchRequest) []core.ProductFacet {
				labels := []string{"Parent"}
				if len(r.FacetSelections) > 0 {
					labels = []string{"Parent", "Child"}
					if r.FacetSelections[0].Label == "Child" {
						labels = []string{"Child"}
					}
				}
				if mode == "missing_child" {
					labels = []string{"Parent"}
				}
				groups := []core.ProductFacet{{Name: "카테고리"}, {Name: "Memory", Options: []core.ProductFacetOption{{Label: "32 GB"}}}}
				for _, label := range labels {
					groups[0].Options = append(groups[0].Options, core.ProductFacetOption{Label: label})
				}
				return markedCatalog(groups, r.FacetSelections)
			})}
			if mode == "denied_child" {
				source.failAt, source.failure = 3, core.ErrBrowserAccessDenied
			}
			r := core.ProductRecommendationRequest{CategoryID: "123456", Proceed: mode == "research", Answers: []core.ProductRecommendationAnswer{{QuestionID: "facet:카테고리", Choice: "Parent"}, {QuestionID: "facet:카테고리", Choice: "Child"}}}
			result, err := New(source).Recommend(context.Background(), r)
			if err != nil {
				t.Fatalf("ordered category navigation rejected: %v", err)
			}
			for i, request := range source.requests {
				wantCategory := "123456"
				if i == 2 {
					wantCategory = "234567"
				} else if i > 2 {
					wantCategory = "345678"
				}
				if request.CategoryID != wantCategory || len(request.FacetSelections) != min(i, 1) {
					t.Fatalf("search %d lost effective category scope or retained conflicting ancestors", i)
				}
			}
			wantCategory, wantLabel, wantSteps := "345678", "Child", 3
			if mode == "missing_child" || mode == "denied_child" {
				wantCategory, wantLabel, wantSteps = "234567", "Parent", 2
				if result.Refinement.Issue == nil || len(source.detailLimits) != 0 {
					t.Fatal("unverified child allowed further research")
				}
				wantCalls := 2
				if mode == "denied_child" {
					wantCalls = 3
				}
				if len(source.requests) != wantCalls {
					t.Fatal("child failure did not occur at the expected source-read step")
				}
			} else if result.Refinement.Issue != nil {
				t.Fatal("verified category chain stopped")
			}
			if result.CategoryID != "123456" || result.AppliedCategoryID != wantCategory || len(result.Refinement.Steps) != wantSteps || len(result.Refinement.AppliedSelections) != 1 || result.Refinement.AppliedSelections[0].Label != wantLabel {
				t.Fatal("category history and effective selections were not separated")
			}
			if result.Refinement.Steps[1].Selection.Label != "Parent" || result.Refinement.Steps[1].CategoryID != "234567" {
				t.Fatal("later selection rewrote earlier catalog evidence")
			}
			if mode == "research" && (len(source.requests) != 7 || len(source.detailLimits) != 1) {
				t.Fatal("category chain did not reach comparisons and detail")
			}
			if mode == "questions" && (len(source.requests) != 3 || result.Status != core.ProductRecommendationNeedsInput) {
				t.Fatal("optional questions lost after category navigation")
			}
		})
	}
}

func (s categoryTransitionSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	items, coverage, err := s.refinementSource.Search(ctx, r)
	if len(r.FacetSelections) > 0 && r.CategoryID == "123456" {
		coverage.AppliedCategoryID = "654321"
	}
	if len(s.requests) == s.driftAt {
		coverage.AppliedCategoryID = "777777"
	}
	return items, coverage, err
}

func TestRecommendContinuesInVerifiedSidebarCategory(t *testing.T) {
	for _, mode := range []string{"research", "later_choice_missing", "later_read_failed", "comparison_drift"} {
		t.Run(mode, func(t *testing.T) {
			groups := []core.ProductFacet{{Name: "카테고리", Options: []core.ProductFacetOption{{Label: "Synthetic child"}}}, {Name: "Memory", Options: []core.ProductFacetOption{{Label: "32 GB"}}}}
			source := categoryTransitionSource{refinementSource: newRefinementSource(func(r core.ProductSearchRequest) []core.ProductFacet { return markedCatalog(groups, r.FacetSelections) })}
			request := core.ProductRecommendationRequest{CategoryID: "123456", Proceed: true, Answers: []core.ProductRecommendationAnswer{{QuestionID: "facet:카테고리", Choice: "Synthetic child"}, {QuestionID: "facet:Memory", Choice: "32 GB"}}}
			if mode == "later_choice_missing" {
				request.Answers[1].Choice = "Missing"
			}
			if mode == "later_read_failed" {
				source.failAt = 3
				source.failure = core.ErrBrowserAccessDenied
			}
			if mode == "comparison_drift" {
				source.driftAt = 4
			}
			result, err := New(source).Recommend(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			for i, r := range source.requests {
				expected := "123456"
				if i >= 2 {
					expected = "654321"
				}
				if r.CategoryID != expected {
					t.Fatalf("search %d returned to the old category: got %s want %s", i, r.CategoryID, expected)
				}
			}
			data, _ := json.Marshal(result)
			var wire map[string]any
			_ = json.Unmarshal(data, &wire)
			if wire["category_id"] != "123456" || wire["applied_category_id"] != "654321" {
				t.Fatal("requested and verified category scope not distinguished")
			}
			if mode == "research" && (len(source.requests) != 7 || len(source.detailLimits) != 1 || result.Refinement.Issue != nil) {
				t.Fatal("research did not continue through all sorts and details")
			}
			if (mode == "later_choice_missing" || mode == "later_read_failed") && (len(source.detailLimits) != 0 || result.Refinement.Issue == nil) {
				t.Fatal("unverified refinement proceeded to details")
			}
			if mode == "comparison_drift" && (len(source.requests) != 4 || len(source.detailLimits) != 0 || result.Audit.DiscoveryStopReason != "source_category_changed" || result.Status != core.ProductRecommendationIncomplete) {
				t.Fatal("comparison scope drift did not stop safely")
			}
			for i, step := range result.Refinement.Steps {
				expected := "123456"
				if i > 0 {
					expected = "654321"
				}
				if step.CategoryID != expected {
					t.Fatal("catalog observation lost its category scope")
				}
			}
		})
	}
}
