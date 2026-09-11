package core

import (
	"strings"
	"testing"
)

func TestReportEditorialNotesRemainBoundToExactCandidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ProductRecommendationReport)
		valid  bool
	}{
		{"valid", func(*ProductRecommendationReport) {}, true},
		{"different_option", func(r *ProductRecommendationReport) { r.CandidateNotes[0].Reference.ItemID = "202" }, false},
		{"duplicate", func(r *ProductRecommendationReport) { r.CandidateNotes = append(r.CandidateNotes, r.CandidateNotes[0]) }, false},
		{"no_title", func(r *ProductRecommendationReport) { r.CandidateNotes[0].Title = " " }, false},
		{"long_summary", func(r *ProductRecommendationReport) { r.Summary = strings.Repeat("한", 1601) }, false},
		{"long_note", func(r *ProductRecommendationReport) { r.CandidateNotes[0].Rationale = strings.Repeat("한", 1201) }, false},
		{"long_tradeoff", func(r *ProductRecommendationReport) {
			r.CandidateNotes[0].Tradeoffs = []string{strings.Repeat("한", 401)}
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := syntheticScoredReport()
			r.CandidateNotes = []ProductReportCandidateNote{{Reference: r.Recommendation.Candidates[0].Product.Reference, Title: "짧은 이름", Rationale: "조건부 판단"}}
			test.change(&r)
			if err := r.Validate(); (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
}

func TestReportDecisionPathsRequireExactReferencesAndBoundedContent(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ProductRecommendationReport)
		valid  bool
	}{
		{"valid", func(*ProductRecommendationReport) {}, true},
		{"wrong_option", func(r *ProductRecommendationReport) { r.DecisionPaths[0].References[0].ItemID = "999" }, false},
		{"duplicate_reference", func(r *ProductRecommendationReport) {
			r.DecisionPaths[0].References = append(r.DecisionPaths[0].References, r.DecisionPaths[0].References[0])
		}, false},
		{"missing_reference", func(r *ProductRecommendationReport) { r.DecisionPaths[0].References = nil }, false},
		{"missing_assumption", func(r *ProductRecommendationReport) { r.DecisionPaths[0].When = " " }, false},
		{"unbounded_reason", func(r *ProductRecommendationReport) { r.DecisionPaths[0].Reason = strings.Repeat("한", 1201) }, false},
		{"too_many_paths", func(r *ProductRecommendationReport) {
			p := r.DecisionPaths[0]
			r.DecisionPaths = []ProductReportDecisionPath{p, p, p, p}
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := syntheticScoredReport()
			r.DecisionPaths = []ProductReportDecisionPath{{Label: "합성 조건", When: "이 조건을 우선한다면", Reason: "합성 근거에 따른 추론", References: []ProductReference{r.Recommendation.Candidates[0].Product.Reference}}}
			test.change(&r)
			if err := r.Validate(); (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
}
