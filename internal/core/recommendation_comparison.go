package core

import (
	"cmp"
	"errors"
)

// Axes describe this comparison only, never an overall quality/fit score.
type ProductPreferenceAxis struct {
	Field     string `json:"field" jsonschema:"price.current_amount, rating, rocket, free_shipping, specifications.memory_gb or specifications.storage_gb; same evidence rules as required_conditions; capacities are derived from standalone native option labels in decimal GB, not measured hardware"`
	Direction string `json:"direction" jsonschema:"Numeric minimize/maximize; boolean prefer_true/prefer_false"`
}

func (a ProductPreferenceAxis) Validate() error {
	switch a.Field {
	case "price.current_amount", "rating", "specifications.memory_gb", "specifications.storage_gb":
		if a.Direction == "minimize" || a.Direction == "maximize" {
			return nil
		}
	case "rocket", "free_shipping":
		if a.Direction == "prefer_true" || a.Direction == "prefer_false" {
			return nil
		}
	}
	return errors.New("unsupported comparison field or direction")
}

type ProductComparableValue struct {
	Field           string                     `json:"field"`
	Unit            string                     `json:"unit"`
	Scope           string                     `json:"scope"`
	ObservedInteger *int64                     `json:"observed_integer,omitempty"`
	ObservedNumber  *float64                   `json:"observed_number,omitempty"`
	ObservedBoolean *bool                      `json:"observed_boolean,omitempty"`
	DerivedInteger  *int64                     `json:"derived_integer,omitempty"`
	Derivation      *ProductCapacityDerivation `json:"derivation,omitempty"`
	Evidence        []ProductFieldEvidence     `json:"evidence"`
	MissingEvidence []string                   `json:"missing_evidence"`
}

// Better/worse are from the left candidate's perspective and only on the
// requested axes. Unknown values cannot be imputed or compensated by a score.
type ProductPairComparison struct {
	Other     ProductReference `json:"other"`
	Relation  string           `json:"relation"`
	BetterOn  []string         `json:"better_on"`
	WorseOn   []string         `json:"worse_on"`
	EqualOn   []string         `json:"equal_on"`
	UnknownOn []string         `json:"unknown_on"`
}

func CompareInspectedProducts(left, right ProductInspection, axes []ProductPreferenceAxis) ProductPairComparison {
	r := ProductPairComparison{Other: right.Product.Reference, Relation: "not_requested", BetterOn: []string{}, WorseOn: []string{}, EqualOn: []string{}, UnknownOn: []string{}}
	for _, axis := range axes {
		if axis.Validate() != nil {
			r.UnknownOn = append(r.UnknownOn, axis.Field)
			continue
		}
		a, b := ReadProductComparableValue(left, axis.Field), ReadProductComparableValue(right, axis.Field)
		if len(a.MissingEvidence) > 0 || len(b.MissingEvidence) > 0 || a.Scope != b.Scope || a.Unit != b.Unit {
			r.UnknownOn = append(r.UnknownOn, axis.Field)
			continue
		}
		order := 0
		switch axis.Field {
		case "price.current_amount":
			order = cmp.Compare(*a.ObservedInteger, *b.ObservedInteger)
		case "specifications.memory_gb", "specifications.storage_gb":
			order = cmp.Compare(*a.DerivedInteger, *b.DerivedInteger)
		case "rating":
			order = cmp.Compare(*a.ObservedNumber, *b.ObservedNumber)
		default:
			if *a.ObservedBoolean != *b.ObservedBoolean {
				order = -1
				if *a.ObservedBoolean {
					order = 1
				}
			}
		}
		if axis.Direction == "minimize" || axis.Direction == "prefer_false" {
			order = -order
		}
		switch {
		case order > 0:
			r.BetterOn = append(r.BetterOn, axis.Field)
		case order < 0:
			r.WorseOn = append(r.WorseOn, axis.Field)
		default:
			r.EqualOn = append(r.EqualOn, axis.Field)
		}
	}
	switch {
	case len(axes) == 0:
	case len(r.UnknownOn) > 0:
		r.Relation = "unknown"
	case len(r.BetterOn) > 0 && len(r.WorseOn) > 0:
		r.Relation = "tradeoff"
	case len(r.BetterOn) > 0:
		r.Relation = "dominates_on_axes"
	case len(r.WorseOn) > 0:
		r.Relation = "dominated_on_axes"
	default:
		r.Relation = "equal_on_axes"
	}
	return r
}
