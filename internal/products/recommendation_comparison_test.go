package products

import (
	"context"
	"strconv"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type preferenceSelectionSource struct{ selectionSource }

func (s *preferenceSelectionSource) Inspect(ctx context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	p, err := s.selectionSource.Inspect(ctx, r)
	if err != nil {
		return p, err
	}
	id, _ := strconv.Atoi(r.ProductID)
	index := id - 1000
	p.Product.Price.CurrentAmount = int64((index + 1) * 100)
	p.Product.Rating = 1 + float64(index)/2
	if index == 7 {
		p.Product.Rating = 0
	}
	if index != 8 {
		e := p.Product.FieldEvidence[0]
		e.Field = "rating"
		e.Locator = "synthetic.rating"
		e.Scope = "product_page"
		e.Reference = core.ProductReference{ProductID: r.ProductID}
		p.Product.FieldEvidence = append(p.Product.FieldEvidence, e)
		p.Product.ObservedFields = append(p.Product.ObservedFields, "rating")
	}
	return p, nil
}

func TestRecommendationRetainsAllRequestedTradeoffsWithoutFixedSlots(t *testing.T) {
	axes := []core.ProductPreferenceAxis{{Field: "price.current_amount", Direction: "minimize"}, {Field: "rating", Direction: "maximize"}}
	for _, limit := range []int{0, 3} {
		source := &preferenceSelectionSource{selectionSource: *newSelectionSource(9)}
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, ComparisonAxes: axes, MaxItems: limit})
		if err != nil {
			t.Fatal(err)
		}
		want := 8
		if limit > 0 {
			want = limit
		}
		if len(source.inspected) != 9 || len(result.Inspected) != 9 || len(result.Candidates) != want || result.Audit.ComparisonCandidates != 8 || result.Audit.ComparisonExcludedCandidates != 1 || result.Audit.PresentationOmittedCandidates != 8-want {
			t.Fatalf("comparison lost coverage: %+v", result.Audit)
		}
		if result.Inspected[7].ExclusionReason != "dominated_on_requested_axes" || result.Inspected[8].ExclusionReason != "" || result.Inspected[8].ComparisonReason != "comparison_evidence_or_scope_unresolved" {
			t.Fatal("dominated and unknown candidates conflated")
		}
		for _, candidate := range result.Inspected[:7] {
			if candidate.ExclusionReason != "" || len(candidate.ComparisonValues) != 2 || candidate.Status != core.ProductRecommendationNeedsVerification {
				t.Fatal("tradeoff lost or suitability invented")
			}
		}
		if result.Status != core.ProductRecommendationIncomplete {
			t.Fatal("unknown comparison claimed complete")
		}
	}
}

func TestRecommendationWithoutPreferenceAxesPreservesCandidates(t *testing.T) {
	source := &preferenceSelectionSource{selectionSource: *newSelectionSource(9)}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 9 || result.Audit.ComparisonExcludedCandidates != 0 {
		t.Fatal("unrequested axes changed selection")
	}
}
