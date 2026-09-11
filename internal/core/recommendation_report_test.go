package core

import (
	"math"
	"testing"
)

func syntheticScoredReport() ProductRecommendationReport {
	value, total := 50.0, 50.0
	return ProductRecommendationReport{Title: "Synthetic", Recommendation: ProductRecommendationResult{
		SchemaVersion: ProductRecommendationSchemaVersion, Status: ProductRecommendationComplete,
		Candidates: []ProductRecommendationCandidate{{Product: ProductCard{Reference: ProductReference{ProductID: "101"}}}},
	}, Axes: []ProductComparisonAxis{{ID: "a", Label: "A", Weight: .4, EvidenceRule: "Synthetic rule A"}, {ID: "b", Label: "B", Weight: .3, EvidenceRule: "Synthetic rule B"}, {ID: "c", Label: "C", Weight: .3, EvidenceRule: "Synthetic rule C"}},
		Scores: []ProductCandidateScore{{ProductID: "101", EvidenceCoverage: 1, WeightedFit: &total, Values: []ProductComparisonValue{
			{AxisID: "a", Score: &value, Provenance: "derived", Explanation: "Synthetic calculation"},
			{AxisID: "b", Score: &value, Provenance: "inferred", Explanation: "Synthetic hypothesis"},
			{AxisID: "c", Score: &value, Provenance: "observed", Explanation: "Synthetic source score"},
		}}},
	}
}

func TestRecommendationReportScoreContract(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*ProductRecommendationReport)
		valid  bool
	}{
		{"complete", func(*ProductRecommendationReport) {}, true},
		{"partial_null", func(r *ProductRecommendationReport) {
			r.Scores[0].Values[0].Score = nil
			r.Scores[0].EvidenceCoverage = .6
			r.Scores[0].WeightedFit = nil
		}, true},
		{"partial_missing", func(r *ProductRecommendationReport) {
			r.Scores[0].Values = r.Scores[0].Values[1:]
			r.Scores[0].EvidenceCoverage = .6
			r.Scores[0].WeightedFit = nil
		}, true},
		{"partial_fabricated_total", func(r *ProductRecommendationReport) {
			r.Scores[0].Values[0].Score = nil
			r.Scores[0].EvidenceCoverage = .6
		}, false},
		{"duplicate_axis_value", func(r *ProductRecommendationReport) { r.Scores[0].Values[1].AxisID = "a" }, false},
		{"duplicate_candidate_score", func(r *ProductRecommendationReport) { r.Scores = append(r.Scores, r.Scores[0]) }, false},
		{"unsupported_provenance", func(r *ProductRecommendationReport) { r.Scores[0].Values[0].Provenance = "scientifically_proven" }, false},
		{"missing_explanation", func(r *ProductRecommendationReport) { r.Scores[0].Values[0].Explanation = "" }, false},
		{"false_coverage", func(r *ProductRecommendationReport) { r.Scores[0].EvidenceCoverage = .5 }, false},
		{"false_total", func(r *ProductRecommendationReport) { n := 70.0; r.Scores[0].WeightedFit = &n }, false},
		{"nan_weight", func(r *ProductRecommendationReport) { r.Axes[0].Weight = math.NaN() }, false},
		{"nan_score", func(r *ProductRecommendationReport) { n := math.NaN(); r.Scores[0].Values[0].Score = &n }, false},
		{"nan_coverage", func(r *ProductRecommendationReport) { r.Scores[0].EvidenceCoverage = math.NaN() }, false},
		{"infinite_total", func(r *ProductRecommendationReport) { n := math.Inf(1); r.Scores[0].WeightedFit = &n }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := syntheticScoredReport()
			test.mutate(&r)
			if err := r.Validate(); (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
}

func TestRecommendationReportPreservesTypedOutcome(t *testing.T) {
	for _, status := range []ProductRecommendationStatus{ProductRecommendationComplete, ProductRecommendationIncomplete, ProductRecommendationNeedsInput, ProductRecommendationNoMatches} {
		r := ProductRecommendationReport{Title: "Synthetic", Recommendation: ProductRecommendationResult{SchemaVersion: ProductRecommendationSchemaVersion, Status: status}}
		if err := r.Validate(); err != nil {
			t.Fatalf("%s cannot be reported: %v", status, err)
		}
	}
	r := syntheticScoredReport()
	r.Recommendation.Status = "unrecognized"
	if r.Validate() == nil {
		t.Fatal("unsupported status accepted")
	}
	r = syntheticScoredReport()
	r.Recommendation.Status = ProductRecommendationNoMatches
	if r.Validate() == nil {
		t.Fatal("no_matches contradicting candidates accepted")
	}
	r = syntheticScoredReport()
	r.Recommendation.SchemaVersion = 3
	if r.Validate() == nil {
		t.Fatal("legacy recommendation accepted as current availability contract")
	}
	r = syntheticScoredReport()
	r.SchemaVersion = 1
	if r.Validate() == nil {
		t.Fatal("legacy report accepted as the new score contract")
	}
}
