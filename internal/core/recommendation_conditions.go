package core

import (
	"errors"
	"math"
	"regexp"
	"slices"
)

// Conditions are explicit predicates, not inferred preferences or overall fit.
// Price means the current listed KRW amount, excluding shipping/quantity totals.
type ProductRecommendationCondition struct {
	ID       string   `json:"id"`
	Field    string   `json:"field" jsonschema:"price.current_amount (integer KRW), rating (number), rocket or free_shipping (boolean), specifications.memory_gb or specifications.storage_gb (integer GB derived only from standalone native option labels; 1 TB = 1000 GB). Compound labels and title heuristics remain unknown; source claims are not measured hardware"`
	Operator string   `json:"operator" jsonschema:"Numeric lte or gte; boolean eq"`
	Integer  *int64   `json:"integer,omitempty"`
	Number   *float64 `json:"number,omitempty"`
	Boolean  *bool    `json:"boolean,omitempty"`
}

type ProductConditionStatus string

const (
	ProductConditionMet     ProductConditionStatus = "met"
	ProductConditionUnmet   ProductConditionStatus = "unmet"
	ProductConditionUnknown ProductConditionStatus = "unknown"
)

type ProductConditionAssessment struct {
	Condition       ProductRecommendationCondition `json:"condition"`
	Status          ProductConditionStatus         `json:"status"`
	ObservedInteger *int64                         `json:"observed_integer,omitempty"`
	ObservedNumber  *float64                       `json:"observed_number,omitempty"`
	ObservedBoolean *bool                          `json:"observed_boolean,omitempty"`
	DerivedInteger  *int64                         `json:"derived_integer,omitempty"`
	Derivation      *ProductCapacityDerivation     `json:"derivation,omitempty"`
	Evidence        []ProductFieldEvidence         `json:"evidence"`
	MissingEvidence []string                       `json:"missing_evidence"`
}

var recommendationConditionID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func (c ProductRecommendationCondition) Validate() error {
	if !recommendationConditionID.MatchString(c.ID) {
		return errors.New("condition id must be a bounded lowercase identifier")
	}
	switch c.Field {
	case "price.current_amount":
		if c.Integer == nil || *c.Integer < 0 || c.Number != nil || c.Boolean != nil {
			return errors.New("price condition requires a nonnegative integer KRW amount only")
		}
	case "specifications.memory_gb", "specifications.storage_gb":
		if c.Integer == nil || *c.Integer < 0 || c.Number != nil || c.Boolean != nil {
			return errors.New("capacity condition requires a nonnegative integer GB amount only")
		}
	case "rating":
		if c.Number == nil || math.IsNaN(*c.Number) || math.IsInf(*c.Number, 0) || *c.Number < 0 || *c.Number > 5 || c.Integer != nil || c.Boolean != nil {
			return errors.New("rating condition requires a finite number between 0 and 5 only")
		}
	case "rocket", "free_shipping":
		if c.Boolean == nil || c.Integer != nil || c.Number != nil || c.Operator != "eq" {
			return errors.New("delivery condition requires a boolean and eq operator only")
		}
		return nil
	default:
		return errors.New("unsupported recommendation condition field")
	}
	if c.Operator != "lte" && c.Operator != "gte" {
		return errors.New("numeric condition operator must be lte or gte")
	}
	return nil
}

func AssessProductCondition(inspection ProductInspection, condition ProductRecommendationCondition) ProductConditionAssessment {
	r := ProductConditionAssessment{Condition: condition, Status: ProductConditionUnknown, Evidence: []ProductFieldEvidence{}, MissingEvidence: []string{}}
	if condition.Validate() != nil {
		r.MissingEvidence = []string{"invalid_condition"}
		return r
	}
	v := ReadProductComparableValue(inspection, condition.Field)
	r.Evidence, r.MissingEvidence = v.Evidence, v.MissingEvidence
	r.ObservedInteger, r.ObservedNumber, r.ObservedBoolean = v.ObservedInteger, v.ObservedNumber, v.ObservedBoolean
	r.DerivedInteger, r.Derivation = v.DerivedInteger, v.Derivation
	if len(v.MissingEvidence) > 0 {
		return r
	}
	switch condition.Field {
	case "price.current_amount":
		r.Status = conditionTruth((condition.Operator == "lte" && *v.ObservedInteger <= *condition.Integer) || (condition.Operator == "gte" && *v.ObservedInteger >= *condition.Integer))
	case "specifications.memory_gb", "specifications.storage_gb":
		r.Status = conditionTruth((condition.Operator == "lte" && *v.DerivedInteger <= *condition.Integer) || (condition.Operator == "gte" && *v.DerivedInteger >= *condition.Integer))
	case "rating":
		r.Status = conditionTruth((condition.Operator == "lte" && *v.ObservedNumber <= *condition.Number) || (condition.Operator == "gte" && *v.ObservedNumber >= *condition.Number))
	default:
		r.Status = conditionTruth(*v.ObservedBoolean == *condition.Boolean)
	}
	return r
}

// ReadProductComparableValue is shared by hard conditions and preference
// comparisons, so changing the decision policy cannot weaken evidence checks.
func ReadProductComparableValue(inspection ProductInspection, field string) ProductComparableValue {
	r := ProductComparableValue{Field: field, Evidence: []ProductFieldEvidence{}, MissingEvidence: []string{}}
	switch field {
	case "specifications.memory_gb", "specifications.storage_gb":
		return readProductCapacity(inspection, field)
	case "price.current_amount":
		r.Unit = "KRW"
	case "rating":
		r.Unit = "rating_out_of_5"
		r.Scope = "product_page"
	case "rocket", "free_shipping":
		r.Unit = "boolean"
	default:
		r.MissingEvidence = []string{"unsupported_comparison_field"}
		return r
	}
	p := inspection.Product
	if field == "price.current_amount" {
		if missing := p.PriceFilterUnavailableFields(); len(missing) > 0 {
			r.MissingEvidence = missing
			return r
		}
		e, _ := p.EvidenceFor(field)
		r.Evidence = append(r.Evidence, e)
		r.Scope = e.Scope
		value := p.Price.CurrentAmount
		r.ObservedInteger = &value
		return r
	}

	// The product card and detail summary may independently expose the same
	// scalar. A conflict or invalid declared record cannot be voted away.
	detailField := "delivery." + field
	cardValue, detailValue := p.FreeShipping, inspection.Delivery.FreeShipping
	if field == "rocket" {
		cardValue, detailValue = p.Rocket, inspection.Delivery.Rocket
	}
	if field == "rating" {
		detailField = "rating.average"
	}
	cardKnown := slices.Contains(p.ObservedFields, field)
	detailKnown := slices.Contains(inspection.Coverage.ObservedFields, detailField)
	if !cardKnown && !detailKnown {
		r.MissingEvidence = []string{field}
		return r
	}
	if cardKnown && detailKnown && ((field == "rating" && p.Rating != inspection.Rating.Average) || (field != "rating" && cardValue != detailValue)) {
		r.MissingEvidence = []string{field + ".conflict"}
		return r
	}
	for _, entry := range []struct {
		known  bool
		detail bool
	}{{cardKnown, false}, {detailKnown, true}} {
		if !entry.known {
			continue
		}
		e, ok := p.EvidenceFor(field)
		if entry.detail {
			e, ok = inspection.EvidenceFor(detailField)
		}
		if !ok || e.Provenance == "inferred" {
			r.MissingEvidence = append(r.MissingEvidence, field+".provenance")
			continue
		}
		validScope := e.Scope == "product" || e.Scope == "product_page"
		if field != "rating" {
			if p.Reference.ItemID != "" || p.Reference.VendorItemID != "" {
				validScope = e.Scope == "selected_option"
			} else {
				validScope = e.Scope == "product"
			}
		}
		if !validScope {
			r.MissingEvidence = append(r.MissingEvidence, field+".scope")
			continue
		}
		r.Evidence = append(r.Evidence, e)
	}
	if len(r.MissingEvidence) > 0 {
		return r
	}
	if field == "rating" {
		value := p.Rating
		if !cardKnown {
			value = inspection.Rating.Average
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 5 {
			r.MissingEvidence = []string{"rating.invalid_value"}
			return r
		}
		r.ObservedNumber = &value
	} else {
		value := cardValue
		if !cardKnown {
			value = detailValue
		}
		r.ObservedBoolean = &value
		r.Scope = r.Evidence[0].Scope
	}
	return r
}

func conditionTruth(met bool) ProductConditionStatus {
	if met {
		return ProductConditionMet
	}
	return ProductConditionUnmet
}
