package core

import (
	"fmt"
	"testing"
)

func TestReportDecisionRenderingBounds(t *testing.T) {
	newReport := func() ProductRecommendationReport {
		return ProductRecommendationReport{Title: "Synthetic", Recommendation: ProductRecommendationResult{SchemaVersion: ProductRecommendationSchemaVersion, Status: ProductRecommendationIncomplete}}
	}
	candidate := func(n int) ProductRecommendationCandidate {
		return ProductRecommendationCandidate{Product: ProductCard{Reference: ProductReference{ProductID: fmt.Sprint(n + 100)}}}
	}
	t.Run("exact references across both lists", func(t *testing.T) {
		r := newReport()
		for n := 0; n < 200; n++ {
			r.Recommendation.Inspected = append(r.Recommendation.Inspected, candidate(n))
		}
		r.Recommendation.Candidates = []ProductRecommendationCandidate{candidate(0)}
		if err := r.Validate(); err != nil {
			t.Fatal("shared inspected/displayed reference counted twice", err)
		}
		r.Recommendation.Candidates = []ProductRecommendationCandidate{candidate(200)}
		if err := r.Validate(); err == nil {
			t.Fatal("201 total decisions accepted")
		}
	})
	t.Run("explicit conditions plus budget", func(t *testing.T) {
		r := newReport()
		c := candidate(0)
		limit := int64(10000)
		for n := 0; n < 17; n++ {
			c.Conditions = append(c.Conditions, ProductConditionAssessment{Condition: ProductRecommendationCondition{ID: fmt.Sprintf("condition_%d", n), Field: "price.current_amount", Operator: "lte", Integer: &limit}})
		}
		r.Recommendation.Inspected = []ProductRecommendationCandidate{c}
		if err := r.Validate(); err != nil {
			t.Fatal("maximum conditions rejected", err)
		}
		c.Conditions = append(c.Conditions, ProductConditionAssessment{Condition: ProductRecommendationCondition{ID: "extra", Field: "price.current_amount", Operator: "lte", Integer: &limit}})
		r.Recommendation.Inspected[0] = c
		if err := r.Validate(); err == nil {
			t.Fatal("18 conditions accepted")
		}
	})
	t.Run("bounded next action references", func(t *testing.T) {
		r := newReport()
		a := ProductRecommendationNextAction{Kind: "inspect_details"}
		for n := 0; n < 1000; n++ {
			a.References = append(a.References, candidate(n).Product.Reference)
		}
		r.Recommendation.NextActions = []ProductRecommendationNextAction{a}
		if err := r.Validate(); err != nil {
			t.Fatal("maximum next action references rejected", err)
		}
		r.Recommendation.NextActions[0].References = append(a.References, candidate(1000).Product.Reference)
		if err := r.Validate(); err == nil {
			t.Fatal("1001 next action references accepted")
		}
	})
}
