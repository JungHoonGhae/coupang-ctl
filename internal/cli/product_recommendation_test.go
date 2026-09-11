package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type recommendationWorkflow struct {
	fixedProductWorkflow
	request core.ProductRecommendationRequest
	calls   int
	result  *core.ProductRecommendationResult
}

func (w *recommendationWorkflow) Recommend(_ context.Context, request core.ProductRecommendationRequest) (core.ProductRecommendationResult, error) {
	w.request = request
	w.calls++
	if w.result != nil {
		return *w.result, nil
	}
	return core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Query: request.Query, CategoryID: request.CategoryID}, nil
}

func TestProductRecommendCLIReviewCountAvailability(t *testing.T) {
	for _, known := range []bool{false, true} {
		var count *int
		if known {
			count = new(int)
		}
		workflow := &recommendationWorkflow{result: &core.ProductRecommendationResult{
			SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete,
			Candidates: []core.ProductRecommendationCandidate{{ReviewsAvailable: count}},
			Audit:      core.ProductRecommendationAudit{ReviewsAvailable: count, TimeBudgetExhausted: known},
		}}
		var out bytes.Buffer
		if err := runProducts(context.Background(), []string{"recommend", "--query", "synthetic", "--proceed"}, &out, workflow); err != nil {
			t.Fatal(err)
		}
		var output map[string]any
		if err := json.Unmarshal(out.Bytes(), &output); err != nil {
			t.Fatal(err)
		}
		if output["schema_version"] != float64(core.ProductRecommendationSchemaVersion) {
			t.Fatal("unexpected recommendation version")
		}
		if output["audit"].(map[string]any)["time_budget_exhausted"] != known {
			t.Fatal("CLI lost explicit time-budget audit flag")
		}
		for _, row := range []map[string]any{output["candidates"].([]any)[0].(map[string]any), output["audit"].(map[string]any)} {
			value, present := row["reviews_available"]
			if present != known || present && value != float64(0) {
				t.Fatal("recommendation review availability changed on CLI wire")
			}
		}
	}
}

func TestProductRecommendCLIUsesTypedWorkflow(t *testing.T) {
	w := &recommendationWorkflow{}
	var out bytes.Buffer
	err := runProducts(context.Background(), []string{"recommend", "--query", "synthetic bowl", "--max-price", "2000", "--limit", "2", "--proceed", "--use-purchase-history", "--answers-json", `[{"question_id":"facet:색상","choice":"화이트"}]`}, &out, w)
	if err != nil {
		t.Fatal(err)
	}
	if w.calls != 1 || w.request.Query != "synthetic bowl" || w.request.MaxPrice != 2000 || w.request.MaxItems != 2 || !w.request.Proceed || !w.request.UsePurchaseHistory || len(w.request.Answers) != 1 {
		t.Fatalf("request=%#v", w.request)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "incomplete" {
		t.Fatalf("status=%v", result["status"])
	}
}

func TestProductRecommendCLIValidatesBeforeSourceRead(t *testing.T) {
	for _, args := range [][]string{{"recommend"}, {"recommend", "--query", "synthetic", "--limit", "201"}, {"recommend", "--query", "synthetic", "--answers-json", "not-json"}, {"recommend", "--query", "synthetic", "--ordinary-browser"}} {
		w := &recommendationWorkflow{}
		if err := runProducts(context.Background(), args, &bytes.Buffer{}, w); err == nil {
			t.Fatalf("accepted %v", args)
		}
		if w.calls != 0 {
			t.Fatal("invalid request reached source")
		}
	}
}

func TestProductRecommendCLIDoesNotInventPresentationCount(t *testing.T) {
	for _, test := range []struct {
		args []string
		want int
	}{
		{nil, 0}, {[]string{"--limit", "6"}, 6},
	} {
		w := &recommendationWorkflow{}
		args := append([]string{"recommend", "--query", "synthetic", "--proceed"}, test.args...)
		if err := runProducts(context.Background(), args, &bytes.Buffer{}, w); err != nil {
			t.Fatal(err)
		}
		if w.request.MaxItems != test.want {
			t.Fatal("CLI imposed arbitrary recommendation count")
		}
	}
}

func TestProductRecommendCLIExplicitConditions(t *testing.T) {
	w := &recommendationWorkflow{}
	condition := `[{"id":"delivery","field":"rocket","operator":"eq","boolean":false}]`
	if err := runProducts(context.Background(), []string{"recommend", "--query", "synthetic", "--proceed", "--conditions-json", condition, "--comparison-axes-json", `[{"field":"price.current_amount","direction":"minimize"}]`}, &bytes.Buffer{}, w); err != nil {
		t.Fatal(err)
	}
	if len(w.request.RequiredConditions) != 1 || w.request.RequiredConditions[0].Boolean == nil || *w.request.RequiredConditions[0].Boolean {
		t.Fatal("explicit false condition not forwarded")
	}
	if len(w.request.ComparisonAxes) != 1 || w.request.ComparisonAxes[0].Direction != "minimize" {
		t.Fatal("explicit preference not forwarded")
	}
	for _, bad := range []string{
		`[{"id":"unsupported","field":"allergen_free","operator":"eq","boolean":true}]`,
		`[{"id":"budget","field":"price.current_amount","operator":"lte"}]`,
		`[{"id":"delivery","field":"rocket","operator":"eq","boolean":true,"invented":true}]`,
		`[] {}`,
	} {
		w = &recommendationWorkflow{}
		if err := runProducts(context.Background(), []string{"recommend", "--query", "synthetic", "--conditions-json", bad}, &bytes.Buffer{}, w); err == nil || w.calls != 0 {
			t.Fatal("invalid condition reached provider")
		}
	}
}

func TestProductRecommendCLICapacityContract(t *testing.T) {
	value := int64(32)
	w := &recommendationWorkflow{result: &core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Inspected: []core.ProductRecommendationCandidate{{Conditions: []core.ProductConditionAssessment{{Status: core.ProductConditionMet, DerivedInteger: &value, Derivation: &core.ProductCapacityDerivation{Provenance: "derived", Method: "standalone_capacity_decimal_gb", Unit: "GB", Input: core.ProductSelectedAttribute{Name: "RAM용량", Value: "32GB"}}}}}}}}
	var out bytes.Buffer
	err := runProducts(context.Background(), []string{"recommend", "--query", "synthetic", "--proceed", "--conditions-json", `[{"id":"ram","field":"specifications.memory_gb","operator":"gte","integer":32}]`, "--comparison-axes-json", `[{"field":"specifications.storage_gb","direction":"maximize"}]`}, &out, w)
	if err != nil {
		t.Fatal(err)
	}
	if w.calls != 1 || *w.request.RequiredConditions[0].Integer != 32 || w.request.ComparisonAxes[0].Field != "specifications.storage_gb" {
		t.Fatal("capacity input lost")
	}
	var r core.ProductRecommendationResult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	c := r.Inspected[0].Conditions[0]
	if c.DerivedInteger == nil || *c.DerivedInteger != 32 || c.ObservedInteger != nil || c.Derivation.Input.Value != "32GB" {
		t.Fatal("derived capacity lost on CLI wire")
	}
}

func TestProductRecommendCLIRejectsInvalidComparisonAxesBeforeSource(t *testing.T) {
	for _, value := range []string{
		`[{"field":"quality","direction":"maximize"}]`,
		`[{"field":"rating","direction":"maximize","weight":0.9}]`,
		`[{"field":"rating","direction":"maximize"},{"field":"rating","direction":"minimize"}]`,
		`[] {}`,
	} {
		w := &recommendationWorkflow{}
		if err := runProducts(context.Background(), []string{"recommend", "--query", "synthetic", "--comparison-axes-json", value}, &bytes.Buffer{}, w); err == nil || w.calls != 0 {
			t.Fatal("invalid comparison reached provider")
		}
	}
}
