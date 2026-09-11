package core

import "testing"

func TestCategoryScopeRequiresExplicitVerifiedChoice(t *testing.T) {
	for _, mode := range []string{"valid", "query", "invalid_destination", "no_choice", "other_choice", "duplicate_choice", "unselected", "disabled", "missing_group", "duplicate_group", "duplicate_label"} {
		t.Run(mode, func(t *testing.T) {
			request := ProductSearchRequest{CategoryID: "123456", FacetSelections: []ProductFacetSelection{{Name: "카테고리", Label: "Synthetic child"}}}
			coverage := ProductCoverage{AppliedCategoryID: "654321", Facets: []ProductFacet{{Name: "카테고리", Options: []ProductFacetOption{{Label: "Synthetic child", Selected: true}}}}}
			switch mode {
			case "query":
				request.CategoryID = ""
				request.Query = "synthetic"
			case "invalid_destination":
				coverage.AppliedCategoryID = "654321?unsafe"
			case "no_choice":
				request.FacetSelections = nil
			case "other_choice":
				request.FacetSelections[0].Name = "Memory"
			case "duplicate_choice":
				request.FacetSelections = append(request.FacetSelections, request.FacetSelections[0])
			case "unselected":
				coverage.Facets[0].Options[0].Selected = false
			case "disabled":
				coverage.Facets[0].Options[0].Disabled = true
			case "missing_group":
				coverage.Facets = nil
			case "duplicate_group":
				coverage.Facets = append(coverage.Facets, coverage.Facets[0])
			case "duplicate_label":
				coverage.Facets[0].Options = append(coverage.Facets[0].Options, coverage.Facets[0].Options[0])
			}
			if err := coverage.ValidateCategoryScope(request); (err == nil) != (mode == "valid") {
				t.Fatalf("unexpected scope validation: %v", err)
			}
		})
	}
	if err := (ProductCoverage{}).ValidateCategoryScope(ProductSearchRequest{Query: "synthetic"}); err != nil {
		t.Fatal("unchanged scope rejected")
	}
}
