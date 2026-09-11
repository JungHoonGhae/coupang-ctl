package products

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func findRecommendationAction(result core.ProductRecommendationResult, kind, reason string) *core.ProductRecommendationNextAction {
	for i := range result.NextActions {
		if result.NextActions[i].Kind == kind && result.NextActions[i].Reason == reason {
			return &result.NextActions[i]
		}
	}
	return nil
}

func TestRecommendationConclusionsSeparateResearchFromConditions(t *testing.T) {
	value := true
	condition := core.ProductRecommendationCondition{ID: "shipping", Field: "free_shipping", Operator: "eq", Boolean: &value}
	source := &conditionSelectionSource{selectionSource: *newSelectionSource(3)}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, RequiredConditions: []core.ProductRecommendationCondition{condition}, SearchPageLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ProductRecommendationIncomplete {
		t.Fatal("research uncertainty disappeared")
	}
	for i, outcome := range []string{"conditions_unmet", "conditions_unverified", "conditions_met"} {
		c := result.Inspected[i]
		if c.Conclusion == nil || c.Conclusion.Outcome != outcome || c.Status != core.ProductRecommendationNeedsVerification {
			t.Fatal("scoped conclusion confused with overall fitness")
		}
	}
	if result.Candidates[0].Conclusion.Outcome != "conditions_met" {
		t.Fatal("supported limited conclusion lost")
	}
	action := findRecommendationAction(result, "inspect_evidence", "required_condition_unverified")
	if action == nil || !reflect.DeepEqual(action.References, []core.ProductReference{{ProductID: "1001"}}) || !reflect.DeepEqual(action.ConditionIDs, []string{"shipping"}) || !reflect.DeepEqual(action.Fields, []string{"free_shipping"}) {
		t.Fatal("next read did not identify the unresolved condition and candidate")
	}
}

func TestRecommendationNextActionsUnaffectedByPresentationCap(t *testing.T) {
	var previous []core.ProductRecommendationNextAction
	for _, limit := range []int{0, 1, 3} {
		source := &preferenceSelectionSource{selectionSource: *newSelectionSource(9)}
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxItems: limit, MaxPrice: 2000, SearchPageLimit: 1, ComparisonAxes: []core.ProductPreferenceAxis{{Field: "price.current_amount", Direction: "minimize"}, {Field: "rating", Direction: "maximize"}}})
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil && !reflect.DeepEqual(previous, result.NextActions) {
			t.Fatal("display cap changed next research actions")
		}
		previous = result.NextActions
		question := findRecommendationAction(result, "clarify_preference", "observed_tradeoff_requires_user_priority")
		if question == nil || !slices.Contains(question.Fields, "price.current_amount") || !slices.Contains(question.Fields, "rating") {
			t.Fatal("known tradeoff lacks targeted preference question")
		}
		if slices.Contains(question.References, core.ProductReference{ProductID: "1007"}) {
			t.Fatal("dominated candidate returned as a choice requiring a question")
		}
		if findRecommendationAction(result, "inspect_evidence", "comparison_evidence_or_scope_unresolved") == nil {
			t.Fatal("missing comparison value hidden")
		}
		for _, c := range result.Inspected {
			if c.Conclusion == nil || c.Conclusion.Outcome != "conditions_met" {
				t.Fatal("limited budget conclusion lost due to comparison uncertainty or exclusion")
			}
		}
	}
}

func TestRecommendationCompleteDoesNotInventSuitabilityOrSufficiency(t *testing.T) {
	product := withSyntheticPriceEvidence(syntheticRecommendationProduct())
	source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: core.ProductInspection{Product: product}}}
	for _, budget := range []int64{0, 2000} {
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxPrice: budget})
		if err != nil {
			t.Fatal(err)
		}
		want := "conditions_not_declared"
		if budget != 0 {
			want = "conditions_met"
		}
		if result.Status != core.ProductRecommendationComplete || result.Candidates[0].Conclusion.Outcome != want || result.Candidates[0].Status != core.ProductRecommendationNeedsVerification {
			t.Fatal("execution state became a claim of suitability")
		}
		if (findRecommendationAction(result, "clarify_requirements", "query_text_is_not_verified_required_conditions") != nil) != (budget == 0) {
			t.Fatal("requirements query detached from actual declared conditions")
		}
		if findRecommendationAction(result, "continue_discovery", "source_results_exhausted") != nil {
			t.Fatal("explicit source exhaustion prompted unsupported continued pagination")
		}
	}
}

func TestRecommendationNextActionsPreserveUninspectedExactOptions(t *testing.T) {
	source := newOptionSelectionSource(22)
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1, MaxItems: 1})
	if err != nil {
		t.Fatal(err)
	}
	action := findRecommendationAction(result, "inspect_details", "discovered_reference_has_no_matching_detail")
	if action == nil || len(action.References) != 3 {
		t.Fatal("uninspected options lost")
	}
	for _, ref := range action.References {
		if slices.Contains(source.requests, ref) {
			t.Fatal("already-inspected option rescheduled")
		}
	}
}

func TestRecommendationNextActionsDoNotRetryDisqualifiedConditions(t *testing.T) {
	product := withSyntheticPriceEvidence(syntheticRecommendationProduct())
	detail := product
	detail.Price.CurrentAmount = 9000
	source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: core.ProductInspection{Product: detail}}}
	wanted := true
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{
		Query: "synthetic", Proceed: true, MaxPrice: 2000,
		RequiredConditions: []core.ProductRecommendationCondition{{ID: "shipping", Field: "free_shipping", Operator: "eq", Boolean: &wanted}},
	})
	if err != nil {
		t.Fatal(err)
	}
	conclusion := result.Inspected[0].Conclusion
	if conclusion.Outcome != "conditions_unmet" || len(conclusion.UnverifiedConditionIDs) != 1 {
		t.Fatal("unknown condition lost on disqualified option")
	}
	if findRecommendationAction(result, "inspect_evidence", "required_condition_unverified") != nil {
		t.Fatal("futile evidence lookup for already disqualified option")
	}
	if findRecommendationAction(result, "reconsider_conditions", "inspected_options_do_not_meet_declared_conditions") == nil {
		t.Fatal("no way forward after source exhaustion and verified rejection")
	}
}

func TestRecommendationNoResultsSuggestsSearchRefinementNotInventedFit(t *testing.T) {
	result, err := New(recommendationSource{}).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ProductRecommendationNoMatches || len(result.Candidates) != 0 || len(result.NextActions) != 1 || result.NextActions[0].Kind != "refine_search" {
		t.Fatal("explicit empty source turned into a fabricated recommendation")
	}
}
