package products

import (
	"math"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// Page-level counts cannot be summed once per option. Unknown or conflicting
// observations poison that page's count instead of silently shrinking the total.
type recommendationReviewCounts struct {
	pages        map[string]*int
	unidentified int
}

func (r *recommendationReviewCounts) add(candidate core.ProductRecommendationCandidate) {
	id := candidate.Product.Reference.ProductID
	if !core.NumericProductIdentifier(id) {
		r.unidentified++
		return
	}
	if r.pages == nil {
		r.pages = make(map[string]*int)
	}
	count := candidate.ReviewsAvailable
	if candidate.ReviewCountScope != "product_page_observed" || count != nil && *count < 0 {
		count = nil
	}
	if previous, exists := r.pages[id]; exists {
		if previous == nil || count == nil || *previous != *count {
			r.pages[id] = nil
		}
		return
	}
	if count != nil {
		copied := *count
		count = &copied
	}
	r.pages[id] = count
}

func (r *recommendationReviewCounts) apply(audit *core.ProductRecommendationAudit) {
	audit.ReviewsAvailable = nil
	audit.ReviewCountPages = 0
	audit.ReviewCountUnavailablePages = r.unidentified
	audit.ReviewCountScope = ""
	total, overflow := 0, false
	for _, count := range r.pages {
		if count == nil {
			audit.ReviewCountUnavailablePages++
			continue
		}
		audit.ReviewCountPages++
		if total > math.MaxInt-*count {
			overflow = true
			continue
		}
		total += *count
	}
	if len(r.pages) > 0 {
		audit.ReviewCountScope = "distinct_product_pages_observed"
	}
	if audit.ReviewCountPages > 0 && audit.ReviewCountUnavailablePages == 0 && !overflow {
		audit.ReviewsAvailable = &total
	}
}
