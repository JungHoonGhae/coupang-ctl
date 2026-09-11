package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type availabilityWorkflow struct{ fixedProductWorkflow }

func (availabilityWorkflow) Search(_ context.Context, r core.ProductSearchRequest) (core.ProductSearchResult, error) {
	return core.ProductSearchResult{SchemaVersion: core.ProductSchemaVersion, Query: r.Query, Items: []core.ProductCard{
		{Name: "Synthetic unknown", Price: core.ProductPrice{Currency: "KRW"}},
		{Name: "Synthetic known", Price: core.ProductPrice{Currency: "KRW"}, ObservedFields: []string{"sponsored", "price.current_amount"}},
	}}, nil
}
func TestCLIProductJSONPreservesAvailability(t *testing.T) {
	var output bytes.Buffer
	if err := runProducts(context.Background(), []string{"search", "--query", "synthetic"}, &output, availabilityWorkflow{}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["schema_version"] != float64(4) {
		t.Fatal("expected wire version 4")
	}
	items := got["items"].([]any)
	unknown := items[0].(map[string]any)
	known := items[1].(map[string]any)
	if _, ok := unknown["sponsored"]; ok {
		t.Fatal("unknown boolean emitted")
	}
	if _, ok := unknown["price"].(map[string]any)["current_amount"]; ok {
		t.Fatal("unknown price emitted")
	}
	if v, ok := known["sponsored"]; !ok || v != false {
		t.Fatal("known false lost")
	}
	if v, ok := known["price"].(map[string]any)["current_amount"]; !ok || v != float64(0) {
		t.Fatal("known zero lost")
	}
}
