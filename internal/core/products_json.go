package core

import (
	"encoding/json"
	"slices"
)

// Availability is not provenance: an available field may have been extracted
// from DOM text. Source classification is carried separately by its adapter.
// Pointers here preserve known zero/false while omitting unverified values.
func availableProductValue[T any](fields []string, field string, value T) *T {
	if !slices.Contains(fields, field) {
		return nil
	}
	return &value
}

type productPriceJSON struct {
	CurrentAmount  *int64 `json:"current_amount,omitempty"`
	OriginalAmount *int64 `json:"original_amount,omitempty"`
	DiscountRate   *int   `json:"discount_rate,omitempty"`
	Currency       string `json:"currency"`
}

func (p ProductCard) MarshalJSON() ([]byte, error) {
	type plain ProductCard
	return json.Marshal(struct {
		plain
		Price        productPriceJSON `json:"price"`
		Rating       *float64         `json:"rating,omitempty"`
		ReviewCount  *int             `json:"review_count,omitempty"`
		Rocket       *bool            `json:"rocket,omitempty"`
		FreeShipping *bool            `json:"free_shipping,omitempty"`
		Coupon       *bool            `json:"coupon,omitempty"`
		Sponsored    *bool            `json:"sponsored,omitempty"`
	}{
		plain: plain(p),
		Price: productPriceJSON{
			CurrentAmount:  availableProductValue(p.ObservedFields, "price.current_amount", p.Price.CurrentAmount),
			OriginalAmount: availableProductValue(p.ObservedFields, "price.original_amount", p.Price.OriginalAmount),
			DiscountRate:   availableProductValue(p.ObservedFields, "price.discount_rate", p.Price.DiscountRate),
			Currency:       p.Price.Currency,
		},
		Rating:       availableProductValue(p.ObservedFields, "rating", p.Rating),
		ReviewCount:  availableProductValue(p.ObservedFields, "review_count", p.ReviewCount),
		Rocket:       availableProductValue(p.ObservedFields, "rocket", p.Rocket),
		FreeShipping: availableProductValue(p.ObservedFields, "free_shipping", p.FreeShipping),
		Coupon:       availableProductValue(p.ObservedFields, "coupon", p.Coupon),
		Sponsored:    availableProductValue(p.ObservedFields, "sponsored", p.Sponsored),
	})
}

func (p ProductInspection) MarshalJSON() ([]byte, error) {
	type plain ProductInspection
	type deliveryJSON struct {
		Summary      string `json:"summary,omitempty"`
		FreeShipping *bool  `json:"free_shipping,omitempty"`
		Rocket       *bool  `json:"rocket,omitempty"`
	}
	type ratingJSON struct {
		Average      *float64        `json:"average,omitempty"`
		Count        *int            `json:"count,omitempty"`
		Distribution *map[string]int `json:"distribution,omitempty"`
	}
	var distribution *map[string]int
	if slices.Contains(p.Coverage.ObservedFields, "rating.distribution") && p.Rating.Distribution != nil {
		distribution = &p.Rating.Distribution
	}
	return json.Marshal(struct {
		plain
		Delivery deliveryJSON `json:"delivery"`
		Rating   ratingJSON   `json:"rating"`
	}{
		plain: plain(p),
		Delivery: deliveryJSON{
			Summary:      p.Delivery.Summary,
			FreeShipping: availableProductValue(p.Coverage.ObservedFields, "delivery.free_shipping", p.Delivery.FreeShipping),
			Rocket:       availableProductValue(p.Coverage.ObservedFields, "delivery.rocket", p.Delivery.Rocket),
		},
		Rating: ratingJSON{
			Average:      availableProductValue(p.Coverage.ObservedFields, "rating.average", p.Rating.Average),
			Count:        availableProductValue(p.Coverage.ObservedFields, "rating.count", p.Rating.Count),
			Distribution: distribution,
		},
	})
}

func (r ProductReview) MarshalJSON() ([]byte, error) {
	type plain ProductReview
	return json.Marshal(struct {
		plain
		Rating       *float64 `json:"rating,omitempty"`
		HelpfulCount *int     `json:"helpful_count,omitempty"`
	}{
		plain:        plain(r),
		Rating:       availableProductValue(r.ObservedFields, "rating", r.Rating),
		HelpfulCount: availableProductValue(r.ObservedFields, "helpful_count", r.HelpfulCount),
	})
}
