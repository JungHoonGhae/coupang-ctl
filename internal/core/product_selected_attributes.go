package core

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ProductSelectedAttribute preserves one source-native row, including compound
// labels. Label order/arity is not enough evidence to split a compound value
// into individual specifications.
type ProductSelectedAttribute struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SelectedAttributesEvidence validates the complete selected tuple at the same
// seam used by adapters and consumers. It never upgrades title heuristics.
func (p ProductInspection) SelectedAttributesEvidence() (ProductFieldEvidence, bool) {
	if len(p.SelectedAttributes) == 0 || len(p.SelectedAttributes) > 20 {
		return ProductFieldEvidence{}, false
	}
	names := make(map[string]bool, len(p.SelectedAttributes))
	for _, a := range p.SelectedAttributes {
		for _, text := range []string{a.Name, a.Value} {
			if !utf8.ValidString(text) || len([]rune(text)) > 300 || strings.TrimSpace(text) == "" || strings.IndexFunc(text, unicode.IsControl) >= 0 {
				return ProductFieldEvidence{}, false
			}
		}
		name := strings.TrimSpace(a.Name)
		if names[name] {
			return ProductFieldEvidence{}, false
		}
		names[name] = true
	}
	e, ok := p.EvidenceFor("selected_attributes")
	return e, ok && e.Source == "product_options" && e.Method == "native_field" && e.Provenance == "observed" &&
		e.Locator == "options.optionRows.selectedAttribute" && e.Scope == "selected_option" &&
		p.Product.Reference.ItemID != "" && p.Product.Reference.VendorItemID != ""
}
