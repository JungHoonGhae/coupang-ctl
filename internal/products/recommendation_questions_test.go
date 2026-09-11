package products

import (
	"context"
	"fmt"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type questionRecommendationSource struct {
	recommendationSource
	facets                []core.ProductFacet
	searches, inspections int
}

func (s *questionRecommendationSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	s.searches++
	items, coverage, err := s.recommendationSource.Search(ctx, r)
	coverage.Facets = cloneFacetCatalog(s.facets)
	for i := range coverage.Facets {
		for j := range coverage.Facets[i].Options {
			for _, selection := range r.FacetSelections {
				if selection.Name == coverage.Facets[i].Name && selection.Label == coverage.Facets[i].Options[j].Label {
					coverage.Facets[i].Options[j].Selected = true
				}
			}
		}
	}
	return items, coverage, err
}

func (s *questionRecommendationSource) Inspect(ctx context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	s.inspections++
	return s.recommendationSource.Inspect(ctx, r)
}

func questionSource(facets []core.ProductFacet) *questionRecommendationSource {
	p := syntheticRecommendationProduct()
	return &questionRecommendationSource{recommendationSource: recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{p}, inspection: core.ProductInspection{Product: p}}}, facets: facets}
}

func TestRecommendationDoesNotPauseWithoutAnswerableQuestions(t *testing.T) {
	for _, facets := range [][]core.ProductFacet{nil, {{Name: "empty"}}, {{Name: "selected", Options: []core.ProductFacetOption{{Label: "known", Selected: true}}}}} {
		source := questionSource(facets)
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic"})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status == core.ProductRecommendationNeedsInput || len(result.Questions) != 0 || source.inspections != 1 || len(result.Inspected) != 1 {
			t.Fatalf("empty question gate blocked evidence gathering: status=%s questions=%d inspections=%d", result.Status, len(result.Questions), source.inspections)
		}
		if source.searches > 25 {
			t.Fatal("automatic continuation exceeded existing search bound")
		}
	}
}

func TestRecommendationDoesNotRepeatAnsweredFacets(t *testing.T) {
	facet := core.ProductFacet{Name: "material", Options: []core.ProductFacetOption{{Label: "steel"}, {Label: "ceramic"}}}
	for _, answer := range []core.ProductRecommendationAnswer{{QuestionID: "facet:material", Choice: "steel"}, {QuestionID: "facet:material", Unbounded: true}} {
		source := questionSource([]core.ProductFacet{facet})
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Answers: []core.ProductRecommendationAnswer{answer}})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status == core.ProductRecommendationNeedsInput || len(result.Questions) != 0 || source.inspections != 1 {
			t.Fatalf("answered facet caused another proceed gate: status=%s questions=%d", result.Status, len(result.Questions))
		}
	}
}

func TestRecommendationKeepsUnansweredQuestionsAndExplicitSkip(t *testing.T) {
	facets := []core.ProductFacet{{Name: "material", Options: []core.ProductFacetOption{{Label: "steel"}}}, {Name: "size", Options: []core.ProductFacetOption{{Label: "small"}}}}
	for _, skip := range []bool{false, true} {
		source := questionSource(facets)
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: skip, Answers: []core.ProductRecommendationAnswer{{QuestionID: "facet:material", Choice: "steel"}}})
		if err != nil {
			t.Fatal(err)
		}
		if !skip && (result.Status != core.ProductRecommendationNeedsInput || len(result.Questions) != 1 || result.Questions[0].ID != "facet:size" || source.searches != 2 || source.inspections != 0) {
			t.Fatalf("unanswered question was lost or answered question repeated: %+v", result.Questions)
		}
		if skip && (result.Status == core.ProductRecommendationNeedsInput || source.inspections != 1) {
			t.Fatal("explicit skip did not continue")
		}
	}
}

func TestRecommendationEmptyOrUnrelatedAnswerDoesNotSuppressQuestion(t *testing.T) {
	for _, answer := range []core.ProductRecommendationAnswer{{QuestionID: "facet:material", Choice: "  "}, {QuestionID: "facet:other", Choice: "steel"}, {QuestionID: "facet:material", Integer: 2}} {
		source := questionSource([]core.ProductFacet{{Name: "material", Options: []core.ProductFacetOption{{Label: "steel"}}}})
		result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Answers: []core.ProductRecommendationAnswer{answer}})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != core.ProductRecommendationNeedsInput || len(result.Questions) != 1 || source.inspections != 0 {
			t.Fatal("non-answer suppressed actual question")
		}
	}
}

func TestRecommendationQuestionLimitAppliesAfterAnsweredFacets(t *testing.T) {
	var facets []core.ProductFacet
	var answers []core.ProductRecommendationAnswer
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("synthetic-%d", i)
		facets = append(facets, core.ProductFacet{Name: name, Options: []core.ProductFacetOption{{Label: "value"}}})
		if i < 24 {
			answers = append(answers, core.ProductRecommendationAnswer{QuestionID: "facet:" + name, Unbounded: true})
		}
	}
	result, err := New(questionSource(facets)).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Answers: answers})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Questions) != 1 || result.Questions[0].ID != "facet:synthetic-24" {
		t.Fatal("question cap hid remaining unanswered facet")
	}
}

func TestRecommendationUnboundedAnswerDoesNotRefineQuery(t *testing.T) {
	source := questionSource([]core.ProductFacet{{Name: "material", Options: []core.ProductFacetOption{{Label: "steel"}}}})
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Answers: []core.ProductRecommendationAnswer{{QuestionID: "facet:material", Choice: "steel", Unbounded: true}}})
	if err != nil || result.Query != "synthetic" || len(result.Refinement.AppliedSelections) != 0 || len(result.Refinement.Steps) != 1 {
		t.Fatal("skipped preference narrowed the query")
	}
}
