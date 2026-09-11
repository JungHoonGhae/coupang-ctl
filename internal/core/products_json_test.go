package core

import (
	"encoding/json"
	"testing"
)

func productJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProductCardJSONOmitsUnverifiedValues(t *testing.T) {
	card := ProductCard{Rating: 4.5, ReviewCount: 12, Rocket: true, FreeShipping: true, Coupon: true,
		Price: ProductPrice{CurrentAmount: 1000, OriginalAmount: 2000, DiscountRate: 50, Currency: "KRW"}}
	got := productJSONMap(t, card)
	for _, field := range []string{"rating", "review_count", "rocket", "free_shipping", "coupon", "sponsored"} {
		if _, exists := got[field]; exists {
			t.Errorf("unknown field %s emitted", field)
		}
	}
	price := got["price"].(map[string]any)
	for _, field := range []string{"current_amount", "original_amount", "discount_rate"} {
		if _, exists := price[field]; exists {
			t.Errorf("unknown price field %s emitted", field)
		}
	}
}

func TestProductCardJSONPreservesVerifiedZeroAndFalse(t *testing.T) {
	card := ProductCard{ObservedFields: []string{"rating", "review_count", "rocket", "free_shipping", "coupon", "sponsored", "price.current_amount", "price.original_amount", "price.discount_rate"}}
	got := productJSONMap(t, card)
	for _, field := range []string{"rocket", "free_shipping", "coupon", "sponsored"} {
		if value, exists := got[field]; !exists || value != false {
			t.Errorf("known false %s lost: %v", field, got)
		}
	}
	for _, field := range []string{"rating", "review_count"} {
		if value, exists := got[field]; !exists || value != float64(0) {
			t.Errorf("known zero %s lost", field)
		}
	}
	price := got["price"].(map[string]any)
	for _, field := range []string{"current_amount", "original_amount", "discount_rate"} {
		if value, exists := price[field]; !exists || value != float64(0) {
			t.Errorf("known zero price %s lost", field)
		}
	}
}

func TestInspectionJSONDistinguishesGranularCoverage(t *testing.T) {
	for _, known := range []bool{false, true} {
		inspection := ProductInspection{}
		if known {
			inspection.Coverage.ObservedFields = []string{"delivery.rocket", "delivery.free_shipping", "rating.average", "rating.count"}
		}
		got := productJSONMap(t, inspection)
		for _, pair := range [][2]string{{"delivery", "rocket"}, {"delivery", "free_shipping"}, {"rating", "average"}, {"rating", "count"}} {
			_, exists := got[pair[0]].(map[string]any)[pair[1]]
			if exists != known {
				t.Errorf("%v known=%v exists=%v", pair, known, exists)
			}
		}
	}
}

func TestProductCardJSONRoundTripPreservesAvailability(t *testing.T) {
	before := ProductCard{ObservedFields: []string{"sponsored", "price.current_amount"}, Price: ProductPrice{Currency: "KRW"}}
	data, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	var after ProductCard
	if err := json.Unmarshal(data, &after); err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(again) {
		t.Fatalf("availability changed: %s / %s", data, again)
	}
}

func TestInspectionJSONPreservesKnownEmptyRatingDistribution(t *testing.T) {
	inspection := ProductInspection{Rating: ProductRatingSummary{Distribution: map[string]int{}}, Coverage: ProductCoverage{ObservedFields: []string{"rating.distribution"}}}
	got := productJSONMap(t, inspection)["rating"].(map[string]any)
	if distribution, ok := got["distribution"].(map[string]any); !ok || len(distribution) != 0 {
		t.Fatal("known empty distribution lost")
	}
	inspection.Coverage.ObservedFields = nil
	if _, present := productJSONMap(t, inspection)["rating"].(map[string]any)["distribution"]; present {
		t.Fatal("unknown distribution became empty distribution")
	}
}

func TestReviewJSONDistinguishesUnknownFromKnownZero(t *testing.T) {
	for _, known := range []bool{false, true} {
		review := ProductReview{Content: "Synthetic review"}
		if known {
			review.ObservedFields = []string{"rating", "helpful_count"}
		}
		got := productJSONMap(t, review)
		for _, field := range []string{"rating", "helpful_count"} {
			value, exists := got[field]
			if exists != known || (known && value != float64(0)) {
				t.Fatalf("%s availability changed: %#v", field, got)
			}
		}
		if got["content"] != review.Content {
			t.Fatal("review content lost")
		}
	}
}
