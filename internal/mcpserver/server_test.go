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

type capturingOrdinaryOrderProvider struct {
	request core.SyncRequest
}

type fixedProductProvider struct{}

type fixedAccountProvider struct{}

type fixedReceiptProvider struct{}

func (fixedReceiptProvider) Status(context.Context) (core.ReceiptRequestStatusSnapshot, error) {
	return core.ReceiptRequestStatusSnapshot{Visibility: "private_local", Statuses: []core.ReceiptRequestStatus{{Kind: core.ReceiptKindCard, CanRequestNew: true}}}, nil
}

func (fixedReceiptProvider) History(_ context.Context, request core.ReceiptHistoryRequest) (core.ReceiptHistoryPage, error) {
	return core.ReceiptHistoryPage{Visibility: "private_local", Kind: request.Kind, PageIndex: request.PageIndex, Items: []core.ReceiptHistoryItem{}}, nil
}

func (fixedReceiptProvider) Summary(_ context.Context, request core.ReceiptSummaryRequest) (core.ReceiptSummary, error) {
	return core.ReceiptSummary{Visibility: "private_local", Kind: request.Kind, From: request.From, To: request.To, TotalCount: 3, TotalAmountKRW: 42000, Installments: core.ReceiptInstallmentInfo{Status: "unavailable"}}, nil
}

func (fixedReceiptProvider) Overview(_ context.Context, request core.ReceiptOverviewRequest) (core.ReceiptOverview, error) {
	return core.ReceiptOverview{
		Visibility: "private_local", From: request.From, To: request.To,
		Totals: []core.ReceiptOverviewKindTotal{{Kind: core.ReceiptKindCard, TotalCount: 6, TotalAmountKRW: 70000}},
	}, nil
}

func (fixedReceiptProvider) Vendor(_ context.Context, request core.VendorReceiptRequest) (core.VendorReceiptSnapshot, error) {
	return core.VendorReceiptSnapshot{Visibility: "private_local", SourceRef: request.SourceRef, VendorCount: 1, Settlement: core.ReceiptSettlementInfo{Status: "source_components_observed"}}, nil
}

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

func (fixedProductProvider) PriceHistory(_ context.Context, request core.ProductPriceHistoryRequest) (core.ProductPriceHistory, error) {
	return core.ProductPriceHistory{SchemaVersion: 1, Visibility: "private_local", ProductID: request.ProductID, ObservationCount: 2}, nil
}

func (fixedProductProvider) AddPriceWatch(_ context.Context, request core.ProductWatchRequest) (core.ProductWatchMutationResult, error) {
	return core.ProductWatchMutationResult{SchemaVersion: 1, Visibility: "private_local", Changed: true, Entry: core.ProductWatchEntry{Reference: core.ProductReference{ProductID: request.ProductID, VendorItemID: request.VendorItemID}}}, nil
}

func (fixedProductProvider) RemovePriceWatch(_ context.Context, request core.ProductWatchRequest) (core.ProductWatchMutationResult, error) {
	return core.ProductWatchMutationResult{SchemaVersion: 1, Visibility: "private_local", Changed: true, Entry: core.ProductWatchEntry{Reference: core.ProductReference{ProductID: request.ProductID, VendorItemID: request.VendorItemID}}}, nil
}

func (fixedProductProvider) PriceWatchlist(context.Context) (core.ProductWatchList, error) {
	return core.ProductWatchList{SchemaVersion: 1, Visibility: "private_local", Count: 1}, nil
}

func (fixedProductProvider) RefreshPriceWatches(context.Context, core.ProductWatchRefreshRequest) (core.ProductWatchRefreshResult, error) {
	return core.ProductWatchRefreshResult{SchemaVersion: 1, Visibility: "private_local", Attempted: 1, Observed: 1}, nil
}

func (fixedProductProvider) AddToCart(_ context.Context, request core.CartAddRequest) (core.CartAddResult, error) {
	return core.CartAddResult{SchemaVersion: 1, Attempted: true, Added: true, Verified: true, Quantity: request.Quantity}, nil
}

func (fixedOrderProvider) Sync(context.Context, core.SyncRequest) (core.SyncResult, error) {
	return core.SyncResult{Complete: true}, nil
}

func (provider *capturingOrdinaryOrderProvider) Sync(_ context.Context, request core.SyncRequest) (core.SyncResult, error) {
	provider.request = request
	return core.SyncResult{Complete: true, PagesProcessed: 2}, nil
}

func (fixedOrderProvider) EnrichCategories(context.Context, core.CategoryEnrichmentRequest) (core.CategoryEnrichmentResult, error) {
	return core.CategoryEnrichmentResult{Complete: true}, nil
}

func (fixedOrderProvider) CategoryCatalog(_ context.Context, request core.CategoryCatalogRequest) (core.CategoryCatalog, error) {
	return core.CategoryCatalog{
		SchemaVersion: core.CategoryCatalogSchemaVersion,
		Visibility:    "private_local",
		Query:         request.Query,
		Categories: []core.CategoryCatalogEntry{{
			CategoryID: "200", Name: "Synthetic category", ObservedProductCount: 2,
		}},
	}, nil
}

func (fixedOrderProvider) CategoryStability(context.Context) (core.CategoryStabilityReport, error) {
	return core.CategoryStabilityReport{
		SchemaVersion:         core.CategoryStabilitySchemaVersion,
		Visibility:            "private_local",
		Source:                core.CategorySourceProductJSONLDBreadcrumb,
		Assessment:            "stable_within_local_observation_window",
		RecheckedProductCount: 2,
		Provenance:            core.CategoryStabilityProvenance{PathAndTimestamp: "observed", Counts: "derived", Assessment: "derived"},
	}, nil
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

func TestOrdinaryBrowserOrderSyncToolUsesItsDedicatedTypedProvider(t *testing.T) {
	ctx := context.Background()
	ordinary := &capturingOrdinaryOrderProvider{}
	server := NewWithProviders(Providers{
		Auth:           fixedStatusProvider{},
		Orders:         fixedOrderProvider{},
		OrdinaryOrders: ordinary,
	}, "v0.1.0-test")
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
		Name: "orders_sync_ordinary_browser", Arguments: map[string]any{"max_pages": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var got core.SyncResult
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Complete || got.PagesProcessed != 2 || ordinary.request.MaxPages != 2 {
		t.Fatalf("unexpected ordinary-browser sync result: %#v, request=%#v", got, ordinary.request)
	}
}

func TestCurrentBrowserOrderSyncToolUsesItsDedicatedTypedProvider(t *testing.T) {
	ctx := context.Background()
	current := &capturingOrdinaryOrderProvider{}
	server := NewWithProviders(Providers{
		Auth:                 fixedStatusProvider{},
		Orders:               fixedOrderProvider{},
		CurrentBrowserOrders: current,
	}, "v0.1.0-test")
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
		Name: "orders_sync_current_browser", Arguments: map[string]any{"max_pages": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var got core.SyncResult
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Complete || got.PagesProcessed != 2 || current.request.MaxPages != 2 {
		t.Fatalf("unexpected current-browser sync result: %#v, request=%#v", got, current.request)
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

	history, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "product_price_history", Arguments: map[string]any{"product_id": "101"},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(history.StructuredContent)
	var historyResult core.ProductPriceHistory
	if err := json.Unmarshal(encoded, &historyResult); err != nil || historyResult.ObservationCount != 2 {
		t.Fatalf("unexpected product price history: %#v, %v", historyResult, err)
	}

	watchlist, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "product_watchlist"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(watchlist.StructuredContent)
	var watchlistResult core.ProductWatchList
	if err := json.Unmarshal(encoded, &watchlistResult); err != nil || watchlistResult.Count != 1 {
		t.Fatalf("unexpected product watchlist: %#v, %v", watchlistResult, err)
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

func TestReceiptToolsUseTypedReadOnlyWorkflow(t *testing.T) {
	ctx := context.Background()
	server := NewWithAllFeaturesAndReceipts(fixedStatusProvider{}, fixedOrderProvider{}, fixedProductProvider{}, fixedAccountProvider{}, fixedReceiptProvider{}, "v0.1.0-test")
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

	response, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "receipts_summary", Arguments: map[string]any{"kind": "card", "from": "2026-08-01", "to": "2026-08-31"},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(response.StructuredContent)
	var got core.ReceiptSummary
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.Visibility != "private_local" || got.TotalCount != 3 || got.Installments.Status != "unavailable" {
		t.Fatalf("unexpected receipt summary: %#v", got)
	}

	response, err = clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "receipts_overview", Arguments: map[string]any{"from": "2024-12-01", "to": "2026-01-15"},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(response.StructuredContent)
	var overview core.ReceiptOverview
	if err := json.Unmarshal(encoded, &overview); err != nil {
		t.Fatal(err)
	}
	if overview.Visibility != "private_local" || overview.Totals[0].TotalAmountKRW != 70000 {
		t.Fatalf("unexpected receipt overview: %#v", overview)
	}

	sourceRef := core.OrderSourceReference("synthetic-order")
	response, err = clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "receipts_vendor", Arguments: map[string]any{"source_ref": sourceRef, "max_pages": 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(response.StructuredContent)
	var vendor core.VendorReceiptSnapshot
	if err := json.Unmarshal(encoded, &vendor); err != nil {
		t.Fatal(err)
	}
	if vendor.Visibility != "private_local" || vendor.SourceRef != sourceRef || vendor.Settlement.Status != "source_components_observed" {
		t.Fatalf("unexpected vendor receipt tool response: %#v", vendor)
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

func TestOrdersCategoryCatalogToolReturnsObservedSearchHandoff(t *testing.T) {
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
		Name: "orders_category_catalog", Arguments: map[string]any{"query": "Synthetic"},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var got core.CategoryCatalog
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.Visibility != "private_local" || got.Query != "Synthetic" || len(got.Categories) != 1 || got.Categories[0].CategoryID != "200" {
		t.Fatalf("unexpected category catalog: %#v", got)
	}
}

func TestOrdersCategoryStabilityToolReturnsTypedEvidence(t *testing.T) {
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

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "orders_category_stability"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var got core.CategoryStabilityReport
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != core.CategoryStabilitySchemaVersion || got.Assessment != "stable_within_local_observation_window" || got.RecheckedProductCount != 2 || got.Provenance.PathAndTimestamp != "observed" {
		t.Fatalf("unexpected category stability tool response: %#v", got)
	}
}

func TestAuthStatusToolReturnsTypedCoreResponse(t *testing.T) {
	ctx := context.Background()
	want := core.AuthStatus{
		State:          core.AuthAccessBlocked,
		Browser:        "Synthetic Chrome",
		ProfilePresent: true,
		CheckedAt:      time.Date(2026, time.September, 1, 3, 0, 0, 0, time.UTC),
		NextAction:     "retry later, or explicitly run `coupangctl auth verify --headed` when an interactive check is acceptable",
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
