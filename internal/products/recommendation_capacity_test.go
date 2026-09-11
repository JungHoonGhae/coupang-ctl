package products

import (
	"context"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type capacitySelectionSource struct{ selectionSource }

func (s *capacitySelectionSource) Inspect(ctx context.Context, request core.ProductInspectRequest) (core.ProductInspection, error) {
	p, err := s.selectionSource.Inspect(ctx, request)
	if err != nil {
		return p, err
	}
	label, value := "RAM용량", "32GB"
	switch request.ProductID {
	case "1000":
		value = "16GB"
	case "1001":
		label, value = "RAM용량 × 저장용량", "64GB × 1TB"
	}
	p.SelectedAttributes = []core.ProductSelectedAttribute{{Name: label, Value: value}}
	p.Coverage.ObservedFields = []string{"selected_attributes"}
	p.FieldEvidence = []core.ProductFieldEvidence{{Field: "selected_attributes", Source: "product_options", Locator: "options.optionRows.selectedAttribute", Method: "native_field", Provenance: "observed", Scope: "selected_option", Reference: p.Product.Reference, CapturedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}}
	return p, nil
}

func TestRecommendationCapacityRetainsMetUnmetAndUnverifiedOptions(t *testing.T) {
	s := &capacitySelectionSource{selectionSource: *newSelectionSource(3)}
	for i, p := range s.items {
		p.Reference.ItemID = p.Reference.ProductID + "1"
		p.Reference.VendorItemID = p.Reference.ProductID + "2"
		p.Name = "Synthetic RAM 64GB SSD 2TB" // Deliberately misleading title.
		s.items[i] = withSyntheticPriceEvidence(p)
	}
	minimum := int64(32)
	result, err := New(s).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, RequiredConditions: []core.ProductRecommendationCondition{{ID: "ram", Field: "specifications.memory_gb", Operator: "gte", Integer: &minimum}}, ComparisonAxes: []core.ProductPreferenceAxis{{Field: "specifications.memory_gb", Direction: "maximize"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Inspected) != 3 || len(result.Candidates) != 1 || result.Candidates[0].Product.Reference.ProductID != "1002" {
		t.Fatal("capacity selection lost investigation or accepted ambiguous options")
	}
	for i, want := range []string{"conditions_unmet", "conditions_unverified", "conditions_met"} {
		p := result.Inspected[i]
		if p.Conclusion == nil || p.Conclusion.Outcome != want {
			t.Fatalf("option %d wrong conclusion: %+v", i, p.Conclusion)
		}
		if i == 1 && (p.Conditions[0].DerivedInteger != nil || p.ExclusionReason != "required_condition_unverified") {
			t.Fatal("compound title heuristic promoted")
		}
		if i < 2 && p.Conclusion.ComparisonReason != "not_compared_due_to_candidate_exclusion" {
			t.Fatal("requested comparison incorrectly reported as not requested")
		}
	}
	met := result.Candidates[0]
	if met.Status != core.ProductRecommendationNeedsVerification || len(met.ComparisonValues) != 1 || met.ComparisonValues[0].DerivedInteger == nil || *met.ComparisonValues[0].DerivedInteger != 32 {
		t.Fatal("condition scope or comparison value lost")
	}
}
