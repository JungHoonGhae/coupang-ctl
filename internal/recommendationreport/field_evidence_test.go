package recommendationreport

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestReportPriceEvidenceMatchesCardsDiscoveryAndPairwise(t *testing.T) {
	for _, known := range []bool{false, true} {
		p := core.ProductCard{Reference: core.ProductReference{ProductID: "101", ItemID: "201"}, Name: "Synthetic", Price: core.ProductPrice{CurrentAmount: 1200, Currency: "KRW"}, ObservedFields: []string{"price.current_amount", "rating"}}
		if known {
			p.FieldEvidence = []core.ProductFieldEvidence{{Field: "price.current_amount", Provenance: "derived", Source: "dom", Locator: "dom.search_card.price", Method: "numeric_parse", Scope: "selected_option", Reference: p.Reference, CapturedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)}}
		}
		r := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationComplete, Candidates: []core.ProductRecommendationCandidate{{Product: p}}, Discovered: []core.ProductDiscoveryCandidate{{Product: p}}}}
		view, err := makeReportView(r)
		if err != nil {
			t.Fatal(err)
		}
		want := "출처·범위·확인 시각 미확인"
		if known {
			want = "계산·텍스트 해석 · 페이지 DOM · numeric_parse · 선택 옵션 · dom.search_card.price · 확인 2026-09-07T00:00:00Z"
		}
		if view.Candidates[0].PriceEvidence != want {
			t.Fatalf("caption=%q", view.Candidates[0].PriceEvidence)
		}
		var pairs []map[string]any
		if err := json.Unmarshal([]byte(view.PairwiseJSON), &pairs); err != nil {
			t.Fatal(err)
		}
		if pairs[0]["price_evidence"] != want || pairs[0]["rating_evidence"] != "출처·범위·확인 시각 미확인" {
			t.Fatal("pairwise labels lost provenance")
		}
		html, err := Render(r)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(html), want) < 2 {
			t.Fatal("candidate and discovered card evidence not both rendered")
		}
		if !strings.Contains(string(html), "x.price_evidence,y.price_evidence") || !strings.Contains(string(html), "x.rating_evidence,y.rating_evidence") {
			t.Fatal("pairwise UI dropped evidence rows")
		}
	}
}

func TestReportDoesNotDisplayInvalidProvenanceAsVerified(t *testing.T) {
	p := core.ProductCard{Reference: core.ProductReference{ProductID: "101"}, ObservedFields: []string{"price.current_amount"}, FieldEvidence: []core.ProductFieldEvidence{{Field: "price.current_amount", Provenance: "observed", Source: "dom", Locator: "dom.price", Method: "native_field", Scope: "product", Reference: core.ProductReference{ProductID: "101"}, CapturedAt: time.Now()}}}
	if fieldEvidenceText(p, "price.current_amount") != "출처·범위·확인 시각 미확인" {
		t.Fatal("invalid DOM provenance displayed as verified")
	}
}
