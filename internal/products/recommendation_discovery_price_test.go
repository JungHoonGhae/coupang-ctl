package products

import (
	"context"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// Discovery prices may be missing or describe a different/stale offer. Only
// the inspected option can establish the recommendation's budget condition.
type discoveryPriceSource struct {
	*selectionSource
	searchPrice string
	detailPrice string
}

func (s *discoveryPriceSource) Inspect(ctx context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	inspection, err := s.selectionSource.Inspect(ctx, r)
	switch s.detailPrice {
	case "missing":
		inspection.Product.ObservedFields = []string{"sponsored"}
		inspection.Product.FieldEvidence = nil
		inspection.Product.Price = core.ProductPrice{}
	case "above_budget":
		inspection.Product.Price.CurrentAmount = 9000
	}
	return inspection, err
}

func (s *discoveryPriceSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	items, coverage, err := s.selectionSource.Search(ctx, r)
	for i := range items {
		switch s.searchPrice {
		case "missing":
			items[i].ObservedFields = []string{"sponsored"}
			items[i].FieldEvidence = nil
			items[i].Price = core.ProductPrice{}
		case "above_budget":
			items[i].Price.CurrentAmount = 9000
		}
	}
	return items, coverage, err
}

func TestRecommendationChecksDetailBeforeBudgetExclusion(t *testing.T) {
	for _, price := range []string{"missing", "above_budget"} {
		t.Run(price, func(t *testing.T) {
			source := &discoveryPriceSource{selectionSource: newSelectionSource(1), searchPrice: price}
			result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{
				Query: "synthetic", Proceed: true, MaxPrice: 2000, SearchPageLimit: 1, DisableAffiliate: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(source.inspected) != 1 || len(result.Discovered) != 1 || len(result.Inspected) != 1 || len(result.Candidates) != 1 {
				t.Fatalf("discovery price removed a potentially eligible option before inspection: inspected=%d discovered=%d candidates=%d", len(source.inspected), len(result.Discovered), len(result.Candidates))
			}
			condition := result.Candidates[0].Conditions[0]
			if condition.Status != core.ProductConditionMet || condition.ObservedInteger == nil || *condition.ObservedInteger != 1200 {
				t.Fatal("budget was not established from the inspected price")
			}
		})
	}
}

func TestExplicitSearchStillFiltersUnknownOrOverBudgetPrice(t *testing.T) {
	for _, price := range []string{"missing", "above_budget"} {
		source := &discoveryPriceSource{selectionSource: newSelectionSource(1), searchPrice: price}
		result, err := New(source).Search(context.Background(), core.ProductSearchRequest{Query: "synthetic", MaxPrice: 2000, DisableAffiliate: true})
		if err != nil || len(result.Items) != 0 || result.Coverage.SourceNoResults {
			t.Fatal("explicit search filtering changed or became source exhaustion")
		}
	}
}

func TestRecommendationDiscoveryDoesNotPromoteUnverifiedBudget(t *testing.T) {
	for _, test := range []struct {
		price  string
		status core.ProductConditionStatus
		reason string
	}{
		{"missing", core.ProductConditionUnknown, "budget_condition_unverified"},
		{"above_budget", core.ProductConditionUnmet, "over_budget"},
	} {
		t.Run(test.price, func(t *testing.T) {
			source := &discoveryPriceSource{selectionSource: newSelectionSource(1), searchPrice: "missing", detailPrice: test.price}
			result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{
				Query: "synthetic", Proceed: true, MaxPrice: 2000, SearchPageLimit: 1, DisableAffiliate: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Candidates) != 0 || len(result.Inspected) != 1 || len(source.inspected) != 1 {
				t.Fatal("unknown discovery price bypassed the required detail budget check")
			}
			candidate := result.Inspected[0]
			if candidate.Conditions[0].Status != test.status || candidate.ExclusionReason != test.reason {
				t.Fatal("unknown and failed budget conditions were conflated")
			}
			if result.Audit.InspectionAttempts != 1 || result.Audit.DocumentReadReservations > result.Audit.DocumentReadBudget {
				t.Fatal("discovery recovery exceeded the existing investigation budget")
			}
		})
	}
}
