package core

// A conclusion answers only the declared conditions, independently of whether
// the research run exhausted its source or budget. It is not overall fitness.
type ProductRecommendationConclusion struct {
	Outcome                string   `json:"outcome" jsonschema:"conditions_met, conditions_unmet, conditions_unverified, or conditions_not_declared; never a claim of overall suitability"`
	Scope                  string   `json:"scope"`
	SupportingConditionIDs []string `json:"supporting_condition_ids"`
	UnmetConditionIDs      []string `json:"unmet_condition_ids"`
	UnverifiedConditionIDs []string `json:"unverified_condition_ids"`
	ComparisonReason       string   `json:"comparison_reason" jsonschema:"Separate preference-comparison reason; meeting required conditions does not imply being preferred"`
	Limitations            []string `json:"limitations"`
}

// ConcludeProductConditions re-evaluates evidence instead of trusting a caller's
// supplied assessment status. The returned IDs link to condition assessments.
func ConcludeProductConditions(inspection ProductInspection, conditions []ProductRecommendationCondition) ProductRecommendationConclusion {
	result := ProductRecommendationConclusion{
		Outcome: "conditions_not_declared", Scope: "declared_conditions_on_observed_reference",
		SupportingConditionIDs: []string{}, UnmetConditionIDs: []string{}, UnverifiedConditionIDs: []string{},
		Limitations: []string{"does_not_establish_category_specific_fit", "does_not_establish_market_wide_optimality"},
	}
	for _, condition := range conditions {
		switch AssessProductCondition(inspection, condition).Status {
		case ProductConditionMet:
			result.SupportingConditionIDs = append(result.SupportingConditionIDs, condition.ID)
		case ProductConditionUnmet:
			result.UnmetConditionIDs = append(result.UnmetConditionIDs, condition.ID)
		default:
			result.UnverifiedConditionIDs = append(result.UnverifiedConditionIDs, condition.ID)
		}
	}
	switch {
	case len(result.UnmetConditionIDs) > 0:
		result.Outcome = "conditions_unmet"
	case len(result.UnverifiedConditionIDs) > 0:
		result.Outcome = "conditions_unverified"
	case len(conditions) > 0:
		result.Outcome = "conditions_met"
	}
	return result
}

// Suggested read-only investigation or conversation, not an executable browser
// command, permission request, purchase action, or automatic retry instruction.
type ProductRecommendationNextAction struct {
	Kind         string             `json:"kind" jsonschema:"Read-only next-step category; restore_source_access means access recovery in the selected session must precede further source reads, not an automatic retry or profile change"`
	Reason       string             `json:"reason"`
	References   []ProductReference `json:"references"`
	ConditionIDs []string           `json:"condition_ids"`
	Fields       []string           `json:"fields"`
}
