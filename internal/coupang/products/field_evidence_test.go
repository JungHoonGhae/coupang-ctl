package products

import (
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestSharedJavaScriptReaderToGoEvidenceContract(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required to execute the actual shared reader")
	}
	const script = `
const fs = require('node:fs'), vm = require('node:vm');
const source = fs.readFileSync('../../browser/product_inspection_reader.js','utf8');
const product = {'@type':'Product',name:'Synthetic bowl',offers:{price:0},aggregateRating:{ratingValue:'4.5',reviewCount:0}};
const document = {body:{innerText:''},querySelector:()=>null,querySelectorAll:selector=>selector.includes('ld+json')?[{textContent:JSON.stringify(product)}]:[]};
const context = vm.createContext({document,location:new URL('https://www.coupang.com/vp/products/101'),URL,setTimeout:fn=>fn(),fetch:async()=>({ok:false})});
vm.runInContext('('+source+')',context)({product_id:'101'}).then(value=>process.stdout.write(value)).catch(()=>process.exit(1));
`
	document, err := exec.Command(node, "-e", script).Output()
	if err != nil {
		t.Fatal("synthetic shared reader execution failed:", err)
	}
	result, err := ParseInspectionDocument(document, core.ProductInspectRequest{ProductID: "101"})
	if err != nil {
		t.Fatal(err)
	}
	for field, provenance := range map[string]string{"price.current_amount": "observed", "rating": "derived", "review_count": "observed", "name": "observed"} {
		evidence, ok := result.Product.EvidenceFor(field)
		if !ok || evidence.Provenance != provenance || evidence.Source != "json_ld" {
			t.Fatalf("reader/parser contract lost %s: %#v", field, evidence)
		}
	}
	if result.Product.Price.CurrentAmount != 0 || result.Product.Rating != 4.5 {
		t.Fatal("synthetic source values changed")
	}
}

func syntheticFieldEvidence() core.ProductFieldEvidence {
	return core.ProductFieldEvidence{Field: "price.current_amount", Provenance: "observed", Source: "json_ld", Locator: "jsonld.Product.offers.price", Method: "native_field", Scope: "selected_option", Reference: core.ProductReference{ProductID: "101", ItemID: "201", VendorItemID: "301"}, CapturedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)}
}

func TestParserDoesNotInventCurrency(t *testing.T) {
	for _, currency := range []string{"", "KRW", "USD", "private text"} {
		card := syntheticEvidenceCard("201", "301")
		card["current_amount"], card["currency"], card["observed_fields"] = 1200, currency, []string{"price.current_amount"}
		result, err := ParseInspectionDocument(evidenceDocument(t, map[string]any{"product": card}), core.ProductInspectRequest{ProductID: "101"})
		if err != nil {
			t.Fatal(err)
		}
		want := currency
		if currency == "private text" {
			want = ""
		}
		if result.Product.Price.Currency != want {
			t.Fatalf("currency=%q want=%q", result.Product.Price.Currency, want)
		}
	}
}

func TestInspectionFieldEvidenceRoundTrip(t *testing.T) {
	for _, kind := range []struct{ source, method, provenance string }{{"json_ld", "native_field", "observed"}, {"dom", "numeric_parse", "derived"}, {"quantity_info", "alias_lookup", "inferred"}} {
		t.Run(kind.provenance, func(t *testing.T) {
			e := syntheticFieldEvidence()
			e.Source, e.Method, e.Provenance = kind.source, kind.method, kind.provenance
			card := syntheticEvidenceCard("201", "301")
			card["current_amount"], card["observed_fields"], card["field_evidence"] = 0, []string{e.Field}, []core.ProductFieldEvidence{e}
			option := e
			option.Field, option.Source, option.Locator, option.Method, option.Provenance = "selected_options", "dom", "dom.option_picker.selected.text", "selected_option_text", "derived"
			payload := map[string]any{"product": card, "selected_options": []string{"Synthetic option"}, "field_evidence": []core.ProductFieldEvidence{option}, "coverage": map[string]any{"observed_fields": []string{"selected_options"}}}
			result, err := ParseInspectionDocument(evidenceDocument(t, payload), core.ProductInspectRequest{ProductID: "101", ItemID: "201", VendorItemID: "301"})
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var decoded core.ProductInspection
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			got, ok := decoded.Product.EvidenceFor(e.Field)
			if !ok || !reflect.DeepEqual(got, e) || decoded.Product.Price.CurrentAmount != 0 {
				t.Fatalf("product evidence changed: %#v", got)
			}
			got, ok = decoded.EvidenceFor(option.Field)
			if !ok || !reflect.DeepEqual(got, option) {
				t.Fatalf("option evidence changed: %#v", got)
			}
		})
	}
}

func TestInspectionRejectsInvalidFieldEvidence(t *testing.T) {
	for name, mutate := range map[string]func(*core.ProductFieldEvidence){
		"missing timestamp":            func(e *core.ProductFieldEvidence) { e.CapturedAt = time.Time{} },
		"wrong product":                func(e *core.ProductFieldEvidence) { e.Reference.ProductID = "999" },
		"wrong option":                 func(e *core.ProductFieldEvidence) { e.Reference.ItemID = "202" },
		"unavailable field":            func(e *core.ProductFieldEvidence) { e.Field = "rating" },
		"raw URL locator":              func(e *core.ProductFieldEvidence) { e.Locator = "https://example.invalid/private" },
		"DOM cannot be native":         func(e *core.ProductFieldEvidence) { e.Source = "dom" },
		"inference cannot be observed": func(e *core.ProductFieldEvidence) { e.Method = "alias_lookup" },
		"unsupported source":           func(e *core.ProductFieldEvidence) { e.Source = "unknown_api" },
		"unbound page scope":           func(e *core.ProductFieldEvidence) { e.Scope = "product_page" },
	} {
		t.Run(name, func(t *testing.T) {
			e := syntheticFieldEvidence()
			mutate(&e)
			card := syntheticEvidenceCard("201", "301")
			card["current_amount"], card["observed_fields"], card["field_evidence"] = 1200, []string{"price.current_amount"}, []core.ProductFieldEvidence{e}
			_, err := ParseInspectionDocument(evidenceDocument(t, map[string]any{"product": card}), core.ProductInspectRequest{ProductID: "101"})
			if !errors.Is(err, ErrProductDataMissing) {
				t.Fatalf("invalid evidence accepted: %v", err)
			}
		})
	}
	e := syntheticFieldEvidence()
	if validFieldEvidence([]core.ProductFieldEvidence{e, e}, e.Reference, []string{e.Field}) {
		t.Fatal("duplicate evidence accepted")
	}
	card := core.ProductCard{Reference: e.Reference, FieldEvidence: []core.ProductFieldEvidence{e}}
	if _, ok := card.EvidenceFor(e.Field); ok {
		t.Fatal("metadata promoted an unavailable value")
	}
	card.ObservedFields, card.FieldEvidence = []string{e.Field}, []core.ProductFieldEvidence{e, e}
	if _, ok := card.EvidenceFor(e.Field); ok {
		t.Fatal("ambiguous evidence accepted by core")
	}
}
