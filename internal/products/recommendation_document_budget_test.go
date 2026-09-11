package products

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type documentBudgetSource struct {
	*selectionSource
	searchLimits, detailLimits []int
	failDetails                bool
	appearAt, failSearchAt     int
}

func (s *documentBudgetSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	s.searchLimits = append(s.searchLimits, r.DocumentReadLimit)
	if len(s.searchLimits) == s.failSearchAt {
		return nil, core.ProductCoverage{}, errors.New("synthetic search unavailable")
	}
	if len(s.searchLimits) < s.appearAt {
		return nil, core.ProductCoverage{}, nil
	}
	return s.selectionSource.Search(ctx, r)
}
func (s *documentBudgetSource) Inspect(ctx context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	s.detailLimits = append(s.detailLimits, r.DocumentReadLimit)
	if s.failDetails {
		return core.ProductInspection{}, errors.New("synthetic unavailable")
	}
	result, err := s.selectionSource.Inspect(ctx, r)
	if r.DocumentReadLimit < 3 {
		result.Coverage.BudgetOmittedFields = []string{"reviews"}
		result.Coverage.UnavailableFields = []string{"reviews"}
	}
	return result, err
}

func TestRecommendationBoundsDocumentReservationsAndPreservesInspectionBreadth(t *testing.T) {
	for _, count := range []int{0, 6, 8, 20, 24} {
		var previous []int
		for _, display := range []int{0, 1, 3} {
			source := &documentBudgetSource{selectionSource: newSelectionSource(count)}
			result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxItems: display, SearchPageLimit: 10})
			if err != nil {
				t.Fatal(err)
			}
			reserved := 0
			for _, limit := range source.searchLimits {
				if limit != 1 {
					t.Fatal("search can retry outside reservation")
				}
				reserved += limit
			}
			for _, limit := range source.detailLimits {
				if limit < 1 || limit > 3 {
					t.Fatal("detail allowance absent or invalid")
				}
				reserved += limit
			}
			if reserved > 40 {
				t.Fatalf("count=%d reserved=%d exceeds total allowance", count, reserved)
			}
			if result.Audit.DocumentReadBudget != 40 || result.Audit.DocumentReadReservations != reserved || !result.Audit.DocumentBudgetConstrained {
				t.Fatal("reported document budget differs from issued allowances")
			}
			want := min(count, 20)
			if len(source.detailLimits) != want {
				t.Fatalf("count=%d inspected=%d want=%d: discovery starved known candidates", count, len(source.detailLimits), want)
			}
			if previous != nil && !reflect.DeepEqual(previous, source.detailLimits) {
				t.Fatal("presentation cap changed investigation allowance")
			}
			previous = source.detailLimits
			if result.Status != core.ProductRecommendationIncomplete {
				t.Fatal("operational budget became evidence sufficiency")
			}
		}
	}
}

func TestRecommendationLateDiscoveryRetainsUninspectedEvidence(t *testing.T) {
	source := &documentBudgetSource{selectionSource: newSelectionSource(20), appearAt: 40}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(source.searchLimits) != 40 || len(source.detailLimits) != 0 || len(result.Discovered) != 20 || result.Audit.UninspectedExactOptions != 20 {
		t.Fatal("late discovery was discarded or caused unreserved inspection")
	}
	if result.Audit.DocumentReadReservations != 40 || result.Audit.DiscoveryStopReason != "document_budget_reached" || result.Audit.InspectionStopReason != "document_budget_reached" || result.Status != core.ProductRecommendationIncomplete {
		t.Fatal("budget exhaustion became source exhaustion or evidence sufficiency")
	}
}

func TestRecommendationAuxiliaryOmissionsHaveExactFollowup(t *testing.T) {
	source := &documentBudgetSource{selectionSource: newSelectionSource(20)}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Audit.InspectionStopReason != "auxiliary_document_budget_limited" || len(result.Inspected) != 20 {
		t.Fatal("auxiliary omission lost or confused with skipped detail")
	}
	for _, candidate := range result.Inspected {
		found := false
		for _, action := range result.NextActions {
			if action.Reason == "document_budget_omitted_fields" && slices.Contains(action.Fields, "reviews") && slices.Contains(action.References, candidate.Product.Reference) {
				found = true
			}
		}
		if !found {
			t.Fatal("budget-omitted review lacks exact-option followup")
		}
	}
}

func TestRecommendationFailedSearchIsIncludedInReservations(t *testing.T) {
	source := &documentBudgetSource{selectionSource: newSelectionSource(20), failSearchAt: 2}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	reserved := len(source.searchLimits)
	for _, limit := range source.detailLimits {
		reserved += limit
	}
	if result.Audit.DocumentReadReservations != reserved || reserved > 40 || result.Audit.SearchesRun != len(source.searchLimits) || result.Audit.SearchPagesRead != len(source.searchLimits)-1 {
		t.Fatal("failed search was refunded or reported as an observed page")
	}
	if result.Status != core.ProductRecommendationIncomplete || result.Audit.DiscoveryStopReason != "source_read_incomplete" {
		t.Fatal("source failure hidden by the operational budget")
	}
}

func TestRecommendationFailedDetailReservationsAreNotRefunded(t *testing.T) {
	source := &documentBudgetSource{selectionSource: newSelectionSource(24), failDetails: true}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	reserved := len(source.searchLimits)
	for _, limit := range source.detailLimits {
		if limit < 1 {
			t.Fatal("missing detail reservation")
		}
		reserved += limit
	}
	if reserved > 40 || len(result.Inspected) != 0 || result.Audit.UninspectedExactOptions != 24 || result.Status != core.ProductRecommendationIncomplete {
		t.Fatal("failed work was refunded or discovery evidence lost")
	}
}
