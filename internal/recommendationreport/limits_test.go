package recommendationreport

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestReportRenderingResourceLimits(t *testing.T) {
	report := core.ProductRecommendationReport{Title: "Synthetic", Recommendation: core.ProductRecommendationResult{SchemaVersion: 4, Status: core.ProductRecommendationIncomplete}}
	large := report
	large.Title = strings.Repeat("A", core.ProductReportMaxInputBytes+1)
	if _, err := Render(large); err == nil {
		t.Fatal("input byte cap missing")
	}
	large = report
	large.Axes = make([]core.ProductComparisonAxis, 33)
	if _, err := Render(large); err == nil {
		t.Fatal("axis resource cap missing")
	}
	large = report
	large.Axes = []core.ProductComparisonAxis{{ID: "a", Label: "A", Weight: 1, EvidenceRule: strings.Repeat("R", 512<<10)}}
	value := 0.0
	for index := 0; index < 20; index++ {
		id := strconv.Itoa(100 + index)
		large.Recommendation.Candidates = append(large.Recommendation.Candidates, core.ProductRecommendationCandidate{Product: core.ProductCard{Reference: core.ProductReference{ProductID: id}}})
		large.Scores = append(large.Scores, core.ProductCandidateScore{ProductID: id, EvidenceCoverage: 1, Values: []core.ProductComparisonValue{{AxisID: "a", Score: &value, Provenance: "derived", Explanation: "Synthetic rule"}}})
	}
	if _, err := Render(large); err == nil {
		t.Fatal("amplified output exceeded byte cap")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RenderResult(ctx, report); err == nil {
		t.Fatal("cancelled rendering request accepted")
	}
}
