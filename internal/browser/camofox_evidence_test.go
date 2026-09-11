package browser

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	productparser "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
	"os/exec"
	"slices"
	"testing"
	"time"
)

// Execute the shipped reader, not a hand-written copy of its output. No browser,
// page body, account data, or network is involved.
func syntheticSearchReaderResult(t *testing.T, mode, script string) json.RawMessage {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for shared reader boundary test")
	}
	const harness = `
const fs=require('node:fs'),vm=require('node:vm');
const mode=process.argv[1], supplied=process.argv[2];
const target='https://www.coupang.com/np/search?q=synthetic';
const url='https://www.coupang.com/vp/products/123?itemId=456';
const price=mode==='missing'?undefined:mode==='derived'?'0':0;
const product={'@type':'Product',url,name:'Synthetic',offers:{price,priceCurrency:mode==='currency_unknown'?undefined:mode==='foreign'?'USD':'KRW',url}};
if(mode==='image')product.image='https://thumbnail.coupangcdn.com/synthetic.jpg';
const list={'@type':'ItemList',url:target,itemListElement:mode==='empty'?[]:[product],...(mode==='empty'?{numberOfItems:0}:{})};
const document={title:'Synthetic',body:{innerText:''},readyState:'complete',querySelector:()=>null,querySelectorAll:s=>s.includes('ld+json')?[{textContent:JSON.stringify(list)}]:[]};
if(mode==='dom'){
 const card={tagName:'LI',classList:['ProductUnit_productUnit__synthetic'],querySelector:s=>s.startsWith('a[')?{href:url}:s.includes('productName')?{textContent:'Synthetic'}:s.includes('.price-value')?{textContent:'0원'}:null};
 document.querySelectorAll=s=>s==='#product-list'?[{children:[card]}]:[];
}
const context=vm.createContext({URL,document,location:{href:target}});
if(supplied) process.stdout.write(vm.runInContext(supplied,context));
else {
 vm.runInContext(fs.readFileSync('search_page_reader.js','utf8').replace('export function','function'),context);
 process.stdout.write(JSON.stringify(context.readSelectedSearchPage(target)));
}`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, node, "-e", harness, mode, script).Output()
	if err != nil {
		t.Fatal("synthetic reader execution failed:", err)
	}
	return output
}

func assertTransportPriceEvidence(t *testing.T, document []byte, mode string) {
	t.Helper()
	items, _, err := productparser.ParseSearchDocument(document)
	if mode == "empty" {
		if err != nil || len(items) != 0 {
			t.Fatal("explicit no-results lost across transport")
		}
		return
	}
	if err != nil || len(items) != 1 {
		t.Fatalf("parser result: count=%d err=%v", len(items), err)
	}
	item := items[0]
	if mode == "image" {
		imageEvidence, ok := item.EvidenceFor("image_url")
		if item.ImageURL != "https://thumbnail.coupangcdn.com/synthetic.jpg" || !ok || imageEvidence.Reference.ProductID != "123" || imageEvidence.Provenance != "observed" {
			t.Fatal("product image or bound image evidence lost across the reader/transport/parser boundary")
		}
	}
	evidence, available := item.EvidenceFor("price.current_amount")
	if mode == "missing" {
		if available || slices.Contains(item.ObservedFields, "price.current_amount") {
			t.Fatal("missing price became observed zero")
		}
		return
	}
	if !available || item.Price.CurrentAmount != 0 || evidence.Scope != "selected_option" || evidence.Reference != item.Reference {
		t.Fatal("zero price or option-bound evidence lost")
	}
	wantProvenance, wantCurrency := "observed", "KRW"
	if mode == "derived" || mode == "dom" {
		wantProvenance = "derived"
	}
	if mode == "dom" && (item.RankSource != "dom_search_list_order" || evidence.Source != "dom") {
		t.Fatal("DOM source order became native structured rank")
	}
	if mode == "foreign" {
		wantCurrency = "USD"
	}
	if mode == "currency_unknown" {
		wantCurrency = ""
	}
	if evidence.Provenance != wantProvenance || item.Price.Currency != wantCurrency {
		t.Fatal("provenance/currency changed")
	}
	if (len(item.PriceFilterUnavailableFields()) == 0) != (wantCurrency == "KRW") {
		t.Fatal("budget eligibility changed")
	}
}
func TestCamofoxSearchReaderPreservesPriceEvidence(t *testing.T) {
	for _, mode := range []string{"observed", "derived", "dom", "missing", "currency_unknown", "foreign", "empty", "image"} {
		t.Run(mode, func(t *testing.T) {
			result := syntheticSearchReaderResult(t, mode, "")
			wire, err := json.Marshal(map[string]any{"result": result})
			if err != nil {
				t.Fatal(err)
			}
			c := &Camofox{run: func(_ context.Context, _ string, mode string, _ core.QRLinkPresenter) ([]byte, error) {
				if mode != "search" {
					t.Fatal("wrong Camofox operation")
				}
				return wire, nil
			}}
			document, err := c.FetchProductSearch(context.Background(), core.ProductSearchRequest{Query: "synthetic"})
			if err != nil {
				t.Fatal("Camofox rejected the bundled reader", err)
			}
			assertTransportPriceEvidence(t, document, mode)
			var envelope struct {
				Coverage core.ProductCoverage `json:"coverage"`
			}
			if json.Unmarshal(document, &envelope) != nil || envelope.Coverage.Source != "camofox_search_document" {
				t.Fatal("incorrect provenance")
			}
		})
	}
}
func TestCamofoxSearchRejectsMalformedPriceEvidence(t *testing.T) {
	var result struct {
		Search searchDocument `json:"search"`
	}
	if err := json.Unmarshal(syntheticSearchReaderResult(t, "observed", ""), &result); err != nil {
		t.Fatal(err)
	}
	baseline, _ := json.Marshal(result.Search)
	for name, mutate := range map[string]func(*searchDocumentItem){
		"missing amount":    func(i *searchDocumentItem) { i.CurrentAmount = nil },
		"unobserved amount": func(i *searchDocumentItem) { i.ObservedFields = []string{"name"} },
		"wrong currency":    func(i *searchDocumentItem) { i.Currency = "krw" },
		"wrong option":      func(i *searchDocumentItem) { i.FieldEvidence[1].Reference.ItemID = "999" },
		"duplicate proof": func(i *searchDocumentItem) {
			i.FieldEvidence = []core.ProductFieldEvidence{i.FieldEvidence[1], i.FieldEvidence[1]}
		},
		"raw locator":       func(i *searchDocumentItem) { i.FieldEvidence[1].Locator = "https://example.test/private" },
		"invalid timestamp": func(i *searchDocumentItem) { i.FieldEvidence[1].CapturedAt = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			var document searchDocument
			if err := json.Unmarshal(baseline, &document); err != nil {
				t.Fatal(err)
			}
			mutate(&document.Items[0])
			wire, err := json.Marshal(map[string]any{"result": map[string]any{"status": "ok", "search": document}})
			if err != nil {
				t.Fatal(err)
			}
			c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) { return wire, nil }}
			if _, err := c.FetchProductSearch(context.Background(), core.ProductSearchRequest{Query: "synthetic"}); !errors.Is(err, ErrDocumentProtocol) {
				t.Fatal("invalid evidence crossed the Camofox interface")
			}
		})
	}
}
