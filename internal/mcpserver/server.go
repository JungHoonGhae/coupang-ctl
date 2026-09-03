package mcpserver

import (
	"context"
	"errors"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const authStatusTool = "auth_status"

type StatusProvider interface {
	Status(context.Context) (core.AuthStatus, error)
}

type OrderProvider interface {
	Sync(context.Context, core.SyncRequest) (core.SyncResult, error)
	EnrichCategories(context.Context, core.CategoryEnrichmentRequest) (core.CategoryEnrichmentResult, error)
	CategoryCatalog(context.Context, core.CategoryCatalogRequest) (core.CategoryCatalog, error)
	List(context.Context, core.OrderFilter) ([]core.Order, error)
	Spend(context.Context, core.OrderFilter) (core.SpendSummary, error)
	Stats(context.Context, core.OrderFilter) (core.OrderStats, error)
	Insights(context.Context, core.OrderFilter) (core.ShoppingInsights, error)
	ProductInsights(context.Context, core.OrderFilter) (core.ProductInsights, error)
	ReorderCandidates(context.Context, core.OrderFilter) ([]core.ReorderCandidate, error)
	Export(context.Context, core.OrderFilter) (core.OrderExport, error)
}

type ProductProvider interface {
	Search(context.Context, core.ProductSearchRequest) (core.ProductSearchResult, error)
	Inspect(context.Context, core.ProductInspectRequest) (core.ProductInspection, error)
	PriceHistory(context.Context, core.ProductPriceHistoryRequest) (core.ProductPriceHistory, error)
	AddPriceWatch(context.Context, core.ProductWatchRequest) (core.ProductWatchMutationResult, error)
	RemovePriceWatch(context.Context, core.ProductWatchRequest) (core.ProductWatchMutationResult, error)
	PriceWatchlist(context.Context) (core.ProductWatchList, error)
	RefreshPriceWatches(context.Context, core.ProductWatchRefreshRequest) (core.ProductWatchRefreshResult, error)
	AddToCart(context.Context, core.CartAddRequest) (core.CartAddResult, error)
}

type AccountProvider interface {
	Snapshot(context.Context, core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error)
}

type ReceiptProvider interface {
	Status(context.Context) (core.ReceiptRequestStatusSnapshot, error)
	History(context.Context, core.ReceiptHistoryRequest) (core.ReceiptHistoryPage, error)
	Summary(context.Context, core.ReceiptSummaryRequest) (core.ReceiptSummary, error)
	Vendor(context.Context, core.VendorReceiptRequest) (core.VendorReceiptSnapshot, error)
}

func New(provider StatusProvider, version string) *mcp.Server {
	return newServer(provider, nil, nil, nil, nil, version)
}

func NewWithOrders(authProvider StatusProvider, orderProvider OrderProvider, version string) *mcp.Server {
	return newServer(authProvider, orderProvider, nil, nil, nil, version)
}

func NewWithFeatures(authProvider StatusProvider, orderProvider OrderProvider, productProvider ProductProvider, version string) *mcp.Server {
	return newServer(authProvider, orderProvider, productProvider, nil, nil, version)
}

func NewWithAllFeatures(authProvider StatusProvider, orderProvider OrderProvider, productProvider ProductProvider, accountProvider AccountProvider, version string) *mcp.Server {
	return newServer(authProvider, orderProvider, productProvider, accountProvider, nil, version)
}

func NewWithAllFeaturesAndReceipts(authProvider StatusProvider, orderProvider OrderProvider, productProvider ProductProvider, accountProvider AccountProvider, receiptProvider ReceiptProvider, version string) *mcp.Server {
	return newServer(authProvider, orderProvider, productProvider, accountProvider, receiptProvider, version)
}

func newServer(provider StatusProvider, orders OrderProvider, products ProductProvider, account AccountProvider, receipts ReceiptProvider, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "coupangctl",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        authStatusTool,
		Description: "Inspect whether coupangctl has a dedicated local browser profile. This does not expose cookies or credentials.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, core.AuthStatus, error) {
		status, err := provider.Status(ctx)
		if err != nil {
			return nil, core.AuthStatus{}, errors.New("browser_unavailable: cannot inspect the dedicated browser profile")
		}
		return nil, status, nil
	})
	if orders != nil {
		addOrderTools(server, orders)
	}
	if products != nil {
		addProductTools(server, products)
	}
	if account != nil {
		addAccountTools(server, account)
	}
	if receipts != nil {
		addReceiptTools(server, receipts)
	}

	return server
}

func Run(ctx context.Context, provider StatusProvider, version string) error {
	return New(provider, version).Run(ctx, &mcp.StdioTransport{})
}

func RunWithOrders(ctx context.Context, authProvider StatusProvider, orderProvider OrderProvider, version string) error {
	return NewWithOrders(authProvider, orderProvider, version).Run(ctx, &mcp.StdioTransport{})
}

func RunWithFeatures(ctx context.Context, authProvider StatusProvider, orderProvider OrderProvider, productProvider ProductProvider, version string) error {
	return NewWithFeatures(authProvider, orderProvider, productProvider, version).Run(ctx, &mcp.StdioTransport{})
}

func RunWithAllFeatures(ctx context.Context, authProvider StatusProvider, orderProvider OrderProvider, productProvider ProductProvider, accountProvider AccountProvider, version string) error {
	return NewWithAllFeatures(authProvider, orderProvider, productProvider, accountProvider, version).Run(ctx, &mcp.StdioTransport{})
}

func RunWithAllFeaturesAndReceipts(ctx context.Context, authProvider StatusProvider, orderProvider OrderProvider, productProvider ProductProvider, accountProvider AccountProvider, receiptProvider ReceiptProvider, version string) error {
	return NewWithAllFeaturesAndReceipts(authProvider, orderProvider, productProvider, accountProvider, receiptProvider, version).Run(ctx, &mcp.StdioTransport{})
}

func addReceiptTools(server *mcp.Server, provider ReceiptProvider) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(true)}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "receipts_status",
		Description: "Read whether a cash or card receipt archive request is currently in progress. This never creates a request and exposes no receipt URL, card number, or account identifier.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, core.ReceiptRequestStatusSnapshot, error) {
		result, err := provider.Status(ctx)
		if err != nil {
			return nil, core.ReceiptRequestStatusSnapshot{}, safeReceiptToolError(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "receipts_list",
		Description: "List bounded cash or card receipt download-request history. Download URLs and card numbers are discarded. This never creates or downloads a receipt.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ReceiptHistoryRequest) (*mcp.CallToolResult, core.ReceiptHistoryPage, error) {
		result, err := provider.History(ctx, input)
		if err != nil {
			return nil, core.ReceiptHistoryPage{}, safeReceiptToolError(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "receipts_summary",
		Description: "Summarize observed cash or card receipt counts and amounts for a bounded date range. Card identifiers are discarded, and installment statistics remain explicitly unavailable because no installment-month field is verified.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ReceiptSummaryRequest) (*mcp.CallToolResult, core.ReceiptSummary, error) {
		result, err := provider.Summary(ctx, input)
		if err != nil {
			return nil, core.ReceiptSummary{}, safeReceiptToolError(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "receipts_vendor",
		Description: "Read the source-native vendor receipt for one hashed source_ref returned by orders_list. Returns private-local payment method, product, and cancellation component fields without exposing the raw order ID. Cancellation components are not labeled as a completed refund settlement, and installment months remain unavailable unless explicitly observed. This performs GET reads only and never creates a receipt.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.VendorReceiptRequest) (*mcp.CallToolResult, core.VendorReceiptSnapshot, error) {
		result, err := provider.Vendor(ctx, input)
		if err != nil {
			return nil, core.VendorReceiptSnapshot{}, safeReceiptToolError(err)
		}
		return nil, result, nil
	})
}

func addAccountTools(server *mcp.Server, provider AccountProvider) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "account_benefits",
		Description: "Read the authenticated user's current WOW membership state, Coupang-reported recent benefit savings, explicit membership-payment evidence when available, registered payment-method brands, and WOW Card reward aggregates. When the source UI exposes a recent-month window, the comparison uses current monthly fee times that month count only as an inferred estimate; it never presents missing order metadata as zero paid or the estimate as confirmed net value. Card rewards and card fees stay separate. Order-payment and installment statistics remain unavailable until transaction evidence is adopted. Account identifiers and raw transaction descriptions are discarded. This never changes membership, payment, or card state.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.AccountBenefitsRequest) (*mcp.CallToolResult, core.AccountBenefitsSnapshot, error) {
		result, err := provider.Snapshot(ctx, input)
		if err != nil {
			return nil, core.AccountBenefitsSnapshot{}, errors.New("coupangctl could not read account benefit data")
		}
		return nil, result, nil
	})
}

func addProductTools(server *mcp.Server, provider ProductProvider) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(true)}
	observingRead := &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true)}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "products_search",
		Description: "Search Coupang from natural language or a source-native category ID. Supports distinct Coupang ranking, sales, latest, price, rating, and review sorts; computer memory/storage and explicit used-item filters; and listing-level diversity by default. Explicitly observed returned prices are appended to private local price history. When Coupang Partners credentials are configured, each canonical URL may also have a separate affiliate_url plus a definite commission disclosure; set disable_affiliate=true to opt out. Search position is evidence of the selected Coupang result order, not absolute unit sales. Never invent an unobserved product field.",
		Annotations: observingRead,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductSearchRequest) (*mcp.CallToolResult, core.ProductSearchResult, error) {
		result, err := provider.Search(ctx, input)
		if err != nil {
			return nil, core.ProductSearchResult{}, safeProductToolError(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "product_inspect",
		Description: "Inspect one products_search candidate: public details, gallery and detail images, current price, delivery, coupons or card benefits when observed, aggregate ratings, and sanitized reviews. An explicitly observed price is appended to private local price history. When configured, affiliate_url remains separate from the canonical URL and carries the same commission disclosure; set disable_affiliate=true to opt out. This never adds to cart or purchases.",
		Annotations: observingRead,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductInspectRequest) (*mcp.CallToolResult, core.ProductInspection, error) {
		result, err := provider.Inspect(ctx, input)
		if err != nil {
			return nil, core.ProductInspection{}, safeProductToolError(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "product_price_history",
		Description: "Read locally accumulated current-price observations for one product. Exact vendor options remain separate series, and trends are deterministic calculations over observations made by prior coupangctl searches or inspections. This does not claim retroactive Coupang price history.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductPriceHistoryRequest) (*mcp.CallToolResult, core.ProductPriceHistory, error) {
		result, err := provider.PriceHistory(ctx, input)
		if err != nil {
			return nil, core.ProductPriceHistory{}, safeProductToolError(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "product_watchlist", Description: "List exact product identities selected for periodic local price observation. This does not access Coupang or refresh prices.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, core.ProductWatchList, error) {
		result, err := provider.PriceWatchlist(ctx)
		if err != nil {
			return nil, core.ProductWatchList{}, safeProductToolError(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "product_watch_add",
		Description: "Add an exact product identity to the local price watchlist. The identity must already have an observed price from products_search or product_inspect; names are never matched. This changes only local watchlist state.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductWatchRequest) (*mcp.CallToolResult, core.ProductWatchMutationResult, error) {
		result, err := provider.AddPriceWatch(ctx, input)
		if err != nil {
			return nil, core.ProductWatchMutationResult{}, safeProductToolError(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "product_watch_remove",
		Description: "Remove one exact identity from the local price watchlist without deleting its price history. This changes only local watchlist state.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: boolPointer(true), IdempotentHint: true, OpenWorldHint: boolPointer(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductWatchRequest) (*mcp.CallToolResult, core.ProductWatchMutationResult, error) {
		result, err := provider.RemovePriceWatch(ctx, input)
		if err != nil {
			return nil, core.ProductWatchMutationResult{}, safeProductToolError(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "product_watch_refresh",
		Description: "Refresh due exact watchlist identities through bounded public product inspection and append observed current prices locally. This never uses affiliate conversion, changes a cart, checks out, orders, or pays.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductWatchRefreshRequest) (*mcp.CallToolResult, core.ProductWatchRefreshResult, error) {
		result, err := provider.RefreshPriceWatches(ctx, input)
		if err != nil {
			return nil, core.ProductWatchRefreshResult{}, safeProductToolError(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "cart_add",
		Description: "Add one exact products_search candidate to the user's Coupang cart. Call only after the user explicitly asks to add that item and set confirmed=true. This changes external cart state but never purchases, orders, or pays. Never retry automatically when verification is inconclusive.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true),
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.CartAddRequest) (*mcp.CallToolResult, core.CartAddResult, error) {
		result, err := provider.AddToCart(ctx, input)
		if err != nil {
			return nil, core.CartAddResult{}, safeProductToolError(err)
		}
		return nil, result, nil
	})
}

func addOrderTools(server *mcp.Server, provider OrderProvider) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(false)}
	mcp.AddTool(server, &mcp.Tool{
		Name: "orders_list", Description: "List normalized local orders using bounded date and result filters.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter) (*mcp.CallToolResult, core.OrderList, error) {
		orders, err := provider.List(ctx, input)
		if err != nil {
			return nil, core.OrderList{}, safeToolError(err)
		}
		return nil, core.OrderList{Orders: orders}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "orders_spend", Description: "Summarize gross normalized order totals for an optional date range.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter) (*mcp.CallToolResult, core.SpendSummary, error) {
		result, err := provider.Spend(ctx, input)
		if err != nil {
			return nil, core.SpendSummary{}, safeToolError(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "orders_stats", Description: "Summarize normalized cancellation and return rates for an optional date range.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter) (*mcp.CallToolResult, core.OrderStats, error) {
		result, err := provider.Stats(ctx, input)
		if err != nil {
			return nil, core.OrderStats{}, safeToolError(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "orders_insights", Description: "Return privacy-conscious, shareable shopping-pattern aggregates for an optional date range.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter) (*mcp.CallToolResult, core.ShoppingInsights, error) {
		result, err := provider.Insights(ctx, input)
		if err != nil {
			return nil, core.ShoppingInsights{}, safeToolError(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "orders_product_insights", Description: "Return private local product-name, paid-amount, and exact spend-day aggregates for an optional date range.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter) (*mcp.CallToolResult, core.ProductInsights, error) {
		result, err := provider.ProductInsights(ctx, input)
		if err != nil {
			return nil, core.ProductInsights{}, safeToolError(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "orders_category_catalog", Description: "Find source-native Coupang category IDs and observed breadcrumb paths from the private local order ledger. Use a returned category_id with products_search instead of guessing a taxonomy label or ID. Local observed-product counts are not Coupang popularity.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.CategoryCatalogRequest) (*mcp.CallToolResult, core.CategoryCatalog, error) {
		result, err := provider.CategoryCatalog(ctx, input)
		if err != nil {
			return nil, core.CategoryCatalog{}, safeToolError(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "orders_reorder_candidates", Description: "List locally derived repeat-purchase candidates without placing an order.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter) (*mcp.CallToolResult, core.ReorderList, error) {
		candidates, err := provider.ReorderCandidates(ctx, input)
		if err != nil {
			return nil, core.ReorderList{}, safeToolError(err)
		}
		return nil, core.NewReorderList(candidates), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "orders_export", Description: "Export bounded normalized local order data. Browser state and raw payloads are never included.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter) (*mcp.CallToolResult, core.OrderExport, error) {
		result, err := provider.Export(ctx, input)
		if err != nil {
			return nil, core.OrderExport{}, safeToolError(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "orders_enrich_categories", Description: "Cache source-native Coupang product breadcrumb categories for uncached local products.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(true),
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.CategoryEnrichmentRequest) (*mcp.CallToolResult, core.CategoryEnrichmentResult, error) {
		result, err := provider.EnrichCategories(ctx, input)
		if err != nil {
			return nil, core.CategoryEnrichmentResult{}, safeToolError(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "orders_sync", Description: "Refresh the normalized local ledger from the authenticated dedicated browser profile.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(true),
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.SyncRequest) (*mcp.CallToolResult, core.SyncResult, error) {
		result, err := provider.Sync(ctx, input)
		if err != nil {
			return nil, core.SyncResult{}, safeToolError(err)
		}
		return nil, result, nil
	})
}

func boolPointer(value bool) *bool { return &value }

func safeToolError(error) error {
	return errors.New("coupangctl could not complete the requested order operation")
}

func safeProductToolError(error) error {
	return errors.New("coupangctl could not complete the requested product operation")
}

func safeReceiptToolError(error) error {
	return errors.New("coupangctl could not complete the requested receipt read")
}
