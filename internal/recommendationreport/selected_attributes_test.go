package recommendationreport

import (
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestReportSelectedAttributeProvenanceIdentityAndEscaping(t *testing.T) {
	for _, state := range []string{"known", "missing evidence", "different candidate", "inferred"} {
		t.Run(state, func(t *testing.T) {
			ref := core.ProductReference{ProductID: "101", ItemID: "201", VendorItemID: "301"}
			p := core.ProductCard{Reference: ref, Name: "Synthetic"}
			value := "32GB × 1TB <script>synthetic</script>"
			i := core.ProductInspection{Product: p, SelectedAttributes: []core.ProductSelectedAttribute{{Name: "RAM용량 × 저장용량", Value: value}},
				Coverage:      core.ProductCoverage{ObservedFields: []string{"selected_attributes"}},
				FieldEvidence: []core.ProductFieldEvidence{{Field: "selected_attributes", Provenance: "observed", Method: "native_field", Source: "product_options", Locator: "options.optionRows.selectedAttribute", Scope: "selected_option", Reference: ref, CapturedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}},
			}
			switch state {
			case "missing evidence":
				i.FieldEvidence = nil
			case "different candidate":
				p.Reference.VendorItemID = "999"
			case "inferred":
				i.FieldEvidence[0].Provenance = "inferred"
			}
			report := core.ProductRecommendationReport{Title: "Synthetic report", Recommendation: core.ProductRecommendationResult{
				SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationComplete,
				Candidates: []core.ProductRecommendationCandidate{{Product: p, Inspection: i}},
			}}
			html, err := Render(report)
			if err != nil {
				t.Fatal(err)
			}
			text := string(html)
			if strings.Contains(text, value) {
				t.Fatal("option interpreted as markup")
			}
			if state == "known" {
				for _, want := range []string{"RAM용량 × 저장용량", "32GB × 1TB &lt;script&gt;synthetic&lt;/script&gt;", "2026-09-01T00:00:00Z", "복합 항목은 원문 그대로입니다."} {
					if !strings.Contains(text, want) {
						t.Fatalf("missing option evidence %q", want)
					}
				}
			} else if !strings.Contains(text, "선택 옵션 원문 미확인") || strings.Contains(text, "32GB × 1TB") {
				t.Fatal("unknown or mismatched tuple presented as observed")
			}
		})
	}
}
