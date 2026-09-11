package core

import "slices"

// AvailableReviewCount reconciles the two representations of the source count.
// Presence, not magnitude, selects a value. A disagreement stays unknown; zero
// must not trigger a fallback to another count. This does not establish scope
// or source provenance, which the adapter must supply separately.
func (p ProductInspection) AvailableReviewCount() *int {
	summaryKnown := slices.Contains(p.Coverage.ObservedFields, "rating.count")
	cardKnown := slices.Contains(p.Product.ObservedFields, "review_count")
	if summaryKnown && p.Rating.Count < 0 || cardKnown && p.Product.ReviewCount < 0 {
		return nil
	}
	if summaryKnown && cardKnown && p.Rating.Count != p.Product.ReviewCount {
		return nil
	}
	if summaryKnown {
		return &p.Rating.Count
	}
	if cardKnown {
		return &p.Product.ReviewCount
	}
	return nil
}
