package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/products"
)

type questionSource struct {
	conclusionSource
	facets bool
}

func (s questionSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	items, coverage, err := s.conclusionSource.Search(ctx, r)
	if s.facets {
		coverage.Facets = []core.ProductFacet{{Name: "material", Options: []core.ProductFacetOption{{Label: "steel"}}}}
		for _, selection := range r.FacetSelections {
			if selection.Name == "material" && selection.Label == "steel" {
				coverage.Facets[0].Options[0].Selected = true
			}
		}
	}
	return items, coverage, err
}

func TestCLIRecommendationDoesNotRequireEmptyOrRepeatedProceed(t *testing.T) {
	for _, test := range []struct {
		name       string
		facets     bool
		answers    string
		needsInput bool
	}{
		{"no questions", false, "[]", false},
		{"unanswered", true, "[]", true},
		{"answered", true, `[{"question_id":"facet:material","choice":"steel"}]`, false},
		{"skipped", true, `[{"question_id":"facet:material","unbounded":true}]`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			service := products.New(questionSource{conclusionSource: conclusionSource{known: true}, facets: test.facets})
			err := runProducts(context.Background(), []string{"recommend", "--query", "synthetic", "--no-affiliate", "--answers-json", test.answers}, &out, service)
			if err != nil {
				t.Fatal(err)
			}
			var result core.ProductRecommendationResult
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if (result.Status == core.ProductRecommendationNeedsInput) != test.needsInput {
				t.Fatalf("unexpected state: %s", result.Status)
			}
			if test.needsInput {
				if len(result.Questions) != 1 || !result.Questions[0].AllowUnbounded || len(result.Inspected) != 0 {
					t.Fatal("optional question contract lost")
				}
			} else if len(result.Questions) != 0 || len(result.Inspected) != 1 || result.Inspected[0].Conclusion.Outcome != "conditions_not_declared" {
				t.Fatal("automatic research missing or facet answer became a verified condition")
			}
			if test.name == "answered" && (len(result.Refinement.AppliedSelections) != 1 || result.Refinement.AppliedSelections[0].Label != "steel" || result.Query != "synthetic") {
				t.Fatal("typed facet application was lost in adapter")
			}
		})
	}
}
