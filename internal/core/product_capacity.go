package core

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ProductCapacityDerivation is a calculation over a source-stated option, not
// a measured hardware specification. Input and evidence remain independently visible.
type ProductCapacityDerivation struct {
	Provenance string                   `json:"provenance"`
	Method     string                   `json:"method" jsonschema:"standalone_capacity_decimal_gb: exact native RAM용량 or 저장용량 label; parse GB or TB with 1 TB = 1000 GB; no compound splitting or title inference"`
	Unit       string                   `json:"unit"`
	Input      ProductSelectedAttribute `json:"input"`
}

var statedCapacity = regexp.MustCompile(`^([0-9]{1,12})(?:\.([0-9]{1,3}))? *(GB|TB)$`)

func readProductCapacity(p ProductInspection, field string) ProductComparableValue {
	r := ProductComparableValue{Field: field, Unit: "GB", Scope: "selected_option", Evidence: []ProductFieldEvidence{}, MissingEvidence: []string{}}
	label := "RAM용량"
	if field == "specifications.storage_gb" {
		label = "저장용량"
	}
	e, ok := p.SelectedAttributesEvidence()
	if !ok || slices.Contains(p.Coverage.UnavailableFields, "selected_attributes") {
		r.MissingEvidence = []string{field + ".selected_attributes_evidence"}
		return r
	}
	var input *ProductSelectedAttribute
	for _, a := range p.SelectedAttributes {
		name := strings.TrimSpace(a.Name)
		if name == label {
			input = &a
		} else if strings.Contains(name, label) {
			// Even an additional standalone row cannot disambiguate a compound
			// row that also purports to describe this specification.
			r.MissingEvidence = []string{field + ".compound_attribute"}
			return r
		}
	}
	if input == nil {
		r.MissingEvidence = []string{field + ".standalone_attribute"}
		return r
	}
	parts := statedCapacity.FindStringSubmatch(strings.TrimSpace(input.Value))
	if parts == nil {
		r.MissingEvidence = []string{field + ".unrecognized_capacity"}
		return r
	}
	whole, _ := strconv.ParseInt(parts[1], 10, 64)
	fraction, _ := strconv.ParseInt(parts[2]+strings.Repeat("0", 3-len(parts[2])), 10, 64)
	// Bounded decimal arithmetic avoids float rounding and implicit GiB conversion.
	value := whole*1000 + fraction
	if parts[3] == "GB" {
		if value%1000 != 0 {
			r.MissingEvidence = []string{field + ".nonintegral_gb"}
			return r
		}
		value /= 1000
	}
	r.DerivedInteger = &value
	r.Derivation = &ProductCapacityDerivation{Provenance: "derived", Method: "standalone_capacity_decimal_gb", Unit: "GB", Input: *input}
	r.Evidence = append(r.Evidence, e)
	return r
}
