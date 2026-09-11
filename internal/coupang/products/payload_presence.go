package products

import (
	"bytes"
	"encoding/json"
	"strings"
)

// The document adapter's availability assertion must also have a concrete
// payload value. Presence alone does not establish source-native provenance.
var cardScalarPaths = map[string]string{
	"price.current_amount":  "current_amount",
	"price.original_amount": "original_amount",
	"price.discount_rate":   "discount_rate",
	"rating":                "rating", "review_count": "review_count",
	"rocket": "rocket", "free_shipping": "free_shipping",
	"coupon": "coupon", "sponsored": "sponsored",
}

func (p *productCardPayload) UnmarshalJSON(data []byte) error {
	type plain productCardPayload
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	value.scalarPresence = make(map[string]bool, len(cardScalarPaths))
	for field, path := range cardScalarPaths {
		value.scalarPresence[field] = nonNullJSON(object[path])
	}
	*p = productCardPayload(value)
	return nil
}

func nonNullJSON(value json.RawMessage) bool {
	return len(value) > 0 && !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func declaredInspectionFieldsPresent(document []byte, fields []string) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(document, &object); err != nil {
		return false
	}
	for _, field := range normalizedStrings(fields, 80, 100) {
		switch field {
		case "delivery.rocket", "delivery.free_shipping", "rating.average", "rating.count", "rating.distribution":
			group, key, _ := strings.Cut(field, ".")
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(object[group], &nested); err != nil || !nonNullJSON(nested[key]) {
				return false
			}
			if field == "rating.distribution" {
				var counts map[string]json.RawMessage
				if err := json.Unmarshal(nested[key], &counts); err != nil {
					return false
				}
				for _, count := range counts {
					if !nonNullJSON(count) {
						return false
					}
				}
			}
		}
	}
	if nonNullJSON(object["reviews"]) {
		var reviews []map[string]json.RawMessage
		if err := json.Unmarshal(object["reviews"], &reviews); err != nil {
			return false
		}
		for _, review := range reviews {
			var declared []string
			if len(review["observed_fields"]) > 0 {
				if err := json.Unmarshal(review["observed_fields"], &declared); err != nil {
					return false
				}
			}
			for _, field := range normalizedStrings(declared, 10, 100) {
				if (field == "rating" || field == "helpful_count") && !nonNullJSON(review[field]) {
					return false
				}
			}
		}
	}
	return true
}
