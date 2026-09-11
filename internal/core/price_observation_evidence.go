package core

import "encoding/json"

func (o ProductPriceObservation) MarshalJSON() ([]byte, error) {
	type plain ProductPriceObservation
	var original *int64
	var discount *int
	if o.HasPriceEvidence() {
		for _, e := range o.FieldEvidence {
			if !e.ValidFor(o.Reference) || !e.CapturedAt.Equal(o.ObservedAt) || e.Provenance == "inferred" {
				continue
			}
			if e.Field == "price.original_amount" {
				v := o.OriginalAmount
				original = &v
			}
			if e.Field == "price.discount_rate" {
				v := o.DiscountRate
				discount = &v
			}
		}
	}
	return json.Marshal(struct {
		plain
		OriginalAmount *int64 `json:"original_amount,omitempty"`
		DiscountRate   *int   `json:"discount_rate,omitempty"`
	}{plain(o), original, discount})
}

// HasPriceEvidence does not retroactively trust a legacy provenance string.
func (o ProductPriceObservation) HasPriceEvidence() bool {
	p := ProductCard{Reference: o.Reference, Price: ProductPrice{CurrentAmount: o.CurrentAmount, Currency: o.Currency}, ObservedFields: []string{"price.current_amount"}, FieldEvidence: o.FieldEvidence}
	if len(p.PriceFilterUnavailableFields()) != 0 {
		return false
	}
	e, ok := p.EvidenceFor("price.current_amount")
	return ok && e.Provenance == o.Provenance && e.CapturedAt.Equal(o.ObservedAt)
}
