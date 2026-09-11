package core

import (
	"encoding/json"
	"testing"
)

func TestReorderPriceZeroIsNotUnknown(t *testing.T) {
	for _, available := range []bool{false, true} {
		r := ReorderPriceComparison{Status: "unavailable_no_local_price_observation"}
		if available {
			r.Status = "available"
		}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"latest_observed_amount_krw", "difference_krw", "difference_percent"} {
			v, ok := fields[key]
			if ok != available || ok && v != float64(0) {
				t.Fatalf("%s lost availability", key)
			}
		}
	}
}
