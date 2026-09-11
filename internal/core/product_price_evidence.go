package core

import "slices"

// PriceFilterUnavailableFields describes why a card cannot prove a KRW budget
// condition. A page-level starting price is not the selected option's price.
// Valid metadata describes acquisition, not freshness or checkout totals.
func (p ProductCard) PriceFilterUnavailableFields() []string {
	var missing []string
	if !slices.Contains(p.ObservedFields, "price.current_amount") || p.Price.CurrentAmount < 0 {
		missing = append(missing, "price.current_amount")
	}
	if p.Price.Currency != "KRW" {
		missing = append(missing, "price.currency")
	}
	evidence, ok := p.EvidenceFor("price.current_amount")
	if !ok || evidence.Provenance == "inferred" {
		missing = append(missing, "price.current_amount.provenance")
	}
	if ok {
		option := p.Reference.ItemID != "" || p.Reference.VendorItemID != ""
		if (option && evidence.Scope != "selected_option") || (!option && evidence.Scope != "product") {
			missing = append(missing, "price.current_amount.scope")
		}
	}
	return missing
}
