package products

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestRecommendationClaimsPreserveFieldEvidence(t *testing.T) {
	inspection := core.ProductInspection{Product: syntheticRecommendationProduct(), SelectedOptions: []string{"Synthetic option"}}
	inspection.Product.Price.Currency = "KRW"
	inspection.Product.Rating = 4.5
	inspection.Product.ObservedFields = append(inspection.Product.ObservedFields, "rating")
	inspection.Coverage.ObservedFields = []string{"selected_options"}
	stamp := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	price := core.ProductFieldEvidence{Field: "price.current_amount", Source: "json_ld", Locator: "jsonld.Product.offers.price", Method: "native_field", Provenance: "observed", Scope: "selected_option", Reference: inspection.Product.Reference, CapturedAt: stamp}
	rating := core.ProductFieldEvidence{Field: "rating", Source: "review_endpoint", Locator: "reviews.ratingSummaryTotal.ratingAverage", Method: "alias_lookup", Provenance: "inferred", Scope: "product_page", Reference: core.ProductReference{ProductID: inspection.Product.Reference.ProductID}, CapturedAt: stamp}
	option := core.ProductFieldEvidence{Field: "selected_options", Source: "dom", Locator: "dom.option_picker.selected.text", Method: "selected_option_text", Provenance: "derived", Scope: "selected_option", Reference: inspection.Product.Reference, CapturedAt: stamp}
	inspection.Product.FieldEvidence = []core.ProductFieldEvidence{price, rating}
	inspection.FieldEvidence = []core.ProductFieldEvidence{option}
	data, err := json.Marshal(genericRecommendationCandidate(inspection))
	if err != nil {
		t.Fatal(err)
	}
	var decoded core.ProductRecommendationCandidate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Claims) != 3 {
		t.Fatalf("claims=%d", len(decoded.Claims))
	}
	for i, want := range []core.ProductFieldEvidence{price, rating, option} {
		got := decoded.Claims[i]
		if got.Evidence == nil || !reflect.DeepEqual(*got.Evidence, want) || got.Provenance != want.Provenance || got.Source != want.Source {
			t.Fatalf("claim lost provenance: %#v", got)
		}
	}
	if decoded.Claims[0].Value != "1200 KRW" {
		t.Fatal("price value changed")
	}
}

func TestRecommendationReviewCountRequiresConsistentAvailability(t *testing.T) {
	positive := 12
	for _, test := range []struct {
		name                    string
		count, cardCount        int
		summaryKnown, cardKnown bool
		want                    *int
	}{
		{"unknown", 0, 0, false, false, nil},
		{"unverified_positive", 7, 9, false, false, nil},
		{"summary_zero", 0, 9, true, false, new(int)},
		{"card_zero", 7, 0, false, true, new(int)},
		{"conflict", 0, 9, true, true, nil},
		{"both_zero", 0, 0, true, true, new(int)},
		{"both_positive", 12, 12, true, true, &positive},
		{"negative_summary", -1, 12, true, false, nil},
		{"negative_card", 12, -1, false, true, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			inspection := core.ProductInspection{Product: syntheticRecommendationProduct(), Rating: core.ProductRatingSummary{Count: test.count}}
			inspection.Product.ReviewCount = test.cardCount
			inspection.Product.ReviewScope = "product_page_observed"
			if test.summaryKnown {
				inspection.Coverage.ObservedFields = []string{"rating.count"}
			}
			if test.cardKnown {
				inspection.Product.ObservedFields = append(inspection.Product.ObservedFields, "review_count")
			}
			candidate := genericRecommendationCandidate(inspection)
			data, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			var output map[string]any
			if err := json.Unmarshal(data, &output); err != nil {
				t.Fatal(err)
			}
			value, present := output["reviews_available"]
			if present != (test.want != nil) || present && value != float64(*test.want) {
				t.Fatalf("reviews_available=%v, present=%v, want=%v", value, present, test.want)
			}
			if test.want != nil && output["review_count_scope"] != "product_page_observed" {
				t.Fatal("review count lost its page-level scope")
			}
			if test.want == nil && !contains(candidate.MissingEvidence, "review_count") {
				t.Fatal("missing review count not disclosed")
			}
		})
	}
}

func TestRecommendationReviewCountAuditDeduplicatesPageScope(t *testing.T) {
	count := func(id string, n int) core.ProductRecommendationCandidate {
		return core.ProductRecommendationCandidate{Product: core.ProductCard{Reference: core.ProductReference{ProductID: id}}, ReviewsAvailable: &n, ReviewCountScope: "product_page_observed"}
	}
	unknown := count("102", 7)
	unknown.ReviewsAvailable = nil
	unscoped := count("102", 7)
	unscoped.ReviewCountScope = "unknown"
	for _, test := range []struct {
		name               string
		candidates         []core.ProductRecommendationCandidate
		known, unavailable int
		want               *int
	}{
		{"none", nil, 0, 0, nil},
		{"known_zero", []core.ProductRecommendationCandidate{count("101", 0)}, 1, 0, new(int)},
		{"same_page_options", []core.ProductRecommendationCandidate{count("101", 7), count("101", 7)}, 1, 0, reviewCountPointer(7)},
		{"different_pages", []core.ProductRecommendationCandidate{count("101", 7), count("102", 3)}, 2, 0, reviewCountPointer(10)},
		{"partial", []core.ProductRecommendationCandidate{count("101", 7), unknown}, 1, 1, nil},
		{"scope_unknown", []core.ProductRecommendationCandidate{count("101", 7), unscoped}, 1, 1, nil},
		{"page_conflict", []core.ProductRecommendationCandidate{count("101", 7), count("101", 9)}, 0, 1, nil},
		{"unknown_not_overwritten", []core.ProductRecommendationCandidate{unknown, count("102", 7)}, 0, 1, nil},
		{"known_not_overwritten", []core.ProductRecommendationCandidate{count("102", 7), unknown}, 0, 1, nil},
		{"unknown_identity", []core.ProductRecommendationCandidate{count("", 7)}, 0, 1, nil},
		{"overflow", []core.ProductRecommendationCandidate{count("101", math.MaxInt), count("102", 1)}, 2, 0, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			counts := recommendationReviewCounts{}
			for _, c := range test.candidates {
				counts.add(c)
			}
			var audit core.ProductRecommendationAudit
			counts.apply(&audit)
			if audit.ReviewCountPages != test.known || audit.ReviewCountUnavailablePages != test.unavailable {
				t.Fatalf("unexpected coverage: %+v", audit)
			}
			got := audit.ReviewsAvailable
			if (got == nil) != (test.want == nil) || got != nil && *got != *test.want {
				t.Fatalf("sum=%v want=%v", got, test.want)
			}
		})
	}
}

func reviewCountPointer(value int) *int { return &value }

func TestRecommendReviewCountAvailabilityReachesCandidateAndAudit(t *testing.T) {
	for _, known := range []bool{false, true} {
		product := syntheticRecommendationProduct()
		product.ReviewScope = "product_page_observed"
		inspection := core.ProductInspection{Product: product}
		if known {
			inspection.Coverage.ObservedFields = []string{"rating.count"}
		}
		source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: inspection}}
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Candidates) != 1 {
			t.Fatal("expected inspected candidate")
		}
		for _, got := range []*int{result.Candidates[0].ReviewsAvailable, result.Audit.ReviewsAvailable} {
			if (got != nil) != known || got != nil && *got != 0 {
				t.Fatal("review count availability was lost")
			}
		}
	}
}

func TestRecommendBudgetPreservesObservedZeroPrice(t *testing.T) {
	product := withSyntheticPriceEvidence(syntheticRecommendationProduct())
	product.Price.CurrentAmount = 0
	source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: core.ProductInspection{Product: product}}}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxPrice: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Product.Price.CurrentAmount != 0 {
		t.Fatal("known zero price was treated as unavailable")
	}
}

func TestRecommendationDoesNotInventProvenanceFromAvailability(t *testing.T) {
	product := syntheticRecommendationProduct()
	product.Rating = 4.5
	product.ObservedFields = append(product.ObservedFields, "rating")
	candidate := genericRecommendationCandidate(core.ProductInspection{Product: product, SelectedOptions: []string{"Synthetic option"}})
	if len(candidate.Claims) != 0 {
		t.Fatal("availability or DOM text alone was promoted to observed claims")
	}
}
