package recommendationreport

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

//go:embed template.html
var reportTemplate string

//go:embed radar.js
var radarScript string

//go:embed decision_flow.html
var decisionFlowTemplate string

type comparisonCell struct{ Label, Weight, Rule, Value, Provenance, Explanation string }
type comparisonView struct {
	Name     string
	Cells    []comparisonCell
	Complete bool
}

type candidateView struct {
	Candidate                    core.ProductRecommendationCandidate
	VisualEvidence               []visualEvidenceView
	VisualCount                  int
	PriceText                    string
	PriceEvidence                string
	ReviewCountText              string
	ReviewCountLabel             string
	SelectedAttributes           []core.ProductSelectedAttribute
	SelectedAttributesCapturedAt string
	Capacities                   []capacityView
	DisplayName                  string
	Rationale                    string
	Tradeoffs                    []string
}

type capacityView struct{ Label, Value, Evidence string }

type visualEvidenceView struct {
	Evidence       core.ProductImageVisualEvidence
	RetrievalLabel string
}

type reportView struct {
	Report          core.ProductRecommendationReport
	Candidates      []candidateView
	RadarJSON       template.JS
	PairwiseJSON    template.JS
	HasScores       bool
	HasRadar        bool
	StatusLabel     string
	Comparisons     []comparisonView
	RadarScript     template.JS
	VisualTotal     int
	Decisions       []decisionView
	NextActions     []nextActionView
	InspectionStop  string
	UninspectedText string
	Purchase        purchaseView
	Flow            decisionFlowView
}

// RenderResult is shared by CLI/MCP and has no account, browser or filesystem
// dependency. It renders supplied evidence, never generates source observations.
func RenderResult(ctx context.Context, report core.ProductRecommendationReport) (core.ProductRecommendationReportRenderResult, error) {
	if err := ctx.Err(); err != nil {
		return core.ProductRecommendationReportRenderResult{}, err
	}
	html, err := Render(report)
	if err != nil {
		return core.ProductRecommendationReportRenderResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return core.ProductRecommendationReportRenderResult{}, err
	}
	return core.ProductRecommendationReportRenderResult{
		SchemaVersion: core.ProductRecommendationReportSchemaVersion, Visibility: "private_local",
		RecommendationStatus: report.Recommendation.Status, MediaType: "text/html; charset=utf-8", HTML: string(html), BytesRendered: len(html),
	}, nil
}

type boundedReportBuffer struct{ buffer bytes.Buffer }

func (b *boundedReportBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *boundedReportBuffer) Write(value []byte) (int, error) {
	if len(value) > core.ProductReportMaxOutputBytes-b.buffer.Len() {
		return 0, errors.New("report exceeds output byte limit")
	}
	return b.buffer.Write(value)
}

func Render(report core.ProductRecommendationReport) ([]byte, error) {
	encoded, err := json.Marshal(report)
	if err != nil || len(encoded) > core.ProductReportMaxInputBytes {
		return nil, core.WithErrorCode("invalid_request", errors.New("report is invalid or exceeds input byte limit"))
	}
	if report.SchemaVersion == 0 {
		report.SchemaVersion = core.ProductRecommendationReportSchemaVersion
	}
	if err := core.ValidateRequest(report); err != nil {
		return nil, err
	}
	view, err := makeReportView(report)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("recommendation-report").Funcs(template.FuncMap{
		"observedPrice": observedPriceText,
		"priceEvidence": func(product core.ProductCard) string { return fieldEvidenceText(product, "price.current_amount") },
		"optionText":    selectedOptionText,
		"stepY":         func(index int) int { return 40 + index*112 },
		"add":           func(a, b int) int { return a + b },
		"money":         func(value int64) string { return formatInteger(value) + "원" },
		"integer": func(value any) string {
			switch typed := value.(type) {
			case int:
				return formatInteger(int64(typed))
			case int64:
				return formatInteger(typed)
			default:
				return fmt.Sprint(value)
			}
		},
		"percent": func(value float64) string { return fmt.Sprintf("%.0f%%", value*100) },
	}).Parse(reportTemplate + decisionFlowTemplate)
	if err != nil {
		return nil, err
	}
	var output boundedReportBuffer
	if err := tmpl.Execute(&output, view); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func makeReportView(report core.ProductRecommendationReport) (reportView, error) {
	labels := map[core.ProductRecommendationStatus]string{
		core.ProductRecommendationComplete:   "설정된 조사 범위 처리 완료",
		core.ProductRecommendationIncomplete: "검증 미완료",
		core.ProductRecommendationNeedsInput: "추가 조건 확인 필요",
		core.ProductRecommendationNoMatches:  "조건에 맞는 후보 없음",
	}
	comparisons := make([]comparisonView, 0, len(report.Scores))
	for _, score := range report.Scores {
		view := comparisonView{Name: score.ProductID, Complete: true}
		for _, candidate := range report.Recommendation.Candidates {
			if candidate.Product.Reference.ProductID == score.ProductID {
				view.Name = candidate.Product.Name + " · " + score.ProductID
				break
			}
		}
		values := make(map[string]core.ProductComparisonValue)
		for _, value := range score.Values {
			values[value.AxisID] = value
		}
		for _, axis := range report.Axes {
			value := values[axis.ID]
			cell := comparisonCell{Label: axis.Label, Weight: fmt.Sprintf("%.1f%%", axis.Weight*100), Rule: axis.EvidenceRule, Value: "미확인", Provenance: "미확인", Explanation: value.Explanation}
			if value.Score != nil {
				cell.Value = fmt.Sprintf("%.1f", *value.Score)
				cell.Provenance = value.Provenance
			} else {
				view.Complete = false
			}
			view.Cells = append(view.Cells, cell)
		}
		comparisons = append(comparisons, view)
	}
	visualByProduct := make(map[string][]visualEvidenceView)
	for _, evidence := range report.VisualEvidence {
		visualByProduct[evidence.ProductID] = append(visualByProduct[evidence.ProductID], visualEvidenceView{
			Evidence: evidence, RetrievalLabel: retrievalLabel(evidence.Retrieval),
		})
	}
	candidates := make([]candidateView, 0, len(report.Recommendation.Candidates))
	for _, candidate := range report.Recommendation.Candidates {
		visuals := visualByProduct[candidate.Product.Reference.ProductID]
		reviewText, reviewLabel := reviewCountText(candidate)
		view := candidateView{Candidate: candidate, VisualEvidence: visuals, VisualCount: len(visuals), PriceText: observedPriceText(candidate.Product), PriceEvidence: fieldEvidenceText(candidate.Product, "price.current_amount"), ReviewCountText: reviewText, ReviewCountLabel: reviewLabel}
		view.DisplayName = candidate.Product.Name
		for _, note := range report.CandidateNotes {
			if note.Reference == candidate.Product.Reference {
				view.DisplayName, view.Rationale, view.Tradeoffs = note.Title, note.Rationale, note.Tradeoffs
			}
		}
		view.Capacities = candidateCapacities(candidate)
		if e, ok := candidate.Inspection.SelectedAttributesEvidence(); ok && candidate.Product.Reference == candidate.Inspection.Product.Reference {
			view.SelectedAttributes = candidate.Inspection.SelectedAttributes
			view.SelectedAttributesCapturedAt = e.CapturedAt.UTC().Format(time.RFC3339)
		}
		candidates = append(candidates, view)
	}
	radar, err := json.Marshal(struct {
		Axes   []core.ProductComparisonAxis `json:"axes"`
		Scores []core.ProductCandidateScore `json:"scores"`
	}{Axes: report.Axes, Scores: report.Scores})
	if err != nil {
		return reportView{}, err
	}
	pairwiseRows := make([]map[string]any, 0, len(report.Recommendation.Candidates))
	for index, candidate := range report.Recommendation.Candidates {
		reviewText, reviewLabel := reviewCountText(candidate)
		capacityText := []string{}
		for _, value := range candidateCapacities(candidate) {
			capacityText = append(capacityText, value.Label+": "+value.Value)
		}
		pairwiseRows = append(pairwiseRows, map[string]any{
			"id": candidate.Product.Reference.ProductID, "name": candidates[index].DisplayName,
			"price_text": observedPriceText(candidate.Product), "rating_text": observedRatingText(candidate.Product),
			"price_evidence": fieldEvidenceText(candidate.Product, "price.current_amount"), "rating_evidence": fieldEvidenceText(candidate.Product, "rating"),
			"reviews_examined": candidate.ReviewsExamined, "review_count_text": reviewText + " · " + reviewLabel,
			"images_found": candidate.DetailImagesFound, "images_reviewed": len(visualByProduct[candidate.Product.Reference.ProductID]),
			"capacity_text": strings.Join(capacityText, " / "),
			"option_text":   selectedOptionText(candidates[index].SelectedAttributes),
			"rationale":     candidates[index].Rationale,
		})
	}
	pairwise, err := json.Marshal(pairwiseRows)
	if err != nil {
		return reportView{}, err
	}
	uninspected := "미기록"
	if report.Recommendation.Audit.InspectionStopReason != "" {
		uninspected = formatInteger(int64(report.Recommendation.Audit.UninspectedExactOptions)) + "개"
	}
	return reportView{
		Report: report, Candidates: candidates, RadarJSON: template.JS(radar), PairwiseJSON: template.JS(pairwise),
		HasScores: len(report.Axes) > 0 && len(report.Scores) > 0, VisualTotal: len(report.VisualEvidence),
		HasRadar:    len(report.Axes) >= 3 && len(report.Scores) > 0,
		StatusLabel: labels[report.Recommendation.Status], Comparisons: comparisons, RadarScript: template.JS(radarScript),
		Decisions: decisionViews(report.Recommendation), NextActions: nextActionViews(report.Recommendation.NextActions),
		InspectionStop: inspectionStopText(report.Recommendation.Audit.InspectionStopReason), UninspectedText: uninspected,
		Purchase: purchasePresentation(report.Recommendation),
		Flow:     decisionFlow(report, candidates, labels[report.Recommendation.Status]),
	}, nil
}

func selectedOptionText(attributes []core.ProductSelectedAttribute) string {
	values := make([]string, 0, len(attributes))
	for _, attribute := range attributes {
		values = append(values, attribute.Value)
	}
	return strings.Join(values, " / ")
}

// Recompute from bound source evidence; supplied comparison values and title
// heuristics may choose visibility, but never establish the displayed number.
func candidateCapacities(candidate core.ProductRecommendationCandidate) []capacityView {
	var result []capacityView
	for _, field := range []struct{ key, native, label string }{
		{"specifications.memory_gb", "RAM용량", "RAM"},
		{"specifications.storage_gb", "저장용량", "저장용량"},
	} {
		show := false
		for _, a := range candidate.Inspection.SelectedAttributes {
			show = show || strings.Contains(a.Name, field.native)
		}
		for _, c := range candidate.Conditions {
			show = show || c.Condition.Field == field.key
		}
		for _, v := range candidate.ComparisonValues {
			show = show || v.Field == field.key
		}
		if !show {
			continue
		}
		view := capacityView{Label: field.label, Value: "미확인", Evidence: "독립된 원본 항목과 정확한 판매 옵션의 연결 근거가 필요합니다."}
		if candidate.Product.Reference == candidate.Inspection.Product.Reference {
			v := core.ReadProductComparableValue(candidate.Inspection, field.key)
			if len(v.MissingEvidence) == 0 && v.DerivedInteger != nil && v.Derivation != nil {
				view.Value = formatInteger(*v.DerivedInteger) + " GB (원문 표기 환산)"
				view.Evidence = v.Derivation.Input.Name + " = " + v.Derivation.Input.Value + " · " + v.Evidence[0].CapturedAt.UTC().Format(time.RFC3339)
			} else if slices.Contains(v.MissingEvidence, field.key+".compound_attribute") {
				view.Evidence = "복합 항목을 개별 사양으로 분리하지 않았습니다."
			}
		}
		result = append(result, view)
	}
	return result
}

func observedPriceText(product core.ProductCard) string {
	for _, field := range product.ObservedFields {
		if field == "price.current_amount" && product.Price.CurrentAmount >= 0 {
			if product.Price.Currency == "KRW" {
				return formatInteger(product.Price.CurrentAmount) + "원"
			}
			if product.Price.Currency == "" {
				return formatInteger(product.Price.CurrentAmount) + " (통화 미확인)"
			}
			return formatInteger(product.Price.CurrentAmount) + " " + product.Price.Currency
		}
	}
	return "가격 미확인"
}

func fieldEvidenceText(product core.ProductCard, field string) string {
	e, ok := product.EvidenceFor(field)
	if !ok {
		return "출처·범위·확인 시각 미확인"
	}
	provenance := map[string]string{"observed": "구조화 값", "derived": "계산·텍스트 해석", "inferred": "추론"}[e.Provenance]
	scope := map[string]string{"product": "상품 단위", "product_page": "상품 페이지 단위", "selected_option": "선택 옵션", "unknown": "범위 미확인"}[e.Scope]
	source := map[string]string{"json_ld": "페이지 JSON-LD", "dom": "페이지 DOM", "quantity_info": "수량·가격 응답", "review_endpoint": "리뷰 응답"}[e.Source]
	return strings.Join([]string{provenance, source, e.Method, scope, e.Locator, "확인 " + e.CapturedAt.UTC().Format(time.RFC3339)}, " · ")
}

func observedRatingText(product core.ProductCard) string {
	if !slices.Contains(product.ObservedFields, "rating") || math.IsNaN(product.Rating) || math.IsInf(product.Rating, 0) || product.Rating < 0 || product.Rating > 5 {
		return "미확인"
	}
	return fmt.Sprintf("%.1f", product.Rating)
}

func reviewCountText(candidate core.ProductRecommendationCandidate) (string, string) {
	count := candidate.Inspection.AvailableReviewCount()
	if count == nil || candidate.ReviewsAvailable == nil || *count != *candidate.ReviewsAvailable {
		return "미확인", "원천 리뷰 수 미확인"
	}
	label := "범위 미확인"
	if candidate.ReviewCountScope == "product_page_observed" && candidate.Inspection.Product.ReviewScope == candidate.ReviewCountScope {
		label = "상품 페이지 기준 · 옵션별 수 아님"
	}
	return formatInteger(int64(*count)), label
}

func retrievalLabel(value core.ProductImageRetrieval) string {
	switch value {
	case core.ProductImageRetrievalCDPCache:
		return "CDP 리소스 캐시"
	case core.ProductImageRetrievalBrowserCache:
		return "브라우저 캐시 우선 · 적중 여부 미확인"
	case core.ProductImageRetrievalNetwork:
		return "네트워크 1회 읽기"
	default:
		return string(value)
	}
}

func formatInteger(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	raw := fmt.Sprintf("%d", value)
	parts := make([]string, 0, (len(raw)+2)/3)
	for len(raw) > 3 {
		parts = append(parts, raw[len(raw)-3:])
		raw = raw[:len(raw)-3]
	}
	parts = append(parts, raw)
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	result := strings.Join(parts, ",")
	if negative {
		return "-" + result
	}
	return result
}
