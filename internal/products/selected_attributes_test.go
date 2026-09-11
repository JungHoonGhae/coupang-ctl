package products

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestInspectPreservesNativeSelectedAttributesSeparatelyFromSpecHeuristics(t *testing.T) {
	ref := core.ProductReference{ProductID: "101", ItemID: "201", VendorItemID: "301"}
	attributes := []core.ProductSelectedAttribute{{Name: "RAM용량 × 저장용량", Value: "32GB × 1TB"}}
	inspection := core.ProductInspection{
		Product:            core.ProductCard{Reference: ref, Name: "Synthetic RAM 64GB SSD 512GB"},
		SelectedAttributes: attributes,
		FieldEvidence:      []core.ProductFieldEvidence{{Field: "selected_attributes", Source: "product_options", Locator: "options.optionRows.selectedAttribute", Method: "native_field", Provenance: "observed", Scope: "selected_option", Reference: ref, CapturedAt: time.Now()}},
		Coverage:           core.ProductCoverage{ObservedFields: []string{"selected_attributes"}},
	}
	s := New(syntheticSource{inspection: inspection})
	r, err := s.Inspect(context.Background(), core.ProductInspectRequest{ProductID: ref.ProductID, ItemID: ref.ItemID, VendorItemID: ref.VendorItemID, DisableAffiliate: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.SelectedAttributes, attributes) {
		t.Fatal("native tuple changed")
	}
	if _, ok := r.SelectedAttributesEvidence(); !ok {
		t.Fatal("source provenance lost")
	}
	specs := r.Product.ComputerSpecs
	if specs == nil || specs.Provenance != "inferred" || specs.Method != "capacity_and_model_regex" || specs.Source != "product_title_heuristic" {
		t.Fatal("title parse presented as a verified specification")
	}
	inspection.FieldEvidence[0].Reference.VendorItemID = "999"
	_, err = New(syntheticSource{inspection: inspection}).Inspect(context.Background(), core.ProductInspectRequest{ProductID: ref.ProductID})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatal("wrong option evidence crossed service seam")
	}
}

func TestSearchMemoryDiscoveryFilterExplainsItsHeuristic(t *testing.T) {
	s := New(syntheticSource{items: []core.ProductCard{{Reference: core.ProductReference{ProductID: "101"}, Name: "Synthetic RAM 32GB SSD 512GB"}}})
	r, err := s.Search(context.Background(), core.ProductSearchRequest{Query: "synthetic", MinMemoryGB: 32, DisableAffiliate: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 1 || r.Items[0].ComputerSpecs.Provenance != "inferred" {
		t.Fatal("search heuristic provenance lost")
	}
	if !strings.Contains(strings.Join(r.Warnings, " "), "title heuristics") {
		t.Fatal("discovery filter implies verified capacity")
	}
}
