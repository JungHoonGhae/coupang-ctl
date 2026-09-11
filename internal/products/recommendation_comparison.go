package products

import "github.com/JungHoonGhae/coupang-ctl/internal/core"

// This is a bounded, axis-scoped comparison over already inspected candidates.
// It is not a fit score, source ranking, or a claim of market-wide optimality.
func applyPreferenceComparison(result *core.ProductRecommendationResult, axes []core.ProductPreferenceAxis) bool {
	if len(axes) == 0 {
		return false
	}
	incomplete := false
	retained := make([]core.ProductRecommendationCandidate, 0, len(result.Candidates))
	byReference := map[core.ProductReference]core.ProductRecommendationCandidate{}
	for i, candidate := range result.Candidates {
		unknown, tradeoff, dominated := false, false, false
		for _, axis := range axes {
			value := core.ReadProductComparableValue(candidate.Inspection, axis.Field)
			candidate.ComparisonValues = append(candidate.ComparisonValues, value)
			unknown = unknown || len(value.MissingEvidence) > 0
		}
		for j, other := range result.Candidates {
			if i == j {
				continue
			}
			comparison := core.CompareInspectedProducts(candidate.Inspection, other.Inspection, axes)
			candidate.Comparisons = append(candidate.Comparisons, comparison)
			unknown = unknown || comparison.Relation == "unknown"
			tradeoff = tradeoff || comparison.Relation == "tradeoff"
			dominated = dominated || comparison.Relation == "dominated_on_axes"
		}
		incomplete = incomplete || unknown
		switch {
		case dominated:
			candidate.ComparisonReason = "dominated_on_requested_axes"
			candidate.ExclusionReason = "dominated_on_requested_axes"
			result.Audit.ComparisonExcludedCandidates++
		case unknown:
			candidate.ComparisonReason = "comparison_evidence_or_scope_unresolved"
		case tradeoff:
			candidate.ComparisonReason = "distinct_tradeoff_on_requested_axes"
		default:
			candidate.ComparisonReason = "not_dominated_on_requested_axes"
		}
		byReference[candidate.Product.Reference] = candidate
		if !dominated {
			retained = append(retained, candidate)
		}
	}
	for i, candidate := range result.Inspected {
		if compared, ok := byReference[candidate.Product.Reference]; ok {
			result.Inspected[i] = compared
		} else if candidate.ExclusionReason != "" {
			result.Inspected[i].ComparisonReason = "not_compared_due_to_candidate_exclusion"
		}
	}
	result.Candidates = retained
	result.Warnings = append(result.Warnings, "preference comparison applies only to the explicitly requested axes; equal values do not prove equivalent products, and no overall suitability or market optimum is claimed")
	return incomplete
}
