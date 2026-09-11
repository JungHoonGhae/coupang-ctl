package products

import (
	"context"
	"slices"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestBooleanFiltersRequireFieldEvidence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request core.ProductSearchRequest
		field   string
		card    core.ProductCard
	}{
		{"rocket", core.ProductSearchRequest{RocketOnly: true}, "rocket", core.ProductCard{Rocket: true}},
		{"free shipping", core.ProductSearchRequest{FreeShippingOnly: true}, "free_shipping", core.ProductCard{FreeShipping: true}},
		{"not sponsored", core.ProductSearchRequest{ExcludeSponsored: true}, "sponsored", core.ProductCard{Sponsored: false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := filterProducts([]core.ProductCard{tc.card}, tc.request); len(got) != 0 {
				t.Fatal("unknown condition passed filter")
			}
			tc.card.ObservedFields = []string{tc.field}
			if got := filterProducts([]core.ProductCard{tc.card}, tc.request); len(got) != 1 {
				t.Fatal("verified condition rejected")
			}
		})
	}
}

func TestSearchReportsUnavailableFilterEvidenceBeforeDisplayLimit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request core.ProductSearchRequest
		field   string
	}{
		{"price", core.ProductSearchRequest{MaxPrice: 2000}, "price.current_amount"},
		{"rating", core.ProductSearchRequest{MinRating: 4}, "rating"},
		{"reviews", core.ProductSearchRequest{MinReviewCount: 1}, "review_count"},
		{"rocket", core.ProductSearchRequest{RocketOnly: true}, "rocket"},
		{"shipping", core.ProductSearchRequest{FreeShippingOnly: true}, "free_shipping"},
		{"ad", core.ProductSearchRequest{ExcludeSponsored: true}, "sponsored"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			known := core.ProductCard{Name: "Synthetic known", URL: "https://www.coupang.com/vp/products/1", Price: core.ProductPrice{CurrentAmount: 1000}, Rating: 5, ReviewCount: 1, Rocket: true, FreeShipping: true, ObservedFields: []string{tc.field}}
			known.Reference.ProductID = "1"
			if tc.field == "price.current_amount" {
				known = withSyntheticPriceEvidence(known)
			}
			unknown := known
			unknown.URL = "https://www.coupang.com/vp/products/2"
			unknown.ObservedFields = nil
			request := tc.request
			request.Query, request.Limit = "synthetic", 1
			for _, cards := range [][]core.ProductCard{{known, unknown}, {unknown}, {known}, {}} {
				got, err := New(syntheticSource{items: cards}).Search(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				wantUnknown := len(cards) == 2 || (len(cards) == 1 && cards[0].ObservedFields == nil)
				if slices.Contains(got.UnavailableFilterFields, tc.field) != wantUnknown {
					t.Fatalf("lost field coverage: %#v", got.UnavailableFilterFields)
				}
				wantItems := 0
				if len(cards) > 0 && cards[0].ObservedFields != nil {
					wantItems = 1
				}
				if len(got.Items) != wantItems {
					t.Fatalf("items=%d want=%d", len(got.Items), wantItems)
				}
			}
			unfiltered, err := New(syntheticSource{items: []core.ProductCard{unknown}}).Search(context.Background(), core.ProductSearchRequest{Query: "synthetic"})
			if err != nil || len(unfiltered.Items) != 1 || len(unfiltered.UnavailableFilterFields) != 0 {
				t.Fatal("unrequested filter removed an unknown product")
			}
		})
	}
}
