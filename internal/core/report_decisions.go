package core

import "errors"

// Rendering limits bound work and output, not the number of recommendations
// a user should receive. Refuse ambiguous duplicates instead of dropping them.
func validateReportDecisions(r ProductRecommendationResult) error {
	if len(r.Inspected) > 200 || len(r.NextActions) > 200 {
		return errors.New("report exceeds decision rendering limits")
	}
	all := map[ProductReference]bool{}
	for _, records := range [][]ProductRecommendationCandidate{r.Inspected, r.Candidates} {
		seen := map[ProductReference]bool{}
		for _, c := range records {
			ref := c.Product.Reference
			if !validReportReference(ref) || seen[ref] {
				return errors.New("report decision references must be valid and unique within each list")
			}
			seen[ref] = true
			all[ref] = true
			if len(c.Conditions) > 17 {
				return errors.New("report supports at most 16 explicit conditions and an implicit budget per candidate")
			}
			ids := map[string]bool{}
			for _, a := range c.Conditions {
				if err := a.Condition.Validate(); err != nil {
					return err
				}
				if ids[a.Condition.ID] {
					return errors.New("report condition ids must be unique per candidate")
				}
				ids[a.Condition.ID] = true
			}
		}
	}
	if len(all) > 200 {
		return errors.New("report exceeds total decision candidate limit")
	}
	for _, a := range r.NextActions {
		if len(a.References) > 1000 || len(a.ConditionIDs) > 17 || len(a.Fields) > 64 {
			return errors.New("report next action exceeds rendering limits")
		}
		for _, ref := range a.References {
			if !validReportReference(ref) {
				return errors.New("report next action reference is invalid")
			}
		}
	}
	if r.Audit.UninspectedExactOptions < 0 {
		return errors.New("report uninspected option count must not be negative")
	}
	if r.Status == ProductRecommendationComplete {
		for _, reason := range []string{r.Audit.DiscoveryStopReason, r.Audit.InspectionStopReason} {
			if reason == "source_access_denied" || reason == "source_authentication_required" || reason == "time_budget_reached" {
				return errors.New("complete report contradicts an interrupted investigation")
			}
		}
		if r.Audit.TimeBudgetExhausted {
			return errors.New("complete report contradicts exhausted time budget")
		}
	}
	return nil
}

func validReportReference(r ProductReference) bool {
	return NumericProductIdentifier(r.ProductID) && (r.ItemID == "" || NumericProductIdentifier(r.ItemID)) && (r.VendorItemID == "" || NumericProductIdentifier(r.VendorItemID))
}
