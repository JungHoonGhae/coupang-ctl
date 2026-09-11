package recommendationreport

import (
	"fmt"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type flowStep struct{ Number, Title, Value string }
type flowProduct struct{ Name, Anchor, Price, PriceEvidence, Options, ObservedAt string }
type flowPath struct {
	LabelLines          []string
	Label, When, Reason string
	Products            []flowProduct
	X, Center, Attach   int
	Outbound, Inbound   string
}
type decisionFlowView struct {
	Steps                                                    []flowStep
	Paths                                                    []flowPath
	Products                                                 []flowProduct
	Requirements                                             []string
	Met, Unmet, Unknown, Undeclared                          int
	Discovery, Detail, Filters, Scope, Selection, Conclusion string
}

// This is a logical evidence map, not a reconstructed execution log. Counts
// come from records and reassessed conditions, never success flags in audit.
func decisionFlow(r core.ProductRecommendationReport, candidates []candidateView, status string) decisionFlowView {
	f := decisionFlowView{Scope: r.Recommendation.Audit.DiscoveryStopReason, Filters: "이번 입력에는 필터 적용을 확인한 기록이 없습니다."}
	discovery := map[core.ProductReference]bool{}
	for _, c := range r.Recommendation.Discovered {
		discovery[c.Product.Reference] = true
	}
	if len(discovery) == 0 {
		f.Discovery = "출발 목록 미기록"
	} else {
		f.Discovery = fmt.Sprintf("출발 목록 %d개 옵션", len(discovery))
	}
	inspected := map[core.ProductReference]bool{}
	for _, c := range r.Recommendation.Inspected {
		if c.Product.Reference == c.Inspection.Product.Reference && !c.Inspection.FetchedAt.IsZero() {
			inspected[c.Product.Reference] = true
		}
	}
	f.Detail = fmt.Sprintf("상세 연결 %d개 옵션", len(inspected))
	if rr := r.Recommendation.Refinement; rr != nil && len(rr.AppliedSelections) > 0 {
		var labels []string
		for _, s := range rr.AppliedSelections {
			labels = append(labels, s.Name+" = "+s.Label)
		}
		f.Filters = "적용 확인 기록: " + strings.Join(labels, " / ")
	}
	seenRequirements := map[string]bool{}
	for _, d := range decisionViews(r.Recommendation) {
		switch d.Outcome {
		case "조건 판정: 충족":
			f.Met++
		case "조건 판정: 불충족":
			f.Unmet++
		case "조건 판정: 미확인":
			f.Unknown++
		default:
			f.Undeclared++
		}
		for _, c := range d.Conditions {
			if !seenRequirements[c.Requirement] {
				f.Requirements = append(f.Requirements, c.Requirement)
				seenRequirements[c.Requirement] = true
			}
		}
	}
	for index, c := range candidates {
		f.Products = append(f.Products, flowProduct{Name: c.DisplayName, Anchor: fmt.Sprintf("candidate-%d", index+1), Price: c.PriceText, PriceEvidence: c.PriceEvidence, Options: selectedOptionText(c.SelectedAttributes), ObservedAt: c.SelectedAttributesCapturedAt})
	}
	for _, path := range r.DecisionPaths {
		p := flowPath{Label: path.Label, When: path.When, Reason: path.Reason}
		for _, ref := range path.References {
			for i, c := range candidates {
				if c.Candidate.Product.Reference == ref {
					p.Products = append(p.Products, f.Products[i])
				}
			}
		}
		f.Paths = append(f.Paths, p)
	}
	if len(f.Paths) == 0 {
		f.Paths = []flowPath{{Label: "조건부 경로 미기록", When: "사용자 우선순위를 임의로 정하지 않습니다.", Reason: "후보별 해설과 확인하지 못한 조건을 읽고 선택 기준을 추가해야 합니다."}}
	}
	for i := range f.Paths {
		p := &f.Paths[i]
		p.LabelLines = flowLabelLines(p.Label)
		p.Center = 480 + (i*2-len(f.Paths)+1)*152
		p.X = p.Center - 128
		p.Attach = 480 + (i*2-len(f.Paths)+1)*48
		if p.Center == 480 {
			p.Outbound = "M 480 584 V 640"
		} else if p.Center < 480 {
			p.Outbound = fmt.Sprintf("M 360 536 H %d Q %d 536 %d 544 V 640", p.Center+8, p.Center, p.Center)
		} else {
			p.Outbound = fmt.Sprintf("M 600 536 H %d Q %d 536 %d 544 V 640", p.Center-8, p.Center, p.Center)
		}
		if p.Center == 480 {
			p.Inbound = "M 480 752 V 832"
		} else {
			delta := 8
			if p.Center > p.Attach {
				delta = -8
			}
			p.Inbound = fmt.Sprintf("M %d 752 V 784 Q %d 792 %d 792 H %d Q %d 792 %d 800 V 832", p.Center, p.Center, p.Center+delta, p.Attach-delta, p.Attach, p.Attach)
		}
	}
	f.Selection = "후보별 조건부 제안"
	f.Conclusion = status + " · 시장 전체 1위는 미판정"
	f.Steps = []flowStep{
		{"01", "요청과 기준", "입력된 필수 조건 확인"},
		{"02", "검색·출발 범위", f.Discovery},
		{"03", "상세 근거 연결", f.Detail},
		{"04", "조건을 다시 계산", fmt.Sprintf("충족 %d · 나머지 %d", f.Met, f.Unmet+f.Unknown+f.Undeclared)},
	}
	return f
}

// CJK consumes more width than Latin. Keep complete short labels on one line
// where they fit instead of breaking a Korean word by raw character count.
func flowLabelLines(label string) []string {
	var lines []string
	var current []rune
	units := 0.0
	for _, r := range label {
		width := .62
		if r >= 0x2e80 {
			width = 1
		}
		if units+width > 11 && len(current) > 0 {
			lines = append(lines, strings.TrimSpace(string(current)))
			current = nil
			units = 0
		}
		current = append(current, r)
		units += width
	}
	if len(current) > 0 {
		lines = append(lines, strings.TrimSpace(string(current)))
	}
	return lines
}
