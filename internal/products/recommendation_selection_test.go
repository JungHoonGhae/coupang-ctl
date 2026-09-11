package products

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type selectionSource struct {
	syntheticSource
	inspected  []string
	overBudget int
}

func (s *selectionSource) Search(_ context.Context, request core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	items := slices.Clone(s.items)
	if request.Sort == core.ProductSortLatest {
		slices.Reverse(items)
	}
	return items, core.ProductCoverage{}, nil
}

func newSelectionSource(count int) *selectionSource {
	s := &selectionSource{}
	for i := 0; i < count; i++ {
		p := syntheticRecommendationProduct()
		p.Reference = core.ProductReference{ProductID: fmt.Sprint(1000 + i)}
		p.URL = "https://www.coupang.com/vp/products/" + p.Reference.ProductID
		p = withSyntheticPriceEvidence(p)
		s.items = append(s.items, p)
	}
	return s
}

func (s *selectionSource) Inspect(_ context.Context, request core.ProductInspectRequest) (core.ProductInspection, error) {
	s.inspected = append(s.inspected, request.ProductID)
	for i, p := range s.items {
		if p.Reference.ProductID == request.ProductID {
			if i < s.overBudget {
				p.Price.CurrentAmount = 9000
			}
			return core.ProductInspection{Product: p}, nil
		}
	}
	return core.ProductInspection{}, fmt.Errorf("synthetic candidate missing")
}

func TestRecommendationPresentationDoesNotLimitInvestigation(t *testing.T) {
	var previous []string
	for _, limit := range []int{0, 1, 3, 6} {
		source := newSelectionSource(8)
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxItems: limit})
		if err != nil {
			t.Fatal(err)
		}
		if len(source.inspected) != 8 {
			t.Fatalf("display limit %d restricted investigation to %d", limit, len(source.inspected))
		}
		if previous != nil && !reflect.DeepEqual(previous, source.inspected) {
			t.Fatal("display changed inspection order")
		}
		previous = source.inspected
		want := limit
		if want == 0 {
			want = 8
		}
		if len(result.Candidates) != want {
			t.Fatalf("displayed %d want %d", len(result.Candidates), want)
		}
		if len(result.Inspected) != 8 || result.Audit.InspectionAttempts != 8 || result.Audit.DetailsInspected != 8 || result.Audit.ComparisonCandidates != 8 || result.Audit.DisplayedCandidates != want || result.Audit.PresentationOmittedCandidates != 8-want || result.Audit.UninspectedModelFamilies != 0 {
			t.Fatalf("lost investigation or presentation coverage: %+v", result.Audit)
		}
		for _, candidate := range result.Candidates {
			if candidate.Status != core.ProductRecommendationNeedsVerification {
				t.Fatal("presentation promoted suitability")
			}
		}
	}
}

func TestRecommendationConsidersCandidateAfterFirstFiveFailBudget(t *testing.T) {
	source := newSelectionSource(6)
	source.overBudget = 5
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxPrice: 2000, MaxItems: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Product.Reference.ProductID != "1005" {
		t.Fatal("valid later candidate lost after five exclusions")
	}
	if len(result.Inspected) != 6 {
		t.Fatal("excluded evidence was discarded")
	}
	for _, candidate := range result.Inspected[:5] {
		if candidate.ExclusionReason != "over_budget" {
			t.Fatal("exclusion reason missing")
		}
	}
}

func TestRecommendationInspectionBudgetIsNotSufficiency(t *testing.T) {
	for _, cap := range []int{1, 200} {
		source := newSelectionSource(24)
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1, ReviewCap: cap})
		if err != nil {
			t.Fatal(err)
		}
		want, reason := 20, "inspection_budget_reached"
		if cap == 1 {
			want, reason = 1, "review_budget_reached"
		}
		if len(source.inspected) != want || result.Audit.UninspectedModelFamilies != 24-want || result.Audit.InspectionStopReason != reason || result.Status != core.ProductRecommendationIncomplete {
			t.Fatalf("operational ceiling became sufficient: %+v status=%s", result.Audit, result.Status)
		}
	}
}

func TestRecommendationDisplayCapDoesNotChangeOptInHistoryScope(t *testing.T) {
	source := newSelectionSource(8)
	history := &syntheticPurchaseHistory{status: "available"}
	result, err := New(source).WithPurchaseHistory(history).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxItems: 1, UsePurchaseHistory: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.refs) != 8 || len(result.Candidates) != 1 {
		t.Fatal("presentation truncated history scope")
	}
}
