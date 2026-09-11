package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type selectedAttributeWorkflow struct{ fixedProductWorkflow }

func (selectedAttributeWorkflow) Inspect(_ context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	ref := core.ProductReference{ProductID: r.ProductID, ItemID: r.ItemID, VendorItemID: r.VendorItemID}
	return core.ProductInspection{SchemaVersion: core.ProductSchemaVersion, Product: core.ProductCard{Reference: ref, Name: "Synthetic"},
		SelectedAttributes: []core.ProductSelectedAttribute{{Name: "RAM용량 × 저장용량", Value: "32GB × 1TB"}},
		FieldEvidence:      []core.ProductFieldEvidence{{Field: "selected_attributes", Source: "product_options", Locator: "options.optionRows.selectedAttribute", Scope: "selected_option", Reference: ref, Provenance: "observed", Method: "native_field", CapturedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}},
		Coverage:           core.ProductCoverage{ObservedFields: []string{"selected_attributes"}},
	}, nil
}

func TestCLISelectedAttributesRetainExactOptionEvidence(t *testing.T) {
	var out bytes.Buffer
	err := runProducts(context.Background(), []string{"inspect", "--product-id", "101", "--item-id", "201", "--vendor-item-id", "301", "--no-affiliate"}, &out, selectedAttributeWorkflow{})
	if err != nil {
		t.Fatal(err)
	}
	var got core.ProductInspection
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if e, ok := got.SelectedAttributesEvidence(); !ok || e.Reference.VendorItemID != "301" {
		t.Fatal("exact option evidence lost")
	}
	if len(got.SelectedAttributes) != 1 || got.SelectedAttributes[0].Value != "32GB × 1TB" {
		t.Fatal("compound option changed")
	}
}
