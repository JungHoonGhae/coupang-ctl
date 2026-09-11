package products

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type optionSelectionSource struct {
	selectionSource
	firstOutcome string
	requests     []core.ProductReference
}

func newOptionSelectionSource(optionCount int) *optionSelectionSource {
	s := &optionSelectionSource{}
	for i := 0; i < optionCount; i++ {
		p := syntheticRecommendationProduct()
		p.ReviewCount = 7
		p.ReviewScope = "product_page_observed"
		p.ObservedFields = append(p.ObservedFields, "review_count")
		p.Reference.ItemID = fmt.Sprint(400 + i)
		p.Reference.VendorItemID = fmt.Sprint(700 + i)
		p.URL = fmt.Sprintf("https://www.coupang.com/vp/products/123?itemId=%s&vendorItemId=%s", p.Reference.ItemID, p.Reference.VendorItemID)
		s.items = append(s.items, withSyntheticPriceEvidence(p))
	}
	other := newSelectionSource(1).items[0]
	other.ReviewCount = 3
	other.ReviewScope = "product_page_observed"
	other.ObservedFields = append(other.ObservedFields, "review_count")
	s.items = append(s.items, other)
	return s
}

func (s *optionSelectionSource) Inspect(_ context.Context, request core.ProductInspectRequest) (core.ProductInspection, error) {
	ref := core.ProductReference{ProductID: request.ProductID, ItemID: request.ItemID, VendorItemID: request.VendorItemID}
	s.requests = append(s.requests, ref)
	for i, p := range s.items {
		if p.Reference != ref {
			continue
		}
		if i == 0 {
			switch s.firstOutcome {
			case "failed":
				return core.ProductInspection{}, errors.New("synthetic read failure")
			case "wrong_option":
				p.Reference = s.items[1].Reference
			case "over_budget":
				p.Price.CurrentAmount = 9000
			}
		}
		return core.ProductInspection{Product: p}, nil
	}
	return core.ProductInspection{}, errors.New("synthetic exact option missing")
}

func TestRecommendationPreservesAlternateOptionsAfterFirstFailure(t *testing.T) {
	for _, outcome := range []string{"failed", "wrong_option", "over_budget", "valid"} {
		t.Run(outcome, func(t *testing.T) {
			source := newOptionSelectionSource(2)
			source.firstOutcome = outcome
			result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxPrice: 2000, SearchPageLimit: 1})
			if err != nil {
				t.Fatal(err)
			}
			wantOrder := []core.ProductReference{source.items[0].Reference, source.items[2].Reference, source.items[1].Reference}
			if !reflect.DeepEqual(source.requests, wantOrder) {
				t.Fatal("options were lost or one family monopolized inspection")
			}
			if len(result.Discovered) != 2 || len(result.Discovered[0].Options) != 2 || len(result.Discovered[1].Options) != 1 {
				t.Fatal("discovery did not group products while retaining exact options")
			}
			wantCandidates, wantInspected := 2, 3
			if outcome == "valid" {
				wantCandidates = 3
			}
			if outcome == "failed" || outcome == "wrong_option" {
				wantInspected = 2
			}
			if len(result.Candidates) != wantCandidates || len(result.Inspected) != wantInspected || result.Candidates[len(result.Candidates)-1].Product.Reference != source.items[1].Reference {
				t.Fatal("valid alternate option discarded or mismatched inspection used")
			}
			if result.Audit.UniqueExactOptions != 3 || result.Audit.UniqueModelFamilies != 2 || result.Audit.UninspectedModelFamilies != 0 || result.Audit.UninspectedExactOptions != 3-wantInspected {
				t.Fatalf("incorrect grouped coverage: %+v", result.Audit)
			}
			if result.Audit.ReviewsAvailable == nil || *result.Audit.ReviewsAvailable != 10 || result.Audit.ReviewCountPages != 2 {
				t.Fatal("page review totals were counted once per option instead of per product")
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var wire core.ProductRecommendationResult
			if err := json.Unmarshal(encoded, &wire); err != nil {
				t.Fatal(err)
			}
			if len(wire.Discovered[0].Options) != 2 || wire.Discovered[0].Options[1].Reference != source.items[1].Reference {
				t.Fatal("wire result lost alternate identity")
			}
		})
	}
}

func TestRecommendationOptionBudgetCountsExactReferences(t *testing.T) {
	source := newOptionSelectionSource(22)
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1, MaxItems: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(source.requests) != recommendationInspectionBudget || source.requests[1] != source.items[22].Reference {
		t.Fatal("budget or cross-product scheduling violated")
	}
	if result.Status != core.ProductRecommendationIncomplete || result.Audit.InspectionStopReason != "inspection_budget_reached" || result.Audit.UninspectedExactOptions != 3 || result.Audit.UninspectedModelFamilies != 0 {
		t.Fatalf("option coverage overstated: %+v", result.Audit)
	}
	if len(result.Discovered[0].Options) != 22 || len(result.Inspected) != 20 || len(result.Candidates) != 1 {
		t.Fatal("budget/display limit erased option evidence")
	}
}
