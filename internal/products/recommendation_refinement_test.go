package products

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type refinementSource struct {
	recommendationSource
	requests     []core.ProductSearchRequest
	detailLimits []int
	catalog      func(core.ProductSearchRequest) []core.ProductFacet
	failAt       int
	failure      error
}

func (s *refinementSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	s.requests = append(s.requests, r)
	if len(s.requests) == s.failAt {
		return nil, core.ProductCoverage{}, s.failure
	}
	items, coverage, err := s.recommendationSource.Search(ctx, r)
	coverage.Facets = s.catalog(r)
	return items, coverage, err
}

func (s *refinementSource) Inspect(ctx context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	s.detailLimits = append(s.detailLimits, r.DocumentReadLimit)
	return s.recommendationSource.Inspect(ctx, r)
}

func newRefinementSource(catalog func(core.ProductSearchRequest) []core.ProductFacet) *refinementSource {
	p := syntheticRecommendationProduct()
	return &refinementSource{recommendationSource: recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{p}, inspection: core.ProductInspection{Product: p}}}, catalog: catalog}
}

func markedCatalog(groups []core.ProductFacet, selections []core.ProductFacetSelection) []core.ProductFacet {
	groups = cloneFacetCatalog(groups)
	for i := range groups {
		for j := range groups[i].Options {
			for _, s := range selections {
				if groups[i].Name == s.Name && groups[i].Options[j].Label == s.Label {
					groups[i].Options[j].Selected = true
				}
			}
		}
	}
	return groups
}

func TestRecommendUsesCumulativeNativeFacetsWithoutChangingQuery(t *testing.T) {
	groups := []core.ProductFacet{{Name: "category", Options: []core.ProductFacetOption{{Label: "computer"}}}, {Name: "memory", Options: []core.ProductFacetOption{{Label: "32GB"}}}}
	source := newRefinementSource(func(r core.ProductSearchRequest) []core.ProductFacet { return markedCatalog(groups, r.FacetSelections) })
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 10, Answers: []core.ProductRecommendationAnswer{{QuestionID: "facet:category", Choice: "computer"}, {QuestionID: "facet:memory", Choice: "32GB"}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Refinement.Issue != nil || len(result.Refinement.AppliedSelections) != 2 || len(result.Refinement.Steps) != 3 {
		t.Fatalf("missing trace: %+v", result.Refinement)
	}
	if len(source.requests) != 7 || len(source.detailLimits) != 1 {
		t.Fatalf("searches=%d detail=%d", len(source.requests), len(source.detailLimits))
	}
	reserved := 0
	for i, r := range source.requests {
		want := min(i, 2)
		if r.Query != "synthetic" || r.Page != 1 || len(r.FacetSelections) != want || r.DocumentReadLimit != want+1 {
			t.Fatalf("request %d lost state: %+v", i, r)
		}
		reserved += r.DocumentReadLimit
	}
	for _, v := range source.detailLimits {
		reserved += v
	}
	if result.Audit.DocumentReadReservations != reserved || reserved > 40 {
		t.Fatal("reservations disagree")
	}
	if result.Refinement.Steps[0].Facets[0].Options[0].Selected {
		t.Fatal("later selection mutated old catalog")
	}
}

func TestRecommendRediscoveryRejectsRemovedCategoryChoiceBeforeReadingAgain(t *testing.T) {
	category := core.ProductFacet{Name: "category", Options: []core.ProductFacetOption{{Label: "appliance"}}}
	source := newRefinementSource(func(r core.ProductSearchRequest) []core.ProductFacet {
		groups := []core.ProductFacet{category, {Name: "detergent_weight", Options: []core.ProductFacetOption{{Label: "1kg"}}}}
		if len(r.FacetSelections) > 0 {
			groups = []core.ProductFacet{category, {Name: "installation", Options: []core.ProductFacetOption{{Label: "built-in"}}}}
		}
		return markedCatalog(groups, r.FacetSelections)
	})
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic appliance", Proceed: true, Answers: []core.ProductRecommendationAnswer{{QuestionID: "facet:category", Choice: "appliance"}, {QuestionID: "facet:detergent_weight", Choice: "1kg"}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ProductRecommendationNeedsInput || result.Refinement.Issue.Reason != "choice_unavailable" || len(source.requests) != 2 || len(source.detailLimits) != 0 || len(result.Discovered) != 1 {
		t.Fatal("stale filter continued research or lost retained candidate")
	}
	if len(result.Facets) != 2 || result.Facets[1].Name != "installation" || len(result.Refinement.Steps[0].Facets) != 2 || result.Refinement.Steps[0].Facets[1].Name != "detergent_weight" {
		t.Fatal("catalog union lost state semantics")
	}
}

func TestRecommendRejectsUnknownDisabledAndAmbiguousChoices(t *testing.T) {
	for _, groups := range [][]core.ProductFacet{
		{{Name: "x", Options: []core.ProductFacetOption{{Label: "different"}}}},
		{{Name: "x", Options: []core.ProductFacetOption{{Label: "a", Disabled: true}}}},
		{{Name: "x", Options: []core.ProductFacetOption{{Label: "a"}, {Label: "a", Disabled: true}}}},
		{{Name: "x", Options: []core.ProductFacetOption{{Label: "a"}}}, {Name: "x", Options: []core.ProductFacetOption{{Label: "a"}}}},
	} {
		source := newRefinementSource(func(core.ProductSearchRequest) []core.ProductFacet { return groups })
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, Answers: []core.ProductRecommendationAnswer{{QuestionID: "facet:x", Choice: "a"}}})
		if err != nil || result.Status != core.ProductRecommendationNeedsInput || len(source.requests) != 1 || len(source.detailLimits) != 0 {
			t.Fatal("invalid choice reached source")
		}
	}
}

func TestRecommendStopsAfterUnverifiedOrDeniedSelection(t *testing.T) {
	for _, failure := range []error{nil, core.ErrBrowserAccessDenied, core.ErrAuthenticationRequired, errors.New("private diagnostic")} {
		source := newRefinementSource(func(core.ProductSearchRequest) []core.ProductFacet {
			return []core.ProductFacet{{Name: "x", Options: []core.ProductFacetOption{{Label: "a"}}}}
		})
		if failure != nil {
			source.failAt = 2
			source.failure = failure
		}
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, Answers: []core.ProductRecommendationAnswer{{QuestionID: "facet:x", Choice: "a"}}})
		if err != nil || result.Status != core.ProductRecommendationIncomplete || len(source.requests) != 2 || len(source.detailLimits) != 0 || len(result.Refinement.AppliedSelections) != 0 || result.Audit.DocumentReadReservations != 3 {
			t.Fatal("failed selection was accepted or refunded")
		}
		if strings.Contains(fmt.Sprint(result), "private diagnostic") {
			t.Fatal("raw error leaked")
		}
		if reason := recommendationAccessStopReason(failure); reason != "" && result.Refinement.Issue.Reason != reason {
			t.Fatal("access reason lost")
		}
	}
}

func TestRecommendSixChoicesStayInsideWholeResearchBudget(t *testing.T) {
	var groups []core.ProductFacet
	var answers []core.ProductRecommendationAnswer
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("g%d", i)
		groups = append(groups, core.ProductFacet{Name: name, Options: []core.ProductFacetOption{{Label: "a"}}})
		answers = append(answers, core.ProductRecommendationAnswer{QuestionID: "facet:" + name, Choice: "a"})
	}
	source := newRefinementSource(func(r core.ProductSearchRequest) []core.ProductFacet { return markedCatalog(groups, r.FacetSelections) })
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, Answers: answers})
	if err != nil || len(result.Refinement.AppliedSelections) != 6 {
		t.Fatalf("refinement failed: %v", err)
	}
	reserved := 0
	for _, r := range source.requests {
		reserved += r.DocumentReadLimit
	}
	for _, limit := range source.detailLimits {
		reserved += limit
	}
	if reserved > 40 || result.Audit.DocumentReadReservations != reserved || len(source.detailLimits) != 1 {
		t.Fatal("refinement overran budget or starved detail")
	}
}

func TestRecommendRetainsUnverifiedSponsorshipWhileWaitingForChoices(t *testing.T) {
	source := newRefinementSource(func(core.ProductSearchRequest) []core.ProductFacet {
		return []core.ProductFacet{{Name: "category", Options: []core.ProductFacetOption{{Label: "computer"}}}}
	})
	source.items[0].ObservedFields = []string{"name"}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic"})
	if err != nil || result.Status != core.ProductRecommendationNeedsInput || len(result.Discovered) != 1 || len(result.Inspected) != 0 || source.requests[0].ExcludeSponsored {
		t.Fatal("unknown sponsorship silently erased discovery")
	}
}

func TestRecommendStopsComparisonWhenSelectedStateIsLost(t *testing.T) {
	source := newRefinementSource(func(r core.ProductSearchRequest) []core.ProductFacet {
		groups := []core.ProductFacet{{Name: "memory", Options: []core.ProductFacetOption{{Label: "32GB"}}}}
		if r.Sort == core.ProductSortCoupangRanking {
			return markedCatalog(groups, r.FacetSelections)
		}
		return groups
	})
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, Answers: []core.ProductRecommendationAnswer{{QuestionID: "facet:memory", Choice: "32GB"}}})
	if err != nil || result.Status != core.ProductRecommendationIncomplete || len(source.requests) != 3 || len(source.detailLimits) != 0 || len(result.Discovered) != 1 || result.Audit.DiscoveryStopReason != "source_filter_unverified" {
		t.Fatal("lost selection caused more reads or false success")
	}
}
