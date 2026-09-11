package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func capacityInspection(attributes ...ProductSelectedAttribute) ProductInspection {
	ref := ProductReference{ProductID: "101", ItemID: "201", VendorItemID: "301"}
	return ProductInspection{Product: ProductCard{Reference: ref}, SelectedAttributes: attributes,
		Coverage:      ProductCoverage{ObservedFields: []string{"selected_attributes"}},
		FieldEvidence: []ProductFieldEvidence{{Field: "selected_attributes", Source: "product_options", Locator: "options.optionRows.selectedAttribute", Provenance: "observed", Method: "native_field", Scope: "selected_option", Reference: ref, CapturedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}},
	}
}

func TestCapacityConditionsUseStandaloneNativeOptionValues(t *testing.T) {
	p := capacityInspection(ProductSelectedAttribute{Name: "RAM용량", Value: "32GB"}, ProductSelectedAttribute{Name: "저장용량", Value: "1TB"})
	for _, tc := range []struct {
		field     string
		threshold int64
		want      ProductConditionStatus
	}{
		{"specifications.memory_gb", 32, ProductConditionMet},
		{"specifications.memory_gb", 64, ProductConditionUnmet},
		{"specifications.storage_gb", 1000, ProductConditionMet},
		{"specifications.storage_gb", 1024, ProductConditionUnmet},
	} {
		c := ProductRecommendationCondition{ID: "capacity", Field: tc.field, Operator: "gte", Integer: &tc.threshold}
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		got := AssessProductCondition(p, c)
		if got.Status != tc.want || len(got.Evidence) != 1 || got.ObservedInteger != nil {
			t.Fatalf("capacity evidence/status lost: %+v", got)
		}
		if got.DerivedInteger == nil || got.Derivation == nil || got.Derivation.Provenance != "derived" || got.Derivation.Unit != "GB" || got.Derivation.Method != "standalone_capacity_decimal_gb" {
			t.Fatal("numeric parsing disguised as an observed value")
		}
	}
	for _, tc := range []struct {
		limit int64
		want  ProductConditionStatus
	}{{32, ProductConditionMet}, {31, ProductConditionUnmet}} {
		if r := AssessProductCondition(p, ProductRecommendationCondition{ID: "ram_max", Field: "specifications.memory_gb", Operator: "lte", Integer: &tc.limit}); r.Status != tc.want {
			t.Fatal("capacity upper bound not assessed")
		}
	}
}

func TestCapacityValuesPreserveSourceAndDecimalArithmeticOnWire(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int64
	}{
		{"32GB", 32}, {" 32 GB ", 32}, {"1TB", 1000}, {"0.512TB", 512}, {"1.5TB", 1500}, {"0GB", 0}, {"32.000GB", 32},
	} {
		i := capacityInspection(ProductSelectedAttribute{Name: "저장용량", Value: tc.raw})
		v := ReadProductComparableValue(i, "specifications.storage_gb")
		if len(v.MissingEvidence) != 0 || v.DerivedInteger == nil || *v.DerivedInteger != tc.want || v.ObservedInteger != nil || v.Derivation.Input.Value != tc.raw || v.Scope != "selected_option" {
			t.Fatalf("%q: %+v", tc.raw, v)
		}
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var decoded ProductComparableValue
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.DerivedInteger == nil || *decoded.DerivedInteger != tc.want || strings.Contains(string(data), "observed_integer") {
			t.Fatal("derived value lost/promoted on wire")
		}
	}
}

func TestCapacityConditionsRejectAmbiguousEvidenceWithoutTitleFallback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ProductInspection)
	}{
		{"compound", func(p *ProductInspection) {
			p.SelectedAttributes = []ProductSelectedAttribute{{Name: "RAM용량 × 저장용량", Value: "32GB × 1TB"}}
		}},
		{"compound alongside standalone", func(p *ProductInspection) {
			p.SelectedAttributes = append(p.SelectedAttributes, ProductSelectedAttribute{Name: "저장용량 × RAM용량", Value: "1TB × 16GB"})
		}},
		{"absent", func(p *ProductInspection) { p.SelectedAttributes = nil }},
		{"alias", func(p *ProductInspection) { p.SelectedAttributes[0].Name = "메모리" }},
		{"unitless", func(p *ProductInspection) { p.SelectedAttributes[0].Value = "32" }},
		{"binary unit", func(p *ProductInspection) { p.SelectedAttributes[0].Value = "32GiB" }},
		{"bit unit", func(p *ProductInspection) { p.SelectedAttributes[0].Value = "32Gb" }},
		{"sum", func(p *ProductInspection) { p.SelectedAttributes[0].Value = "16GB + 16GB" }},
		{"fractional GB", func(p *ProductInspection) { p.SelectedAttributes[0].Value = "0.5GB" }},
		{"overflow", func(p *ProductInspection) { p.SelectedAttributes[0].Value = "999999999999999999999999TB" }},
		{"inferred", func(p *ProductInspection) { p.FieldEvidence[0].Provenance = "inferred" }},
		{"wrong option", func(p *ProductInspection) { p.FieldEvidence[0].Reference.VendorItemID = "999" }},
		{"missing evidence", func(p *ProductInspection) { p.FieldEvidence = nil }},
		{"unobserved", func(p *ProductInspection) { p.Coverage.ObservedFields = nil }},
		{"unavailable", func(p *ProductInspection) { p.Coverage.UnavailableFields = []string{"selected_attributes"} }},
		{"duplicate", func(p *ProductInspection) {
			p.SelectedAttributes = append(p.SelectedAttributes, p.SelectedAttributes[0])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := capacityInspection(ProductSelectedAttribute{Name: "RAM용량", Value: "32GB"})
			p.Product.Name = "Synthetic RAM 64GB SSD 2TB"
			p.Product.ComputerSpecs = &ComputerSpecifications{MemoryGB: 64, StorageGB: 2048, Provenance: "inferred"}
			tc.mutate(&p)
			r := AssessProductCondition(p, ProductRecommendationCondition{ID: "ram", Field: "specifications.memory_gb", Operator: "gte", Integer: conditionTestPointer(int64(32))})
			if r.Status != ProductConditionUnknown || r.DerivedInteger != nil || r.Derivation != nil || r.ObservedInteger != nil || len(r.MissingEvidence) == 0 {
				t.Fatalf("unproven capacity promoted: %+v", r)
			}
		})
	}
}

func TestCapacityComparisonAndConclusionShareConditionEvidence(t *testing.T) {
	left := capacityInspection(ProductSelectedAttribute{Name: "RAM용량", Value: "32GB"}, ProductSelectedAttribute{Name: "저장용량", Value: "512GB"})
	right := capacityInspection(ProductSelectedAttribute{Name: "RAM용량", Value: "16GB"}, ProductSelectedAttribute{Name: "저장용량", Value: "1TB"})
	axes := []ProductPreferenceAxis{{Field: "specifications.memory_gb", Direction: "maximize"}, {Field: "specifications.storage_gb", Direction: "maximize"}}
	if r := CompareInspectedProducts(left, right, axes); r.Relation != "tradeoff" || len(r.BetterOn) != 1 || r.BetterOn[0] != "specifications.memory_gb" || len(r.WorseOn) != 1 {
		t.Fatalf("wrong comparison: %+v", r)
	}
	right.SelectedAttributes[0].Name = "RAM용량 × 색상"
	if r := CompareInspectedProducts(left, right, axes); r.Relation != "unknown" {
		t.Fatal("compound compared as numeric")
	}
	conditions := []ProductRecommendationCondition{{ID: "ram", Field: "specifications.memory_gb", Operator: "gte", Integer: conditionTestPointer(int64(32))}}
	if ConcludeProductConditions(left, conditions).Outcome != "conditions_met" || ConcludeProductConditions(right, conditions).Outcome != "conditions_unverified" {
		t.Fatal("conclusion disagrees with native capacity evidence")
	}
	for _, c := range []ProductRecommendationCondition{
		{ID: "ram", Field: "specifications.memory_gb", Operator: "eq", Integer: conditionTestPointer(int64(32))},
		{ID: "ram", Field: "specifications.memory_gb", Operator: "gte", Integer: conditionTestPointer(int64(-1))},
		{ID: "ram", Field: "specifications.memory_gb", Operator: "gte", Number: conditionTestPointer(32.0)},
		{ID: "ram", Field: "specifications.storage_gb", Operator: "gte", Integer: conditionTestPointer(int64(32)), Boolean: conditionTestPointer(true)},
	} {
		if c.Validate() == nil {
			t.Fatal("invalid capacity condition accepted")
		}
	}
	if (ProductPreferenceAxis{Field: "specifications.memory_gb", Direction: "prefer_true"}).Validate() == nil {
		t.Fatal("numeric axis accepted boolean direction")
	}
}
