package core

import "testing"

func comparisonTestInspection(id string, amount int64, rating float64) ProductInspection {
	p := conditionTestInspection("price.current_amount")
	p.Product.Reference.ProductID = id
	p.Product.Price.CurrentAmount = amount
	p.Product.Rating = rating
	p.Product.FieldEvidence[0].Reference = p.Product.Reference
	e := p.Product.FieldEvidence[0]
	e.Field = "rating"
	e.Locator = "synthetic.rating"
	e.Scope = "product_page"
	e.Reference = ProductReference{ProductID: id}
	p.Product.FieldEvidence = append(p.Product.FieldEvidence, e)
	p.Product.ObservedFields = append(p.Product.ObservedFields, "rating")
	return p
}

func TestProductPreferenceComparisonPreservesTradeoffsAndUnknowns(t *testing.T) {
	axes := []ProductPreferenceAxis{{Field: "price.current_amount", Direction: "minimize"}, {Field: "rating", Direction: "maximize"}}
	left := comparisonTestInspection("111", 100, 4)
	for _, tc := range []struct {
		price    int64
		rating   float64
		relation string
	}{
		{200, 5, "tradeoff"}, {200, 4, "dominates_on_axes"}, {50, 5, "dominated_on_axes"}, {100, 4, "equal_on_axes"},
	} {
		right := comparisonTestInspection("222", tc.price, tc.rating)
		r := CompareInspectedProducts(left, right, axes)
		if r.Relation != tc.relation || r.Other != right.Product.Reference {
			t.Fatalf("relation=%s want=%s", r.Relation, tc.relation)
		}
		if tc.relation == "tradeoff" && (len(r.BetterOn) != 1 || r.BetterOn[0] != "price.current_amount" || len(r.WorseOn) != 1 || r.WorseOn[0] != "rating") {
			t.Fatal("tradeoff explanation lost")
		}
	}
	right := comparisonTestInspection("222", 200, 3)
	right.Product.FieldEvidence = right.Product.FieldEvidence[:1]
	if r := CompareInspectedProducts(left, right, axes); r.Relation != "unknown" || len(r.UnknownOn) != 1 || r.UnknownOn[0] != "rating" {
		t.Fatal("unknown was discarded using known cheaper price")
	}
	right = comparisonTestInspection("222", 200, 3)
	right.Product.Reference = ProductReference{ProductID: "222"}
	right.Product.FieldEvidence[0].Scope = "product"
	right.Product.FieldEvidence[0].Reference = right.Product.Reference
	if CompareInspectedProducts(left, right, axes).Relation != "unknown" {
		t.Fatal("different price scopes compared as equivalent")
	}
	if CompareInspectedProducts(left, right, nil).Relation != "not_requested" {
		t.Fatal("invented default preferences")
	}
}

func TestProductPreferenceComparisonIntegerPrecisionAndBooleanDirection(t *testing.T) {
	left, right := comparisonTestInspection("111", 9007199254740992, 4), comparisonTestInspection("222", 9007199254740993, 4)
	if CompareInspectedProducts(left, right, []ProductPreferenceAxis{{Field: "price.current_amount", Direction: "minimize"}}).Relation != "dominates_on_axes" {
		t.Fatal("integer comparison lost precision")
	}
	left, right = conditionTestInspection("rocket"), conditionTestInspection("rocket")
	right.Product.Rocket = true
	if CompareInspectedProducts(left, right, []ProductPreferenceAxis{{Field: "rocket", Direction: "prefer_false"}}).Relation != "dominates_on_axes" {
		t.Fatal("explicit false preference lost")
	}
	right.Product.ObservedFields = nil
	if CompareInspectedProducts(left, right, []ProductPreferenceAxis{{Field: "rocket", Direction: "prefer_false"}}).Relation != "unknown" {
		t.Fatal("missing boolean became false")
	}
}

func TestProductPreferenceAxesRejectUnsupportedOrDuplicateInputs(t *testing.T) {
	for _, axes := range [][]ProductPreferenceAxis{
		{{Field: "rating", Direction: "prefer_true"}}, {{Field: "unknown", Direction: "maximize"}},
		{{Field: "rating", Direction: "maximize"}, {Field: "rating", Direction: "minimize"}},
	} {
		if (ProductRecommendationRequest{Query: "synthetic", ComparisonAxes: axes}).Validate() == nil {
			t.Fatal("ambiguous axes accepted")
		}
	}
}
