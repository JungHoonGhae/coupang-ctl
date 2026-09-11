package core

import (
	"reflect"
	"testing"
)

func TestConditionConclusionPreservesScopeAndUncertainty(t *testing.T) {
	price := ProductRecommendationCondition{ID: "budget", Field: "price.current_amount", Operator: "lte", Integer: conditionTestPointer(int64(2000))}
	shipping := ProductRecommendationCondition{ID: "shipping", Field: "free_shipping", Operator: "eq", Boolean: conditionTestPointer(true)}
	for _, test := range []struct {
		name                string
		amount              int64
		known               bool
		conditions          []ProductRecommendationCondition
		outcome             string
		met, unmet, unknown []string
	}{
		{"none", 1200, true, nil, "conditions_not_declared", nil, nil, nil},
		{"known_zero", 0, true, []ProductRecommendationCondition{price}, "conditions_met", []string{"budget"}, nil, nil},
		{"unknown_zero", 0, false, []ProductRecommendationCondition{price}, "conditions_unverified", nil, nil, []string{"budget"}},
		{"unmet", 3000, true, []ProductRecommendationCondition{price}, "conditions_unmet", nil, []string{"budget"}, nil},
		{"mixed_unknown", 1200, true, []ProductRecommendationCondition{price, shipping}, "conditions_unverified", []string{"budget"}, nil, []string{"shipping"}},
		{"unmet_and_unknown", 3000, true, []ProductRecommendationCondition{price, shipping}, "conditions_unmet", nil, []string{"budget"}, []string{"shipping"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			inspection := conditionTestInspection("price.current_amount")
			inspection.Product.Price.CurrentAmount = test.amount
			if !test.known {
				inspection.Product.FieldEvidence = nil
			}
			result := ConcludeProductConditions(inspection, test.conditions)
			if result.Outcome != test.outcome || result.Scope != "declared_conditions_on_observed_reference" || len(result.Limitations) == 0 {
				t.Fatalf("scope or conclusion incorrect: %+v", result)
			}
			for _, pair := range [][2][]string{{result.SupportingConditionIDs, test.met}, {result.UnmetConditionIDs, test.unmet}, {result.UnverifiedConditionIDs, test.unknown}} {
				if !reflect.DeepEqual(append([]string{}, pair[0]...), append([]string{}, pair[1]...)) {
					t.Fatal("condition basis missing")
				}
			}
		})
	}
}
