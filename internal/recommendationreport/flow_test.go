package recommendationreport

import (
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestDecisionFlowRecomputesInsteadOfTrustingClaimedSuccess(t *testing.T) {
	r := editorialReport(0)
	c := decisionCandidate("101", "Synthetic decision", "16GB")
	c.Inspection.FetchedAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	r.Recommendation.Candidates = []core.ProductRecommendationCandidate{c}
	r.Recommendation.Inspected = []core.ProductRecommendationCandidate{c}
	r.Recommendation.Audit.DetailsInspected = 99
	r.DecisionPaths = []core.ProductReportDecisionPath{{Label: "합성 경로", When: "명시한 조건을 우선한다면", Reason: "Synthetic condition", References: []core.ProductReference{c.Product.Reference}}}
	v, err := makeReportView(r)
	if err != nil {
		t.Fatal(err)
	}
	if v.Flow.Met != 0 || v.Flow.Unmet != 1 || v.Flow.Detail != "상세 연결 1개 옵션" {
		t.Fatalf("fabricated success trusted: %+v", v.Flow)
	}
	if len(v.Flow.Paths[0].Products) != 1 || v.Flow.Paths[0].Products[0].Anchor != "candidate-1" {
		t.Fatal("path lost exact candidate link")
	}
	r.Recommendation.Inspected[0].Inspection.Product.Reference.ItemID = "999"
	v, err = makeReportView(r)
	if err != nil {
		t.Fatal(err)
	}
	if v.Flow.Unknown != 1 || v.Flow.Detail != "상세 연결 0개 옵션" {
		t.Fatal("wrong-option evidence promoted to verified detail")
	}
}

func TestDecisionFlowDoesNotInventResearchOrFilters(t *testing.T) {
	r := editorialReport(0)
	r.Recommendation.Audit.ListingsObserved = 999
	r.Recommendation.Audit.SourceFacetsObserved = 20
	v, err := makeReportView(r)
	if err != nil {
		t.Fatal(err)
	}
	if v.Flow.Discovery != "출발 목록 미기록" || !strings.Contains(v.Flow.Filters, "확인한 기록이 없습니다") || v.Flow.Paths[0].Label != "조건부 경로 미기록" {
		t.Fatal("missing provenance fabricated")
	}
}

func TestDecisionFlowEscapesEditorialInputAndHasStaticAccessibleDiagram(t *testing.T) {
	r := editorialReport(1)
	r.DecisionPaths = []core.ProductReportDecisionPath{{Label: "<img> 합성", When: "<script>bad()</script>", Reason: "조건부 <판단>", References: []core.ProductReference{r.Recommendation.Candidates[0].Product.Reference}}}
	data, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	h := string(data)
	for _, want := range []string{`id="decision-flow"`, `aria-labelledby="recommendation-flow-title recommendation-flow-desc"`, `<title id="recommendation-flow-title">`, `class="flow-mobile"`, `href="#candidate-1"`, "가정 · 추론", "&lt;script&gt;bad()&lt;/script&gt;", "출발 목록 미기록"} {
		if !strings.Contains(h, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(h, "<script>bad()") || strings.Count(h, "</script>") != 1 {
		t.Fatal("diagram content became script")
	}
}

func TestDecisionFlowLabelsFitTwoLines(t *testing.T) {
	for _, label := range []string{"Windows 포함 우선", "직접 설치 가능", strings.Repeat("가", 20)} {
		lines := flowLabelLines(label)
		if len(lines) < 1 || len(lines) > 2 {
			t.Fatalf("label needs more than two lines: %q", label)
		}
	}
	if lines := flowLabelLines("Windows 포함 우선"); len(lines) != 1 {
		t.Fatalf("short mixed-language label split unnecessarily: %v", lines)
	}
}
