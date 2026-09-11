package mcpserver

import (
	"context"

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

type Providers struct {
	Auth         StatusProvider
	AuthRecovery AuthRecoveryProvider
	Orders       OrderProvider
	Products     ProductProvider
	Account      AccountProvider
	Receipts     ReceiptProvider
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

type ProductRecommendationProvider interface {
	Recommend(context.Context, core.ProductRecommendationRequest) (core.ProductRecommendationResult, error)
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
	var factories ProviderFactories
	if providers.Auth != nil {
		factories.Auth = fixedProvider(providers.Auth)
	}
	if providers.AuthRecovery != nil {
		factories.AuthRecovery = fixedProvider(providers.AuthRecovery)
	}
	if providers.Orders != nil {
		factories.Orders = fixedProvider(providers.Orders)
	}
	if preview, ok := providers.Orders.(OrderPreviewProvider); ok {
		factories.OrderPreview = fixedProvider(preview)
	}
	if providers.Products != nil {
		factories.Products = fixedProvider(providers.Products)
	}
	if recommendations, ok := providers.Products.(ProductRecommendationProvider); ok {
		factories.Recommendations = fixedProvider(recommendations)
	}
	if providers.Account != nil {
		factories.Account = fixedProvider(providers.Account)
	}
	if providers.Receipts != nil {
		factories.Receipts = fixedProvider(providers.Receipts)
	}
	return NewWithFactories(factories, version)
}

// NewWithFactories registers typed tools without initializing their resources.
// Dependencies are resolved only after the SDK validates the tool input.
func NewWithFactories(providers ProviderFactories, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "coupangctl",
		Version: version,
	}, nil)
	server.AddReceivingMiddleware(errorContractMiddleware)

	addProviderTool(server, providers.Auth, &mcp.Tool{
		Name:        authStatusTool,
		Description: "Quietly check source authentication in the dedicated profile without opening a visible browser or exposing cookies or credentials. verified with verification_scope=authenticated_session does not establish stable account identity, order access or history coverage; check those with the requested read.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}, provider StatusProvider) (*mcp.CallToolResult, core.AuthStatus, error) {
		status, err := provider.Status(ctx)
		if err != nil {
			return nil, core.AuthStatus{}, err
		}
		return nil, status, nil
	})
	if providers.AuthRecovery != nil {
		addProviderTool(server, providers.AuthRecovery, &mcp.Tool{
			Name:        "auth_login_if_needed",
			Description: "Quietly check the dedicated profile first, then open the visible QR login browser only when the profile is missing or the session is confirmed expired. Set confirmed=true only after telling the user a browser may open. A temporary access_blocked state never triggers login. No QR link, cookie, credential, OTP, or profile path is returned.",
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true),
			},
		}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.AuthRecoveryRequest, provider AuthRecoveryProvider) (*mcp.CallToolResult, core.AuthRecoveryResult, error) {
			result, err := provider.Recover(ctx, input)
			if err != nil {
				return nil, core.AuthRecoveryResult{}, err
			}
			return nil, result, nil
		})
	}
	if providers.Orders != nil {
		addOrderTools(server, providers.Orders)
	}
	if providers.OrderPreview != nil {
		addOrderPreviewTool(server, providers.OrderPreview)
	}
	if providers.Products != nil || providers.Recommendations != nil {
		addProductTools(server, providers.Products, providers.Recommendations)
	}
	addProductReportTool(server)
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

func addReceiptTools(server *mcp.Server, factory ProviderFactory[ReceiptProvider]) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(true)}
	addProviderTool(server, factory, &mcp.Tool{
		Name:        "receipts_status",
		Description: "Read whether Coupang marks a new cash or card receipt archive request as possible or impossible. The source does not prove why an impossible state occurred, so request_in_progress remains null. This never creates a request and exposes no receipt URL, card number, or account identifier.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}, provider ReceiptProvider) (*mcp.CallToolResult, core.ReceiptRequestStatusSnapshot, error) {
		result, err := provider.Status(ctx)
		if err != nil {
			return nil, core.ReceiptRequestStatusSnapshot{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name:        "receipts_list",
		Description: "List bounded cash or card receipt download-request history. Download URLs and card numbers are discarded. This never creates or downloads a receipt.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ReceiptHistoryRequest, provider ReceiptProvider) (*mcp.CallToolResult, core.ReceiptHistoryPage, error) {
		result, err := provider.History(ctx, input)
		if err != nil {
			return nil, core.ReceiptHistoryPage{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name:        "receipts_summary",
		Description: "Summarize observed cash or card receipt counts and amounts for a bounded date range. Card identifiers are discarded, and installment statistics remain explicitly unavailable because no installment-month field is verified.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ReceiptSummaryRequest, provider ReceiptProvider) (*mcp.CallToolResult, core.ReceiptSummary, error) {
		result, err := provider.Summary(ctx, input)
		if err != nil {
			return nil, core.ReceiptSummary{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name:        "receipts_overview",
		Description: "Aggregate separate cash and card receipt-source totals over a multi-year date range using non-overlapping calendar-year reads. Payment-method totals are derived from observed receipt summaries; they are not relabeled as order spend, and installment months remain unavailable.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ReceiptOverviewRequest, provider ReceiptProvider) (*mcp.CallToolResult, core.ReceiptOverview, error) {
		result, err := provider.Overview(ctx, input)
		if err != nil {
			return nil, core.ReceiptOverview{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name:        "receipts_vendor",
		Description: "Read the source-native vendor receipt for one hashed source_ref returned by orders_list. Returns private-local payment method, product, and cancellation component fields without exposing the raw order ID. Cancellation components are not labeled as a completed refund settlement, and installment months remain unavailable unless explicitly observed. This performs GET reads only and never creates a receipt.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.VendorReceiptRequest, provider ReceiptProvider) (*mcp.CallToolResult, core.VendorReceiptSnapshot, error) {
		result, err := provider.Vendor(ctx, input)
		if err != nil {
			return nil, core.VendorReceiptSnapshot{}, err
		}
		return nil, result, nil
	})
}

func addAccountTools(server *mcp.Server, factory ProviderFactory[AccountProvider]) {
	addProviderTool(server, factory, &mcp.Tool{
		Name:        "account_benefits",
		Description: "Read the authenticated user's current WOW membership state, current fee and source fee-change date, Coupang-reported recent benefit savings, explicit membership-payment evidence when available, registered payment-method brands, and WOW Card reward aggregates. The fee-change date is schedule metadata, not historical charge evidence. When the source UI exposes a recent-month window, the comparison uses current monthly fee times that month count only as an inferred estimate; it never presents missing order metadata as zero paid or the estimate as confirmed net value. Card rewards and card fees stay separate. Order-payment and installment statistics remain unavailable until transaction evidence is adopted. Account identifiers and raw transaction descriptions are discarded. This never changes membership, payment, or card state.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.AccountBenefitsRequest, provider AccountProvider) (*mcp.CallToolResult, core.AccountBenefitsSnapshot, error) {
		result, err := provider.Snapshot(ctx, input)
		if err != nil {
			return nil, core.AccountBenefitsSnapshot{}, err
		}
		return nil, result, nil
	})
}

func addProductTools(server *mcp.Server, factory ProviderFactory[ProductProvider], recommendations ProviderFactory[ProductRecommendationProvider]) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(true)}
	observingRead := &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true)}
	if recommendations != nil {
		addProviderTool(server, recommendations, &mcp.Tool{
			Name:        "products_recommend",
			Description: "Research product candidates through bounded search comparisons and detail inspection. Provide exactly one of query or an observed numeric starting category_id. Research continues automatically when no answerable unanswered source-facet questions remain. Set proceed=true to skip remaining optional questions; ask the user only questions that materially affect their request. First discover facets, then send cumulative observed facet choices in answers, in application order. Each choice is verified against the current catalog, applied and rediscovered. Repeated facet:카테고리 answers also work with a query: prior categories are replayed as refinement.category_trail, while only the last category remains an active filter. An explicit sidebar category choice may change category-only scope when its selected label matches the destination's native breadcrumb; applied_category_id reports the verified destination used by subsequent refinement and sorts, while category_id retains the starting request. Query, requested sort and page are never silently replaced. Inspect refinement.steps, each step's category_id and refinement.issue; proceed never overrides a stale or unverified choice. With choices, discovery is limited to page one per sort and navigation consumes the document budget. Sidebar selection is not proof of detailed product suitability. Unknown sponsorship remains explicit rather than erasing search candidates. Camofox supports search refinement and exact-identity detail inspection. Returns observed evidence and explicit missing evidence, not a universal fit score. When the user requests purchase-aware recommendations, use_purchase_history=true includes private local identity-matched purchase aggregates and sync coverage; it does not infer preference or reorder need. incomplete and no_matches are not successful recommendations; complete means bounded research finished, while candidate fit may still need verification. Observed prices may be saved locally and affiliate URLs may be added unless disable_affiliate=true. Never orders, posts reviews, or changes the cart.",
			Annotations: observingRead,
		}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductRecommendationRequest, provider ProductRecommendationProvider) (*mcp.CallToolResult, core.ProductRecommendationResult, error) {
			if err := input.Validate(); err != nil {
				return nil, core.ProductRecommendationResult{}, err
			}
			result, err := provider.Recommend(ctx, input)
			if err != nil {
				return nil, core.ProductRecommendationResult{}, err
			}
			return nil, result, nil
		})
	}
	if factory == nil {
		return
	}
	addProviderTool(server, factory, &mcp.Tool{
		Name:        "products_search",
		Description: "Search Coupang from natural language or a source-native starting category ID. First search without facet_selections to discover coverage.facets, then pass exact observed name/label pairs in facet_selections to narrow results. Rediscover facets after narrowing, especially after category selection: available filters vary by category. Category-only sidebar navigation requires a selected category label and matching native breadcrumb; coverage.applied_category_id identifies the verified destination, while applied_filters.category_id retains the starting scope. Requested query, sort and page are not silently replaced. Pass cumulative active selections with distinct groups on page one. For nested query categories, pass previously observed labels in category_trail and the final category in facet_selections; the trail is replayed, not treated as active filters. Trail plus active selections is limited to six navigation steps. more_available means additional options have not been expanded. Supports distinct Coupang ranking, sales, latest, price, rating, and review sorts; computer memory/storage and explicit used-item filters; and listing-level diversity by default. Explicitly observed returned prices are appended to private local price history. When Coupang Partners credentials are configured, each canonical URL may also have a separate affiliate_url plus a definite commission disclosure; set disable_affiliate=true to opt out. Search position is evidence of the selected Coupang result order, not absolute unit sales. Never invent an unobserved product field.",
		Annotations: observingRead,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductSearchRequest, provider ProductProvider) (*mcp.CallToolResult, core.ProductSearchResult, error) {
		result, err := provider.Search(ctx, input)
		if err != nil {
			return nil, core.ProductSearchResult{}, err
		}
		return nil, result, nil
	})
	addProviderTool(server, factory, &mcp.Tool{
		Name:        "product_inspect",
		Description: "Inspect one products_search candidate using its exact product_id, item_id and vendor_item_id: public details, gallery and detail images, current price, delivery, coupons or card benefits when observed, aggregate ratings, and sanitized reviews. selected_attributes preserves source-native labels/values mapped to that exact option with field_evidence; compound labels stay unsplit because label order or arity can be inconsistent. computer_specs is explicitly inferred from title/option text, not verified hardware specifications. Missing selected_attributes is unknown, not evidence of no options. An explicitly observed price is appended to private local price history. When configured, affiliate_url remains separate from the canonical URL and carries the same commission disclosure; set disable_affiliate=true to opt out. This never adds to cart or purchases.",
		Annotations: observingRead,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductInspectRequest, provider ProductProvider) (*mcp.CallToolResult, core.ProductInspection, error) {
		result, err := provider.Inspect(ctx, input)
		if err != nil {
			return nil, core.ProductInspection{}, err
		}
		return nil, result, nil
	})
	addProviderTool(server, factory, &mcp.Tool{
		Name:        "product_price_history",
		Description: "Read locally accumulated current-price observations for one product. Exact vendor options remain separate series, and trends are deterministic calculations over observations made by prior coupangctl searches or inspections. This does not claim retroactive Coupang price history.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductPriceHistoryRequest, provider ProductProvider) (*mcp.CallToolResult, core.ProductPriceHistory, error) {
		result, err := provider.PriceHistory(ctx, input)
		if err != nil {
			return nil, core.ProductPriceHistory{}, err
		}
		return nil, result, nil
	})
	addProviderTool(server, factory, &mcp.Tool{
		Name: "product_watchlist", Description: "List exact product identities selected for periodic local price observation. This does not access Coupang or refresh prices.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}, provider ProductProvider) (*mcp.CallToolResult, core.ProductWatchList, error) {
		result, err := provider.PriceWatchlist(ctx)
		if err != nil {
			return nil, core.ProductWatchList{}, err
		}
		return nil, result, nil
	})
	addProviderTool(server, factory, &mcp.Tool{
		Name:        "product_watch_add",
		Description: "Add an exact product identity to the local price watchlist. The identity must already have an observed price from products_search or product_inspect; names are never matched. This changes only local watchlist state.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductWatchRequest, provider ProductProvider) (*mcp.CallToolResult, core.ProductWatchMutationResult, error) {
		result, err := provider.AddPriceWatch(ctx, input)
		if err != nil {
			return nil, core.ProductWatchMutationResult{}, err
		}
		return nil, result, nil
	})
	addProviderTool(server, factory, &mcp.Tool{
		Name:        "product_watch_remove",
		Description: "Remove one exact identity from the local price watchlist without deleting its price history. This changes only local watchlist state.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: boolPointer(true), IdempotentHint: true, OpenWorldHint: boolPointer(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductWatchRequest, provider ProductProvider) (*mcp.CallToolResult, core.ProductWatchMutationResult, error) {
		result, err := provider.RemovePriceWatch(ctx, input)
		if err != nil {
			return nil, core.ProductWatchMutationResult{}, err
		}
		return nil, result, nil
	})
	addProviderTool(server, factory, &mcp.Tool{
		Name:        "product_watch_refresh",
		Description: "Refresh due exact watchlist identities through bounded public product inspection and append observed current prices locally. This never uses affiliate conversion, changes a cart, checks out, orders, or pays.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductWatchRefreshRequest, provider ProductProvider) (*mcp.CallToolResult, core.ProductWatchRefreshResult, error) {
		result, err := provider.RefreshPriceWatches(ctx, input)
		if err != nil {
			return nil, core.ProductWatchRefreshResult{}, err
		}
		return nil, result, nil
	})
	addProviderTool(server, factory, &mcp.Tool{
		Name:        "cart_add",
		Description: "Add one exact products_search candidate to the user's Coupang cart. Call only after the user explicitly asks to add that item and set confirmed=true. This changes external cart state but never purchases, orders, or pays. Never retry automatically when verification is inconclusive.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true),
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.CartAddRequest, provider ProductProvider) (*mcp.CallToolResult, core.CartAddResult, error) {
		result, err := provider.AddToCart(ctx, input)
		if err != nil {
			return nil, core.CartAddResult{}, err
		}
		return nil, result, nil
	})
}

func addOrderTools(server *mcp.Server, factory ProviderFactory[OrderProvider]) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(false)}
	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_sync_status", Description: "Read sync-status v3 latest local attempt and its cumulative scan in one local snapshot without contacting Coupang or opening a browser. scan is null for untracked legacy attempts; otherwise it reports committed pages across resumptions, the saved cursor, and distinct retained orders observed/not observed in that scan. active is persisted scan state, not process liveness. Cursor exhaustion and legacy completion flags do not verify account identity or complete history. Unobserved retained orders are not deleted or assumed canceled.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}, provider OrderProvider) (*mcp.CallToolResult, core.SyncStatus, error) {
		status, err := provider.SyncStatus(ctx)
		if err != nil {
			return nil, core.SyncStatus{}, err
		}
		return nil, status, nil
	})
	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_list", Description: "List normalized local orders using bounded date and result filters.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter, provider OrderProvider) (*mcp.CallToolResult, core.OrderList, error) {
		orders, err := provider.List(ctx, input)
		if err != nil {
			return nil, core.OrderList{}, err
		}
		return nil, core.OrderList{Orders: orders}, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_spend", Description: "Read spend v2 gross normalized order totals and ledger-wide sync evidence in one local snapshot for an optional date range. Does not open a browser. Retained history is not verified current-source coverage; capture time is not source freshness, and non-canceled totals are not net spending after refunds.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter, provider OrderProvider) (*mcp.CallToolResult, core.SpendSummary, error) {
		result, err := provider.Spend(ctx, input)
		if err != nil {
			return nil, core.SpendSummary{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_stats", Description: "Read stats v3 normalized order aggregates and ledger-wide sync evidence in one local snapshot for an optional date range. Cancellation/return rates are null when their denominator is zero; numeric zero means a computed zero rate over a positive denominator. Does not open a browser. Retained history is not verified current-source coverage and capture time is not source freshness. Existing missing-quantity and source-state limitations still apply.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter, provider OrderProvider) (*mcp.CallToolResult, core.OrderStats, error) {
		result, err := provider.Stats(ctx, input)
		if err != nil {
			return nil, core.OrderStats{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_insights", Description: "Read private local shopping-pattern v3 aggregates, categories, and ledger-wide sync evidence in one snapshot for an optional date range. Top-level timing, cancellation, return, delivery and brand rates are null for zero denominators; counts are in samples and top_brand. Time associations are not causal effects. Nested legacy metrics retain their existing missing-data limitations. Does not open a browser. Capture time is not source freshness and retained history is not verified date-range coverage. This raw response is not public-safe; public recap sharing has a separate field allowlist.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter, provider OrderProvider) (*mcp.CallToolResult, core.ShoppingInsights, error) {
		result, err := provider.Insights(ctx, input)
		if err != nil {
			return nil, core.ShoppingInsights{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_product_insights", Description: "Read private local product-insights v2 names, paid amounts, exact spend days, and ledger-wide sync evidence in one snapshot for an optional date range. Does not open a browser. Capture time is not source freshness; retained history is not verified date-range coverage or reconciled net spending after refunds.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter, provider OrderProvider) (*mcp.CallToolResult, core.ProductInsights, error) {
		result, err := provider.ProductInsights(ctx, input)
		if err != nil {
			return nil, core.ProductInsights{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_category_catalog", Description: "Find source-native Coupang category IDs and observed breadcrumb paths from the private local order ledger. Use a returned category_id with products_search instead of guessing a taxonomy label or ID. Local observed-product counts are not Coupang popularity.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.CategoryCatalogRequest, provider OrderProvider) (*mcp.CallToolResult, core.CategoryCatalog, error) {
		result, err := provider.CategoryCatalog(ctx, input)
		if err != nil {
			return nil, core.CategoryCatalog{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_category_stability", Description: "Report whether exact source-native breadcrumb paths changed across locally retained observations. Counts, observation days, insufficient-evidence states, and the single-ledger limitation remain explicit; this does not claim population-wide stability.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}, provider OrderProvider) (*mcp.CallToolResult, core.CategoryStabilityReport, error) {
		result, err := provider.CategoryStability(ctx)
		if err != nil {
			return nil, core.CategoryStabilityReport{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_reorder_candidates", Description: "List locally derived repeat-purchase candidates without placing an order.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter, provider OrderProvider) (*mcp.CallToolResult, core.ReorderList, error) {
		candidates, err := provider.ReorderCandidates(ctx, input)
		if err != nil {
			return nil, core.ReorderList{}, err
		}
		return nil, core.NewReorderList(candidates), nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_export", Description: "Export bounded normalized local order data. Browser state and raw payloads are never included.", Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.OrderFilter, provider OrderProvider) (*mcp.CallToolResult, core.OrderExport, error) {
		result, err := provider.Export(ctx, input)
		if err != nil {
			return nil, core.OrderExport{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_enrich_categories", Description: "Cache source-native Coupang product breadcrumb categories for uncached local products. Set recheck=true only to explicitly append fresh observations for cached products, oldest cache entries first.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true),
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.CategoryEnrichmentRequest, provider OrderProvider) (*mcp.CallToolResult, core.CategoryEnrichmentResult, error) {
		result, err := provider.EnrichCategories(ctx, input)
		if err != nil {
			return nil, core.CategoryEnrichmentResult{}, err
		}
		return nil, result, nil
	})

	addProviderTool(server, factory, &mcp.Tool{
		Name: "orders_sync", Description: "Refresh the normalized local ledger from the authenticated dedicated browser profile.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false, DestructiveHint: boolPointer(false), IdempotentHint: false, OpenWorldHint: boolPointer(true),
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.SyncRequest, provider OrderProvider) (*mcp.CallToolResult, core.SyncResult, error) {
		result, err := provider.Sync(ctx, input)
		if err != nil {
			return nil, core.SyncResult{}, err
		}
		return nil, result, nil
	})
}

func boolPointer(value bool) *bool { return &value }
