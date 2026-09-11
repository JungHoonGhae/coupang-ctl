package recommendationreport

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestRenderShowsDiscoveryReviewAndVisualEvidence(t *testing.T) {
	reviewCount := 1200
	report := core.ProductRecommendationReport{
		Title: "200만 원 데스크탑을 꼼꼼하게 비교했어요",
		Recommendation: core.ProductRecommendationResult{
			SchemaVersion: core.ProductRecommendationSchemaVersion,
			Status:        core.ProductRecommendationComplete,
			Query:         "200만원 예산 데스크탑",
			Audit:         core.ProductRecommendationAudit{DiscoveryTarget: 100, UniqueModelFamilies: 100, DetailsInspected: 5, DetailImagesFound: 8},
			Discovered: []core.ProductDiscoveryCandidate{{
				Product:     core.ProductCard{Reference: core.ProductReference{ProductID: "101"}, Name: "Synthetic desktop", ImageURL: "https://thumbnail.coupangcdn.com/synthetic-card.jpg"},
				Appearances: []core.ProductDiscoveryAppearance{{Sort: core.ProductSortSales, Page: 1, Position: 3}}, AxisCount: 1, CrossListCoverage: 0.2,
			}},
			Candidates: []core.ProductRecommendationCandidate{{
				Product:         core.ProductCard{Reference: core.ProductReference{ProductID: "101"}, Name: "Synthetic desktop", URL: "https://www.coupang.com/vp/products/101", ImageURL: "https://thumbnail.coupangcdn.com/synthetic-card.jpg"},
				Inspection:      core.ProductInspection{Reviews: []core.ProductReview{{Rating: 4, Content: "Synthetic review"}}},
				ReviewsExamined: 1, ReviewsAvailable: &reviewCount, DetailImagesFound: 8,
			}},
		},
		VisualEvidence: []core.ProductImageVisualEvidence{
			{ProductID: "101", ImageURL: "https://thumbnail.coupangcdn.com/synthetic-detail-1.jpg", Kind: core.ProductImageKindDetail, Source: "coupang_product_detail", Retrieval: core.ProductImageRetrievalBrowserCache, Findings: []string{"Synthetic port layout observed"}},
			{ProductID: "101", ImageURL: "https://thumbnail.coupangcdn.com/synthetic-detail-2.jpg", Kind: core.ProductImageKindDetail, Source: "coupang_product_detail", Retrieval: core.ProductImageRetrievalCDPCache, Findings: []string{"Synthetic cooling layout observed"}},
		},
	}
	html, err := Render(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{
		"고유 모델군 100개", "synthetic-card.jpg", "리뷰 정렬·표본 메타데이터가 보존되지 않았습니다.", "가격 미확인",
		"상세 이미지 8장 중 2장 직접 확인", "Synthetic port layout observed", "브라우저 캐시",
	} {
		if !strings.Contains(string(html), wanted) {
			t.Fatalf("rendered report is missing %q", wanted)
		}
	}
}

func TestReportPriceRequiresObservedEvidence(t *testing.T) {
	for _, test := range []struct {
		product core.ProductCard
		want    string
	}{
		{core.ProductCard{}, "가격 미확인"},
		{core.ProductCard{ObservedFields: []string{"price.current_amount"}}, "0 (통화 미확인)"},
		{core.ProductCard{Price: core.ProductPrice{Currency: "KRW"}, ObservedFields: []string{"price.current_amount"}}, "0원"},
		{core.ProductCard{Price: core.ProductPrice{CurrentAmount: 1200}}, "가격 미확인"},
		{core.ProductCard{Price: core.ProductPrice{CurrentAmount: 1200, Currency: "KRW"}, ObservedFields: []string{"price.current_amount"}}, "1,200원"},
		{core.ProductCard{Price: core.ProductPrice{CurrentAmount: 12, Currency: "USD"}, ObservedFields: []string{"price.current_amount"}}, "12 USD"},
	} {
		if got := observedPriceText(test.product); got != test.want {
			t.Fatalf("price=%s want=%s", got, test.want)
		}
	}
}

func TestReportDoesNotInterpretProductNamesAsHTML(t *testing.T) {
	name := `</script><img src=x onerror=alert(1)>`
	report := core.ProductRecommendationReport{
		Title: name,
		Recommendation: core.ProductRecommendationResult{
			SchemaVersion: core.ProductRecommendationSchemaVersion,
			Status:        core.ProductRecommendationComplete,
			Candidates:    []core.ProductRecommendationCandidate{{Product: core.ProductCard{Reference: core.ProductReference{ProductID: "123"}, Name: name}}},
		},
	}
	html, err := Render(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), name) || strings.Count(string(html), "</script>") != 1 {
		t.Fatal("raw product markup escaped HTML/JavaScript context")
	}
	if strings.Contains(string(html), "innerHTML") || !strings.Contains(string(html), "option.textContent=item.name") {
		t.Fatal("client-side product labels must use text nodes")
	}
}

func TestReportReviewAndRatingAvailabilityInCardsAndComparison(t *testing.T) {
	for _, known := range []bool{false, true} {
		product := core.ProductCard{Reference: core.ProductReference{ProductID: "101"}, Name: "Synthetic", ReviewScope: "product_page_observed"}
		inspection := core.ProductInspection{Product: product}
		if known {
			product.ObservedFields = []string{"rating", "price.current_amount"}
			product.Price.Currency = "KRW"
			inspection.Coverage.ObservedFields = []string{"rating.count"}
		} else {
			product.Rating = 4.5
			product.Price.CurrentAmount = 999
		}
		candidate := core.ProductRecommendationCandidate{Product: product, Inspection: inspection, ReviewsAvailable: inspection.AvailableReviewCount(), ReviewCountScope: "product_page_observed"}
		report := core.ProductRecommendationReport{Title: "Synthetic report", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationComplete, Candidates: []core.ProductRecommendationCandidate{candidate}, Discovered: []core.ProductDiscoveryCandidate{{Product: product}}}}
		view, err := makeReportView(report)
		if err != nil {
			t.Fatal(err)
		}
		var pairs []map[string]any
		if err := json.Unmarshal([]byte(view.PairwiseJSON), &pairs); err != nil {
			t.Fatal(err)
		}
		wantRating, wantPrice, wantReview := "미확인", "가격 미확인", "미확인"
		if known {
			wantRating, wantPrice, wantReview = "0.0", "0원", "0"
		}
		if pairs[0]["rating_text"] != wantRating || pairs[0]["price_text"] != wantPrice || view.Candidates[0].ReviewCountText != wantReview {
			t.Fatalf("availability lost: %#v", pairs)
		}
		html, err := Render(report)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(html), "<b>0 / "+wantReview+"</b>") {
			t.Fatal("review count card differs from typed availability")
		}
		if !strings.Contains(string(html), "<b>"+wantPrice+"</b>") {
			t.Fatal("discovery price ignores availability")
		}
		if strings.Contains(string(html), "x.rating||") || strings.Contains(string(html), "실제 읽은 리뷰 / 전체 리뷰") {
			t.Fatal("report still interprets zero as unknown or page counts as unscoped totals")
		}
	}
}

func TestReportReviewCountDoesNotTrustUnsupportedSummary(t *testing.T) {
	count := 12
	candidate := core.ProductRecommendationCandidate{ReviewsAvailable: &count, ReviewCountScope: "product_page_observed"}
	if text, _ := reviewCountText(candidate); text != "미확인" {
		t.Fatal("report accepted summary without underlying evidence")
	}
	candidate.Inspection.Coverage.ObservedFields = []string{"rating.count"}
	candidate.Inspection.Rating.Count = 12
	if text, label := reviewCountText(candidate); text != "12" || label != "범위 미확인" {
		t.Fatal("report invented page scope")
	}
	candidate.Inspection.Rating.Count = 13
	if text, _ := reviewCountText(candidate); text != "미확인" {
		t.Fatal("report accepted conflicting summary")
	}
}

func TestReportShowsPartialOutcomeAndWarnings(t *testing.T) {
	r := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{
		SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete,
		Warnings: []string{"Synthetic missing source evidence"}, Audit: core.ProductRecommendationAudit{DiscoveryStopReason: "source_read_incomplete"},
	}}
	html, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"검증 미완료", "Synthetic missing source evidence", "source_read_incomplete"} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("missing outcome evidence: %s", want)
		}
	}
}

func TestReportExplainsInternalTimeBudgetWithoutWarningText(t *testing.T) {
	for _, expired := range []bool{false, true} {
		r := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{
			SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete,
			Audit: core.ProductRecommendationAudit{TimeBudgetExhausted: expired},
		}}
		html, err := Render(r)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(html), "조사 시간 한도에 도달했습니다") != expired {
			t.Fatal("report lost or invented time-budget exhaustion")
		}
	}
}

func TestReportNeverPlotsUnknownAsZero(t *testing.T) {
	if strings.Contains(reportTemplate, "by[axis.id]??0") {
		t.Fatal("missing comparison axis is drawn as zero")
	}
}

func TestRenderedReportScriptPreservesMissingAxesAndKnownZero(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required to execute the rendered report script; pure Go contract tests remain available")
	}
	zero, middle := 0.0, 50.0
	r := core.ProductRecommendationReport{Title: "Synthetic comparison", Recommendation: core.ProductRecommendationResult{
		SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete,
		Candidates: []core.ProductRecommendationCandidate{
			{Product: core.ProductCard{Reference: core.ProductReference{ProductID: "101"}, Name: "Synthetic complete"}},
			{Product: core.ProductCard{Reference: core.ProductReference{ProductID: "102"}, Name: "Synthetic partial"}},
		},
	}, Axes: []core.ProductComparisonAxis{
		{ID: "a", Label: "A", Weight: .4, EvidenceRule: "Synthetic rule A"},
		{ID: "b", Label: "B", Weight: .3, EvidenceRule: "Synthetic rule B"},
		{ID: "c", Label: "C", Weight: .3, EvidenceRule: "Synthetic rule C"},
	}}
	for _, id := range []string{"101", "102"} {
		score := core.ProductCandidateScore{ProductID: id, EvidenceCoverage: 1, Values: []core.ProductComparisonValue{
			{AxisID: "a", Score: &zero, Provenance: "derived", Explanation: "Synthetic calculation"},
			{AxisID: "b", Score: &middle, Provenance: "inferred", Explanation: "Synthetic hypothesis"},
			{AxisID: "c", Score: &middle, Provenance: "observed", Explanation: "Synthetic original score"},
		}}
		if id == "102" {
			score.Values[0].Score = nil
			score.EvidenceCoverage = .6
		}
		r.Scores = append(r.Scores, score)
	}
	html, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"검증 미완료", "Synthetic rule A", "Synthetic hypothesis", "inferred", "미확인 축이 있어", "<td>0.0</td>", "<td>미확인</td>", "report schema 2"} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("missing evidence presentation: %s", want)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Execute the actual HTML's embedded script with a synthetic SVG DOM. No
	// browser, source request, external image, or account data is involved.
	cmd := exec.CommandContext(ctx, node, "-e", `
const fs = require('node:fs'), vm = require('node:vm');
const html = fs.readFileSync(0, 'utf8');
const source = html.match(/<script>([\s\S]*?)<\/script>/)[1];
const svg = {children:[],replaceChildren(){this.children=[]},appendChild(n){this.children.push(n)},ownerDocument:{createElementNS(_ns,tag){return {tag,attrs:{},setAttribute(k,v){this.attrs[k]=v}}}}};
const document = {querySelector(s){return s === '#radar' ? svg : null}};
vm.runInNewContext(source, {document}, {timeout:1000});
const polygons = svg.children.filter(n=>n.attrs['data-role']==='score');
if(polygons.length!==1 || polygons[0].attrs['data-product-id']!=='101' || polygons[0].attrs.points.split(' ')[0]!=='380,290') process.exit(1);
`)
	cmd.Stdin = strings.NewReader(string(html))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rendered script failed: %v %s", err, output)
	}
}
