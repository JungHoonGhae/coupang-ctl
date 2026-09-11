package products

import (
	"context"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	productparser "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
)

func TestSearchReaderParserAndBudgetFilterAgree(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the actual shared search reader")
	}
	const script = `
const fs=require('node:fs'),vm=require('node:vm');
const source=fs.readFileSync('../browser/search_page_reader.js','utf8').replace('export function','function');
const mode=process.argv[1],url='https://www.coupang.com/vp/products/123?itemId=456';
const product={'@type':'Product',name:'Synthetic',url,offers:{price:0,priceCurrency:mode==='unknown_currency'?undefined:'KRW',url:mode==='unbound'?undefined:url}};
const target='https://www.coupang.com/np/search?q=synthetic';
const context=vm.createContext({URL,location:{href:target},document:{title:'Synthetic',body:{innerText:''},readyState:'complete',querySelector:()=>null,querySelectorAll:s=>s.includes('ld+json')?[{textContent:JSON.stringify({'@type':'ItemList',url:target,itemListElement:[product]})}]:[]}});
vm.runInContext(source,context);process.stdout.write(JSON.stringify(context.readSelectedSearchPage(target).search));
`
	for _, mode := range []string{"bound", "unbound", "unknown_currency"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			doc, err := exec.CommandContext(ctx, node, "-e", script, mode).Output()
			if err != nil {
				t.Fatal("synthetic shared search reader failed:", err)
			}
			items, _, err := productparser.ParseSearchDocument(doc)
			if err != nil || len(items) != 1 {
				t.Fatalf("reader/parser contract failed: %v", err)
			}
			result, err := New(syntheticSource{items: items}).Search(ctx, core.ProductSearchRequest{Query: "synthetic", MaxPrice: 2000})
			if err != nil {
				t.Fatal(err)
			}
			if (len(result.Items) == 1) != (mode == "bound") {
				t.Fatalf("mode=%s items=%d missing=%v", mode, len(result.Items), result.UnavailableFilterFields)
			}
			if mode == "bound" && result.Items[0].Price.CurrentAmount != 0 {
				t.Fatal("known zero lost")
			}
		})
	}
}

func withSyntheticPriceEvidence(p core.ProductCard) core.ProductCard {
	p.Price.Currency = "KRW"
	e := core.ProductFieldEvidence{Field: "price.current_amount", Provenance: "observed", Source: "json_ld", Locator: "jsonld.Product.offers.price", Method: "native_field", Scope: "product", Reference: p.Reference, CapturedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)}
	if p.Reference.ItemID != "" || p.Reference.VendorItemID != "" {
		e.Scope = "selected_option"
	}
	p.FieldEvidence = []core.ProductFieldEvidence{e}
	return p
}

func TestPriceFiltersRequireMatchingScopeAndProvenance(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*core.ProductCard)
		missing string
	}{
		{"bound native", func(p *core.ProductCard) {}, ""},
		{"bound parsed text", func(p *core.ProductCard) {
			p.FieldEvidence[0].Source = "dom"
			p.FieldEvidence[0].Method = "numeric_parse"
			p.FieldEvidence[0].Provenance = "derived"
		}, ""},
		{"zero", func(p *core.ProductCard) { p.Price.CurrentAmount = 0 }, ""},
		{"page price is not option price", func(p *core.ProductCard) {
			p.FieldEvidence[0].Scope = "product"
			p.FieldEvidence[0].Reference = core.ProductReference{ProductID: p.Reference.ProductID}
		}, "price.current_amount.scope"},
		{"unknown scope", func(p *core.ProductCard) {
			p.FieldEvidence[0].Scope = "unknown"
			p.FieldEvidence[0].Reference = core.ProductReference{ProductID: p.Reference.ProductID}
		}, "price.current_amount.scope"},
		{"missing metadata", func(p *core.ProductCard) { p.FieldEvidence = nil }, "price.current_amount.provenance"},
		{"wrong option", func(p *core.ProductCard) { p.FieldEvidence[0].Reference.ItemID = "999" }, "price.current_amount.provenance"},
		{"heuristic price", func(p *core.ProductCard) {
			p.FieldEvidence[0].Method = "alias_lookup"
			p.FieldEvidence[0].Provenance = "inferred"
		}, "price.current_amount.provenance"},
		{"different currency", func(p *core.ProductCard) { p.Price.Currency = "USD" }, "price.currency"},
		{"unknown currency", func(p *core.ProductCard) { p.Price.Currency = "" }, "price.currency"},
		{"unavailable", func(p *core.ProductCard) { p.ObservedFields = nil }, "price.current_amount"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := withSyntheticPriceEvidence(syntheticRecommendationProduct())
			tc.mutate(&p)
			request := core.ProductSearchRequest{Query: "synthetic", MaxPrice: 2000}
			result, err := New(syntheticSource{items: []core.ProductCard{p}}).Search(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if (len(result.Items) == 1) != (tc.missing == "") {
				t.Fatalf("filtered items=%d missing=%v", len(result.Items), result.UnavailableFilterFields)
			}
			if tc.missing != "" && !slices.Contains(result.UnavailableFilterFields, tc.missing) {
				t.Fatalf("missing reason=%v", result.UnavailableFilterFields)
			}
			request.MaxPrice = 0
			unfiltered, err := New(syntheticSource{items: []core.ProductCard{p}}).Search(context.Background(), request)
			if err != nil || len(unfiltered.Items) != 1 {
				t.Fatal("unrequested price filter removed candidate")
			}
		})
	}
}

func TestRecommendDistinguishesUnknownOptionPriceFromVerifiedBudgetFailure(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		p := withSyntheticPriceEvidence(syntheticRecommendationProduct())
		detail := withSyntheticPriceEvidence(syntheticRecommendationProduct())
		detail.Price.CurrentAmount = 5000
		want := core.ProductRecommendationNoMatches
		if unknown {
			detail.FieldEvidence[0].Scope = "product"
			detail.FieldEvidence[0].Reference = core.ProductReference{ProductID: detail.Reference.ProductID}
			want = core.ProductRecommendationIncomplete
		}
		result, err := New(recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{p}, inspection: core.ProductInspection{Product: detail}}}).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxPrice: 2000})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != want || len(result.Candidates) != 0 || result.Audit.DetailsInspected != 1 {
			t.Fatalf("status=%s candidates=%d details=%d", result.Status, len(result.Candidates), result.Audit.DetailsInspected)
		}
	}
}
