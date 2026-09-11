package recommendationreport

import (
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func decisionCandidate(id, name, capacity string) core.ProductRecommendationCandidate {
	ref := core.ProductReference{ProductID: id, ItemID: id + "1", VendorItemID: id + "2"}
	p := core.ProductCard{Reference: ref, Name: name}
	i := core.ProductInspection{Product: p, SelectedAttributes: []core.ProductSelectedAttribute{{Name: "RAM용량", Value: capacity}}, Coverage: core.ProductCoverage{ObservedFields: []string{"selected_attributes"}}, FieldEvidence: []core.ProductFieldEvidence{{Field: "selected_attributes", Source: "product_options", Locator: "options.optionRows.selectedAttribute", Method: "native_field", Provenance: "observed", Scope: "selected_option", Reference: ref, CapturedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}}}
	threshold := int64(32)
	return core.ProductRecommendationCandidate{Product: p, Inspection: i, Conditions: []core.ProductConditionAssessment{{Condition: core.ProductRecommendationCondition{ID: "ram", Field: "specifications.memory_gb", Operator: "gte", Integer: &threshold}, Status: core.ProductConditionMet}}, Conclusion: &core.ProductRecommendationConclusion{Outcome: "conditions_met"}}
}

func TestReportShowsExcludedAndUnverifiedConditionEvidence(t *testing.T) {
	met := decisionCandidate("101", "Synthetic met", "32GB")
	unmet := decisionCandidate("102", "Synthetic unmet", "16GB")
	unmet.ExclusionReason = "required_condition_unmet"
	unknown := decisionCandidate("103", "Synthetic unknown", "32GB × 1TB")
	unknown.Inspection.SelectedAttributes[0].Name = "RAM용량 × 저장용량"
	unknown.ExclusionReason = "required_condition_unverified"
	r := core.ProductRecommendationReport{Title: "Synthetic decisions", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Candidates: []core.ProductRecommendationCandidate{met}, Inspected: []core.ProductRecommendationCandidate{met, unmet, unknown}, Audit: core.ProductRecommendationAudit{DiscoveryStopReason: "unique_model_target_reached", InspectionStopReason: "source_access_denied", UninspectedExactOptions: 7}}}
	html, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"필수 조건 검토", "Synthetic unmet", "Synthetic unknown", "조건 판정: 충족", "조건 판정: 불충족", "조건 판정: 미확인", "RAM ≥ 32 GB", "16 GB (원문 표기 환산)", "정확한 판매 옵션", "2026-09-01T00:00:00Z", "source_access_denied", "상세 확인을 마치지 못한 판매 옵션: 7개"} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("report omitted %q", want)
		}
	}
}

func TestReportReassessesConditionsAndPreservesOptionIdentity(t *testing.T) {
	for _, state := range []string{"forged result", "mismatched detail", "missing source", "same product other option", "candidate only", "no conditions"} {
		t.Run(state, func(t *testing.T) {
			c := decisionCandidate("101", "Synthetic decision", "16GB")
			r := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Inspected: []core.ProductRecommendationCandidate{c}}}
			want := "조건 판정: 불충족"
			switch state {
			case "forged result":
				value := int64(64)
				r.Recommendation.Inspected[0].Conditions[0].DerivedInteger = &value
			case "mismatched detail":
				r.Recommendation.Inspected[0].Inspection.Product.Reference.VendorItemID = "999"
				want = "조건 판정: 미확인"
			case "missing source":
				r.Recommendation.Inspected[0].Inspection.FieldEvidence = nil
				want = "조건 판정: 미확인"
			case "same product other option":
				other := decisionCandidate("101", "Synthetic second option", "64GB")
				other.Product.Reference.VendorItemID = "999"
				other.Inspection.Product.Reference = other.Product.Reference
				other.Inspection.FieldEvidence[0].Reference = other.Product.Reference
				r.Recommendation.Inspected = append(r.Recommendation.Inspected, other)
			case "candidate only":
				r.Recommendation.Inspected = nil
				r.Recommendation.Candidates = []core.ProductRecommendationCandidate{c}
			case "no conditions":
				r.Recommendation.Inspected[0].Conditions = nil
				want = "선언된 필수 조건 없음"
			}
			html, err := Render(r)
			if err != nil {
				t.Fatal(err)
			}
			text := string(html)
			if !strings.Contains(text, want) {
				t.Fatalf("missing outcome %s", want)
			}
			if state != "same product other option" && strings.Contains(text, "조건 판정: 충족") {
				t.Fatal("supplied success overrode source evidence")
			}
			if state == "same product other option" && (strings.Count(text, "data-condition-reference=") != 2 || !strings.Contains(text, "조건 판정: 충족") || !strings.Contains(text, "판매 옵션 999")) {
				t.Fatal("different vendor option collapsed")
			}
		})
	}
}

func TestReportConditionsPreserveZeroFalseAndThresholdPrecision(t *testing.T) {
	c := decisionCandidate("101", "Synthetic scalars", "32GB")
	p := &c.Inspection.Product
	p.Price = core.ProductPrice{Currency: "KRW"}
	p.Rating = 4.567
	p.ObservedFields = []string{"price.current_amount", "free_shipping", "rating"}
	for _, field := range p.ObservedFields {
		e := c.Inspection.FieldEvidence[0]
		e.Field = field
		e.Source = "json_ld"
		e.Locator = "synthetic.value"
		if field == "rating" {
			e.Scope = "product_page"
			e.Reference = core.ProductReference{ProductID: p.Reference.ProductID}
		}
		p.FieldEvidence = append(p.FieldEvidence, e)
	}
	zero, no, rating := int64(0), false, 4.568
	c.Conditions = []core.ProductConditionAssessment{{Condition: core.ProductRecommendationCondition{ID: "price", Field: "price.current_amount", Operator: "lte", Integer: &zero}}, {Condition: core.ProductRecommendationCondition{ID: "shipping", Field: "free_shipping", Operator: "eq", Boolean: &no}}, {Condition: core.ProductRecommendationCondition{ID: "rating", Field: "rating", Operator: "gte", Number: &rating}}}
	r := core.ProductRecommendationReport{Title: "Synthetic scalars", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Inspected: []core.ProductRecommendationCandidate{c}}}
	html, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"표시 가격 ≤ 0원", "무료 배송 = 아니요", "4.568 / 5", "4.567 / 5", "조건 판정: 불충족"} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("lost scalar semantics: %s", want)
		}
	}
}

func TestReportShowsNextActionsWithoutExecutingOrInterpretingMarkup(t *testing.T) {
	malicious := `</script><img src=x onerror=alert(1)>`
	c := decisionCandidate("101", malicious, "32GB")
	r := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Inspected: []core.ProductRecommendationCandidate{c}, NextActions: []core.ProductRecommendationNextAction{{Kind: "restore_source_access", Reason: "source_access_denied"}, {Kind: "clarify_preference", Reason: malicious, References: []core.ProductReference{c.Product.Reference}, ConditionIDs: []string{"ram"}, Fields: []string{"specifications.memory_gb"}}}}}
	html, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	text := string(html)
	for _, want := range []string{"다음에 확인할 것", "현재 연결의 접속 상태부터 확인", "source_access_denied", "비교 항목의 우선순위 확인", "대상 판매 옵션 1개", "specifications.memory_gb", "조회를 재시도하거나 로그인 창을 열거나 구매를 실행하지 않습니다."} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing next action: %s", want)
		}
	}
	if strings.Contains(text, malicious) || strings.Count(text, "</script>") != 1 {
		t.Fatal("decision/next-action interpreted as executable markup")
	}
}

func TestReportRejectsDuplicateDecisionReferencesAndOversizedConditionLists(t *testing.T) {
	c := decisionCandidate("101", "Synthetic", "32GB")
	for _, mutate := range []func(*core.ProductRecommendationResult){
		func(r *core.ProductRecommendationResult) { r.Inspected = append(r.Inspected, c) },
		func(r *core.ProductRecommendationResult) { r.Inspected[0].Product.Reference.ItemID = "not-numeric" },
		func(r *core.ProductRecommendationResult) {
			r.Inspected[0].Conditions = append(r.Inspected[0].Conditions, r.Inspected[0].Conditions[0])
		},
		func(r *core.ProductRecommendationResult) { r.Inspected[0].Conditions[0].Condition.Integer = nil },
		func(r *core.ProductRecommendationResult) {
			r.Inspected = make([]core.ProductRecommendationCandidate, 201)
		},
		func(r *core.ProductRecommendationResult) {
			r.NextActions = make([]core.ProductRecommendationNextAction, 201)
		},
	} {
		base := decisionCandidate("101", "Synthetic", "32GB")
		r := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Inspected: []core.ProductRecommendationCandidate{base}}}
		mutate(&r.Recommendation)
		if data, err := Render(r); err == nil || len(data) != 0 {
			t.Fatal("ambiguous or unbounded decision data rendered")
		}
	}
}

func TestReportPrefersInvestigationRecordAndWarnsAboutConflictingProjection(t *testing.T) {
	inspected := decisionCandidate("101", "Synthetic recorded", "16GB")
	shown := decisionCandidate("101", "Synthetic projected", "64GB")
	r := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Inspected: []core.ProductRecommendationCandidate{inspected}, Candidates: []core.ProductRecommendationCandidate{shown}}}
	html, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	text := string(html)
	if !strings.Contains(text, "표시 후보와 상세 조사 기록이 서로 다릅니다.") || !strings.Contains(text, "조건 판정: 불충족") || strings.Count(text, "data-condition-reference=") != 1 {
		t.Fatal("ambiguous projection replaced investigation")
	}
}

func TestReportDoesNotInventUninspectedZeroOrCompleteAfterAccessDenial(t *testing.T) {
	r := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete}}
	html, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "상세 확인을 마치지 못한 판매 옵션: 미기록") {
		t.Fatal("missing audit promoted to zero")
	}
	r.Recommendation.Status = core.ProductRecommendationComplete
	r.Recommendation.Audit.InspectionStopReason = "source_access_denied"
	if _, err := Render(r); err == nil {
		t.Fatal("complete status with interrupted research accepted")
	}
}

func TestExcludedDecisionKeepsBoundOptionTextWithoutInferringCapacity(t *testing.T) {
	for _, state := range []string{"bound", "wrong option", "unavailable", "inferred", "missing source"} {
		t.Run(state, func(t *testing.T) {
			c := decisionCandidate("101", "Synthetic excluded", "32GB × 1TB")
			c.Inspection.SelectedAttributes[0].Name = "RAM용량 × 저장용량"
			c.ExclusionReason = "required_condition_unverified"
			switch state {
			case "wrong option":
				c.Inspection.Product.Reference.VendorItemID = "999"
			case "unavailable":
				c.Inspection.Coverage.UnavailableFields = []string{"selected_attributes"}
			case "inferred":
				c.Inspection.FieldEvidence[0].Provenance = "inferred"
			case "missing source":
				c.Inspection.FieldEvidence = nil
			}
			r := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Inspected: []core.ProductRecommendationCandidate{c}}}
			data, err := Render(r)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			if strings.Contains(text, "32GB × 1TB") != (state == "bound") {
				t.Fatal("excluded option text lost or unbound source displayed")
			}
			if strings.Contains(text, "32 GB (원문 표기 환산)") || !strings.Contains(text, "조건 판정: 미확인") {
				t.Fatal("raw compound option promoted to verified capacity")
			}
			if state == "bound" && !strings.Contains(text, "관찰 시각: 2026-09-01T00:00:00Z") {
				t.Fatal("raw option observation time missing")
			}
		})
	}
}
