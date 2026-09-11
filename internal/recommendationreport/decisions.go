package recommendationreport

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type decisionView struct {
	Name, Reference, Selection, Outcome, IdentityWarning, RecordWarning string
	Conditions                                                          []conditionView
	SelectedAttributes                                                  []core.ProductSelectedAttribute
	OptionsObservedAt                                                   string
}

type conditionView struct {
	ID, Requirement, Status, Value, Derivation, Missing string
	Evidence                                            []string
}

// Inspected contains the investigation, including exclusions and candidates
// omitted by a presentation cap. Candidates is only the displayed subset.
// Reassess conditions through the same core seam instead of trusting supplied
// status, conclusion, observed values or derivation fields in a saved report.
func decisionViews(r core.ProductRecommendationResult) []decisionView {
	displayed := make(map[core.ProductReference]core.ProductRecommendationCandidate, len(r.Candidates))
	for _, c := range r.Candidates {
		displayed[c.Product.Reference] = c
	}
	seen := make(map[core.ProductReference]bool)
	var views []decisionView
	appendCandidate := func(c core.ProductRecommendationCandidate) {
		if seen[c.Product.Reference] {
			return
		}
		seen[c.Product.Reference] = true
		view := decisionView{Name: c.Product.Name, Reference: referenceText(c.Product.Reference), Selection: "표시 목록에 없음 · 제외 사유 미기록"}
		shown, isDisplayed := displayed[c.Product.Reference]
		if isDisplayed {
			view.Selection = "보고서 후보에 표시"
			if !reflect.DeepEqual(c.Inspection, shown.Inspection) || !reflect.DeepEqual(conditionDefinitions(c), conditionDefinitions(shown)) {
				view.RecordWarning = "표시 후보와 상세 조사 기록이 서로 다릅니다. 아래 판정은 상세 조사 기록을 기준으로 합니다."
			}
		}
		if c.ExclusionReason != "" {
			view.Selection = "조사 기록에서 제외: " + exclusionText(c.ExclusionReason)
			if isDisplayed {
				view.Selection = "표시 목록과 제외 기록이 충돌합니다: " + exclusionText(c.ExclusionReason)
			}
		}
		inspection := c.Inspection
		if c.Product.Reference != inspection.Product.Reference {
			view.IdentityWarning = "상품과 상세 자료의 판매 옵션이 달라 조건을 확인할 수 없습니다."
			inspection = core.ProductInspection{Product: core.ProductCard{Reference: c.Product.Reference}}
		}
		if e, ok := inspection.SelectedAttributesEvidence(); ok && !slices.Contains(inspection.Coverage.UnavailableFields, "selected_attributes") {
			view.SelectedAttributes = inspection.SelectedAttributes
			view.OptionsObservedAt = e.CapturedAt.UTC().Format(time.RFC3339)
		}
		conditions := make([]core.ProductRecommendationCondition, 0, len(c.Conditions))
		for _, original := range c.Conditions {
			conditions = append(conditions, original.Condition)
			assessment := core.AssessProductCondition(inspection, original.Condition)
			view.Conditions = append(view.Conditions, conditionPresentation(assessment))
		}
		outcome := core.ConcludeProductConditions(inspection, conditions).Outcome
		view.Outcome = map[string]string{"conditions_met": "조건 판정: 충족", "conditions_unmet": "조건 판정: 불충족", "conditions_unverified": "조건 판정: 미확인", "conditions_not_declared": "선언된 필수 조건 없음"}[outcome]
		views = append(views, view)
	}
	for _, c := range r.Inspected {
		appendCandidate(c)
	}
	// Older/supplied reports may contain candidates without an inspected list.
	// Preserve their available evidence without inventing an investigation count.
	for _, c := range r.Candidates {
		appendCandidate(c)
	}
	return views
}

func conditionDefinitions(c core.ProductRecommendationCandidate) []core.ProductRecommendationCondition {
	result := make([]core.ProductRecommendationCondition, 0, len(c.Conditions))
	for _, a := range c.Conditions {
		result = append(result, a.Condition)
	}
	return result
}

func inspectionStopText(reason string) string {
	labels := map[string]string{
		"source_access_denied":              "사이트가 상세 조회를 거부해 중단했습니다.",
		"source_authentication_required":    "로그인이 필요해 상세 조회를 중단했습니다.",
		"source_read_incomplete":            "일부 상세 자료를 읽지 못했습니다.",
		"discovered_candidates_inspected":   "발견한 후보의 상세 조사를 처리했습니다. 전체 시장을 조사했다는 뜻은 아닙니다.",
		"inspection_budget_reached":         "상세 조사 횟수 한도에 도달했습니다.",
		"review_budget_reached":             "리뷰 조사 한도에 도달했습니다.",
		"document_budget_reached":           "문서 조회 한도에 도달했습니다.",
		"auxiliary_document_budget_limited": "조회 한도로 일부 보조 자료를 읽지 못했습니다.",
		"time_budget_reached":               "조사 시간 한도에 도달했습니다.",
		"refinement_not_verified":           "검색 필터 적용을 확인하지 못했습니다.",
	}
	return labels[reason]
}

func referenceText(r core.ProductReference) string {
	parts := []string{"상품 " + r.ProductID}
	if r.ItemID != "" {
		parts = append(parts, "아이템 "+r.ItemID)
	}
	if r.VendorItemID != "" {
		parts = append(parts, "판매 옵션 "+r.VendorItemID)
	}
	return strings.Join(parts, " · ")
}

func conditionPresentation(a core.ProductConditionAssessment) conditionView {
	c := a.Condition
	v := conditionView{ID: c.ID, Requirement: c.Field, Status: map[core.ProductConditionStatus]string{core.ProductConditionMet: "충족", core.ProductConditionUnmet: "불충족", core.ProductConditionUnknown: "미확인"}[a.Status], Value: "미확인"}
	if c.Validate() != nil {
		v.Missing = "지원되는 조건 형식이 아닙니다."
		return v
	}
	label := map[string]string{"price.current_amount": "표시 가격", "rating": "상품 페이지 평점", "rocket": "로켓 배송", "free_shipping": "무료 배송", "specifications.memory_gb": "RAM", "specifications.storage_gb": "저장용량"}[c.Field]
	op := map[string]string{"gte": "≥", "lte": "≤", "eq": "="}[c.Operator]
	wanted := ""
	if c.Integer != nil {
		wanted = conditionNumber(c.Field, *c.Integer)
	}
	if c.Number != nil {
		wanted = strconv.FormatFloat(*c.Number, 'f', -1, 64) + " / 5"
	}
	if c.Boolean != nil {
		wanted = conditionBoolean(*c.Boolean)
	}
	v.Requirement = label + " " + op + " " + wanted
	if a.ObservedInteger != nil {
		v.Value = conditionNumber(c.Field, *a.ObservedInteger)
	}
	if a.ObservedNumber != nil {
		v.Value = strconv.FormatFloat(*a.ObservedNumber, 'f', -1, 64) + " / 5"
	}
	if a.ObservedBoolean != nil {
		v.Value = conditionBoolean(*a.ObservedBoolean)
	}
	if a.DerivedInteger != nil && a.Derivation != nil {
		v.Value = conditionNumber(c.Field, *a.DerivedInteger) + " (원문 표기 환산)"
		v.Derivation = "파생 · " + a.Derivation.Input.Name + " = " + a.Derivation.Input.Value + " · " + a.Derivation.Method
	}
	for _, e := range a.Evidence {
		scope := map[string]string{"selected_option": "정확한 판매 옵션", "product": "상품 단위", "product_page": "상품 페이지 단위"}[e.Scope]
		v.Evidence = append(v.Evidence, fmt.Sprintf("%s · %s · %s · %s · %s", scope, e.Provenance, e.Source, e.Locator, e.CapturedAt.UTC().Format(time.RFC3339)))
	}
	if len(a.MissingEvidence) > 0 {
		v.Missing = "판단할 값이나 출처·범위 근거가 부족합니다."
		for _, field := range a.MissingEvidence {
			if strings.HasSuffix(field, ".compound_attribute") {
				v.Missing = "복합 옵션을 개별 사양으로 분리할 근거가 없습니다."
				break
			}
			if strings.HasSuffix(field, ".conflict") {
				v.Missing = "원본 자료의 값이 서로 다릅니다."
				break
			}
			if strings.HasSuffix(field, ".unrecognized_capacity") || strings.HasSuffix(field, ".nonintegral_gb") {
				v.Missing = "원본 용량을 지원하는 단위의 정수 GB로 해석할 수 없습니다."
				break
			}
		}
	}
	return v
}

func conditionNumber(field string, value int64) string {
	if field == "price.current_amount" {
		return formatInteger(value) + "원"
	}
	return formatInteger(value) + " GB"
}

func conditionBoolean(value bool) string {
	if value {
		return "예"
	}
	return "아니요"
}

func exclusionText(reason string) string {
	labels := map[string]string{"required_condition_unmet": "필수 조건 불충족", "required_condition_unverified": "필수 조건 미확인", "over_budget": "예산 초과", "budget_condition_unverified": "예산 조건 미확인", "dominated_on_requested_axes": "요청한 비교 축에서 다른 후보가 우세"}
	if label, ok := labels[reason]; ok {
		return label + " (" + reason + ")"
	}
	return reason
}

type nextActionView struct {
	Label, Reason                    string
	References, ConditionIDs, Fields []string
}

func nextActionViews(actions []core.ProductRecommendationNextAction) []nextActionView {
	labels := map[string]string{"restore_source_access": "현재 연결의 접속 상태부터 확인", "rediscover_filters": "현재 검색 필터 다시 확인", "inspect_evidence": "미확인 항목의 근거 확인", "clarify_preference": "비교 항목의 우선순위 확인", "inspect_details": "아직 확인하지 못한 판매 옵션의 상세 조회", "clarify_requirements": "필수 조건 확인", "review_purchase_coverage": "구매 이력의 수집 범위 확인", "continue_discovery": "남은 검색 범위 조사", "refine_search": "검색 조건 조정", "reconsider_conditions": "불충족 조건을 유지할지 확인"}
	var result []nextActionView
	for _, a := range actions {
		label := labels[a.Kind]
		if label == "" {
			label = "추가 확인 (" + a.Kind + ")"
		}
		v := nextActionView{Label: label, Reason: a.Reason, ConditionIDs: a.ConditionIDs, Fields: a.Fields}
		for _, ref := range a.References {
			v.References = append(v.References, referenceText(ref))
		}
		result = append(result, v)
	}
	return result
}
