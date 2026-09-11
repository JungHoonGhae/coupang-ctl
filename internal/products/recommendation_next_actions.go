package products

import (
	"slices"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// Run before the presentation cap: a hidden candidate must not lose its
// conclusion or make unresolved research appear finished.
func finalizeRecommendationConclusions(result *core.ProductRecommendationResult, conditions []core.ProductRecommendationCondition) {
	byReference := make(map[core.ProductReference]*core.ProductRecommendationConclusion)
	retained := make(map[core.ProductReference]bool)
	for i := range result.Inspected {
		candidate := &result.Inspected[i]
		conclusion := core.ConcludeProductConditions(candidate.Inspection, conditions)
		conclusion.ComparisonReason = candidate.ComparisonReason
		candidate.Conclusion = &conclusion
		byReference[candidate.Product.Reference] = &conclusion
	}
	for i := range result.Candidates {
		result.Candidates[i].Conclusion = byReference[result.Candidates[i].Product.Reference]
		retained[result.Candidates[i].Product.Reference] = true
	}

	result.NextActions = []core.ProductRecommendationNextAction{}
	accessStop := ""
	for _, reason := range []string{result.Audit.DiscoveryStopReason, result.Audit.InspectionStopReason} {
		if reason == "source_access_denied" || reason == "source_authentication_required" {
			accessStop = reason
			break
		}
	}
	add := func(kind, reason string, refs []core.ProductReference, ids, fields []string) {
		// These remain unfulfilled investigations, not instructions to repeat
		// reads in a session that has just denied access. Their evidence stays
		// in Discovered/Inspected; recovery must precede another source request.
		if accessStop != "" && (kind == "inspect_evidence" || kind == "inspect_details" || kind == "continue_discovery") {
			return
		}
		index := -1
		for i, action := range result.NextActions {
			if action.Kind == kind && action.Reason == reason {
				index = i
				break
			}
		}
		if index < 0 {
			index = len(result.NextActions)
			result.NextActions = append(result.NextActions, core.ProductRecommendationNextAction{
				Kind: kind, Reason: reason, References: []core.ProductReference{}, ConditionIDs: []string{}, Fields: []string{},
			})
		}
		action := &result.NextActions[index]
		for _, ref := range refs {
			if !slices.Contains(action.References, ref) {
				action.References = append(action.References, ref)
			}
		}
		for _, id := range ids {
			if !slices.Contains(action.ConditionIDs, id) {
				action.ConditionIDs = append(action.ConditionIDs, id)
			}
		}
		for _, field := range fields {
			if !slices.Contains(action.Fields, field) {
				action.Fields = append(action.Fields, field)
			}
		}
	}
	if accessStop != "" {
		add("restore_source_access", accessStop, nil, nil, nil)
	}
	if result.Audit.DiscoveryStopReason == "source_filter_unverified" {
		add("rediscover_filters", "source_filter_unverified", nil, nil, nil)
		return
	}
	for _, candidate := range result.Inspected {
		// Resolving unknowns on an already-disqualified option cannot make it
		// satisfy all current conditions. Keep its evidence, but don't prioritize
		// that unnecessary read as the user's next task.
		if candidate.Conclusion.Outcome == "conditions_unmet" {
			continue
		}
		refs := []core.ProductReference{candidate.Product.Reference}
		if fields := candidate.Inspection.Coverage.BudgetOmittedFields; len(fields) > 0 {
			add("inspect_evidence", "document_budget_omitted_fields", refs, nil, fields)
		}
		for _, condition := range candidate.Conditions {
			if condition.Status == core.ProductConditionUnknown {
				add("inspect_evidence", "required_condition_unverified", refs, []string{condition.Condition.ID}, []string{condition.Condition.Field})
			}
		}
		if candidate.ExclusionReason != "" {
			continue
		}
		for _, value := range candidate.ComparisonValues {
			if len(value.MissingEvidence) > 0 {
				add("inspect_evidence", "comparison_evidence_or_scope_unresolved", refs, nil, []string{value.Field})
			}
		}
		for _, comparison := range candidate.Comparisons {
			if !retained[comparison.Other] {
				continue
			}
			pair := []core.ProductReference{candidate.Product.Reference, comparison.Other}
			if comparison.Relation == "unknown" {
				add("inspect_evidence", "comparison_evidence_or_scope_unresolved", pair, nil, comparison.UnknownOn)
			}
			if comparison.Relation == "tradeoff" {
				fields := append(slices.Clone(comparison.BetterOn), comparison.WorseOn...)
				add("clarify_preference", "observed_tradeoff_requires_user_priority", pair, nil, fields)
			}
		}
	}
	for _, group := range result.Discovered {
		for _, option := range group.Options {
			ref, inspected := option.Reference, false
			for _, candidate := range result.Inspected {
				actual := candidate.Product.Reference
				if ref.ProductID == actual.ProductID && (ref.ItemID == "" || ref.ItemID == actual.ItemID) && (ref.VendorItemID == "" || ref.VendorItemID == actual.VendorItemID) {
					inspected = true
					break
				}
			}
			if !inspected {
				add("inspect_details", "discovered_reference_has_no_matching_detail", []core.ProductReference{ref}, nil, nil)
			}
		}
	}
	if len(conditions) == 0 && len(result.Inspected) > 0 {
		reason := "query_text_is_not_verified_required_conditions"
		if result.CategoryID != "" {
			reason = "category_scope_is_not_verified_required_conditions"
		}
		add("clarify_requirements", reason, nil, nil, nil)
	}
	if result.PurchaseContext != nil && result.PurchaseContext.Status != "available" {
		add("review_purchase_coverage", "requested_history_is_partial_or_unavailable", nil, nil, nil)
	}
	if reason := result.Audit.DiscoveryStopReason; reason != "source_results_exhausted" && reason != "" {
		add("continue_discovery", reason, nil, nil, nil)
	}
	if len(result.Candidates) == 0 && result.Audit.DiscoveryStopReason == "source_results_exhausted" && result.Audit.UninspectedExactOptions == 0 {
		if len(result.Inspected) == 0 {
			add("refine_search", "source_reported_no_results", nil, nil, nil)
		} else if len(result.NextActions) == 0 {
			add("reconsider_conditions", "inspected_options_do_not_meet_declared_conditions", nil, nil, nil)
		}
	}
}
