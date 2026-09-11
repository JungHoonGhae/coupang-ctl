package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPriceHistoryOptionalZeroRequiresFieldEvidence(t *testing.T) {
	stamp := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	ref := ProductReference{ProductID: "101"}
	e := ProductFieldEvidence{Field: "price.current_amount", Source: "json_ld", Locator: "jsonld.Product.offers.price", Method: "native_field", Provenance: "observed", Scope: "product", Reference: ref, CapturedAt: stamp}
	o := ProductPriceObservation{Reference: ref, Currency: "KRW", Provenance: "observed", ObservedAt: stamp, FieldEvidence: []ProductFieldEvidence{e}}
	for _, known := range []bool{false, true} {
		if known {
			e.Field = "price.original_amount"
			o.FieldEvidence = append(o.FieldEvidence, e)
			e.Field = "price.discount_rate"
			o.FieldEvidence = append(o.FieldEvidence, e)
		}
		data, err := json.Marshal(o)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"original_amount", "discount_rate"} {
			value, present := fields[field]
			if present != known || present && value != float64(0) {
				t.Fatalf("optional zero %s lost availability", field)
			}
		}
	}
}
