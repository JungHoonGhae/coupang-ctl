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

type AuthRecoveryProvider interface {
	Recover(context.Context, core.AuthRecoveryRequest) (core.AuthRecoveryResult, error)
}

type OrderProvider interface {
	Sync(context.Context, core.SyncRequest) (core.SyncResult, error)
	SyncStatus(context.Context) (core.SyncStatus, error)
	EnrichCategories(context.Context, core.CategoryEnrichmentRequest) (core.CategoryEnrichmentResult, error)
	CategoryCatalog(context.Context, core.CategoryCatalogRequest) (core.CategoryCatalog, error)
	CategoryStability(context.Context) (core.CategoryStabilityReport, error)
	List(context.Context, core.OrderFilter) ([]core.Order, error)
	Spend(context.Context, core.OrderFilter) (core.SpendSummary, error)
	Stats(context.Context, core.OrderFilter) (core.OrderStats, error)
	Insights(context.Context, core.OrderFilter) (core.ShoppingInsights, error)
	ProductInsights(context.Context, core.OrderFilter) (core.ProductInsights, error)
	ReorderCandidates(context.Context, core.OrderFilter) ([]core.ReorderCandidate, error)
	Export(context.Context, core.OrderFilter) (core.OrderExport, error)
}

type OrdinaryOrderProvider interface {
	Sync(context.Context, core.SyncRequest) (core.SyncResult, error)
}

type CurrentBrowserStatusProvider interface {
	Status(context.Context) (core.CurrentBrowserStatus, error)
}

type Providers struct {
	Auth                 StatusProvider
	AuthRecovery         AuthRecoveryProvider
	Orders               OrderProvider
	CurrentBrowserStatus CurrentBrowserStatusProvider
	CurrentBrowserOrders OrdinaryOrderProvider
	OrdinaryOrders       OrdinaryOrderProvider
	Products             ProductProvider
	Account              AccountProvider
	Receipts             ReceiptProvider
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
	Overview(context.Context, core.ReceiptOverviewRequest) (core.ReceiptOverview, error)
	Vendor(context.Context, core.VendorReceiptRequest) (core.VendorReceiptSnapshot, error)
}

func New(provider StatusProvider, version string) *mcp.Server {
	return NewWithProviders(Providers{Auth: provider}, version)
}

func NewWithOrders(authProvider StatusProvider, orderProvider OrderProvider, version string) *mcp.Server {
	return NewWithProviders(Providers{Auth: authProvider, Orders: orderProvider}, version)
}

func NewWithFeatures(authProvider StatusProvider, orderProvider OrderProvider, productProvider ProductProvider, version string) *mcp.Server {
	return NewWithProviders(Providers{Auth: authProvider, Orders: orderProvider, Products: productProvider}, version)
}

func NewWithAllFeatures(authProvider StatusProvider, orderProvider OrderProvider, productProvider ProductProvider, accountProvider AccountProvider, version string) *mcp.Server {
	return NewWithProviders(Providers{Auth: authProvider, Orders: orderProvider, Products: productProvider, Account: accountProvider}, version)
}

func NewWithAllFeaturesAndReceipts(authProvider StatusProvider, orderProvider OrderProvider, productProvider ProductProvider, accountProvider AccountProvider, receiptProvider ReceiptProvider, version string) *mcp.Server {
	return NewWithProviders(Providers{Auth: authProvider, Orders: orderProvider, Products: productProvider, Account: accountProvider, Receipts: receiptProvider}, version)
}

func NewWithProviders(providers Providers, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "coupangctl",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        authStatusTool,
		Description: "Quietly verify the dedicated profile's background read readiness without opening a visible browser or exposing cookies or credentials.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, core.AuthStatus, error) {
		status, err := providers.Auth.Status(ctx)
		if err != nil {
			return nil, core.AuthStatus{}, errors.New("browser_unavailable: cannot inspect the dedicated browser profile")
		}
		return nil, status, nil
	})
	if providers.AuthRecovery != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "auth_login_if_needed",
			Description: "Quietly check the dedicated profile first, then open the visible QR login browser only when the profile is missing or the session is confirmed expired. Set confirmed=true only after telling the user a browser may open. A temporary access_blocked state never triggers login. No QR link, cookie, credential, OTP, or profile path is returned.",
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true),
			},
		}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.AuthRecoveryRequest) (*mcp.CallToolResult, core.AuthRecoveryResult, error) {
			result, err := providers.AuthRecovery.Recover(ctx, input)
			if err != nil {
				return nil, core.AuthRecoveryResult{}, safeAuthRecoveryToolError(err)
			}
			return nil, result, nil
		})
	}
	if providers.CurrentBrowserStatus != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "current_browser_status",
			Description: "Passively check whether a running Chrome 144+ browser exposes its private loopback debugging endpoint. This does not connect to the debugger, trigger Chrome's approval prompt, inspect tabs, create a tab, or expose the port, token, or profile path.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, core.CurrentBrowserStatus, error) {
			status, err := providers.CurrentBrowserStatus.Status(ctx)
			if err != nil {
				return nil, core.CurrentBrowserStatus{}, errors.New("current_browser_unavailable: cannot inspect current-browser readiness")
			}
			return nil, status, nil
		})
	}
	if providers.Orders != nil {
		addOrderTools(server, providers.Orders)
	}
	if providers.OrdinaryOrders != nil {
		addOrdinaryOrderTools(server, providers.OrdinaryOrders)
	}
	if providers.CurrentBrowserOrders != nil {
		addCurrentBrowserOrderTools(server, providers.CurrentBrowserOrders)
	}
	if providers.Products != nil {
		addProductTools(server, providers.Products)
	}
	if providers.Account != nil {
		addAccountTools(server, providers.Account)
	}
	if providers.Receipts != nil {
		addReceiptTools(server, providers.Receipts)
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

func RunWithProviders(ctx context.Context, providers Providers, version string) error {
	return NewWithProviders(providers, version).Run(ctx, &mcp.StdioTransport{})
}

func addReceiptTools(server *mcp.Server, provider ReceiptProvider) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(true)}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "receipts_status",
		Description: "Read whether Coupang marks a new cash or card receipt archive request as possible or impossible. The source does not prove why an impossible state occurred, so request_in_progress remains null. This never creates a request and exposes no receipt URL, card number, or account identifier.",
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
		Name:        "receipts_overview",
		Description: "Aggregate separate cash and card receipt-source totals over a multi-year date range using non-overlapping calendar-year reads. Payment-method totals are derived from observed receipt summaries; they are not relabeled as order spend, and installment months remain unavailable.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ReceiptOverviewRequest) (*mcp.CallToolResult, core.ReceiptOverview, error) {
		result, err := provider.Overview(ctx, input)
		if err != nil {
			return nil, core.ReceiptOverview{}, safeReceiptToolError(err)
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
		Description: "Read the authenticated user's current WOW membership state, current fee and source fee-change date, Coupang-reported recent benefit savings, explicit membership-payment evidence when available, registered payment-method brands, and WOW Card reward aggregates. The fee-change date is schedule metadata, not historical charge evidence. When the source UI exposes a recent-month window, the comparison uses current monthly fee times that month count only as an inferred estimate; it never presents missing order metadata as zero paid or the estimate as confirmed net value. Card rewards and card fees stay separate. Order-payment and installment statistics remain unavailable until transaction evidence is adopted. Account identifiers and raw transaction descriptions are discarded. This never changes membership, payment, or card state.",
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
		Name: "orders_sync_status", Description: "Read the latest local order-sync attempt, acquisition source, observed provenance, page/order counts, and complete-history evidence without contacting Coupang or opening a browser.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, core.SyncStatus, error) {
		status, err := provider.SyncStatus(ctx)
		if err != nil {
			return nil, core.SyncStatus{}, safeToolError(err)
		}
		return nil, status, nil
	})
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
		Name: "orders_category_stability", Description: "Report whether exact source-native breadcrumb paths changed across locally retained observations. Counts, observation days, insufficient-evidence states, and the single-ledger limitation remain explicit; this does not claim population-wide stability.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, core.CategoryStabilityReport, error) {
		result, err := provider.CategoryStability(ctx)
		if err != nil {
			return nil, core.CategoryStabilityReport{}, safeToolError(err)
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
		Name: "orders_enrich_categories", Description: "Cache source-native Coupang product breadcrumb categories for uncached local products. Set recheck=true only to explicitly append fresh observations for cached products, oldest cache entries first.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true),
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

func addOrdinaryOrderTools(server *mcp.Server, provider OrdinaryOrderProvider) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "orders_sync_ordinary_browser",
		Description: "Optional compatibility path; use only when dedicated-profile and approved current-browser sync are unsuitable. Refresh the normalized local ledger from the user-selected order-list tab in ordinary Chrome. Before calling, tell the user to keep that tab selected and click the installed coupangctl extension once while this tool is waiting. The extension uses activeTab for this one action, sends only normalized bounded order data through the local native host, and never copies cookies. This changes only the local ledger and never orders or pays.",
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

func addCurrentBrowserOrderTools(server *mcp.Server, provider OrdinaryOrderProvider) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "orders_sync_current_browser",
		Description: "Refresh the normalized local ledger through a user-approved connection to a running Chrome 144+ browser. The user must first enable remote debugging at chrome://inspect/#remote-debugging and approve Chrome's connection prompt. coupangctl creates and closes only its own tab, never copies browser state, and never closes the user's browser. This changes only the local ledger and never orders or pays.",
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

func safeAuthRecoveryToolError(err error) error {
	if errors.Is(err, core.ErrInteractiveConfirmationRequired) {
		return errors.New("interactive_confirmation_required: tell the user that a visible QR login browser may open, then retry with confirmed=true")
	}
	return errors.New("coupangctl could not complete interactive authentication")
}
