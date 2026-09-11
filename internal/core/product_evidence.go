package core

import (
	"regexp"
	"slices"
	"time"
)

// ProductFieldEvidence describes how a returned value was obtained. It is not
// proof of source accuracy, freshness, or a reverse-engineered field's meaning.
// Locators are bounded adapter paths, never raw URLs, payloads or user text.
type ProductFieldEvidence struct {
	Field      string           `json:"field"`
	Provenance string           `json:"provenance"`
	Source     string           `json:"source"`
	Locator    string           `json:"locator"`
	Method     string           `json:"method"`
	Scope      string           `json:"scope"`
	Reference  ProductReference `json:"reference"`
	CapturedAt time.Time        `json:"captured_at"`
}

var productEvidencePath = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\[\]]{0,159}$`)

func (e ProductFieldEvidence) ValidFor(reference ProductReference) bool {
	if e.CapturedAt.IsZero() || !NumericProductIdentifier(reference.ProductID) || e.Reference.ProductID != reference.ProductID || !productEvidencePath.MatchString(e.Field) || !productEvidencePath.MatchString(e.Locator) {
		return false
	}
	switch e.Source {
	case "json_ld", "quantity_info", "review_endpoint", "dom":
	case "product_options":
		if e.Field != "selected_attributes" || e.Method != "native_field" || e.Scope != "selected_option" ||
			e.Locator != "options.optionRows.selectedAttribute" || reference.ItemID == "" || reference.VendorItemID == "" {
			return false
		}
	default:
		return false
	}
	switch e.Method {
	case "native_field":
		if e.Source == "dom" || e.Provenance != "observed" {
			return false
		}
	case "numeric_parse", "selected_option_text", "dom_text":
		if e.Provenance != "derived" {
			return false
		}
	case "alias_lookup":
		if e.Provenance != "inferred" {
			return false
		}
	default:
		return false
	}
	switch e.Scope {
	case "selected_option":
		return e.Reference == reference && (reference.ItemID != "" || reference.VendorItemID != "") &&
			(reference.ItemID == "" || NumericProductIdentifier(reference.ItemID)) && (reference.VendorItemID == "" || NumericProductIdentifier(reference.VendorItemID))
	case "product", "product_page", "unknown":
		return e.Reference.ItemID == "" && e.Reference.VendorItemID == ""
	default:
		return false
	}
}

func findProductFieldEvidence(values []ProductFieldEvidence, reference ProductReference, available []string, field string) (ProductFieldEvidence, bool) {
	if !slices.Contains(available, field) {
		return ProductFieldEvidence{}, false
	}
	var found ProductFieldEvidence
	count := 0
	for _, value := range values {
		if value.Field != field {
			continue
		}
		if !value.ValidFor(reference) {
			return ProductFieldEvidence{}, false
		}
		found = value
		count++
	}
	return found, count == 1
}

func (p ProductCard) EvidenceFor(field string) (ProductFieldEvidence, bool) {
	return findProductFieldEvidence(p.FieldEvidence, p.Reference, p.ObservedFields, field)
}

func (p ProductInspection) EvidenceFor(field string) (ProductFieldEvidence, bool) {
	return findProductFieldEvidence(p.FieldEvidence, p.Product.Reference, p.Coverage.ObservedFields, field)
}
