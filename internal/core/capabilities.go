package core

type CapabilityStatus string

const (
	CapabilityAvailable    CapabilityStatus = "available"
	CapabilityExperimental CapabilityStatus = "experimental"
	CapabilityResearched   CapabilityStatus = "researched"
	CapabilityPlanned      CapabilityStatus = "planned"
)

type Capability struct {
	ID           string           `json:"id"`
	Priority     string           `json:"priority"`
	Status       CapabilityStatus `json:"status"`
	UserValue    string           `json:"user_value"`
	Interface    []string         `json:"interface"`
	NextWork     string           `json:"next_work,omitempty"`
	LastVerified string           `json:"last_verified,omitempty"`
}

type CapabilityReport struct {
	SchemaVersion int          `json:"schema_version"`
	Capabilities  []Capability `json:"capabilities"`
}

func CurrentCapabilities() CapabilityReport {
	return CapabilityReport{SchemaVersion: 1, Capabilities: []Capability{
		{
			ID: "native_auth_session", Priority: "P0", Status: CapabilityAvailable,
			UserValue: "one headed login followed by reusable read-only sessions, with visible QR, private PNG, or explicit ephemeral app-link presentation",
			Interface: []string{"cli", "mcp"}, LastVerified: "2026-09-02",
		},
		{
			ID: "full_order_history", Priority: "P0", Status: CapabilityAvailable,
			UserValue: "complete normalized order history with resumable synchronization",
			Interface: []string{"cli", "mcp"}, LastVerified: "2026-09-02",
		},
		{
			ID: "spend_cancel_return_stats", Priority: "P0", Status: CapabilityAvailable,
			UserValue: "gross ledger spend plus explicit product-purchase, membership-fee, cancellation, and return breakdowns",
			Interface: []string{"cli", "mcp"}, LastVerified: "2026-09-02",
		},
		{
			ID: "account_membership_benefits", Priority: "P0", Status: CapabilityExperimental,
			UserValue: "current WOW state and fee, Coupang-reported benefit usage, registered payment-method brands, and observed monthly WOW Card cash rewards",
			Interface: []string{"cli", "mcp"}, NextWork: "adopt historical membership-payment evidence before computing lifetime fees or net value", LastVerified: "2026-09-02",
		},
		{
			ID: "purchase_delivery_trends", Priority: "P0", Status: CapabilityAvailable,
			UserValue: "purchase hour, weekday, month, and delivery-duration trends",
			Interface: []string{"cli", "mcp"}, LastVerified: "2026-09-02",
		},
		{
			ID: "shopping_type_recap", Priority: "P0", Status: CapabilityAvailable,
			UserValue: "explainable four-axis shopping type, achievements, and public-safe or explicitly private standalone HTML recaps",
			Interface: []string{"cli", "mcp"}, LastVerified: "2026-09-02",
		},
		{
			ID: "private_product_insights", Priority: "P0", Status: CapabilityAvailable,
			UserValue: "product-level quantity, purchase-frequency, spend, paid-unit, and spend-day receipts with explicit coverage",
			Interface: []string{"cli", "mcp"}, LastVerified: "2026-09-02",
		},
		{
			ID: "natural_language_product_discovery", Priority: "P0", Status: CapabilityExperimental,
			UserValue: "AI-friendly product search and inspection with current price, normalized computer-title specifications, public images, selected options when observed, delivery, benefits, ratings, and sanitized reviews",
			Interface: []string{"cli", "mcp"}, NextWork: "validate selected-option and structured card-benefit coverage across more layouts without guessing missing fields", LastVerified: "2026-09-02",
		},
		{
			ID: "source_native_product_rankings", Priority: "P0", Status: CapabilityExperimental,
			UserValue: "product-type or real-category rankings with separate Coupang ranking, sales, latest, price, rating, and review semantics",
			Interface: []string{"cli", "mcp"}, NextWork: "discover source-native category labels without requiring callers to supply a numeric category ID and validate selected-option evidence across layouts", LastVerified: "2026-09-02",
		},
		{
			ID: "transparent_affiliate_deeplinks", Priority: "P0", Status: CapabilityExperimental,
			UserValue: "optional Coupang Partners deep links alongside canonical URLs, with commission disclosure, price verification notice, and per-request or global opt-out",
			Interface: []string{"cli", "mcp"}, NextWork: "await final channel approval, issue API keys, and validate the official deeplink response live without recording credentials", LastVerified: "2026-09-03",
		},
		{
			ID: "explicit_cart_add", Priority: "P1", Status: CapabilityExperimental,
			UserValue: "add an exact searched vendor item to cart after explicit user confirmation without checkout or payment",
			Interface: []string{"cli", "mcp"}, NextWork: "verify the reversible mutation on an explicitly authorized live item; never auto-retry an unverified attempt",
		},
		{
			ID: "batch_receipts", Priority: "P1", Status: CapabilityExperimental,
			UserValue: "read cash and card request status, history, and summaries, then privately download an already-completed archive without exposing its URL",
			Interface: []string{"cli", "mcp"}, NextWork: "validate a completed archive download live and research vendor-receipt reads; request creation remains excluded", LastVerified: "2026-09-03",
		},
		{
			ID: "payment_method_installment_insights", Priority: "P1", Status: CapabilityExperimental,
			UserValue: "show observed payment-method counts and amounts while keeping lump-sum and installment statistics explicitly unavailable until sales-slip evidence exposes them",
			Interface: []string{"cli", "mcp"}, NextWork: "capture a redacted sales-slip detail shape and adopt installment months only if explicitly observed", LastVerified: "2026-09-03",
		},
		{
			ID: "product_categories", Priority: "P1", Status: CapabilityExperimental,
			UserValue: "source-native category totals with explicit coverage and no product-name guessing",
			Interface: []string{"cli", "mcp"}, NextWork: "validate breadcrumb-path stability over time and across more accounts", LastVerified: "2026-09-02",
		},
		{
			ID: "price_and_repurchase", Priority: "P2", Status: CapabilityPlanned,
			UserValue: "price history and higher-confidence repurchase suggestions",
			Interface: []string{"cli", "mcp"}, NextWork: "stabilize category and receipt adapters first",
		},
	}}
}
