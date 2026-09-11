package products

import "github.com/JungHoonGhae/coupang-ctl/internal/core"

func priceObservationFromCard(p core.ProductCard, source string) (core.ProductPriceObservation, bool) {
	if len(p.PriceFilterUnavailableFields()) != 0 {
		return core.ProductPriceObservation{}, false
	}
	e, _ := p.EvidenceFor("price.current_amount")
	o := core.ProductPriceObservation{Reference: p.Reference, Name: p.Name, CanonicalURL: "https://www.coupang.com/vp/products/" + p.Reference.ProductID, CurrentAmount: p.Price.CurrentAmount, Currency: p.Price.Currency, Source: source, Provenance: e.Provenance, ObservedAt: e.CapturedAt, FieldEvidence: []core.ProductFieldEvidence{e}}
	for _, field := range []string{"price.original_amount", "price.discount_rate"} {
		v, ok := p.EvidenceFor(field)
		if !ok || v.Provenance == "inferred" || v.Scope != e.Scope || v.Reference != e.Reference || !v.CapturedAt.Equal(e.CapturedAt) {
			continue
		}
		o.FieldEvidence = append(o.FieldEvidence, v)
		if field == "price.original_amount" {
			o.OriginalAmount = p.Price.OriginalAmount
		} else {
			o.DiscountRate = p.Price.DiscountRate
		}
	}
	return o, true
}
