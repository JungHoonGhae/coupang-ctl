package core

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func conditionTestPointer[T any](v T) *T { return &v }

func conditionTestInspection(field string) ProductInspection {
	ref := ProductReference{ProductID: "123", ItemID: "456", VendorItemID: "789"}
	e := ProductFieldEvidence{Field: field, Provenance: "observed", Source: "json_ld", Locator: "synthetic.value", Method: "native_field", Scope: "selected_option", Reference: ref, CapturedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)}
	if field == "rating" {
		e.Scope = "product_page"
		e.Reference = ProductReference{ProductID: "123"}
	}
	return ProductInspection{Product: ProductCard{Reference: ref, Price: ProductPrice{CurrentAmount: 0, Currency: "KRW"}, ObservedFields: []string{field}, FieldEvidence: []ProductFieldEvidence{e}}}
}

func TestConditionAssessmentPreservesKnownZeroAndFalse(t *testing.T) {
	for _, condition := range []ProductRecommendationCondition{
		{ID: "price", Field: "price.current_amount", Operator: "lte", Integer: conditionTestPointer(int64(0))},
		{ID: "rating", Field: "rating", Operator: "gte", Number: conditionTestPointer(0.0)},
		{ID: "delivery", Field: "free_shipping", Operator: "eq", Boolean: conditionTestPointer(false)},
	} {
		p := conditionTestInspection(condition.Field)
		r := AssessProductCondition(p, condition)
		if r.Status != ProductConditionMet || len(r.Evidence) != 1 || len(r.MissingEvidence) != 0 {
			t.Fatalf("known scalar lost: %+v", r)
		}
		encoded, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		key := "observed_integer"
		if condition.Number != nil {
			key = "observed_number"
		}
		if condition.Boolean != nil {
			key = "observed_boolean"
		}
		if !strings.Contains(string(encoded), key) {
			t.Fatal("known zero/false disappeared on wire")
		}
		p.Product.ObservedFields = nil
		r = AssessProductCondition(p, condition)
		if r.Status != ProductConditionUnknown || r.ObservedInteger != nil || r.ObservedNumber != nil || r.ObservedBoolean != nil {
			t.Fatal("absence promoted to zero/false")
		}
	}
}

func TestConditionAssessmentRejectsWeakOrMismatchedEvidence(t *testing.T) {
	c := ProductRecommendationCondition{ID: "shipping", Field: "free_shipping", Operator: "eq", Boolean: conditionTestPointer(false)}
	for _, mutate := range []func(*ProductInspection){
		func(p *ProductInspection) { p.Product.FieldEvidence = nil },
		func(p *ProductInspection) {
			e := &p.Product.FieldEvidence[0]
			e.Method = "alias_lookup"
			e.Provenance = "inferred"
		},
		func(p *ProductInspection) {
			e := &p.Product.FieldEvidence[0]
			e.Scope = "product"
			e.Reference = ProductReference{ProductID: "123"}
		},
		func(p *ProductInspection) { p.Product.FieldEvidence[0].Reference.VendorItemID = "999" },
		func(p *ProductInspection) {
			p.Product.FieldEvidence = append(p.Product.FieldEvidence, p.Product.FieldEvidence[0])
		},
		func(p *ProductInspection) {
			p.Coverage.ObservedFields = []string{"delivery.free_shipping"}
			p.Delivery.FreeShipping = true
		},
	} {
		p := conditionTestInspection(c.Field)
		mutate(&p)
		r := AssessProductCondition(p, c)
		if r.Status != ProductConditionUnknown || r.ObservedBoolean != nil || len(r.MissingEvidence) == 0 {
			t.Fatal("weak/conflicting evidence passed condition")
		}
	}
}

func TestConditionAssessmentUsesDetailEvidenceWithoutInventingCardValues(t *testing.T) {
	c := ProductRecommendationCondition{ID: "delivery", Field: "rocket", Operator: "eq", Boolean: conditionTestPointer(true)}
	p := conditionTestInspection(c.Field)
	e := p.Product.FieldEvidence[0]
	e.Field = "delivery.rocket"
	p.Product.ObservedFields = nil
	p.Product.FieldEvidence = nil
	p.Delivery.Rocket = true
	p.FieldEvidence = []ProductFieldEvidence{e}
	p.Coverage.ObservedFields = []string{"delivery.rocket"}
	r := AssessProductCondition(p, c)
	if r.Status != ProductConditionMet || r.ObservedBoolean == nil || !*r.ObservedBoolean || len(r.Evidence) != 1 || r.Evidence[0].Field != "delivery.rocket" {
		t.Fatal("detail evidence lost")
	}
	c.Boolean = conditionTestPointer(false)
	if AssessProductCondition(p, c).Status != ProductConditionUnmet {
		t.Fatal("verified mismatch not unmet")
	}
}

func TestConditionAssessmentRatingDoesNotAcceptNonfiniteOrOptionScope(t *testing.T) {
	c := ProductRecommendationCondition{ID: "rating", Field: "rating", Operator: "gte", Number: conditionTestPointer(4.0)}
	for _, value := range []float64{math.NaN(), math.Inf(1), -1, 6} {
		p := conditionTestInspection(c.Field)
		p.Product.Rating = value
		r := AssessProductCondition(p, c)
		if r.Status != ProductConditionUnknown || r.ObservedNumber != nil {
			t.Fatal("invalid rating passed")
		}
	}
	p := conditionTestInspection(c.Field)
	p.Product.Rating = 4.5
	if AssessProductCondition(p, c).Status != ProductConditionMet {
		t.Fatal("known product-page rating failed")
	}
	p.Product.FieldEvidence[0].Scope = "selected_option"
	p.Product.FieldEvidence[0].Reference = p.Product.Reference
	if AssessProductCondition(p, c).Status != ProductConditionUnknown {
		t.Fatal("option rating promoted to page rating")
	}
}

func TestRecommendationConditionsValidateBeforeResearch(t *testing.T) {
	good := ProductRecommendationCondition{ID: "budget", Field: "price.current_amount", Operator: "lte", Integer: conditionTestPointer(int64(2000))}
	for _, mutate := range []func(*ProductRecommendationCondition){
		func(c *ProductRecommendationCondition) { c.ID = "not a safe id" },
		func(c *ProductRecommendationCondition) { c.Field = "allergen_free" },
		func(c *ProductRecommendationCondition) { c.Integer = nil },
		func(c *ProductRecommendationCondition) { c.Boolean = conditionTestPointer(false) },
		func(c *ProductRecommendationCondition) { c.Operator = "approx" },
		func(c *ProductRecommendationCondition) {
			c.Field = "rating"
			c.Integer = nil
			c.Number = conditionTestPointer(math.NaN())
		},
	} {
		bad := good
		mutate(&bad)
		if (ProductRecommendationRequest{Query: "synthetic", RequiredConditions: []ProductRecommendationCondition{bad}}).Validate() == nil {
			t.Fatal("invalid condition accepted")
		}
	}
	if (ProductRecommendationRequest{Query: "synthetic", RequiredConditions: []ProductRecommendationCondition{good, good}}).Validate() == nil {
		t.Fatal("duplicate ids accepted")
	}
	good.ID = "max_price"
	if (ProductRecommendationRequest{Query: "synthetic", MaxPrice: 2000, RequiredConditions: []ProductRecommendationCondition{good}}).Validate() == nil {
		t.Fatal("implicit budget id conflict accepted")
	}
}
