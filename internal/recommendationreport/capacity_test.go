package recommendationreport

import (
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestReportCapacityRecomputesBoundValuesAndKeepsUnknowns(t *testing.T) {
	for _, state := range []string{"known", "compound", "wrong option", "inferred", "missing evidence"} {
		t.Run(state, func(t *testing.T) {
			ref := core.ProductReference{ProductID: "101", ItemID: "201", VendorItemID: "301"}
			p := core.ProductCard{Reference: ref, Name: "Synthetic RAM 64GB"}
			i := core.ProductInspection{Product: p, SelectedAttributes: []core.ProductSelectedAttribute{{Name: "RAM용량", Value: "32GB"}}, Coverage: core.ProductCoverage{ObservedFields: []string{"selected_attributes"}}, FieldEvidence: []core.ProductFieldEvidence{{Field: "selected_attributes", Source: "product_options", Locator: "options.optionRows.selectedAttribute", Provenance: "observed", Method: "native_field", Scope: "selected_option", Reference: ref, CapturedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}}}
			switch state {
			case "compound":
				i.SelectedAttributes[0] = core.ProductSelectedAttribute{Name: "RAM용량 × 저장용량", Value: "32GB × 1TB"}
			case "wrong option":
				p.Reference.VendorItemID = "999"
			case "inferred":
				i.FieldEvidence[0].Provenance = "inferred"
			case "missing evidence":
				i.FieldEvidence = nil
			}
			forged := int64(64)
			report := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Candidates: []core.ProductRecommendationCandidate{{Product: p, Inspection: i, ComparisonValues: []core.ProductComparableValue{{Field: "specifications.memory_gb", DerivedInteger: &forged}}}}}}
			html, err := Render(report)
			if err != nil {
				t.Fatal(err)
			}
			text := string(html)
			if strings.Contains(text, "64 GB (원문 표기 환산)") {
				t.Fatal("trusted supplied comparison value")
			}
			if state == "known" {
				if !strings.Contains(text, "32 GB (원문 표기 환산)") || !strings.Contains(text, "실제 장착 용량·성능을 측정한 결과가 아닙니다.") {
					t.Fatal("derivation disclosure missing")
				}
			} else if strings.Contains(text, "32 GB (원문 표기 환산)") || !strings.Contains(text, "<dd>미확인") {
				t.Fatal("unverified capacity displayed")
			}
		})
	}
}
