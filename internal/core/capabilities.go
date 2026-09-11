package core

import "sort"

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
			ID: "recommendation_report", Priority: "P1", Status: CapabilityExperimental,
			UserValue:    "inspect supplied recommendation evidence without starting another shopping session",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"products report and products_report_render share an offline bounded report v2 renderer for recommendation v5", "source query/category scope, verified selections, unverified choices, incomplete outcomes, missing values and provenance are preserved", "bounded live category research and MCP report rendering retain incomplete evidence without claiming recommendation sufficiency", "default response is private-local HTML in JSON with no file creation; CLI --output creates a new mode-0600 file without overwriting"},
			NextWork:     "verify browser rendering/accessibility and user understanding across broader recommendations; supplied evidence is not independently verified by rendering",
			NextStepKind: CapabilityNextLiveValidation, LastVerified: "2026-09-11",
		},
		{
			ID: "native_auth_session", Priority: "P0", Status: CapabilityAvailable,
			UserValue:    "dedicated Camofox authentication reused by headless CLI and MCP reads",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"isolated persisted Camofox profile without importing daily-use browser data", "explicit manual authentication or ephemeral QR app link", "source-native boolean authentication check independent of order parsing, with authenticated_session verification scope", "verified sessions do not open a login window; unknown authentication is not treated as expiration", "blocked reads never silently select another browser", "local runtime checks and profile locking"},
			NextWork:     "complete fresh mobile approval and release-platform verification; SMS automation and QR file export are not supported",
			NextStepKind: CapabilityNextLiveValidation, LastVerified: "2026-09-11",
		},
		{
			ID: "full_order_history", Priority: "P0", Status: CapabilityExperimental,
			UserValue:    "retained normalized order history with resumable acquisition and explicit unverified coverage",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"orders preview and orders_preview read one current source entry page without opening or persisting a ledger; identity and complete history remain unverified", "bounded cursor pagination", "atomic page, attempt observations and resumable SQLite checkpoint commits", "unobserved local orders are retained, not deleted or marked cancelled", "sync result v2 separates cursor exhaustion from unverified coverage; sync status v3 adds cumulative scan evidence across attempts", "browserless CLI, MCP and aggregate snapshots expose scan page/attempt counts, saved cursor and distinct retained observed/unobserved order counts", "legacy completion flags are preserved in storage but are not verified whole-account coverage"},
			NextWork:     "connect verified source-account identity and account-scoped ledgers, then validate requested-range and source-end evidence before claiming complete account history",
			NextStepKind: CapabilityNextImplementation, LastVerified: "2026-09-11",
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
			UserValue:    "source-backed membership costs and benefits with separate observation windows",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"bounded headless Camofox membership and fixed cash GET reads under one dedicated-profile lock", "private-local schema v6 preserves missing versus observed zero/false for membership flags, benefit components, usage counts and recurring-payment summaries", "sequential cash pagination retains partial versus source-terminated coverage", "raw payment identifiers and cash descriptions are discarded in-page", "live CLI two-page and MCP one-page reads verified; both retained partial history"},
			NextWork:     "connect verified historical membership receipts before claiming exact paid costs or net benefits; broaden live account coverage",
			NextStepKind: CapabilityNextImplementation,
			LastVerified: "2026-09-11",
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
			ID: "source_native_product_rankings", Priority: "P0", Status: CapabilityExperimental,
			UserValue:    "query- or category-based rankings with observed sidebar choices and explicit scope",
			Interface:    []string{"cli", "mcp"},
			Implemented:  []string{"Camofox query or source-native category-ID search with bounded sidebar discovery and verified cumulative selections", "category bestAsc and query scoreDesc retain their source-specific Coupang-ranking semantics", "CLI and MCP category-only requests and memory-filtered budget searches verified", "explicit category-only sidebar navigation requires both the selected label and a matching native destination breadcrumb; starting and applied category IDs remain separate", "ordered category answers preserve historical catalogs and only the last verified active category; a two-hop path was live-checked through CLI, MCP and report rendering", "recommendations preserve category identity across refinement, sort/page discovery and report output; bounded live category research reached all five sorts and exact detail inspection", "source ranking preserved separately from local rating/review sorts", "missing prices never become zero-price evidence"},
			NextWork:     "extend ordered category refinement to query-based discovery, diagnose intermittent filter-verification failures, and validate more destination categories and sort-preserving transitions",
			NextStepKind: CapabilityNextImplementation, LastVerified: "2026-09-11",
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
			ID: "explicit_cart_add", Priority: "P1", Status: CapabilityPlanned,
			UserValue:    "review exact products before any separately authorized reversible cart action",
			Interface:    []string{},
			Implemented:  []string{"typed exact-identity and explicit-confirmation validation retained; Camofox mutations are disabled"},
			NextWork:     "separate future scope and approval are required for cart integration; checkout and payment remain excluded",
			NextStepKind: CapabilityNextImplementation,
			BlockedBy:    []string{"no Camofox cart mutation is implemented"},
		},
		{
			ID: "batch_receipts", Priority: "P1", Status: CapabilityPlanned,
			UserValue:    "private receipt history and non-overwriting downloads",
			Interface:    []string{},
			Implemented:  []string{"typed receipt parsers, summaries and private file-write checks retained"},
			NextWork:     "connect and validate bounded Camofox receipt reads; current CLI returns unsupported",
			NextStepKind: CapabilityNextImplementation,
			BlockedBy:    []string{"no Camofox receipt source is connected"},
		},
		{
			ID: "payment_method_installment_insights", Priority: "P1", Status: CapabilityPlanned,
			UserValue:    "source-native payment-method totals without inventing installment information",
			Interface:    []string{},
			Implemented:  []string{"receipt summary calculations preserve unknown installment status"},
			NextWork:     "connect Camofox receipt reads before assessing source-native payment and installment fields",
			NextStepKind: CapabilityNextImplementation,
			BlockedBy:    []string{"Camofox receipt acquisition and installment evidence remain missing"},
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
	sort.SliceStable(capabilities, func(i, j int) bool {
		return capabilities[i].Priority < capabilities[j].Priority
	})
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
