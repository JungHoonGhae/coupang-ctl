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
	SchemaVersion int               `json:"schema_version"`
	Summary       CapabilitySummary `json:"summary"`
	Capabilities  []Capability      `json:"capabilities"`
}

type CapabilitySummary struct {
	Total                             int                      `json:"total"`
	StatusCounts                      CapabilityStatusCounts   `json:"status_counts"`
	NextStepCounts                    CapabilityNextStepCounts `json:"next_step_counts"`
	ImplementationNextSteps           int                      `json:"implementation_next_steps"`
	ValidationOrCoordinationNextSteps int                      `json:"validation_or_coordination_next_steps"`
}

type CapabilityStatusCounts struct {
	Available    int `json:"available"`
	Experimental int `json:"experimental"`
	Researched   int `json:"researched"`
	Planned      int `json:"planned"`
}

type CapabilityNextStepCounts struct {
	Maintenance            int `json:"maintenance"`
	Implementation         int `json:"implementation"`
	EvidenceRequired       int `json:"evidence_required"`
	LiveValidation         int `json:"live_validation"`
	ExternalDependency     int `json:"external_dependency"`
	UserAuthorization      int `json:"user_authorization"`
	LongitudinalValidation int `json:"longitudinal_validation"`
}

func CurrentCapabilities() CapabilityReport {
	capabilities := []Capability{
		{
			ID: "native_auth_session", Priority: "P0", Status: CapabilityAvailable,
			UserValue:    "one headed login followed by reusable read-only sessions, with visible QR, private PNG, or explicit ephemeral app-link presentation",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"headed QR and app-link login plus headed phone/OTP UI assistance that leaves CAPTCHA to the user", "browser-owned session state in a dedicated local Chrome profile without a separate cookie export", "all default status, verification, CLI, MCP, and scheduled reads stay non-visible without opening a headed fallback", "confirmation-gated MCP login-if-needed that quietly checks first and opens QR only for a missing or expired session", "typed access_blocked status for a denied passive check without guessing logout or opening login", "command-aware access-denied remediation that recommends only supported modes and never repeats an already-failed headed or current-browser mode", "quiet doctor checks that separate browser installation, background-session readiness, and SQLite health", "one bounded same-session retry for transient protected-read denial", "explicit --headed mode for a user-requested visible attempt", "ephemeral loopback debugging port discovered through a validated private DevToolsActivePort file", "cross-platform non-blocking profile lock with a stable profile_in_use error", "private browser-family and major-version profile marker that permits upgrades and rejects family changes or downgrades", "native Linux, macOS, and Windows profile compatibility contracts that gate tagged releases"},
			NextStepKind: CapabilityNextMaintenance, LastVerified: "2026-09-03",
		},
		{
			ID: "current_browser_connection", Priority: "P0", Status: CapabilityExperimental,
			UserValue:    "extension-free order sync through a running Chrome browser after Chrome's own explicit remote-debugging opt-in and connection approval",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"passive current-browser status CLI command and current_browser_status MCP tool without debugger attachment or endpoint disclosure", "native Linux, macOS, and Windows status contracts that gate tagged releases", "explicit --current-browser CLI mode and orders_sync_current_browser MCP tool", "private DevToolsActivePort validation with loopback-only endpoint verification", "one allowlisted product-created tab per read", "disconnect without closing Chrome or copying browser session state"},
			NextWork:     "validate Chrome 144+ approval and repeated order-sync behavior on clean desktop profiles across macOS, Windows, and Linux",
			NextStepKind: CapabilityNextLiveValidation, LastVerified: "2026-09-03",
		},
		{
			ID: "ordinary_browser_bridge", Priority: "P0", Status: CapabilityExperimental,
			UserValue:    "optional selected-tab compatibility path when dedicated and approved current-browser modes are unsuitable",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"same-account ordinary-versus-dedicated Chrome comparison with redacted success markers", "MV3 activeTab action with isolated top-frame execution and no cookie or host permission", "Chrome Native Messaging plus an authenticated single-use CLI rendezvous", "closed normalized order-page protocol with independent extension and Go validation", "embedded per-user install, seven-check doctor with a synthetic native-host ping, ownership-checked uninstall, and digest-authorized resumable upgrades on macOS, Linux, and Windows", "native Windows CI coverage for isolated real-HKCU install, doctor, conflict, and uninstall", "CLI and MCP typed sync surfaces", "four consecutive redacted live one-page CLI-to-SQLite reads after a managed macOS host install, including the installed extension bundle", "six-target allowlisted release archives with checksums, per-archive SBOMs, and provenance-ready tag automation"},
			NextWork:     "validate clean Chrome profiles and Linux and Windows installations, complete Chrome Web Store privacy review and distribution, and add native OS signing",
			NextStepKind: CapabilityNextLiveValidation, LastVerified: "2026-09-03",
		},
		{
			ID: "full_order_history", Priority: "P0", Status: CapabilityAvailable,
			UserValue:    "complete normalized order history with resumable synchronization",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"bounded cursor pagination", "resumable SQLite checkpoints", "complete-history reconciliation without duplicate orders", "schema-versioned sync source and observed provenance selected by the acquisition workflow", "browserless CLI and MCP latest-sync audit with timestamps, counts, failure code, and complete-history evidence", "explicit unknown_legacy provenance for pre-migration sync runs"},
			NextStepKind: CapabilityNextMaintenance, LastVerified: "2026-09-03",
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
			UserValue:   "current WOW state and fee, source fee-change schedule metadata, source-labeled recent benefit usage, an explicitly inferred current-fee comparison, registered payment-method brands, and observed monthly WOW Card cash rewards",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"current WOW status, current monthly fee, and source fee-change date", "observed recent benefit amount and window", "monthly WOW Card rewards and registered card brands", "explicit-only membership cost aggregation with inferred comparison kept separate"},
			NextWork:    "adopt membership-fee receipt evidence for exact historical costs; the complete live order history has no membership-specific item metadata", NextStepKind: CapabilityNextEvidenceRequired,
			BlockedBy: []string{"the current account exposes no explicit historical membership-payment rows in the synchronized order data or available receipt history"}, LastVerified: "2026-09-03",
		},
		{
			ID: "purchase_delivery_trends", Priority: "P0", Status: CapabilityAvailable,
			UserValue:    "purchase hour, weekday, month, and delivery-duration trends",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"KST purchase hour and weekday series", "monthly order and spend series", "average, median, p90, and sample-count yearly delivery durations", "schema-versioned baseline-to-latest average, median, and p90 deltas with direct sample evidence", "typed definition that direction uses average hours", "worked multi-sample distribution contract in the full release test suite"},
			NextStepKind: CapabilityNextMaintenance, LastVerified: "2026-09-03",
		},
		{
			ID: "shopping_type_recap", Priority: "P0", Status: CapabilityAvailable,
			UserValue:   "explainable four-axis shopping type, achievements, public-safe or explicitly private HTML, and a preview-gated public-safe PNG share card",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"versioned four-axis evidence rules", "16 doodle characters and deterministic badges", "public-safe and explicit private-product standalone HTML", "two-step exact-field preview and 1080x1350 public-safe PNG export", "release-gated one-to-one field ID and value contract between the public preview and visible share-card elements"},
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
			ID: "natural_language_product_discovery", Priority: "P0", Status: CapabilityAvailable,
			UserValue:   "AI-friendly product search and inspection with current price, normalized computer-title specifications, public images, selected options when observed, delivery, benefits, ratings, and sanitized reviews",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"typed natural-language search filters", "separate exact product inspection", "current price, public images, delivery, benefit, rating, review, and computer-spec evidence with per-field coverage", "bounded selected-option extraction with parser-reconciled unavailable states for missing option labels and unobserved card benefits"},
			NextWork:    "keep layout coverage and honest unavailable-field behavior in release checks", NextStepKind: CapabilityNextMaintenance, LastVerified: "2026-09-03",
		},
		{
			ID: "source_native_product_rankings", Priority: "P0", Status: CapabilityAvailable,
			UserValue:   "product-type or observed real-category rankings with label-to-ID discovery and separate Coupang ranking, sales, latest, price, rating, and review semantics",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"ordinary query and source-native category-ID scopes", "separate Coupang ranking, sales, latest, and price controls", "source order preserved without treating unavailable prices as zero", "local rating and review-count sorts limited to explicitly observed fields", "observed local category label-to-ID catalog"},
			NextWork:    "keep query and observed-category sort semantics in release checks", NextStepKind: CapabilityNextMaintenance, LastVerified: "2026-09-03",
		},
		{
			ID: "transparent_affiliate_deeplinks", Priority: "P0", Status: CapabilityExperimental,
			UserValue:   "optional Coupang Partners deep links alongside canonical URLs, with commission disclosure, price verification notice, and per-request or global opt-out",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"official signed Partners deeplink adapter", "canonical and affiliate URLs kept separate", "definite commission disclosure, price notice, self-purchase exclusion, and opt-out"},
			NextWork:    "complete final channel approval, then issue API keys and validate the official deeplink response live without recording credentials", NextStepKind: CapabilityNextExternalDependency,
			BlockedBy: []string{"the live Partners API page disables API-key generation until final approval is complete"}, LastVerified: "2026-09-03",
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
			UserValue:   "source-native category totals, a searchable observed label/ID catalog, and longitudinal path-stability evidence with no product-name guessing",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"bounded resumable breadcrumb enrichment", "leaf category aggregates with classification coverage", "local label-to-ID path catalog for AI search handoff", "append-only category observations, explicit bounded rechecks, and same-product multi-day stability assessment"},
			NextWork:    "broaden the live recheck sample and validate breadcrumb-path behavior across additional consenting accounts", NextStepKind: CapabilityNextLongitudinalValidation,
			BlockedBy: []string{"the first five-product multi-day recheck had no path changes, but cross-account validation requires additional consenting account samples"}, LastVerified: "2026-09-03",
		},
		{
			ID: "price_and_repurchase", Priority: "P2", Status: CapabilityExperimental,
			UserValue:   "exact-option local price history, last-paid-unit comparison, bounded watch refresh, and reviewable daily scheduler artifacts",
			Interface:   []string{"cli", "mcp"},
			Implemented: []string{"exact vendor-item price observations", "local history and last-paid-unit comparison", "bounded due-only watchlist refresh", "reviewable launchd, systemd, cron, and Windows daily schedule artifacts"},
			NextWork:    "validate real longitudinal price changes before tuning the 24-hour threshold", NextStepKind: CapabilityNextLongitudinalValidation,
			BlockedBy: []string{"real price-change validation requires future observations of the same exact option"}, LastVerified: "2026-09-03",
		},
	}
	return CapabilityReport{
		SchemaVersion: 3,
		Summary:       summarizeCapabilities(capabilities),
		Capabilities:  capabilities,
	}
}

func summarizeCapabilities(capabilities []Capability) CapabilitySummary {
	result := CapabilitySummary{Total: len(capabilities)}
	for _, capability := range capabilities {
		switch capability.Status {
		case CapabilityAvailable:
			result.StatusCounts.Available++
		case CapabilityExperimental:
			result.StatusCounts.Experimental++
		case CapabilityResearched:
			result.StatusCounts.Researched++
		case CapabilityPlanned:
			result.StatusCounts.Planned++
		}
		switch capability.NextStepKind {
		case CapabilityNextMaintenance:
			result.NextStepCounts.Maintenance++
		case CapabilityNextImplementation:
			result.NextStepCounts.Implementation++
			result.ImplementationNextSteps++
		case CapabilityNextEvidenceRequired:
			result.NextStepCounts.EvidenceRequired++
			result.ValidationOrCoordinationNextSteps++
		case CapabilityNextLiveValidation:
			result.NextStepCounts.LiveValidation++
			result.ValidationOrCoordinationNextSteps++
		case CapabilityNextExternalDependency:
			result.NextStepCounts.ExternalDependency++
			result.ValidationOrCoordinationNextSteps++
		case CapabilityNextUserAuthorization:
			result.NextStepCounts.UserAuthorization++
			result.ValidationOrCoordinationNextSteps++
		case CapabilityNextLongitudinalValidation:
			result.NextStepCounts.LongitudinalValidation++
			result.ValidationOrCoordinationNextSteps++
		}
	}
	return result
}
