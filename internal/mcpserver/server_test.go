package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fixedStatusProvider struct {
	status core.AuthStatus
}

type fixedOrderProvider struct{}

type fixedProductProvider struct{}

type fixedAccountProvider struct{}

func (fixedAccountProvider) Snapshot(_ context.Context, request core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error) {
	return core.AccountBenefitsSnapshot{
		SchemaVersion: 1,
		Membership:    core.WowMembership{Status: "MEMBER", IsMember: true},
		Coverage:      core.AccountBenefitsCoverage{CashTransactionPagesRead: request.MaxCashTransactionPages},
	}, nil
}

func (fixedProductProvider) Search(_ context.Context, request core.ProductSearchRequest) (core.ProductSearchResult, error) {
	return core.ProductSearchResult{SchemaVersion: 1, Query: request.Query, Currency: "KRW", Items: []core.ProductCard{{Name: "Synthetic hub"}}}, nil
}

func (fixedProductProvider) Inspect(_ context.Context, request core.ProductInspectRequest) (core.ProductInspection, error) {
	return core.ProductInspection{SchemaVersion: 1, Product: core.ProductCard{Reference: core.ProductReference{ProductID: request.ProductID}}}, nil
}

func (fixedProductProvider) AddToCart(_ context.Context, request core.CartAddRequest) (core.CartAddResult, error) {
	return core.CartAddResult{SchemaVersion: 1, Attempted: true, Added: true, Verified: true, Quantity: request.Quantity}, nil
}

func (fixedOrderProvider) Sync(context.Context, core.SyncRequest) (core.SyncResult, error) {
	return core.SyncResult{Complete: true}, nil
}

func (fixedOrderProvider) EnrichCategories(context.Context, core.CategoryEnrichmentRequest) (core.CategoryEnrichmentResult, error) {
	return core.CategoryEnrichmentResult{Complete: true}, nil
}

func (fixedOrderProvider) List(context.Context, core.OrderFilter) ([]core.Order, error) {
	return []core.Order{}, nil
}

func (fixedOrderProvider) Spend(context.Context, core.OrderFilter) (core.SpendSummary, error) {
	return core.SpendSummary{Currency: "KRW", OrderCount: 2, TotalAmount: 42000}, nil
}

func (fixedOrderProvider) Stats(context.Context, core.OrderFilter) (core.OrderStats, error) {
	return core.OrderStats{OrderCount: 2, ReturnedUnits: 1, ReturnedUnitRate: 0.5}, nil
}

func (fixedOrderProvider) Insights(context.Context, core.OrderFilter) (core.ShoppingInsights, error) {
	return core.ShoppingInsights{OrderCount: 2, LongestActiveMonthStreak: 12}, nil
}

func (fixedOrderProvider) ProductInsights(context.Context, core.OrderFilter) (core.ProductInsights, error) {
	return core.ProductInsights{
		Visibility: "private_local",
		TopBySpend: core.ProductAggregate{Name: "Synthetic private product", TotalPaidAmount: 42000},
	}, nil
}

func (fixedOrderProvider) ReorderCandidates(context.Context, core.OrderFilter) ([]core.ReorderCandidate, error) {
	return []core.ReorderCandidate{}, nil
}

func (fixedOrderProvider) Export(context.Context, core.OrderFilter) (core.OrderExport, error) {
	return core.OrderExport{SchemaVersion: 1, Orders: []core.Order{}}, nil
}

func (f fixedStatusProvider) Status(context.Context) (core.AuthStatus, error) {
	return f.status, nil
}

func TestOrdersSpendToolUsesSharedTypedWorkflow(t *testing.T) {
	ctx := context.Background()
	server := NewWithOrders(fixedStatusProvider{}, fixedOrderProvider{}, "v0.1.0-test")
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "orders_spend", Arguments: map[string]any{"from": "2026-08-01", "to": "2026-08-31"},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var got core.SpendSummary
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.TotalAmount != 42000 || got.OrderCount != 2 {
		t.Fatalf("unexpected spend result: %#v", got)
	}
}

func TestProductToolsExposeNaturalLanguageSearchAndConfirmedCartMutation(t *testing.T) {
	ctx := context.Background()
	server := NewWithFeatures(fixedStatusProvider{}, fixedOrderProvider{}, fixedProductProvider{}, "v0.1.0-test")
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	search, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "products_search", Arguments: map[string]any{"query": "후기 좋은 10만원 아래 맥북 허브", "max_price": 100000},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(search.StructuredContent)
	var searchResult core.ProductSearchResult
	if err := json.Unmarshal(encoded, &searchResult); err != nil || searchResult.Query == "" {
		t.Fatalf("unexpected product search result: %#v, %v", searchResult, err)
	}

	cart, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "cart_add", Arguments: map[string]any{"product_id": "101", "vendor_item_id": "301", "quantity": 1, "confirmed": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(cart.StructuredContent)
	var cartResult core.CartAddResult
	if err := json.Unmarshal(encoded, &cartResult); err != nil || !cartResult.Verified {
		t.Fatalf("unexpected cart result: %#v, %v", cartResult, err)
	}
}

func TestAccountBenefitsToolUsesTypedPrivateLocalResponse(t *testing.T) {
	ctx := context.Background()
	server := NewWithAllFeatures(fixedStatusProvider{}, fixedOrderProvider{}, fixedProductProvider{}, fixedAccountProvider{}, "v0.1.0-test")
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	response, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "account_benefits", Arguments: map[string]any{"max_cash_transaction_pages": 7}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(response.StructuredContent)
	var got core.AccountBenefitsSnapshot
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Membership.IsMember || got.Coverage.CashTransactionPagesRead != 7 {
		t.Fatalf("unexpected account tool response: %#v", got)
	}
}

func TestOrdersInsightsToolUsesSharedTypedWorkflow(t *testing.T) {
	ctx := context.Background()
	server := NewWithOrders(fixedStatusProvider{}, fixedOrderProvider{}, "v0.1.0-test")
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "orders_insights"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var got core.ShoppingInsights
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.OrderCount != 2 || got.LongestActiveMonthStreak != 12 {
		t.Fatalf("unexpected insights result: %#v", got)
	}
}

func TestOrdersProductInsightsToolUsesSharedTypedWorkflow(t *testing.T) {
	ctx := context.Background()
	server := NewWithOrders(fixedStatusProvider{}, fixedOrderProvider{}, "v0.1.0-test")
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "orders_product_insights"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var got core.ProductInsights
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.Visibility != "private_local" || got.TopBySpend.Name != "Synthetic private product" || got.TopBySpend.TotalPaidAmount != 42000 {
		t.Fatalf("unexpected product insight result: %#v", got)
	}
}

func TestAuthStatusToolReturnsTypedCoreResponse(t *testing.T) {
	ctx := context.Background()
	want := core.AuthStatus{
		State:          core.AuthUnverified,
		Browser:        "Synthetic Chrome",
		ProfilePresent: true,
		CheckedAt:      time.Date(2026, time.September, 1, 3, 0, 0, 0, time.UTC),
		NextAction:     "the dedicated profile exists; read-only session verification is not implemented yet",
	}
	server := New(fixedStatusProvider{status: want}, "v0.1.0-test")
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "coupangctl-test", Version: "v0.1.0-test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: authStatusTool})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool returned an error: %#v", result.Content)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var got core.AuthStatus
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("status = %#v, want %#v", got, want)
	}
}
