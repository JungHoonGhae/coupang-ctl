package products

import (
	"context"
	"reflect"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestQueryCategoryTrailReplaysAcrossRefinementAndResearch(t *testing.T) {
	for _, mode := range []string{"research", "questions", "missing_child", "denied_child"} {
		t.Run(mode, func(t *testing.T) {
			source := newRefinementSource(func(r core.ProductSearchRequest) []core.ProductFacet {
				category := core.ProductFacet{Name: "카테고리", Options: []core.ProductFacetOption{{Label: "Parent"}}}
				for _, selection := range r.FacetSelections {
					if selection.Name == "카테고리" {
						if mode != "missing_child" {
							category.Options = append(category.Options, core.ProductFacetOption{Label: "Child"})
						}
						if selection.Label == "Child" && !reflect.DeepEqual(r.CategoryTrail, []string{"Parent"}) {
							t.Error("child was requested without its observed navigation trail")
						}
					}
				}
				return markedCatalog([]core.ProductFacet{category, {Name: "Memory", Options: []core.ProductFacetOption{{Label: "32 GB"}}}, {Name: "Other", Options: []core.ProductFacetOption{{Label: "Optional"}}}}, r.FacetSelections)
			})
			if mode == "denied_child" {
				source.failAt, source.failure = 4, core.ErrBrowserAccessDenied
			}
			result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic desktop", Proceed: mode != "questions", DisableAffiliate: true, Answers: []core.ProductRecommendationAnswer{
				{QuestionID: "facet:카테고리", Choice: "Parent"}, {QuestionID: "facet:Memory", Choice: "32 GB"}, {QuestionID: "facet:카테고리", Choice: "Child"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if result.Query != "synthetic desktop" || result.CategoryID != "" || result.AppliedCategoryID != "" {
				t.Fatal("query turned into category-only navigation")
			}
			wantSearches, wantDetails := 8, 1
			switch mode {
			case "questions":
				wantSearches, wantDetails = 4, 0
			case "missing_child":
				wantSearches, wantDetails = 3, 0
			case "denied_child":
				wantSearches, wantDetails = 4, 0
			}
			if len(source.requests) != wantSearches || len(source.detailLimits) != wantDetails {
				t.Fatalf("searches=%d details=%d", len(source.requests), len(source.detailLimits))
			}
			reserved := 0
			for i, r := range source.requests {
				if r.Query != result.Query || r.CategoryID != "" || r.Page != 1 || !r.DisableAffiliate {
					t.Fatal("request lost scope")
				}
				if r.DocumentReadLimit != len(r.FacetSelections)+len(r.CategoryTrail)+1 {
					t.Fatal("trail navigation was not reserved")
				}
				if i >= 3 && (!reflect.DeepEqual(r.CategoryTrail, []string{"Parent"}) || len(r.FacetSelections) != 2 || r.FacetSelections[0].Label != "Child" || r.FacetSelections[1].Label != "32 GB") {
					t.Fatal("subsequent reads lost the trail or active filters")
				}
				reserved += r.DocumentReadLimit
			}
			for _, limit := range source.detailLimits {
				reserved += limit
			}
			if result.Audit.DocumentReadReservations != reserved || reserved > 40 {
				t.Fatal("incorrect whole-workflow allowance")
			}
			if mode == "missing_child" || mode == "denied_child" {
				if len(result.Refinement.CategoryTrail) != 0 || result.Refinement.AppliedSelections[0].Label != "Parent" || len(result.Refinement.Steps) != 3 || len(result.Discovered) != 1 {
					t.Fatal("failed child replaced verified state")
				}
				if mode == "denied_child" && result.Refinement.Issue.Reason != "source_access_denied" {
					t.Fatal("denial lost")
				}
			} else if !reflect.DeepEqual(result.Refinement.CategoryTrail, []string{"Parent"}) || len(result.Refinement.Steps) != 4 || result.Refinement.Steps[1].Selection.Label != "Parent" || result.Refinement.AppliedSelections[0].Label != "Child" {
				t.Fatal("historical trail and active category conflated")
			}
		})
	}
}
