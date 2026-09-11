package products

import (
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func selectedAttributeDocument(t *testing.T) map[string]any {
	t.Helper()
	e := syntheticFieldEvidence()
	e.Field, e.Source, e.Locator = "selected_attributes", "product_options", "options.optionRows.selectedAttribute"
	return map[string]any{
		"product":             syntheticEvidenceCard("201", "301"),
		"selected_attributes": []core.ProductSelectedAttribute{{Name: "RAM용량 × 저장용량", Value: "32GB × 1TB"}},
		"field_evidence":      []core.ProductFieldEvidence{e},
		"coverage":            core.ProductCoverage{ObservedFields: []string{"selected_attributes"}, UnavailableFields: []string{"selected_attributes"}},
	}
}

func TestSelectedAttributesParserPreservesSourceRowsAndBinding(t *testing.T) {
	d := selectedAttributeDocument(t)
	r, err := ParseInspectionDocument(evidenceDocument(t, d), core.ProductInspectRequest{ProductID: "101", ItemID: "201", VendorItemID: "301"})
	if err != nil {
		t.Fatal(err)
	}
	e, ok := r.SelectedAttributesEvidence()
	if !ok || e.Reference != r.Product.Reference || !reflect.DeepEqual(r.SelectedAttributes, d["selected_attributes"]) {
		t.Fatal("native tuple or exact option binding lost")
	}
	if slices.Contains(r.Coverage.UnavailableFields, "selected_attributes") {
		t.Fatal("contradictory missing field retained")
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip core.ProductInspection
	if json.Unmarshal(encoded, &roundtrip) != nil {
		t.Fatal("invalid response")
	}
	if _, ok := roundtrip.SelectedAttributesEvidence(); !ok {
		t.Fatal("response round trip lost evidence")
	}
}

func TestSelectedAttributesParserRejectsInventedOrAmbiguousFacts(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"missing evidence": func(d map[string]any) { delete(d, "field_evidence") },
		"missing coverage": func(d map[string]any) { delete(d, "coverage") },
		"missing value":    func(d map[string]any) { delete(d, "selected_attributes") },
		"wrong option": func(d map[string]any) {
			d["field_evidence"].([]core.ProductFieldEvidence)[0].Reference.VendorItemID = "999"
		},
		"product scope": func(d map[string]any) {
			e := &d["field_evidence"].([]core.ProductFieldEvidence)[0]
			e.Scope = "product"
			e.Reference = core.ProductReference{ProductID: "101"}
		},
		"inference": func(d map[string]any) {
			e := &d["field_evidence"].([]core.ProductFieldEvidence)[0]
			e.Provenance = "inferred"
			e.Method = "alias_lookup"
		},
		"empty label": func(d map[string]any) { d["selected_attributes"].([]core.ProductSelectedAttribute)[0].Name = " " },
		"control": func(d map[string]any) {
			d["selected_attributes"].([]core.ProductSelectedAttribute)[0].Value = "32GB\n1TB"
		},
		"overlong": func(d map[string]any) {
			d["selected_attributes"].([]core.ProductSelectedAttribute)[0].Value = strings.Repeat("a", 301)
		},
		"duplicate": func(d map[string]any) {
			a := d["selected_attributes"].([]core.ProductSelectedAttribute)
			d["selected_attributes"] = append(a, a[0])
		},
		"wrong locator": func(d map[string]any) {
			d["field_evidence"].([]core.ProductFieldEvidence)[0].Locator = "options.unverified"
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := selectedAttributeDocument(t)
			mutate(d)
			_, err := ParseInspectionDocument(evidenceDocument(t, d), core.ProductInspectRequest{ProductID: "101"})
			if !errors.Is(err, ErrProductDataMissing) {
				t.Fatal("invalid selected attribute accepted")
			}
		})
	}
	r, err := ParseInspectionDocument(evidenceDocument(t, map[string]any{"product": syntheticEvidenceCard("201", "301"), "coverage": map[string]any{"observed_fields": []string{"selected_attributes"}}}), core.ProductInspectRequest{ProductID: "101"})
	if err != nil || slices.Contains(r.Coverage.ObservedFields, "selected_attributes") || !slices.Contains(r.Coverage.UnavailableFields, "selected_attributes") {
		t.Fatal("absent attributes did not remain unknown")
	}
}

func TestNativeSelectedAttributesSharedReaderToParser(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for shared reader")
	}
	const script = `
const fs=require('node:fs'),vm=require('node:vm');
const href='https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301';
const a={valueId:'1',name:'32GB × 1TB',selected:true};
const options={optionRows:[{name:'RAM용량 × 저장용량',attributes:[a],selectedAttribute:a}],attributeVendorItemMap:{'1':{itemId:201,vendorItemId:301}}};
const product={'@type':'Product',url:href,name:'Synthetic',offers:{url:href,price:1200,priceCurrency:'KRW'}};
const document={scripts:[{textContent:JSON.stringify({options})}],body:{innerText:''},querySelector:()=>null,querySelectorAll:s=>s.includes('ld+json')?[{textContent:JSON.stringify(product)}]:[]};
const context=vm.createContext({document,location:new URL(href),URL,setTimeout:f=>f(),fetch:()=>{throw Error('read budget exceeded')}});
vm.runInContext('('+fs.readFileSync('../../browser/product_inspection_reader.js','utf8')+')',context)({product_id:'101',item_id:'201',vendor_item_id:'301',document_read_limit:1}).then(v=>process.stdout.write(v)).catch(()=>process.exit(1));
`
	d, err := exec.Command(node, "-e", script).Output()
	if err != nil {
		t.Fatal("synthetic reader failed", err)
	}
	r, err := ParseInspectionDocument(d, core.ProductInspectRequest{ProductID: "101", ItemID: "201", VendorItemID: "301"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.SelectedAttributesEvidence(); !ok || len(r.SelectedAttributes) != 1 || r.SelectedAttributes[0].Value != "32GB × 1TB" {
		t.Fatal("shared source contract lost")
	}
}
