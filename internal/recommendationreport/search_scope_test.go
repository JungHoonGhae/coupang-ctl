package recommendationreport

import (
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestReportPreservesCategoryAndVerifiedSelections(t *testing.T) {
	report := core.ProductRecommendationReport{Title: "Synthetic category", Recommendation: core.ProductRecommendationResult{
		SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, CategoryID: "123456",
		Refinement: &core.ProductSearchRefinement{AppliedSelections: []core.ProductFacetSelection{{Name: "Memory", Label: "32 GB"}},
			Issue: &core.ProductFacetSelectionIssue{Selection: core.ProductFacetSelection{Name: "GPU", Label: "<unverified>"}, Reason: "choice_unavailable"}},
	}}
	data, err := Render(report)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{"조회 범위: 쿠팡 카테고리 123456", "적용이 확인된 필터", "Memory: 32 GB", "적용을 확인하지 못한 선택: GPU: &lt;unverified&gt;", "필터 선택은 개별 상품의 조건 충족을 보증하지 않습니다."} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing search context: %s", want)
		}
	}
	if strings.Contains(html, "GPU: <unverified>") {
		t.Fatal("untrusted selection rendered as HTML")
	}
	report.Recommendation.Refinement.AppliedSelections = nil
	data, err = Render(report)
	if err != nil || !strings.Contains(string(data), "적용이 확인된 필터가 없습니다.") {
		t.Fatal("unverified selection became applied")
	}
	report.Recommendation.CategoryID = ""
	report.Recommendation.Query = "synthetic <query>"
	data, err = Render(report)
	if err != nil || !strings.Contains(string(data), "조회 범위: 검색어 synthetic &lt;query&gt;") {
		t.Fatal("query scope lost")
	}
}

func TestReportRejectsAmbiguousOrInvalidCategoryScope(t *testing.T) {
	for _, scope := range []core.ProductRecommendationResult{{CategoryID: "123456", Query: "synthetic"}, {CategoryID: "../search"}} {
		scope.SchemaVersion = core.ProductRecommendationSchemaVersion
		scope.Status = core.ProductRecommendationIncomplete
		if _, err := Render(core.ProductRecommendationReport{Title: "Synthetic", Recommendation: scope}); err == nil {
			t.Fatal("invalid report category accepted")
		}
	}
}

func TestReportSeparatesQueryNavigationTrailFromActiveCategory(t *testing.T) {
	report := core.ProductRecommendationReport{Title: "Synthetic query trail", Recommendation: core.ProductRecommendationResult{
		SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationNeedsInput, Query: "synthetic desktop",
		Refinement: &core.ProductSearchRefinement{CategoryTrail: []string{"Synthetic <parent>"}, AppliedSelections: []core.ProductFacetSelection{{Name: "카테고리", Label: "Synthetic child"}}},
	}}
	data, err := Render(report)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{"조회 범위: 검색어 synthetic desktop", "현재 활성 필터가 아닙니다.", "<ol><li>Synthetic &lt;parent&gt;</li></ol>", "<li>카테고리: Synthetic child</li>"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing query navigation evidence: %s", want)
		}
	}
	if strings.Contains(html, "<li>카테고리: Synthetic &lt;parent&gt;</li>") {
		t.Fatal("previous selection became active filter")
	}
}

func TestReportShowsVerifiedDestinationWithoutErasingStartingCategory(t *testing.T) {
	report := core.ProductRecommendationReport{Title: "Synthetic transition", Recommendation: core.ProductRecommendationResult{
		SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, CategoryID: "123456", AppliedCategoryID: "654321",
		Facets:     []core.ProductFacet{{Name: "카테고리", Options: []core.ProductFacetOption{{Label: "Synthetic child", Selected: true}}}},
		Refinement: &core.ProductSearchRefinement{AppliedSelections: []core.ProductFacetSelection{{Name: "카테고리", Label: "Synthetic child"}}},
	}}
	data, err := Render(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"조회 범위: 쿠팡 카테고리 654321", "시작 카테고리 123456", "카테고리: Synthetic child"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing scope evidence: %s", want)
		}
	}
	for _, mode := range []string{"no_start", "invalid_destination", "no_choice", "unselected"} {
		changed := report
		switch mode {
		case "no_start":
			changed.Recommendation.CategoryID = ""
		case "invalid_destination":
			changed.Recommendation.AppliedCategoryID = "bad"
		case "no_choice":
			changed.Recommendation.Refinement = nil
		case "unselected":
			changed.Recommendation.Facets = nil
		}
		if _, err := Render(changed); err == nil {
			t.Fatalf("invalid scope report accepted: %s", mode)
		}
	}
}
