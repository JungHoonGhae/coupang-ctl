package core

import (
	"fmt"
	"strings"
	"testing"
)

func TestQueryCategoryTrailBounds(t *testing.T) {
	for _, mode := range []string{"valid", "category_only", "missing_active_category", "blank", "oversize", "utf8", "page", "allowance", "too_many"} {
		t.Run(mode, func(t *testing.T) {
			r := ProductSearchRequest{Query: "synthetic", CategoryTrail: []string{"Parent"}, FacetSelections: []ProductFacetSelection{{Name: "카테고리", Label: "Child"}}, DocumentReadLimit: 3}
			switch mode {
			case "category_only":
				r.Query, r.CategoryID = "", "123456"
			case "missing_active_category":
				r.FacetSelections = nil
			case "blank":
				r.CategoryTrail[0] = " "
			case "oversize":
				r.CategoryTrail[0] = strings.Repeat("a", 401)
			case "utf8":
				r.CategoryTrail[0] = string([]byte{255})
			case "page":
				r.Page = 2
			case "allowance":
				r.DocumentReadLimit = 2
			case "too_many":
				r.CategoryTrail = []string{"A", "B", "C", "D", "E", "F"}
				r.DocumentReadLimit = 8
			}
			if (r.Validate() == nil) != (mode == "valid") {
				t.Fatal("invalid trail accepted or valid trail rejected")
			}
		})
	}
}

func TestFacetDocumentAllowanceMatchesSelectedNavigation(t *testing.T) {
	for n := 0; n <= 6; n++ {
		r := ProductSearchRequest{Query: "synthetic", DocumentReadLimit: n + 1}
		for i := 0; i < n; i++ {
			r.FacetSelections = append(r.FacetSelections, ProductFacetSelection{Name: fmt.Sprintf("g%d", i), Label: "a"})
		}
		if err := r.Validate(); err != nil {
			t.Fatalf("%d choices rejected: %v", n, err)
		}
		if n > 1 {
			r.DocumentReadLimit = n
			if r.Validate() == nil {
				t.Fatal("insufficient selection allowance accepted")
			}
		}
	}
}

func TestRecommendationFacetInputBounds(t *testing.T) {
	r := ProductRecommendationRequest{Query: "synthetic"}
	for i := 0; i < 7; i++ {
		r.Answers = append(r.Answers, ProductRecommendationAnswer{QuestionID: fmt.Sprintf("facet:g%d", i), Choice: "a"})
	}
	if r.Validate() == nil {
		t.Fatal("unbounded selected navigation")
	}
	for i := range r.Answers {
		r.Answers[i].Unbounded = true
	}
	if err := r.Validate(); err != nil {
		t.Fatal("skipped choices consumed navigation budget")
	}
}

func TestRecommendationCategoryPathAnswersAreBoundedAndExplicit(t *testing.T) {
	chain := []ProductRecommendationAnswer{{QuestionID: "facet:카테고리", Choice: "Parent"}, {QuestionID: "facet:카테고리", Choice: "Child"}}
	r := ProductRecommendationRequest{CategoryID: "123456", Answers: chain}
	if err := r.Validate(); err != nil {
		t.Fatalf("category-only path rejected: %v", err)
	}
	if err := (ProductRecommendationRequest{Query: "synthetic", Answers: chain}).Validate(); err != nil {
		t.Fatalf("query category path rejected: %v", err)
	}
	for _, mode := range []string{"other_group", "skip_first", "skip_last", "empty", "integer", "dimensions", "too_many"} {
		t.Run(mode, func(t *testing.T) {
			r := ProductRecommendationRequest{CategoryID: "123456", Answers: append([]ProductRecommendationAnswer{}, chain...)}
			switch mode {
			case "other_group":
				r.Answers[0].QuestionID, r.Answers[1].QuestionID = "facet:Memory", "facet:Memory"
			case "skip_first":
				r.Answers[0].Unbounded = true
			case "skip_last":
				r.Answers[1].Unbounded = true
			case "empty":
				r.Answers[1].Choice = " "
			case "integer":
				r.Answers[0].Integer = 32
			case "dimensions":
				r.Answers[1].DimensionsMM = &ProductDimensionsMM{Width: 1, Height: 1, Depth: 1}
			case "too_many":
				for len(r.Answers) < 7 {
					r.Answers = append(r.Answers, chain[1])
				}
			}
			if r.Validate() == nil {
				t.Fatal("ambiguous or unbounded category chain accepted")
			}
		})
	}
	// Search inputs remain a simultaneously applied selection set, not a path.
	if (ProductSearchRequest{CategoryID: "123456", FacetSelections: []ProductFacetSelection{{Name: "카테고리", Label: "Parent"}, {Name: "카테고리", Label: "Child"}}}).Validate() == nil {
		t.Fatal("search accepted conflicting active categories")
	}
}
