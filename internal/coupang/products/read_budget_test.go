package products

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestInspectionPreservesBudgetOmissionAsUnavailableEvidence(t *testing.T) {
	for _, fields := range [][]string{{"reviews"}, {"quantity_info", "reviews"}} {
		payload := map[string]any{"product": map[string]any{"product_id": "101", "name": "Synthetic", "url": "https://www.coupang.com/vp/products/101"}, "coverage": map[string]any{"budget_omitted_fields": fields}}
		doc, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		result, err := ParseInspectionDocument(doc, core.ProductInspectRequest{ProductID: "101", DocumentReadLimit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(result.Coverage.BudgetOmittedFields, fields) {
			t.Fatal("budget reason lost")
		}
		for _, f := range fields {
			if !slices.Contains(result.Coverage.UnavailableFields, f) {
				t.Fatal("budget omission became known absence")
			}
		}
	}
}

func TestInspectionRejectsUnrecognizedBudgetOmission(t *testing.T) {
	doc := []byte(`{"product":{"product_id":"101","name":"Synthetic","url":"https://www.coupang.com/vp/products/101"},"coverage":{"budget_omitted_fields":["anything"]}}`)
	if _, err := ParseInspectionDocument(doc, core.ProductInspectRequest{ProductID: "101"}); err == nil {
		t.Fatal("unbounded budget fields accepted")
	}
}

func TestInspectionRejectsOverlongBudgetOmissionBeforeTruncating(t *testing.T) {
	doc := []byte(`{"product":{"product_id":"101","name":"Synthetic","url":"https://www.coupang.com/vp/products/101"},"coverage":{"budget_omitted_fields":["quantity_info","reviews","anything"]}}`)
	if _, err := ParseInspectionDocument(doc, core.ProductInspectRequest{ProductID: "101"}); err == nil {
		t.Fatal("unrecognized tail disappeared through truncation")
	}
}
