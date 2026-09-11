package core

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const ProductRecommendationSchemaVersion = 5

type ProductRecommendationStatus string

const (
	ProductRecommendationNeedsInput ProductRecommendationStatus = "needs_input"
	ProductRecommendationComplete   ProductRecommendationStatus = "complete"
	ProductRecommendationIncomplete ProductRecommendationStatus = "incomplete"
	ProductRecommendationNoMatches  ProductRecommendationStatus = "no_matches"
)

type ProductRecommendationCandidateStatus string

const (
	ProductRecommendationEligible          ProductRecommendationCandidateStatus = "eligible"
	ProductRecommendationNeedsVerification ProductRecommendationCandidateStatus = "needs_verification"
)

type ProductRecommendationValueKind string

const (
	ProductRecommendationValueChoice     ProductRecommendationValueKind = "choice"
	ProductRecommendationValueInteger    ProductRecommendationValueKind = "integer"
	ProductRecommendationValueMoney      ProductRecommendationValueKind = "money"
	ProductRecommendationValueDimensions ProductRecommendationValueKind = "dimensions"
)

type ProductDimensionsMM struct {
	Width  int `json:"width"`
	Depth  int `json:"depth"`
	Height int `json:"height"`
}

type ProductRecommendationAnswer struct {
	QuestionID   string               `json:"question_id"`
	Choice       string               `json:"choice,omitempty"`
	Integer      int64                `json:"integer,omitempty"`
	DimensionsMM *ProductDimensionsMM `json:"dimensions_mm,omitempty"`
	Unbounded    bool                 `json:"unbounded,omitempty"`
}

type ProductRecommendationRequest struct {
	ComparisonAxes     []ProductPreferenceAxis          `json:"comparison_axes,omitempty" jsonschema:"Explicit preference axes for comparison, no default axes or weights; omission preserves candidates without declaring dominance"`
	RequiredConditions []ProductRecommendationCondition `json:"required_conditions,omitempty" jsonschema:"Explicit hard conditions to assess against detail evidence, not inferred from query text; at most 16"`
	UsePurchaseHistory bool                             `json:"use_purchase_history,omitempty" jsonschema:"Explicitly include private local purchase aggregates for returned candidates; does not infer preference or reorder need"`
	DisableAffiliate   bool                             `json:"disable_affiliate,omitempty" jsonschema:"Return canonical product URLs without affiliate conversion"`
	Query              string                           `json:"query,omitempty" jsonschema:"Product query to research; provide exactly one of query or category_id. Express hard requirements separately in required_conditions"`
	CategoryID         string                           `json:"category_id,omitempty" jsonschema:"Source-native numeric starting category ID observed during discovery. Only an explicit verified sidebar category choice may change it; subsequent research keeps the verified destination. Use instead of query"`
	Answers            []ProductRecommendationAnswer    `json:"answers,omitempty" jsonschema:"Cumulative answers to observed facet questions, applied in array order with rediscovery after each selection. At most six choices. Repeat facet:카테고리 with explicit choices to follow an observed path; only the last verified category remains active. Query searches replay prior categories without changing the query. Other question IDs must be unique. Category-only navigation requires a selected sidebar choice and matching source breadcrumb. Unbounded skips that optional question. Choices never become query keywords"`
	Proceed            bool                             `json:"proceed,omitempty" jsonschema:"Skip remaining optional source-facet questions and run bounded research. Research continues automatically when no answerable unanswered facet questions remain; this is not an authorization flag for writes"`
	MaxPrice           int64                            `json:"max_price,omitempty" jsonschema:"Maximum current product price in KRW"`
	MaxItems           int                              `json:"max_items,omitempty" jsonschema:"Optional presentation cap from 1 to 200; omitted or 0 returns all retained comparison candidates within the investigation budget. Does not change investigation depth or imply verified suitability"`
	ReviewCap          int                              `json:"review_cap,omitempty" jsonschema:"Maximum unique sanitized review samples examined across the recommendation; default 200,maximum 500"`
	DiscoveryTarget    int                              `json:"discovery_target,omitempty" jsonschema:"Target number of unique model families to screen from list pages; default 100,maximum 200"`
	SearchPageLimit    int                              `json:"search_page_limit,omitempty" jsonschema:"Maximum source pages per search axis from 1 to 10; default 5"`
}

func (r ProductRecommendationRequest) Validate() error {
	if err := (ProductSearchRequest{Query: r.Query, CategoryID: r.CategoryID}).Validate(); err != nil {
		return err
	}
	if r.MaxItems < 0 || r.MaxItems > 200 {
		return errors.New("max_items must be between 0 and 200; 0 means no explicit presentation cap")
	}
	if r.ReviewCap < 0 || r.ReviewCap > 500 {
		return errors.New("review_cap must be between 1 and 500")
	}
	if r.DiscoveryTarget < 0 || r.DiscoveryTarget > 200 {
		return errors.New("discovery_target must be between 1 and 200")
	}
	if r.SearchPageLimit < 0 || r.SearchPageLimit > 10 {
		return errors.New("search_page_limit must be between 1 and 10")
	}
	if r.MaxPrice < 0 {
		return errors.New("max_price must not be negative")
	}
	comparisonFields := map[string]bool{}
	for _, axis := range r.ComparisonAxes {
		if err := axis.Validate(); err != nil {
			return err
		}
		if comparisonFields[axis.Field] {
			return errors.New("comparison fields must be unique")
		}
		comparisonFields[axis.Field] = true
	}
	if len(r.RequiredConditions) > 16 {
		return errors.New("at most 16 required conditions are supported")
	}
	conditionIDs := map[string]bool{"max_price": r.MaxPrice > 0}
	for _, condition := range r.RequiredConditions {
		if err := condition.Validate(); err != nil {
			return err
		}
		if conditionIDs[condition.ID] {
			return errors.New("condition ids must be unique, including max_price when supplied")
		}
		conditionIDs[condition.ID] = true
	}
	if len(r.Answers) > 50 {
		return errors.New("at most 50 recommendation answers are supported")
	}
	choices := 0
	// Category navigation may revisit the category question. These
	// answers are an ordered path, not simultaneous filters or duplicate skips.
	seen := make(map[string]bool, len(r.Answers))
	for _, answer := range r.Answers {
		id := strings.TrimSpace(answer.QuestionID)
		if id == "" {
			return errors.New("recommendation answer question_id is required")
		}
		categoryChoice := id == "facet:카테고리" && !answer.Unbounded && strings.TrimSpace(answer.Choice) != "" && answer.Integer == 0 && answer.DimensionsMM == nil
		if previousCategoryChoice, exists := seen[id]; exists && !(previousCategoryChoice && categoryChoice) {
			return errors.New("recommendation answers must have unique question_id values")
		}
		seen[id] = categoryChoice
		if !utf8.ValidString(id) || len(id) > 206 || !utf8.ValidString(answer.Choice) || len(answer.Choice) > 400 {
			return errors.New("recommendation answer exceeds bounded UTF-8 fields")
		}
		if strings.HasPrefix(id, "facet:") && !answer.Unbounded && strings.TrimSpace(answer.Choice) != "" {
			choices++
		}
		if answer.Integer < 0 {
			return errors.New("recommendation integer answers must not be negative")
		}
		if dimensions := answer.DimensionsMM; dimensions != nil && (dimensions.Width <= 0 || dimensions.Depth <= 0 || dimensions.Height <= 0) {
			return errors.New("recommendation dimensions must all be positive millimetres")
		}
	}
	if choices > 6 {
		return errors.New("at most six sidebar choices are supported")
	}
	return nil
}

type ProductRecommendationOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type ProductRecommendationQuestion struct {
	ID             string                         `json:"id"`
	Prompt         string                         `json:"prompt"`
	Kind           ProductRecommendationValueKind `json:"kind"`
	Required       bool                           `json:"required"`
	AllowUnbounded bool                           `json:"allow_unbounded"`
	Unit           string                         `json:"unit,omitempty"`
	Options        []ProductRecommendationOption  `json:"options,omitempty"`
	SourceFacet    string                         `json:"source_facet,omitempty"`
}

type ProductRecommendationClaim struct {
	Field      string                `json:"field"`
	Value      string                `json:"value"`
	Provenance string                `json:"provenance"`
	Source     string                `json:"source"`
	Evidence   *ProductFieldEvidence `json:"evidence,omitempty"`
}

type ProductRecommendationCandidate struct {
	Conclusion        *ProductRecommendationConclusion     `json:"conclusion,omitempty" jsonschema:"Evidence-backed conclusion limited to declared conditions; broader candidate suitability status remains separate"`
	ComparisonValues  []ProductComparableValue             `json:"comparison_values"`
	Comparisons       []ProductPairComparison              `json:"comparisons"`
	ComparisonReason  string                               `json:"comparison_reason"`
	Conditions        []ProductConditionAssessment         `json:"conditions"`
	ExclusionReason   string                               `json:"exclusion_reason,omitempty" jsonschema:"Reason an inspected candidate was not retained for comparison; does not erase its evidence"`
	Product           ProductCard                          `json:"product"`
	Inspection        ProductInspection                    `json:"inspection"`
	Status            ProductRecommendationCandidateStatus `json:"status"`
	Claims            []ProductRecommendationClaim         `json:"claims"`
	MissingEvidence   []string                             `json:"missing_evidence"`
	ReviewsExamined   int                                  `json:"reviews_examined"`
	ReviewsAvailable  *int                                 `json:"reviews_available,omitempty" jsonschema:"Available source review count; absent if missing or conflicting; not a count for the selected option unless scope explicitly says so"`
	ReviewCountScope  string                               `json:"review_count_scope,omitempty"`
	DetailImagesFound int                                  `json:"detail_images_found"`
}

type ProductDiscoveryAppearance struct {
	Sort       ProductSort `json:"sort"`
	Page       int         `json:"page"`
	Position   int         `json:"position"`
	RankSource string      `json:"rank_source"`
}

type ProductDiscoveryCandidate struct {
	Product              ProductCard                  `json:"product"`
	Options              []ProductCard                `json:"options" jsonschema:"Distinct observed references within this product group; the representative product does not establish suitability of every option"`
	Appearances          []ProductDiscoveryAppearance `json:"appearances"`
	AxisCount            int                          `json:"axis_count"`
	CrossListCoverage    float64                      `json:"cross_list_coverage"`
	ReciprocalRankFusion float64                      `json:"reciprocal_rank_fusion"`
	Provenance           string                       `json:"provenance"`
}

type ProductRecommendationAudit struct {
	TimeBudgetExhausted           bool   `json:"time_budget_exhausted" jsonschema:"Internal research deadline expired; retained evidence is partial, not sufficient; caller cancellation remains an error"`
	DocumentReadBudget            int    `json:"document_read_budget" jsonschema:"Operational ceiling for reserved explicit source document attempts, not a psychologically optimal investigation size"`
	DocumentReadReservations      int    `json:"document_read_reservations" jsonschema:"Sum of allowances issued before search/detail calls, including potential auxiliary reads and failed calls; a conservative bound, not observed network usage"`
	DocumentBudgetConstrained     bool   `json:"document_budget_constrained" jsonschema:"Document budget constrained discovery, detail breadth, or auxiliary depth; does not establish sufficient evidence"`
	ComparisonExcludedCandidates  int    `json:"comparison_excluded_candidates"`
	InspectionBudget              int    `json:"inspection_budget"`
	InspectionAttempts            int    `json:"inspection_attempts"`
	InspectionStopReason          string `json:"inspection_stop_reason" jsonschema:"Why detail investigation stopped; source_access_denied or source_authentication_required stop further source reads and retain earlier evidence as incomplete"`
	UninspectedModelFamilies      int    `json:"uninspected_model_families"`
	UninspectedExactOptions       int    `json:"uninspected_exact_options" jsonschema:"Discovered references without a successful matching detail read, including failed and budget-omitted reads"`
	ComparisonCandidates          int    `json:"comparison_candidates"`
	DisplayedCandidates           int    `json:"displayed_candidates"`
	PresentationOmittedCandidates int    `json:"presentation_omitted_candidates"`
	DiscoveryTarget               int    `json:"discovery_target"`
	SearchPagesRead               int    `json:"search_pages_read"`
	DiscoveryStopReason           string `json:"discovery_stop_reason" jsonschema:"Why discovery stopped; source access/authentication failure is distinct from explicit source exhaustion and operational budgets"`
	SearchesRun                   int    `json:"searches_run"`
	ListingsObserved              int    `json:"listings_observed"`
	UniqueExactOptions            int    `json:"unique_exact_options"`
	UniqueModelFamilies           int    `json:"unique_model_families"`
	DetailsInspected              int    `json:"details_inspected"`
	ReviewsExamined               int    `json:"reviews_examined"`
	ReviewsAvailable              *int   `json:"reviews_available,omitempty" jsonschema:"Sum over distinct inspected product pages only when all page counts and scopes are available and consistent; not unique review samples"`
	ReviewCountPages              int    `json:"review_count_pages"`
	ReviewCountUnavailablePages   int    `json:"review_count_unavailable_pages"`
	ReviewCountScope              string `json:"review_count_scope,omitempty"`
	DetailImagesFound             int    `json:"detail_images_found"`
	DetailImagesVisuallyReviewed  int    `json:"detail_images_visually_reviewed"`
	SourceFacetsObserved          int    `json:"source_facets_observed"`
}

type ProductRecommendationResult struct {
	Refinement        *ProductSearchRefinement          `json:"refinement,omitempty" jsonschema:"Verified sidebar selection sequence; facets is the latest catalog, not a union across states. Selection does not prove detailed product suitability"`
	NextActions       []ProductRecommendationNextAction `json:"next_actions" jsonschema:"Reasoned read-only investigation or conversation suggestions from all inspected/discovered candidates, not just the displayed subset; does not authorize side effects"`
	ComparisonAxes    []ProductPreferenceAxis           `json:"comparison_axes"`
	Inspected         []ProductRecommendationCandidate  `json:"inspected" jsonschema:"Successful detail reads including excluded and presentation-omitted candidates; candidate suitability remains explicitly scoped by status and missing evidence"`
	PurchaseContext   *RecommendationPurchaseContext    `json:"purchase_context,omitempty"`
	Facets            []ProductFacet                    `json:"facets,omitempty"`
	SchemaVersion     int                               `json:"schema_version"`
	Status            ProductRecommendationStatus       `json:"status"`
	Query             string                            `json:"query"`
	CategoryID        string                            `json:"category_id,omitempty" jsonschema:"Requested starting source category; applied_category_id identifies a verified sidebar destination when different. Neither establishes candidate suitability"`
	AppliedCategoryID string                            `json:"applied_category_id,omitempty" jsonschema:"Last verified category after an explicit sidebar choice; later refinement and sort research use this destination. Absent means no verified scope change"`
	Questions         []ProductRecommendationQuestion   `json:"questions"`
	Candidates        []ProductRecommendationCandidate  `json:"candidates"`
	Discovered        []ProductDiscoveryCandidate       `json:"discovered"`
	Audit             ProductRecommendationAudit        `json:"audit"`
	Warnings          []string                          `json:"warnings"`
}

// RecommendationPurchaseContext is private evidence, not a preference score.
// An absent match means only no eligible purchase in the available local rows.
const RecommendationPurchaseProvenance = "derived_from_normalized_structured_order_model"

type RecommendationPurchaseContext struct {
	Visibility  string                        `json:"visibility"`
	Status      string                        `json:"status"` // available, partial, unavailable
	Provenance  string                        `json:"provenance"`
	Sync        *SyncStatus                   `json:"sync,omitempty"`
	Matches     []RecommendationPurchaseMatch `json:"matches"`
	Limitations []string                      `json:"limitations"`
}

type RecommendationPurchaseMatch struct {
	Reference           ProductReference `json:"reference"`
	IdentityScope       string           `json:"identity_scope"` // product_and_vendor_item or product_only
	OrderCount          int              `json:"order_count"`
	ItemLineCount       int              `json:"item_line_count"`
	RetainedUnits       int              `json:"retained_units"`
	FirstPurchaseMonth  string           `json:"first_purchase_month,omitempty"`
	LatestPurchaseMonth string           `json:"latest_purchase_month,omitempty"`
}
