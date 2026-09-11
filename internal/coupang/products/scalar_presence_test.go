package products

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestCardDeclaredScalarRequiresPresentNonNullValue(t *testing.T) {
	for _, tc := range []struct {
		field, key string
		zero       any
	}{
		{"price.current_amount", "current_amount", 0},
		{"price.original_amount", "original_amount", 0},
		{"price.discount_rate", "discount_rate", 0},
		{"rating", "rating", 0}, {"review_count", "review_count", 0},
		{"rocket", "rocket", false}, {"free_shipping", "free_shipping", false},
		{"coupon", "coupon", false}, {"sponsored", "sponsored", false},
	} {
		t.Run(tc.field, func(t *testing.T) {
			for _, mode := range []string{"missing", "null", "zero"} {
				t.Run(mode, func(t *testing.T) {
					card := syntheticEvidenceCard("201", "301")
					card["observed_fields"] = []string{tc.field}
					if mode == "null" {
						card[tc.key] = nil
					}
					if mode == "zero" {
						card[tc.key] = tc.zero
					}
					doc := evidenceDocument(t, map[string]any{"items": []any{card}})
					items, _, err := ParseSearchDocument(doc)
					if mode != "zero" {
						if !errors.Is(err, ErrProductDataMissing) || len(items) != 0 {
							t.Fatal("declared but absent scalar accepted")
						}
					} else if err != nil || len(items) != 1 {
						t.Fatalf("known zero rejected: %v", err)
					}
				})
			}
		})
	}
}

func TestInspectionDeclaredScalarRequiresPresentNonNullValue(t *testing.T) {
	for _, field := range []string{"delivery.rocket", "delivery.free_shipping", "rating.average", "rating.count", "rating.distribution"} {
		t.Run(field, func(t *testing.T) {
			for _, mode := range []string{"missing", "null", "zero"} {
				doc := map[string]any{"product": syntheticEvidenceCard("201", "301"), "coverage": map[string]any{"observed_fields": []string{field}}}
				group, key := "rating", "average"
				var zero any = 0
				switch field {
				case "delivery.rocket":
					group, key, zero = "delivery", "rocket", false
				case "delivery.free_shipping":
					group, key, zero = "delivery", "free_shipping", false
				case "rating.count":
					key = "count"
				case "rating.distribution":
					key, zero = "distribution", map[string]int{}
				}
				if mode == "null" {
					doc[group] = map[string]any{key: nil}
				}
				if mode == "zero" {
					doc[group] = map[string]any{key: zero}
				}
				_, err := ParseInspectionDocument(evidenceDocument(t, doc), core.ProductInspectRequest{ProductID: "101"})
				if mode == "zero" && err != nil {
					t.Fatalf("known value rejected: %v", err)
				}
				if mode != "zero" && !errors.Is(err, ErrProductDataMissing) {
					t.Fatalf("%s %s accepted", field, mode)
				}
			}
		})
	}
}

func TestSearchReportsRejectedCardsInsteadOfCompleteCoverage(t *testing.T) {
	valid := syntheticEvidenceCard("201", "301")
	invalid := syntheticEvidenceCard("202", "302")
	invalid["observed_fields"] = []string{"price.current_amount"}
	items, coverage, err := ParseSearchDocument(evidenceDocument(t, map[string]any{"items": []any{valid, invalid, valid}}))
	if err != nil || len(items) != 1 {
		t.Fatalf("valid subset lost: %d %v", len(items), err)
	}
	var got map[string]any
	if err := json.Unmarshal(evidenceDocument(t, coverage), &got); err != nil {
		t.Fatal(err)
	}
	if got["rejected_items"] != float64(1) || got["duplicate_items"] != float64(1) || got["source_items"] != float64(3) {
		t.Fatalf("missing partial coverage counts: %#v", got)
	}
}

func TestPresenceNormalizationCannotCreateObservation(t *testing.T) {
	card := syntheticEvidenceCard("201", "301")
	card["observed_fields"] = []string{" price.current_amount "}
	_, _, err := ParseSearchDocument(evidenceDocument(t, map[string]any{"items": []any{card}}))
	if !errors.Is(err, ErrProductDataMissing) {
		t.Fatal("normalization manufactured a price observation")
	}
	doc := map[string]any{"product": syntheticEvidenceCard("201", "301"), "coverage": map[string]any{"observed_fields": []string{" rating.count "}}}
	_, err = ParseInspectionDocument(evidenceDocument(t, doc), core.ProductInspectRequest{ProductID: "101"})
	if !errors.Is(err, ErrProductDataMissing) {
		t.Fatal("normalization manufactured a rating count")
	}
}

func TestUndeclaredScalarValuesRemainUnverified(t *testing.T) {
	card := syntheticEvidenceCard("201", "301")
	card["current_amount"], card["sponsored"] = 1200, false
	items, _, err := ParseSearchDocument(evidenceDocument(t, map[string]any{"items": []any{card}}))
	if err != nil || len(items) != 1 {
		t.Fatalf("card lost: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(evidenceDocument(t, items[0]), &got); err != nil {
		t.Fatal(err)
	}
	if _, present := got["sponsored"]; present {
		t.Fatal("payload presence promoted to verified advertising status")
	}
	if _, present := got["price"].(map[string]any)["current_amount"]; present {
		t.Fatal("payload presence promoted to verified price")
	}
}

func TestSearchCoverageCountsAreCalculatedNotTrusted(t *testing.T) {
	for _, empty := range []bool{false, true} {
		cards := []any{syntheticEvidenceCard("201", "301")}
		if empty {
			cards = nil
		}
		_, coverage, err := ParseSearchDocument(evidenceDocument(t, map[string]any{"items": cards, "no_results": empty,
			"coverage": map[string]any{"source_items": 999, "rejected_items": 999, "duplicate_items": 999, "source_no_results": !empty}}))
		if err != nil {
			t.Fatal(err)
		}
		if coverage.SourceItems != len(cards) || coverage.RejectedItems != 0 || coverage.DuplicateItems != 0 || coverage.SourceNoResults != empty {
			t.Fatal("source-supplied audit counts trusted")
		}
	}
}

func TestInspectionRejectsInvalidRatingSummary(t *testing.T) {
	for _, rating := range []any{map[string]any{"average": 6}, map[string]any{"average": -1}, map[string]any{"count": -1}, map[string]any{"distribution": map[string]any{"5": -1}}} {
		_, err := ParseInspectionDocument(evidenceDocument(t, map[string]any{"product": syntheticEvidenceCard("201", "301"), "rating": rating}), core.ProductInspectRequest{ProductID: "101"})
		if !errors.Is(err, ErrProductDataMissing) {
			t.Fatalf("invalid summary accepted: %#v", rating)
		}
	}
}

func TestInspectionNullRatingBucketDoesNotBecomeZero(t *testing.T) {
	doc := map[string]any{"product": syntheticEvidenceCard("201", "301"),
		"rating":   map[string]any{"distribution": map[string]any{"5": nil}},
		"coverage": map[string]any{"observed_fields": []string{"rating.distribution"}}}
	_, err := ParseInspectionDocument(evidenceDocument(t, doc), core.ProductInspectRequest{ProductID: "101"})
	if !errors.Is(err, ErrProductDataMissing) {
		t.Fatal("null rating bucket became observed zero")
	}
}

func TestReviewDeclaredScalarsRequirePayloadAndPreserveZero(t *testing.T) {
	for _, field := range []string{"rating", "helpful_count"} {
		for _, mode := range []string{"missing", "null", "zero"} {
			review := map[string]any{"content": "Synthetic review", "observed_fields": []string{" " + field + " "}}
			if mode == "null" {
				review[field] = nil
			}
			if mode == "zero" {
				review[field] = 0
			}
			doc := map[string]any{"product": syntheticEvidenceCard("201", "301"), "reviews": []any{review}}
			result, err := ParseInspectionDocument(evidenceDocument(t, doc), core.ProductInspectRequest{ProductID: "101"})
			if mode != "zero" {
				if !errors.Is(err, ErrProductDataMissing) {
					t.Fatalf("review %s %s accepted", field, mode)
				}
				continue
			}
			if err != nil || len(result.Reviews) != 1 {
				t.Fatalf("known zero lost: %v", err)
			}
			var wire map[string]any
			if err := json.Unmarshal(evidenceDocument(t, result.Reviews[0]), &wire); err != nil {
				t.Fatal(err)
			}
			if value, ok := wire[field]; !ok || value != float64(0) {
				t.Fatal("review zero disappeared on wire")
			}
		}
	}
}
