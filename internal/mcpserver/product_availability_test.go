package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type availabilityProductProvider struct{ fixedProductProvider }

func availabilityCards() []core.ProductCard {
	return []core.ProductCard{
		{Reference: core.ProductReference{ProductID: "101"}, Name: "Synthetic unknown", URL: "https://www.coupang.com/vp/products/101", Price: core.ProductPrice{Currency: "KRW"}},
		{Reference: core.ProductReference{ProductID: "102"}, Name: "Synthetic observed zero", URL: "https://www.coupang.com/vp/products/102", Price: core.ProductPrice{Currency: "KRW"}, ObservedFields: []string{"price.current_amount", "sponsored", "review_count"}, FieldEvidence: []core.ProductFieldEvidence{{Field: "price.current_amount", Source: "dom", Locator: "dom.search_card.price", Method: "numeric_parse", Provenance: "derived", Scope: "product", Reference: core.ProductReference{ProductID: "102"}, CapturedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)}}},
	}
}

func (availabilityProductProvider) Search(_ context.Context, r core.ProductSearchRequest) (core.ProductSearchResult, error) {
	return core.ProductSearchResult{SchemaVersion: core.ProductSchemaVersion, Query: r.Query, Items: availabilityCards()}, nil
}

func (availabilityProductProvider) Inspect(_ context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	result := core.ProductInspection{SchemaVersion: core.ProductSchemaVersion, Product: availabilityCards()[0]}
	result.Reviews = []core.ProductReview{{Content: "Synthetic review"}}
	if r.ProductID == "102" {
		result.Product = availabilityCards()[1]
		result.Coverage.ObservedFields = []string{"delivery.rocket", "delivery.free_shipping", "rating.average", "rating.count"}
		result.Reviews[0].ObservedFields = []string{"rating", "helpful_count"}
	}
	return result, nil
}

func TestMCPProductOutputValidatesUnknownAndKnownZero(t *testing.T) {
	ctx := context.Background()
	server := NewWithFeatures(fixedStatusProvider{}, fixedOrderProvider{}, availabilityProductProvider{}, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "availability-test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_search", Arguments: map[string]any{"query": "synthetic"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("output schema rejected availability contract: %#v", result.Content)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["schema_version"] != float64(4) {
		t.Fatalf("wire version not bumped: %s", data)
	}
	items := decoded["items"].([]any)
	unknown := items[0].(map[string]any)
	known := items[1].(map[string]any)
	if _, ok := unknown["sponsored"]; ok {
		t.Fatal("unknown sponsored became false")
	}
	if _, ok := unknown["price"].(map[string]any)["current_amount"]; ok {
		t.Fatal("unknown price became zero")
	}
	if v, ok := known["sponsored"]; !ok || v != false {
		t.Fatal("known false lost")
	}
	if v, ok := known["price"].(map[string]any)["current_amount"]; !ok || v != float64(0) {
		t.Fatal("known zero lost")
	}
	evidence := known["field_evidence"].([]any)[0].(map[string]any)
	if evidence["provenance"] != "derived" || evidence["source"] != "dom" || evidence["scope"] != "product" || evidence["captured_at"] != "2026-09-07T00:00:00Z" {
		t.Fatal("MCP output lost field provenance")
	}
	for _, id := range []string{"101", "102"} {
		t.Run("inspection-"+id, func(t *testing.T) {
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "product_inspect", Arguments: map[string]any{"product_id": id}})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("inspection schema rejected: %#v", result.Content)
			}
			data, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded["schema_version"] != float64(4) {
				t.Fatal("inspection wire version not bumped")
			}
			for _, pair := range [][2]string{{"delivery", "rocket"}, {"delivery", "free_shipping"}, {"rating", "average"}, {"rating", "count"}} {
				value, present := decoded[pair[0]].(map[string]any)[pair[1]]
				if present != (id == "102") {
					t.Fatalf("availability lost: %v, present=%v", pair, present)
				}
				if present && value != false && value != float64(0) {
					t.Fatalf("known zero changed: %v", value)
				}
			}
			review := decoded["reviews"].([]any)[0].(map[string]any)
			for _, field := range []string{"rating", "helpful_count"} {
				value, present := review[field]
				if present != (id == "102") || (present && value != float64(0)) {
					t.Fatalf("review %s availability changed: %#v", field, review)
				}
			}
		})
	}
}
