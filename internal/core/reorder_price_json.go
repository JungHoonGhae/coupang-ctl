package core

import "encoding/json"

func (r ReorderPriceComparison) MarshalJSON() ([]byte, error) {
	type plain ReorderPriceComparison
	var amount, difference *int64
	var percent *float64
	if r.Status == "available" {
		a, d, p := r.LatestObservedAmountKRW, r.DifferenceKRW, r.DifferencePercent
		amount, difference, percent = &a, &d, &p
	}
	return json.Marshal(struct {
		plain
		Amount     *int64   `json:"latest_observed_amount_krw,omitempty"`
		Difference *int64   `json:"difference_krw,omitempty"`
		Percent    *float64 `json:"difference_percent,omitempty"`
	}{plain(r), amount, difference, percent})
}
