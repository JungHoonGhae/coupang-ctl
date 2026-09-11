package products

import (
	"context"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type conditionSelectionSource struct{ selectionSource }

func (s *conditionSelectionSource) Inspect(ctx context.Context, request core.ProductInspectRequest) (core.ProductInspection, error) {
	p, err := s.selectionSource.Inspect(ctx, request)
	if err != nil {
		return p, err
	}
	p.Product.FreeShipping = request.ProductID != "1000"
	p.Product.ObservedFields = append(p.Product.ObservedFields, "free_shipping")
	if request.ProductID != "1001" {
		e := p.Product.FieldEvidence[0]
		e.Field = "free_shipping"
		e.Locator = "synthetic.delivery.free_shipping"
		p.Product.FieldEvidence = append(p.Product.FieldEvidence, e)
	}
	return p, nil
}

func TestRecommendationFiltersExplicitConditionsWithoutDiscardingUnknownEvidence(t *testing.T) {
	value := true
	condition := core.ProductRecommendationCondition{ID: "shipping", Field: "free_shipping", Operator: "eq", Boolean: &value}
	source := &conditionSelectionSource{selectionSource: *newSelectionSource(3)}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxItems: 1, RequiredConditions: []core.ProductRecommendationCondition{condition}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Product.Reference.ProductID != "1002" || result.Candidates[0].Status != core.ProductRecommendationNeedsVerification {
		t.Fatal("condition mismatch/unknown returned as verified recommendation")
	}
	if len(result.Inspected) != 3 {
		t.Fatal("lost condition investigation")
	}
	for i, want := range []core.ProductConditionStatus{core.ProductConditionUnmet, core.ProductConditionUnknown, core.ProductConditionMet} {
		if len(result.Inspected[i].Conditions) != 1 || result.Inspected[i].Conditions[0].Status != want {
			t.Fatalf("condition state %d lost", i)
		}
	}
	if result.Inspected[0].ExclusionReason != "required_condition_unmet" || result.Inspected[1].ExclusionReason != "required_condition_unverified" {
		t.Fatal("failed and unknown conditions conflated")
	}
}

func TestRecommendationBudgetProducesExactConditionEvidence(t *testing.T) {
	source := newSelectionSource(1)
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxPrice: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 || len(result.Candidates[0].Conditions) != 1 {
		t.Fatal("budget condition missing")
	}
	r := result.Candidates[0].Conditions[0]
	if r.Condition.ID != "max_price" || r.Condition.Integer == nil || *r.Condition.Integer != 2000 || r.ObservedInteger == nil || *r.ObservedInteger != 1200 || r.Status != core.ProductConditionMet || len(r.Evidence) != 1 {
		t.Fatal("budget threshold, observed amount or evidence lost")
	}
}
