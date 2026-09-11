package products

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type timeBudgetSource struct {
	*selectionSource
	searches, details        int
	pauseSearch, pauseDetail int
	cancel                   context.CancelFunc
}

func (s *timeBudgetSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	s.searches++
	if s.searches == s.pauseSearch {
		<-ctx.Done()
		return nil, core.ProductCoverage{}, ctx.Err()
	}
	return s.selectionSource.Search(ctx, r)
}

func (s *timeBudgetSource) Inspect(ctx context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	s.details++
	if s.details == s.pauseDetail {
		if s.cancel != nil {
			s.cancel()
		}
		<-ctx.Done()
		return core.ProductInspection{}, ctx.Err()
	}
	return s.selectionSource.Inspect(ctx, r)
}

func TestRecommendationTimeBudgetPreservesCompletedEvidence(t *testing.T) {
	for _, test := range []struct {
		name                                                             string
		search, detail, wantSearches, wantDetails, discovered, inspected int
	}{
		{"initial search", 1, 0, 1, 0, 0, 0},
		{"discovery", 3, 0, 3, 0, 3, 0},
		{"inspection", 0, 2, 5, 2, 3, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &timeBudgetSource{selectionSource: newSelectionSource(3), pauseSearch: test.search, pauseDetail: test.detail}
			result, err := New(source).recommendWithTimeBudget(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1, DisableAffiliate: true}, 100*time.Millisecond)
			if err != nil {
				t.Fatalf("internal time budget discarded partial result: %v", err)
			}
			if result.Status != core.ProductRecommendationIncomplete || len(result.Discovered) != test.discovered || len(result.Inspected) != test.inspected {
				t.Fatal("partial evidence was lost or promoted to complete")
			}
			if !result.Audit.TimeBudgetExhausted || result.Audit.SearchPagesRead != test.wantSearches-boolInt(test.search > 0) || result.Audit.InspectionAttempts != test.wantDetails || result.Audit.DocumentReadReservations != test.wantSearches+3*test.wantDetails {
				t.Fatal("deadline audit confused attempts with completed evidence")
			}
			if source.searches != test.wantSearches || source.details != test.wantDetails || result.Audit.UninspectedExactOptions != test.discovered-test.inspected {
				t.Fatal("new reads started after deadline or unfinished evidence was lost")
			}
			if test.detail == 0 && result.Audit.DiscoveryStopReason != "time_budget_reached" {
				t.Fatal("discovery deadline is not distinct from source failure")
			}
			if result.Audit.InspectionStopReason != "time_budget_reached" {
				t.Fatal("inspection deadline missing")
			}
		})
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type timeBudgetHistory struct{ calls int }

func (h *timeBudgetHistory) RecommendationPurchaseContext(ctx context.Context, _ []core.ProductReference) (core.RecommendationPurchaseContext, error) {
	h.calls++
	<-ctx.Done()
	return core.RecommendationPurchaseContext{}, ctx.Err()
}

func TestRecommendationTimeBudgetDuringHistoryRetainsProductEvidence(t *testing.T) {
	history := &timeBudgetHistory{}
	source := &timeBudgetSource{selectionSource: newSelectionSource(3)}
	result, err := New(source).WithPurchaseHistory(history).recommendWithTimeBudget(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1, UsePurchaseHistory: true}, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if history.calls != 1 || len(result.Inspected) != 3 || result.Status != core.ProductRecommendationIncomplete || !result.Audit.TimeBudgetExhausted {
		t.Fatal("history timeout erased completed inspections")
	}
	if result.PurchaseContext == nil || result.PurchaseContext.Status != "unavailable" || result.PurchaseContext.Visibility != "private_local" {
		t.Fatal("unfinished history was promoted to available")
	}
	if result.Audit.InspectionStopReason != "discovered_candidates_inspected" {
		t.Fatal("history timeout retroactively changed completed detail reads")
	}
}

func TestRecommendationTimeBudgetDoesNotStartHistoryAfterExpiredDetail(t *testing.T) {
	history := &timeBudgetHistory{}
	source := &timeBudgetSource{selectionSource: newSelectionSource(3), pauseDetail: 2}
	result, err := New(source).WithPurchaseHistory(history).recommendWithTimeBudget(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1, UsePurchaseHistory: true}, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if history.calls != 0 || len(result.Inspected) != 1 || result.PurchaseContext == nil || result.PurchaseContext.Status != "unavailable" {
		t.Fatal("history read started after internal deadline")
	}
}

func TestRecommendationAlreadyCanceledCallerStartsNoReads(t *testing.T) {
	source := &timeBudgetSource{selectionSource: newSelectionSource(3)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(source).Recommend(ctx, core.ProductRecommendationRequest{Query: "synthetic", Proceed: true})
	if !errors.Is(err, context.Canceled) || source.searches != 0 || source.details != 0 {
		t.Fatal("already canceled caller started a source read")
	}
}

func TestRecommendationExplicitCallerCancellationRemainsAnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &timeBudgetSource{selectionSource: newSelectionSource(3), pauseDetail: 2, cancel: cancel}
	_, err := New(source).Recommend(ctx, core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1})
	if !errors.Is(err, context.Canceled) || source.details != 2 {
		t.Fatal("explicit cancellation became successful partial output or started more reads")
	}
}

func TestRecommendationCallerDeadlineRemainsAnError(t *testing.T) {
	source := &timeBudgetSource{selectionSource: newSelectionSource(3), pauseDetail: 2}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := New(source).recommendWithTimeBudget(ctx, core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1}, time.Hour)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("caller deadline was swallowed as an internal budget")
	}
}
