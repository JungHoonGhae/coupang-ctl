package products

import (
	"context"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangproducts "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
)

type filteredRecommendationPages struct {
	selectionSource
	pages []int
}

func (s *filteredRecommendationPages) Search(_ context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	s.pages = append(s.pages, r.Page)
	product := s.items[0]
	if r.Page == 1 {
		product.Price.CurrentAmount = 9000
	}
	return []core.ProductCard{product}, core.ProductCoverage{}, nil
}

func TestRecommendationDoesNotExhaustSourceAfterLocalBudgetFilter(t *testing.T) {
	source := &filteredRecommendationPages{selectionSource: *newSelectionSource(1)}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", MaxPrice: 2000, Proceed: true, SearchPageLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 {
		t.Fatal("valid candidate on next page lost after local filter emptied page one")
	}
}

func TestRecommendationUnknownPaginationIsNotSourceExhaustion(t *testing.T) {
	for _, count := range []int{0, 1} {
		result, err := New(newSelectionSource(count)).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if result.Audit.DiscoveryStopReason != "search_page_limit_reached" || result.Status != core.ProductRecommendationIncomplete || result.Audit.SearchesRun != 5 {
			t.Fatalf("unverified end claimed complete: stop=%s status=%s", result.Audit.DiscoveryStopReason, result.Status)
		}
	}
}

type explicitTerminalPages struct {
	recommendationSource
	pages []int
}

func (s *explicitTerminalPages) Search(_ context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	s.pages = append(s.pages, r.Page)
	if r.Page > 1 {
		return coupangproducts.ParseSearchDocument([]byte(`{"items":[],"no_results":true}`))
	}
	// A forged coverage summary cannot override the actual nonempty envelope.
	return coupangproducts.ParseSearchDocument([]byte(`{"items":[{"product_id":"123","item_id":"456","vendor_item_id":"789","name":"Synthetic bowl","url":"https://www.coupang.com/vp/products/123?itemId=456&vendorItemId=789","sponsored":false,"observed_fields":["sponsored"]}],"coverage":{"source_no_results":true}}`))
}

func TestRecommendationUsesParsedSourceEndAcrossAllSearchAxes(t *testing.T) {
	source := &explicitTerminalPages{recommendationSource: recommendationSource{syntheticSource: syntheticSource{inspection: core.ProductInspection{Product: syntheticRecommendationProduct()}}}}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(source.pages) != 10 || result.Audit.SearchPagesRead != 10 || result.Audit.DiscoveryStopReason != "source_results_exhausted" || result.Status != core.ProductRecommendationComplete || len(result.Candidates) != 1 {
		t.Fatalf("source termination did not reach recommendation: %+v", result.Audit)
	}
	for _, page := range source.pages {
		if page > 2 {
			t.Fatal("continued after explicit terminal page")
		}
	}
}

type contradictoryEmptySource struct{ syntheticSource }

func (s contradictoryEmptySource) Search(context.Context, core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	return s.items, core.ProductCoverage{SourceNoResults: true}, nil
}

func TestSearchRejectsContradictoryEmptyEvidenceBeforeFiltering(t *testing.T) {
	product := withSyntheticPriceEvidence(syntheticRecommendationProduct())
	_, err := New(contradictoryEmptySource{syntheticSource: syntheticSource{items: []core.ProductCard{product}}}).Search(context.Background(), core.ProductSearchRequest{Query: "synthetic", MaxPrice: 1})
	if err == nil {
		t.Fatal("filter erased contradictory source items and validated no-results")
	}
}

func TestRecommendationDiscoveryTargetIsNotEvidenceSufficiency(t *testing.T) {
	result, err := New(newSelectionSource(8)).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, DiscoveryTarget: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ProductRecommendationIncomplete || result.Audit.DiscoveryStopReason != "unique_model_target_reached" {
		t.Fatal("operational discovery count was presented as sufficient")
	}
}
