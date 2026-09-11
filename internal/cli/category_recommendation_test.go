package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestProductRecommendCLIRetainsCategoryScope(t *testing.T) {
	w := &recommendationWorkflow{}
	var out bytes.Buffer
	err := runProducts(context.Background(), []string{"recommend", "--category-id", "123456", "--max-price", "2000000", "--no-affiliate", "--answers-json", `[{"question_id":"facet:Memory","choice":"32 GB"}]`}, &out, w)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(w.request)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if json.Unmarshal(encoded, &wire) != nil || wire["category_id"] != "123456" || w.request.Query != "" || w.calls != 1 || w.request.MaxPrice != 2000000 || !w.request.DisableAffiliate || len(w.request.Answers) != 1 {
		t.Fatal("category/filter intent did not reach recommendation workflow")
	}
	if json.Unmarshal(out.Bytes(), &wire) != nil || wire["category_id"] != "123456" {
		t.Fatal("CLI response lost source category")
	}
	chain := `[{"question_id":"facet:카테고리","choice":"Parent"},{"question_id":"facet:카테고리","choice":"Child"}]`
	if err := runProducts(context.Background(), []string{"recommend", "--category-id", "123456", "--answers-json", chain}, &bytes.Buffer{}, w); err != nil || w.calls != 2 || len(w.request.Answers) != 2 || w.request.Answers[0].Choice != "Parent" || w.request.Answers[1].Choice != "Child" {
		t.Fatal("CLI dropped or reordered observed category navigation")
	}
	for _, args := range [][]string{{"recommend", "--category-id", "../search"}, {"recommend", "--category-id", "123456", "--query", "synthetic"}} {
		if err := runProducts(context.Background(), args, &bytes.Buffer{}, w); err == nil || w.calls != 2 {
			t.Fatal("invalid scope reached workflow")
		}
	}
	if err := runProducts(context.Background(), []string{"recommend", "--query", "synthetic", "--answers-json", chain}, &bytes.Buffer{}, w); err != nil || w.calls != 3 || w.request.Query != "synthetic" || w.request.CategoryID != "" || len(w.request.Answers) != 2 {
		t.Fatal("CLI rejected a query category trail")
	}
}
