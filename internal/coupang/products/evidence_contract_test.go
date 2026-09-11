package products

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func syntheticEvidenceCard(item, vendor string) map[string]any {
	return map[string]any{"product_id": "101", "item_id": item, "vendor_item_id": vendor,
		"name": "Synthetic option", "url": "https://www.coupang.com/vp/products/101"}
}

func evidenceDocument(t *testing.T, value any) []byte {
	t.Helper()
	document, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func TestInspectionRequiresRequestedOptionEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, item, vendor string
		request            core.ProductInspectRequest
		wantError          bool
	}{
		{"matching exact option", "201", "301", core.ProductInspectRequest{ProductID: "101", ItemID: "201", VendorItemID: "301"}, false},
		{"wrong item", "202", "301", core.ProductInspectRequest{ProductID: "101", ItemID: "201", VendorItemID: "301"}, true},
		{"wrong vendor", "201", "302", core.ProductInspectRequest{ProductID: "101", VendorItemID: "301"}, true},
		{"missing item", "", "301", core.ProductInspectRequest{ProductID: "101", ItemID: "201"}, true},
		{"missing vendor", "201", "", core.ProductInspectRequest{ProductID: "101", VendorItemID: "301"}, true},
		{"product-only request", "201", "301", core.ProductInspectRequest{ProductID: "101"}, false},
		{"item-only request", "201", "301", core.ProductInspectRequest{ProductID: "101", ItemID: "201"}, false},
		{"wrong product", "201", "301", core.ProductInspectRequest{ProductID: "999"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := evidenceDocument(t, map[string]any{"product": syntheticEvidenceCard(tc.item, tc.vendor)})
			result, err := ParseInspectionDocument(doc, tc.request)
			if tc.wantError {
				if !errors.Is(err, ErrProductDataMissing) || result.Product.Reference.ProductID != "" {
					t.Fatalf("unverified identity accepted: result=%#v error=%v", result.Product.Reference, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSearchRejectsContradictoryNoResults(t *testing.T) {
	for name, item := range map[string]map[string]any{"valid card": syntheticEvidenceCard("201", "301"), "invalid card": {"name": "Synthetic invalid card"}} {
		t.Run(name, func(t *testing.T) {
			doc := evidenceDocument(t, map[string]any{"no_results": true, "items": []any{item}})
			items, _, err := ParseSearchDocument(doc)
			if !errors.Is(err, ErrProductDataMissing) || len(items) != 0 {
				t.Fatalf("contradictory no_results accepted: items=%d error=%v", len(items), err)
			}
		})
	}
}

func TestCardAcceptsMatchingOrOmittedOptionURL(t *testing.T) {
	for _, suffix := range []string{"", "?itemId=201", "?vendorItemId=301", "?itemId=201&vendorItemId=301", "?itemId=%32%30%31&source=synthetic"} {
		t.Run(suffix, func(t *testing.T) {
			card := syntheticEvidenceCard("201", "301")
			card["url"] = "https://www.coupang.com/vp/products/101" + suffix
			_, err := ParseInspectionDocument(evidenceDocument(t, map[string]any{"product": card}), core.ProductInspectRequest{ProductID: "101", ItemID: "201", VendorItemID: "301"})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSearchPreservesDistinctItemOptionsWithoutVendorID(t *testing.T) {
	doc := evidenceDocument(t, map[string]any{"items": []any{
		syntheticEvidenceCard("201", ""), syntheticEvidenceCard("202", ""), syntheticEvidenceCard("201", ""),
	}})
	items, _, err := ParseSearchDocument(doc)
	if err != nil || len(items) != 2 || items[0].Reference.ItemID != "201" || items[1].Reference.ItemID != "202" {
		t.Fatalf("distinct options were collapsed: items=%#v error=%v", items, err)
	}
}

func TestCardRejectsContradictoryOptionURL(t *testing.T) {
	for _, suffix := range []string{"?itemId=202", "?vendorItemId=302", "?itemId=201&itemId=202", "?itemId=", "?vendorItemId=%ZZ"} {
		t.Run(suffix, func(t *testing.T) {
			card := syntheticEvidenceCard("201", "301")
			card["url"] = "https://www.coupang.com/vp/products/101" + suffix
			_, err := ParseInspectionDocument(evidenceDocument(t, map[string]any{"product": card}), core.ProductInspectRequest{ProductID: "101"})
			if !errors.Is(err, ErrProductDataMissing) {
				t.Fatalf("contradictory option URL accepted: %v", err)
			}
		})
	}
}
