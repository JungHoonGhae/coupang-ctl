package recommendationreport

import (
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func syntheticPurchaseReport() core.ProductRecommendationReport {
	c := decisionCandidate("101", "Synthetic purchase candidate", "32GB")
	exhausted := true
	return core.ProductRecommendationReport{Title: "Synthetic private report", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Candidates: []core.ProductRecommendationCandidate{c}, Inspected: []core.ProductRecommendationCandidate{c}, PurchaseContext: &core.RecommendationPurchaseContext{
		Visibility: "private_local", Status: "partial", Provenance: core.RecommendationPurchaseProvenance,
		Matches: []core.RecommendationPurchaseMatch{{Reference: core.ProductReference{ProductID: "101", VendorItemID: "1012"}, IdentityScope: "product_and_vendor_item", OrderCount: 2, ItemLineCount: 3, RetainedUnits: 5, FirstPurchaseMonth: "2026-07", LatestPurchaseMonth: "2026-08"}},
		Sync:    &core.SyncStatus{SchemaVersion: core.SyncStatusSchemaVersion, Visibility: "private_local", State: core.SyncRunCompleted, Source: core.SyncSourceCamofox, CoverageStatus: core.SyncCoverageUnverified, StartedAt: "2026-09-01T00:00:00Z", CompletedAt: "2026-09-01T00:01:00Z", PagesProcessed: 1, OrdersSeen: 3, CursorExhausted: &exhausted, ErrorCode: "synthetic-private-error", Scan: &core.SyncScanEvidence{State: core.SyncScanCursorExhausted, StartedAt: "2026-08-31T00:00:00Z", EndedAt: "2026-09-01T00:01:00Z", StartsFromBeginning: true, Attempts: 2, PagesProcessed: 4, RetainedOrdersObserved: 6, RetainedOrdersNotObserved: 2}},
	}}}
}

func TestReportShowsPurchaseScopeCountsAndSyncSeparately(t *testing.T) {
	r := syntheticPurchaseReport()
	r.Recommendation.PurchaseContext.Matches = append(r.Recommendation.PurchaseContext.Matches, core.RecommendationPurchaseMatch{Reference: core.ProductReference{ProductID: "101"}, IdentityScope: "product_only", OrderCount: 3, ItemLineCount: 4, RetainedUnits: 7})
	data, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`id="purchase-context"`, "개인용 로컬 보고서", "부분 자료 기반 구매 집계", "상품·판매 옵션 ID 일치", "상품 단위 · 여러 판매 옵션을 합친 집계", "<dt>구매 주문 수</dt><dd>2건", "<dt>구매 품목 행 수</dt><dd>3행", "<dt>취소·반품 차감 수량</dt><dd>5개", "2026-07", "2026-08", "<dt>확인된 최초 구매 월</dt><dd>미기록", "시도 종료 · 전체 이력 확보를 뜻하지 않음", "2026-09-01T00:01:00Z", "<dt>누적 저장 페이지</dt><dd>4", "<dt>마지막 시도에서 처리한 페이지</dt><dd>1", "<dt>현재 로컬 주문 중 이 스캔에서 못 본 주문</dt><dd>2", "전체 계정의 구매 이력 확보는 미검증", "겹칠 수 있어 합산하지 않습니다", "현재 보유량이 아닙니다", `name="referrer" content="no-referrer"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("purchase report omitted %q", want)
		}
	}
	if strings.Contains(text, "synthetic-private-error") || strings.Contains(text, "<dd>12개") {
		t.Fatal("raw error exposed or overlapping purchase aggregates summed")
	}
}

func TestReportPurchaseMissingStatesDoNotInventHistory(t *testing.T) {
	for _, state := range []string{"not requested", "unavailable", "no match", "no sync", "legacy scan", "available is not verified"} {
		t.Run(state, func(t *testing.T) {
			r := syntheticPurchaseReport()
			p := r.Recommendation.PurchaseContext
			want := ""
			switch state {
			case "not requested":
				r.Recommendation.PurchaseContext = nil
				want = "이 결과에는 구매 이력 집계가 없습니다."
			case "unavailable":
				p.Status, p.Provenance, p.Matches, p.Sync = "unavailable", "", nil, nil
				want = "구매 자료를 읽지 못했습니다."
			case "no match":
				p.Matches = nil
				want = "구매한 적이 없다는 뜻은 아닙니다."
			case "no sync":
				p.Sync = nil
				want = "동기화 기록이 없어 수집 범위와 마지막 시도 시각을 확인할 수 없습니다."
			case "legacy scan":
				p.Sync.SchemaVersion, p.Sync.Scan = 2, nil
				want = "누적 스캔 기록이 없습니다."
			case "available is not verified":
				p.Status = "available"
				want = "전체 계정의 구매 이력 확보는 미검증"
			}
			data, err := Render(r)
			if err != nil || !strings.Contains(string(data), want) {
				t.Fatal("missing purchase state misrepresented", err)
			}
		})
	}
}

func TestReportRejectsInvalidPurchaseClaimsWithoutEchoingInput(t *testing.T) {
	for name, mutate := range map[string]func(*core.RecommendationPurchaseContext){
		"public":            func(p *core.RecommendationPurchaseContext) { p.Visibility = "public" },
		"unrelated":         func(p *core.RecommendationPurchaseContext) { p.Matches[0].Reference.VendorItemID = "999" },
		"item identity":     func(p *core.RecommendationPurchaseContext) { p.Matches[0].Reference.ItemID = "1011" },
		"scope":             func(p *core.RecommendationPurchaseContext) { p.Matches[0].IdentityScope = "product_only" },
		"count":             func(p *core.RecommendationPurchaseContext) { p.Matches[0].RetainedUnits = 0 },
		"duplicate":         func(p *core.RecommendationPurchaseContext) { p.Matches = append(p.Matches, p.Matches[0]) },
		"date precision":    func(p *core.RecommendationPurchaseContext) { p.Matches[0].FirstPurchaseMonth = "2026-07-01" },
		"date order":        func(p *core.RecommendationPurchaseContext) { p.Matches[0].FirstPurchaseMonth = "2026-09" },
		"source":            func(p *core.RecommendationPurchaseContext) { p.Provenance = "synthetic-sensitive-marker" },
		"unavailable rows":  func(p *core.RecommendationPurchaseContext) { p.Status = "unavailable" },
		"unknown status":    func(p *core.RecommendationPurchaseContext) { p.Status = "synthetic-sensitive-marker" },
		"full history":      func(p *core.RecommendationPurchaseContext) { p.Sync.HistoryComplete = true },
		"negative scan":     func(p *core.RecommendationPurchaseContext) { p.Sync.Scan.RetainedOrdersNotObserved = -1 },
		"invalid timestamp": func(p *core.RecommendationPurchaseContext) { p.Sync.StartedAt = "synthetic-sensitive-marker" },
	} {
		t.Run(name, func(t *testing.T) {
			r := syntheticPurchaseReport()
			mutate(r.Recommendation.PurchaseContext)
			data, err := Render(r)
			if err == nil || len(data) != 0 || strings.Contains(err.Error(), "synthetic-sensitive-marker") {
				t.Fatal("invalid private aggregate accepted or echoed")
			}
		})
	}
}

func TestReportEscapesPurchaseAnnotations(t *testing.T) {
	r := syntheticPurchaseReport()
	markup := `</script><img src=x onerror=alert(1)>`
	r.Recommendation.PurchaseContext.Limitations = []string{markup}
	r.Recommendation.PurchaseContext.Sync.Limitations = []string{markup}
	data, err := Render(r)
	if err != nil || strings.Contains(string(data), markup) || strings.Count(string(data), "</script>") != 1 {
		t.Fatal("purchase annotations interpreted as markup", err)
	}
}
