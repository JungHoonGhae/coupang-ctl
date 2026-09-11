package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/products"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type storedPriceProvider struct {
	fixedProductProvider
	service *products.Service
}

func (p storedPriceProvider) PriceHistory(ctx context.Context, r core.ProductPriceHistoryRequest) (core.ProductPriceHistory, error) {
	return p.service.PriceHistory(ctx, r)
}

func TestMCPPriceHistoryPreservesStoredDerivedZeroEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ref := core.ProductReference{ProductID: "101"}
	stamp := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	o := core.ProductPriceObservation{Reference: ref, Name: "Synthetic", Currency: "KRW", Source: "coupang_product_search", Provenance: "derived", ObservedAt: stamp, FieldEvidence: []core.ProductFieldEvidence{{Field: "price.current_amount", Source: "dom", Locator: "dom.price", Method: "numeric_parse", Provenance: "derived", Scope: "product", Reference: ref, CapturedAt: stamp}}}
	if err := db.RecordPriceObservations(ctx, []core.ProductPriceObservation{o}); err != nil {
		t.Fatal(err)
	}
	provider := storedPriceProvider{service: products.NewWithAffiliateAndPrices(nil, nil, db)}
	server := NewWithFeatures(fixedStatusProvider{}, fixedOrderProvider{}, provider, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "price-evidence-test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "product_price_history", Arguments: map[string]any{"product_id": "101"}})
	if err != nil || result.IsError {
		t.Fatalf("price history tool failed: %v", err)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var decoded core.ProductPriceHistory
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != 2 || len(decoded.Series) != 1 || len(decoded.Series[0].Observations) != 1 {
		t.Fatal("v2 stored price history shape lost")
	}
	got := decoded.Series[0].Observations[0]
	if got.Provenance != "derived" || got.CurrentAmount != 0 || !got.HasPriceEvidence() {
		t.Fatal("stored provenance or zero lost across MCP")
	}
	if decoded.Series[0].Trend == nil || decoded.Series[0].Trend.ChangeFromFirstReturnedPercent != nil {
		t.Fatal("zero baseline percentage invented")
	}
}
