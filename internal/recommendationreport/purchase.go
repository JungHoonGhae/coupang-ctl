package recommendationreport

import (
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type purchaseDatum struct{ Label, Value string }
type purchaseMatchView struct {
	Name, Reference, Scope, FirstMonth, LatestMonth string
	Orders, Lines, Units                            int
}
type purchaseView struct {
	Status, Empty       string
	HasContext, HasSync bool
	HasScan             bool
	Matches             []purchaseMatchView
	SyncRows, ScanRows  []purchaseDatum
	Limitations         []string
}

// This is a view of supplied local aggregates, not a second aggregation or an
// account-binding check. In particular, product-only rows overlap exact rows.
func purchasePresentation(r core.ProductRecommendationResult) purchaseView {
	p := r.PurchaseContext
	if p == nil {
		return purchaseView{Status: "구매 이력 집계 미포함", Empty: "이 결과에는 구매 이력 집계가 없습니다. 구매 이력을 사용한 개인화 추천으로 해석하지 마세요."}
	}
	v := purchaseView{HasContext: true, Status: map[string]string{"available": "입력된 구매 집계 사용 가능", "partial": "부분 자료 기반 구매 집계", "unavailable": "구매 자료 확인 불가"}[p.Status], Limitations: append([]string(nil), p.Limitations...)}
	if p.Status == "unavailable" {
		v.Empty = "구매 자료를 읽지 못했습니다. 후보의 구매 여부를 판단할 수 없습니다."
	} else if len(p.Matches) == 0 {
		v.Empty = "현재 집계에 연결된 구매 기록이 없습니다. 구매한 적이 없다는 뜻은 아닙니다."
	}
	for _, m := range p.Matches {
		scope := "상품 단위 · 여러 판매 옵션을 합친 집계"
		if m.IdentityScope == "product_and_vendor_item" {
			scope = "상품·판매 옵션 ID 일치 · 주문 자료에는 아이템 ID가 없어 비교하지 않음"
		}
		v.Matches = append(v.Matches, purchaseMatchView{Name: purchaseMatchName(r, m.Reference), Reference: referenceText(m.Reference), Scope: scope, Orders: m.OrderCount, Lines: m.ItemLineCount, Units: m.RetainedUnits, FirstMonth: recordedValue(m.FirstPurchaseMonth), LatestMonth: recordedValue(m.LatestPurchaseMonth)})
	}
	s := p.Sync
	if s == nil {
		return v
	}
	v.HasSync = true
	v.Limitations = append(v.Limitations, s.Limitations...)
	state := map[core.SyncRunState]string{core.SyncRunNeverRun: "시도 기록 없음", core.SyncRunRunning: "진행 중으로 기록됨 · 현재 실행 여부는 확인하지 않음", core.SyncRunCompleted: "시도 종료 · 전체 이력 확보를 뜻하지 않음", core.SyncRunFailed: "시도 실패 · 이전에 저장된 자료는 남아 있을 수 있음"}[s.State]
	coverage := map[core.SyncCoverageStatus]string{core.SyncCoverageNotAssessed: "아직 평가하지 않음", core.SyncCoverageUnverified: "계정 연결과 전체 수집 범위 미검증", core.SyncCoverageUnknownLegacy: "이전 기록 · 계정과 수집 범위 근거 미확인"}[s.CoverageStatus]
	v.SyncRows = []purchaseDatum{{"마지막 동기화 시도", state}, {"전체 이력 검증", coverage}}
	if s.State != core.SyncRunNeverRun {
		source := map[core.SyncSource]string{core.SyncSourceCamofox: "전용 Camofox", core.SyncSourceAside: "이전 Aside 연결", core.SyncSourceDedicatedBrowser: "전용 브라우저", core.SyncSourceCurrentBrowser: "이전 현재 브라우저 연결", core.SyncSourceOrdinaryBrowser: "이전 선택 탭 연결", core.SyncSourceAppleEvents: "이전 Apple Events 연결", core.SyncSourceUnknownLegacy: "이전 수집 경로 미확인"}[s.Source]
		v.SyncRows = append(v.SyncRows, purchaseDatum{"기록된 수집 경로", recordedValue(source)}, purchaseDatum{"마지막 시도 시작", recordedValue(s.StartedAt)}, purchaseDatum{"마지막 시도 종료 (실패 종료 포함)", recordedValue(s.CompletedAt)}, purchaseDatum{"마지막 시도에서 처리한 페이지", formatInteger(int64(s.PagesProcessed))}, purchaseDatum{"마지막 시도에서 관찰한 주문 행", formatInteger(int64(s.OrdersSeen))})
	}
	cursor := "기록 없음"
	if s.CursorExhausted != nil {
		cursor = "이어갈 커서가 있었음"
		if *s.CursorExhausted {
			cursor = "마지막 저장 페이지에 다음 커서 없음 · 전체 이력 완료 아님"
		}
	}
	v.SyncRows = append(v.SyncRows, purchaseDatum{"마지막 페이지 커서", cursor})
	if scan := s.Scan; scan != nil {
		v.HasScan = true
		state := map[core.SyncScanState]string{core.SyncScanActive: "미종료로 기록됨", core.SyncScanCursorExhausted: "커서 소진으로 기록됨", core.SyncScanSuperseded: "새 스캔으로 대체됨"}[scan.State]
		start := "저장된 커서에서 이어 시작"
		if scan.StartsFromBeginning {
			start = "저장된 커서 없이 시작 · 전체 계정 이력의 시작점 보증 아님"
		}
		v.ScanRows = []purchaseDatum{{"누적 스캔 상태", state}, {"시작 방식", start}, {"스캔 시작", recordedValue(scan.StartedAt)}, {"스캔 종료", recordedValue(scan.EndedAt)}, {"스캔 대체 시각", recordedValue(scan.SupersededAt)}, {"이 스캔의 시도 횟수", formatInteger(int64(scan.Attempts))}, {"누적 저장 페이지", formatInteger(int64(scan.PagesProcessed))}, {"현재 로컬 주문 중 이 스캔에서 본 주문", formatInteger(int64(scan.RetainedOrdersObserved))}, {"현재 로컬 주문 중 이 스캔에서 못 본 주문", formatInteger(int64(scan.RetainedOrdersNotObserved))}}
	}
	return v
}

func recordedValue(value string) string {
	if value == "" {
		return "미기록"
	}
	return value
}

func purchaseMatchName(r core.ProductRecommendationResult, ref core.ProductReference) string {
	names := map[string]bool{}
	for _, records := range [][]core.ProductRecommendationCandidate{r.Candidates, r.Inspected} {
		for _, c := range records {
			if c.Product.Reference.ProductID == ref.ProductID && (ref.VendorItemID == "" || c.Product.Reference.VendorItemID == ref.VendorItemID) && strings.TrimSpace(c.Product.Name) != "" {
				names[c.Product.Name] = true
			}
		}
	}
	if len(names) == 1 {
		for name := range names {
			return name
		}
	}
	return referenceText(ref)
}
