package products

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const htmlSearchTarget = "https://www.coupang.com/np/search?q=synthetic"

var htmlCapturedAt = time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)

func htmlSearchProduct() map[string]any {
	return map[string]any{"@type": "Product", "name": "Synthetic bowl", "url": "https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301", "offers": map[string]any{"price": 0, "priceCurrency": "KRW"}}
}
func htmlSearchList(entries ...any) map[string]any {
	return map[string]any{"@type": "ItemList", "url": htmlSearchTarget, "itemListElement": entries}
}
func htmlSearchDocument(values ...any) []byte {
	var body strings.Builder
	body.WriteString("<!doctype html><title>Synthetic search</title>")
	for _, value := range values {
		encoded, _ := json.Marshal(value)
		fmt.Fprintf(&body, `<script type="application/ld+json">%s</script>`, encoded)
	}
	return []byte(body.String())
}

func TestNormalizeSearchHTMLCanonicalEvidence(t *testing.T) {
	for _, selected := range []bool{false, true} {
		product := htmlSearchProduct()
		if selected {
			product["offers"].(map[string]any)["url"] = product["url"]
		}
		html := htmlSearchDocument(htmlSearchList(map[string]any{"position": 7, "item": product}))
		if _, _, err := ParseSearchDocument(html); !errors.Is(err, ErrProductDataMissing) {
			t.Fatal("canonical parser accepted raw HTML")
		}
		doc, err := NormalizeSearchHTML(html, htmlSearchTarget, htmlCapturedAt)
		if err != nil {
			t.Fatal(err)
		}
		items, coverage, err := ParseSearchDocument(doc)
		if err != nil || len(items) != 1 || coverage.Source != "coupang_search_html_jsonld" {
			t.Fatalf("canonical result: items=%d err=%v", len(items), err)
		}
		item := items[0]
		if item.Reference.VendorItemID != "301" || item.SearchPosition != 7 || item.Price.CurrentAmount != 0 || item.Price.Currency != "KRW" {
			t.Fatal("source values changed")
		}
		evidence, ok := item.EvidenceFor("price.current_amount")
		wantScope := "product"
		if selected {
			wantScope = "selected_option"
		}
		if !ok || evidence.Scope != wantScope || evidence.Provenance != "observed" || evidence.CapturedAt != htmlCapturedAt {
			t.Fatal("price evidence lost or promoted")
		}
	}
}

func TestNormalizeSearchHTMLRequiresRequestBoundUniqueList(t *testing.T) {
	for _, bound := range []string{"", htmlSearchTarget + "&page=2", htmlSearchTarget + "&sorter=salePriceAsc", strings.ReplaceAll(htmlSearchTarget, "synthetic", "other")} {
		list := htmlSearchList(htmlSearchProduct())
		if bound == "" {
			delete(list, "url")
		} else {
			list["url"] = bound
		}
		_, err := NormalizeSearchHTML(htmlSearchDocument(list), htmlSearchTarget, htmlCapturedAt)
		if !errors.Is(err, ErrProductDataMissing) {
			t.Fatal("unbound/other search accepted")
		}
	}
	for _, owner := range []string{"SearchResultsPage", "CollectionPage"} {
		list := htmlSearchList(htmlSearchProduct())
		delete(list, "url")
		page := map[string]any{"@type": owner, "url": htmlSearchTarget, "mainEntity": list}
		if _, err := NormalizeSearchHTML(htmlSearchDocument(map[string]any{"@graph": []any{page}}), htmlSearchTarget, htmlCapturedAt); err != nil {
			t.Fatal(err)
		}
		list["url"] = htmlSearchTarget + "&page=2"
		if _, err := NormalizeSearchHTML(htmlSearchDocument(page), htmlSearchTarget, htmlCapturedAt); !errors.Is(err, ErrProductDataMissing) {
			t.Fatal("owner overrode conflicting list identity")
		}
	}
	list := htmlSearchList(htmlSearchProduct())
	if _, err := NormalizeSearchHTML(htmlSearchDocument(list, list), htmlSearchTarget, htmlCapturedAt); err != nil {
		t.Fatal("exact duplicated list rejected")
	}
	other := htmlSearchProduct()
	other["offers"].(map[string]any)["price"] = 100
	if _, err := NormalizeSearchHTML(htmlSearchDocument(list, htmlSearchList(other)), htmlSearchTarget, htmlCapturedAt); !errors.Is(err, ErrProductDataMissing) {
		t.Fatal("conflicting lists merged")
	}
}

func TestNormalizeSearchHTMLDoesNotInventEmptyResults(t *testing.T) {
	empty := htmlSearchList()
	empty["itemListElement"] = []any{}
	if _, err := NormalizeSearchHTML(htmlSearchDocument(empty), htmlSearchTarget, htmlCapturedAt); !errors.Is(err, ErrProductDataMissing) {
		t.Fatal("empty array invented no-results")
	}
	empty["numberOfItems"] = 0
	doc, err := NormalizeSearchHTML(htmlSearchDocument(empty), htmlSearchTarget, htmlCapturedAt)
	if err != nil {
		t.Fatal(err)
	}
	items, coverage, err := ParseSearchDocument(doc)
	if err != nil || len(items) != 0 || !coverage.SourceNoResults {
		t.Fatal("explicit no-results lost")
	}
	for _, suffix := range []string{`<a href="/vp/products/101">Synthetic</a>`, `<input type="password">`, `<h1>Access Denied</h1>`} {
		if _, err := NormalizeSearchHTML(append(htmlSearchDocument(empty), suffix...), htmlSearchTarget, htmlCapturedAt); err == nil {
			t.Fatal("contradictory empty page accepted")
		}
	}
	empty["itemListElement"] = []any{htmlSearchProduct()}
	if _, err := NormalizeSearchHTML(htmlSearchDocument(empty), htmlSearchTarget, htmlCapturedAt); !errors.Is(err, ErrProductDataMissing) {
		t.Fatal("nonempty no-results contradiction accepted")
	}
}

func TestNormalizeSearchHTMLAccessAndInertData(t *testing.T) {
	data := htmlSearchDocument(htmlSearchList(htmlSearchProduct()))
	for _, test := range []struct {
		suffix string
		want   error
	}{
		{`<h1>Access Denied</h1>`, core.ErrBrowserAccessDenied},
		{`<input type="password"><h1>Access Denied</h1>`, core.ErrAuthenticationRequired},
		{`<input type="password"><p>CAPTCHA</p>`, core.ErrBrowserAccessDenied},
	} {
		if _, err := NormalizeSearchHTML(append(append([]byte{}, data...), test.suffix...), htmlSearchTarget, htmlCapturedAt); !errors.Is(err, test.want) {
			t.Fatalf("access classification: %v", err)
		}
	}
	for _, tag := range []string{"template", "noscript", "svg"} {
		wrapped := []byte("<" + tag + ">" + string(data) + "</" + tag + ">")
		if _, err := NormalizeSearchHTML(wrapped, htmlSearchTarget, htmlCapturedAt); !errors.Is(err, ErrProductDataMissing) {
			t.Fatal("inert structured content accepted")
		}
	}
	if _, err := NormalizeSearchHTML(append(data, `<script>throw new Error("Access Denied CAPTCHA")</script>`...), htmlSearchTarget, htmlCapturedAt); err != nil {
		t.Fatal("script text was treated as rendered denial")
	}
}

func TestNormalizeSearchHTMLPriceMissingAndDerived(t *testing.T) {
	for _, price := range []any{nil, true, -1, 1.5, "", "bad", "1,00", "7,900원"} {
		product := htmlSearchProduct()
		product["offers"].(map[string]any)["price"] = price
		delete(product["offers"].(map[string]any), "priceCurrency")
		doc, err := NormalizeSearchHTML(htmlSearchDocument(htmlSearchList(product)), htmlSearchTarget, htmlCapturedAt)
		if err != nil {
			t.Fatal(err)
		}
		items, _, err := ParseSearchDocument(doc)
		if err != nil || len(items) != 1 {
			t.Fatal("missing price removed a valid named candidate")
		}
		evidence, present := items[0].EvidenceFor("price.current_amount")
		if price == "7,900원" {
			if !present || evidence.Provenance != "derived" || items[0].Price.CurrentAmount != 7900 {
				t.Fatal("parsed numeric evidence lost")
			}
		} else if present {
			t.Fatal("invalid price became observed zero")
		}
		if items[0].Price.Currency != "" {
			t.Fatal("currency was inferred")
		}
	}
}

func TestNormalizeSearchHTMLIdentityAndSizeGuards(t *testing.T) {
	data := htmlSearchDocument(htmlSearchList(htmlSearchProduct()))
	for _, target := range []string{htmlSearchTarget + "&q=other", htmlSearchTarget + "&page=01", htmlSearchTarget + "&sorter=", htmlSearchTarget + "#fragment", strings.ReplaceAll(htmlSearchTarget, "www.coupang.com", "user:pass@www.coupang.com"), strings.ReplaceAll(htmlSearchTarget, "www.coupang.com", "www.coupang.com:444")} {
		if _, err := NormalizeSearchHTML(data, target, htmlCapturedAt); !errors.Is(err, ErrProductDataMissing) {
			t.Fatal("unsafe or ambiguous search identity accepted")
		}
	}
	for _, document := range [][]byte{nil, []byte("<html>loading</html>"), make([]byte, maxProductDocumentBytes+1), {0xff}} {
		if _, err := NormalizeSearchHTML(document, htmlSearchTarget, htmlCapturedAt); !errors.Is(err, ErrProductDataMissing) {
			t.Fatal("missing/oversized/invalid document accepted")
		}
	}
	if _, err := NormalizeSearchHTML(data, htmlSearchTarget, time.Time{}); !errors.Is(err, ErrProductDataMissing) {
		t.Fatal("missing capture time accepted")
	}
}

// Exercise the real shared JavaScript reader against the same synthetic JSON-LD
// and compare its normalized core contract. Node is a test tool, not a runtime
// dependency of the new static HTML adapter.
func TestSearchHTMLMatchesSharedReaderStructuredEvidence(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for cross-reader contract verification")
	}
	const script = `
const fs=require('node:fs'), vm=require('node:vm');
const input=JSON.parse(fs.readFileSync(0,'utf8'));
const source=fs.readFileSync('../../browser/search_page_reader.js','utf8').replace('export function','function');
const document={title:'Synthetic',body:{innerText:''},readyState:'complete',querySelector:()=>null,querySelectorAll:s=>s.includes('ld+json')?input.scripts.map(x=>({textContent:JSON.stringify(x)})):[]};
const FixedDate=class extends Date { constructor(){super('2026-09-08T00:00:00Z')} };
const context=vm.createContext({URL,location:{href:input.target},document,performance:{getEntriesByType:()=>[]},Date:FixedDate});
vm.runInContext(source,context);
process.stdout.write(JSON.stringify(context.readSelectedSearchPage(input.target)));
`
	for _, name := range []string{"product-price", "option-price", "parsed-price", "missing-price", "multiple-offers", "wrong-offer-option", "duplicate-products", "owner-list", "unbound-list", "wrong-page", "exact-list-copy", "conflicting-lists", "explicit-empty"} {
		t.Run(name, func(t *testing.T) {
			product := htmlSearchProduct()
			list := htmlSearchList(product)
			scripts := []any{list}
			switch name {
			case "option-price":
				product["offers"].(map[string]any)["url"] = product["url"]
			case "parsed-price":
				product["offers"].(map[string]any)["price"] = "7,900원"
			case "missing-price":
				delete(product["offers"].(map[string]any), "price")
			case "multiple-offers":
				product["offers"] = []any{map[string]any{"price": 100}, map[string]any{"price": 200}}
			case "wrong-offer-option":
				product["offers"].(map[string]any)["url"] = "https://www.coupang.com/vp/products/101?itemId=999&vendorItemId=301"
			case "duplicate-products":
				list["itemListElement"] = []any{product, product}
			case "owner-list":
				delete(list, "url")
				scripts = []any{map[string]any{"@type": "CollectionPage", "url": htmlSearchTarget, "mainEntity": list}}
			case "unbound-list":
				delete(list, "url")
			case "wrong-page":
				list["url"] = htmlSearchTarget + "&page=2"
			case "exact-list-copy":
				scripts = append(scripts, list)
			case "conflicting-lists":
				other := htmlSearchProduct()
				other["name"] = "Other"
				scripts = append(scripts, htmlSearchList(other))
			case "explicit-empty":
				list["itemListElement"] = []any{}
				list["numberOfItems"] = 0
			}
			input, _ := json.Marshal(map[string]any{"target": htmlSearchTarget, "scripts": scripts})
			command := exec.Command(node, "-e", script)
			command.Stdin = strings.NewReader(string(input))
			output, err := command.Output()
			if err != nil {
				t.Fatal("synthetic shared reader failed")
			}
			var expected struct {
				Status string          `json:"status"`
				Search json.RawMessage `json:"search"`
			}
			if err := json.Unmarshal(output, &expected); err != nil {
				t.Fatal(err)
			}
			doc, err := NormalizeSearchHTML(htmlSearchDocument(scripts...), htmlSearchTarget, htmlCapturedAt)
			if expected.Status != "ok" {
				if !errors.Is(err, ErrProductDataMissing) {
					t.Fatal("Go accepted a list rejected by the shared reader")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want, wantCoverage, err := ParseSearchDocument(expected.Search)
			if err != nil {
				t.Fatal("shared reader emitted invalid canonical data")
			}
			got, gotCoverage, err := ParseSearchDocument(doc)
			if err != nil || !reflect.DeepEqual(got, want) || gotCoverage.SourceNoResults != wantCoverage.SourceNoResults {
				t.Fatal("static HTML adapter and shared reader evidence differ")
			}
		})
	}
}
