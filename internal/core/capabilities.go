package core

type CapabilityStatus string
type CapabilityNextStepKind string

const (
	CapabilityAvailable    CapabilityStatus = "available"
	CapabilityExperimental CapabilityStatus = "experimental"
	CapabilityResearched   CapabilityStatus = "researched"
	CapabilityPlanned      CapabilityStatus = "planned"
)

const (
	CapabilityNextMaintenance            CapabilityNextStepKind = "maintenance"
	CapabilityNextImplementation         CapabilityNextStepKind = "implementation"
	CapabilityNextEvidenceRequired       CapabilityNextStepKind = "evidence_required"
	CapabilityNextLiveValidation         CapabilityNextStepKind = "live_validation"
	CapabilityNextExternalDependency     CapabilityNextStepKind = "external_dependency"
	CapabilityNextUserAuthorization      CapabilityNextStepKind = "user_authorization"
	CapabilityNextLongitudinalValidation CapabilityNextStepKind = "longitudinal_validation"
)

type Capability struct {
	ID           string                 `json:"id"`
	Priority     string                 `json:"priority"`
	Status       CapabilityStatus       `json:"status"`
	UserValue    string                 `json:"user_value"`
	Interface    []string               `json:"interface"`
	Implemented  []string               `json:"implemented"`
	NextWork     string                 `json:"next_work,omitempty"`
	NextStepKind CapabilityNextStepKind `json:"next_step_kind,omitempty"`
	BlockedBy    []string               `json:"blocked_by,omitempty"`
	LastVerified string                 `json:"last_verified,omitempty"`
}

type CapabilityReport struct {
	SchemaVersion int          `json:"schema_version"`
	Capabilities  []Capability `json:"capabilities"`
}

func CurrentCapabilities() CapabilityReport {
	return CapabilityReport{SchemaVersion: 2, Capabilities: []Capability{
		{
			ID: "native_auth_session", Priority: "P0", Status: CapabilityAvailable,
			UserValue:    "one headed login followed by reusable read-only sessions, with visible QR, private PNG, or explicit ephemeral app-link presentation",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"headed QR, app-link, and manual SMS login", "dedicated local Chrome profile with structured session persistence", "headless-first authenticated verification and reads with headed fallback where available"},
			NextStepKind: CapabilityNextMaintenance, LastVerified: "2026-09-02",
		},
		{
			ID: "full_order_history", Priority: "P0", Status: CapabilityAvailable,
			UserValue:    "complete normalized order history with resumable synchronization",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"bounded cursor pagination", "resumable SQLite checkpoints", "complete-history reconciliation without duplicate orders"},
			NextStepKind: CapabilityNextMaintenance, LastVerified: "2026-09-02",
		},
		{
			ID: "spend_cancel_return_stats", Priority: "P0", Status: CapabilityAvailable,
			UserValue:    "gross ledger spend plus explicit product-purchase, membership-fee, cancellation, and return breakdowns",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"gross and non-canceled spend", "product, explicit membership, and unclassified buckets", "order, item, cancellation, and return statistics"},
			NextStepKind: CapabilityNextEvidenceRequired,
			BlockedBy:    []string{"vendor receipts expose source-native cancellation components, but exact post-refund net spend still requires verified settlement status and agreement across canceled and returned samples"}, LastVerified: "2026-09-03",
		},
		{
			ID: "account_membership_benefits", Priority: "P0", Status: CapabilityExperimental,
			UserValue:   "current WOW state and fee, source-labeled recent benefit usage, an explicitly inferred current-fee comparison, registered payment-method brands, and observed monthly WOW Card cash rewards",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"current WOW status and current monthly fee", "observed recent benefit amount and window", "monthly WOW Card rewards and registered card brands", "explicit-only membership cost aggregation with inferred comparison kept separate"},
			NextWork:    "adopt membership-fee receipt evidence for exact historical costs; the complete live order history has no membership-specific item metadata", NextStepKind: CapabilityNextEvidenceRequired,
			BlockedBy: []string{"the current account exposes no explicit historical membership-payment rows in the synchronized order data or available receipt history"}, LastVerified: "2026-09-03",
		},
		{
			ID: "purchase_delivery_trends", Priority: "P0", Status: CapabilityAvailable,
			UserValue:    "purchase hour, weekday, month, and delivery-duration trends",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"KST purchase hour and weekday series", "monthly order and spend series", "average, median, p90, yearly delivery durations, and baseline-to-latest trend"},
			NextStepKind: CapabilityNextMaintenance, LastVerified: "2026-09-02",
		},
		{
			ID: "shopping_type_recap", Priority: "P0", Status: CapabilityAvailable,
			UserValue:   "explainable four-axis shopping type, achievements, public-safe or explicitly private HTML, and a preview-gated public-safe PNG share card",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"versioned four-axis evidence rules", "16 doodle characters and deterministic badges", "public-safe and explicit private-product standalone HTML", "two-step exact-field preview and 1080x1350 public-safe PNG export"},
			NextWork:    "keep the share-field preview and rendered image content aligned in release tests", NextStepKind: CapabilityNextMaintenance, LastVerified: "2026-09-03",
		},
		{
			ID: "private_product_insights", Priority: "P0", Status: CapabilityAvailable,
			UserValue:    "product-level quantity, purchase-frequency, spend, paid-unit, and spend-day receipts with explicit coverage",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"top products by retained units, orders, and paid amount", "highest and lowest eligible paid-unit highlights", "highest and lowest spend-day product receipts with ID coverage"},
			NextStepKind: CapabilityNextMaintenance, LastVerified: "2026-09-02",
		},
		{
			ID: "natural_language_product_discovery", Priority: "P0", Status: CapabilityExperimental,
			UserValue:   "AI-friendly product search and inspection with current price, normalized computer-title specifications, public images, selected options when observed, delivery, benefits, ratings, and sanitized reviews",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"typed natural-language search filters", "separate exact product inspection", "current price, public images, delivery, benefit, rating, review, and computer-spec evidence with per-field coverage"},
			NextWork:    "validate selected-option and structured card-benefit coverage across more layouts without guessing missing fields", NextStepKind: CapabilityNextLiveValidation,
			BlockedBy: []string{"the latest repeated public detail probe was denied in both headless and CDP-controlled headed Chrome; retry later without bypassing the access control"}, LastVerified: "2026-09-02",
		},
		{
			ID: "source_native_product_rankings", Priority: "P0", Status: CapabilityExperimental,
			UserValue:   "product-type or observed real-category rankings with label-to-ID discovery and separate Coupang ranking, sales, latest, price, rating, and review semantics",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"ordinary query and source-native category-ID scopes", "separate Coupang ranking, sales, latest, and price controls", "honest local rating and review-count sorts", "observed local category label-to-ID catalog"},
			NextWork:    "repeat current category-search availability in release checks and validate selected-option evidence across more layouts", NextStepKind: CapabilityNextLiveValidation, LastVerified: "2026-09-03",
		},
		{
			ID: "transparent_affiliate_deeplinks", Priority: "P0", Status: CapabilityExperimental,
			UserValue:   "optional Coupang Partners deep links alongside canonical URLs, with commission disclosure, price verification notice, and per-request or global opt-out",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"official signed Partners deeplink adapter", "canonical and affiliate URLs kept separate", "definite commission disclosure, price notice, self-purchase exclusion, and opt-out"},
			NextWork:    "await final channel approval, issue API keys, and validate the official deeplink response live without recording credentials", NextStepKind: CapabilityNextExternalDependency,
			BlockedBy: []string{"Coupang Partners final channel approval and official API key issuance"}, LastVerified: "2026-09-03",
		},
		{
			ID: "explicit_cart_add", Priority: "P1", Status: CapabilityExperimental,
			UserValue:   "add an exact searched vendor item to cart after explicit user confirmation without checkout or payment",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"exact observed vendor-item validation", "explicit confirmation gate", "single reversible cart mutation with no automatic retry", "checkout, order, and payment controls excluded"},
			NextWork:    "verify the reversible mutation on an explicitly authorized live item; never auto-retry an unverified attempt", NextStepKind: CapabilityNextUserAuthorization,
			BlockedBy: []string{"a specific live item and explicit authorization are required for the mutation test"},
		},
		{
			ID: "batch_receipts", Priority: "P1", Status: CapabilityExperimental,
			UserValue:   "read cash and card request availability, history, single-period and multi-year summaries; inspect one order's vendor receipt by hashed reference; then privately download an already-completed archive without exposing its URL",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"cash and card request availability", "bounded existing request history", "single-period and multi-year receipt totals", "payment-method rankings across calendar-year reads", "source-native vendor receipt by hashed order reference", "private new-file download of an already-completed archive"},
			NextWork:    "validate a completed archive download live; request creation remains excluded", NextStepKind: CapabilityNextEvidenceRequired,
			BlockedBy: []string{"the current receipt history has no completed archive item to validate"}, LastVerified: "2026-09-03",
		},
		{
			ID: "payment_method_installment_insights", Priority: "P1", Status: CapabilityExperimental,
			UserValue:   "show multi-year receipt-summary payment-method totals and one order's source-native vendor payment method while keeping lump-sum and installment statistics explicitly unavailable until an installment field is observed",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"observed receipt payment-method display names", "per-method receipt counts and amounts", "multi-year non-overlapping calendar-year aggregation", "per-order vendor payment type and cancellation components", "installment status explicitly unavailable rather than inferred"},
			NextWork:    "adopt installment months only if an explicit field is observed in a source-native receipt response", NextStepKind: CapabilityNextEvidenceRequired,
			BlockedBy: []string{"no explicit installment-month field was present in the verified receipt summary, history, or vendor-receipt response shapes"}, LastVerified: "2026-09-03",
		},
		{
			ID: "product_categories", Priority: "P1", Status: CapabilityExperimental,
			UserValue:   "source-native category totals and a searchable observed label/ID catalog with explicit coverage and no product-name guessing",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"bounded resumable breadcrumb enrichment", "leaf category aggregates with classification coverage", "local label-to-ID path catalog for AI search handoff"},
			NextWork:    "validate breadcrumb-path stability over time and across more accounts", NextStepKind: CapabilityNextLongitudinalValidation,
			BlockedBy: []string{"path stability requires observations at later dates and additional consenting account samples"}, LastVerified: "2026-09-03",
		},
		{
			ID: "price_and_repurchase", Priority: "P2", Status: CapabilityExperimental,
			UserValue:   "exact-option local price history, last-paid-unit comparison, bounded watch refresh, and reviewable daily scheduler artifacts",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"exact vendor-item price observations", "local history and last-paid-unit comparison", "bounded due-only watchlist refresh", "reviewable launchd, systemd, cron, and Windows daily schedule artifacts"},
			NextWork:    "validate real longitudinal price changes before tuning the 24-hour threshold", NextStepKind: CapabilityNextLongitudinalValidation,
			BlockedBy: []string{"real price-change validation requires future observations of the same exact option"}, LastVerified: "2026-09-03",
		},
	}}
}
