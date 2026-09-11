package core

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const ProductSchemaVersion = 4

type ProductSort string

const (
	ProductSortRelevance      ProductSort = "relevance"
	ProductSortPriceAsc       ProductSort = "price_asc"
	ProductSortPriceDesc      ProductSort = "price_desc"
	ProductSortRating         ProductSort = "rating"
	ProductSortReviewCount    ProductSort = "review_count"
	ProductSortCoupangRanking ProductSort = "coupang_ranking"
	ProductSortSales          ProductSort = "sales"
	ProductSortLatest         ProductSort = "latest"
)

type ProductSearchRequest struct {
	CategoryTrail     []string                `json:"category_trail,omitempty" jsonschema:"Previously observed sidebar category choices to replay in order before the active category in facet_selections; query searches only. Prior choices are navigation history, not active filters. Trail plus active selections is limited to six steps"`
	FacetSelections   []ProductFacetSelection `json:"facet_selections,omitempty" jsonschema:"Source sidebar selections from coverage.facets for this query or category. Discover with a plain search first; never invent labels. Supported by the Camofox search adapter"`
	DocumentReadLimit int                     `json:"document_read_limit,omitempty" jsonschema:"Explicit document-attempt allowance: plain search 1 or 2, sidebar search requires category_trail + facet_selections + 1 (up to 7); 0 preserves bounded transport defaults. Excludes automatic browser subresources"`
	Page              int                     `json:"page,omitempty"`
	Query             string                  `json:"query,omitempty" jsonschema:"Natural-language product query,for example: 맥북용 허브 중 후기 좋고 10만원 아래. Provide exactly one of query or category_id"`
	CategoryID        string                  `json:"category_id,omitempty" jsonschema:"Source-native numeric starting Coupang category ID; use instead of query. A verified sidebar category change is reported in coverage.applied_category_id"`
	Limit             int                     `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 20"`
	MinPrice          int64                   `json:"min_price,omitempty" jsonschema:"Minimum current price in KRW"`
	MaxPrice          int64                   `json:"max_price,omitempty" jsonschema:"Maximum current price in KRW"`
	MinRating         float64                 `json:"min_rating,omitempty" jsonschema:"Minimum rating from 0 to 5"`
	MinReviewCount    int                     `json:"min_review_count,omitempty" jsonschema:"Minimum review count"`
	RocketOnly        bool                    `json:"rocket_only,omitempty" jsonschema:"Only include items explicitly marked for Rocket delivery"`
	FreeShippingOnly  bool                    `json:"free_shipping_only,omitempty" jsonschema:"Only include items explicitly marked as free shipping"`
	ExcludeSponsored  bool                    `json:"exclude_sponsored,omitempty" jsonschema:"Exclude items explicitly marked as sponsored"`
	MinMemoryGB       int                     `json:"min_memory_gb,omitempty" jsonschema:"Title-heuristic memory discovery prefilter in GB; not proof of the selected option's specifications"`
	MinStorageGB      int                     `json:"min_storage_gb,omitempty" jsonschema:"Title-heuristic storage discovery prefilter in GB; not proof of the selected option's specifications"`
	ExcludeUsed       bool                    `json:"exclude_used,omitempty" jsonschema:"Exclude titles explicitly marked used,refurbished,or display-unit"`
	IncludeVariants   bool                    `json:"include_variants,omitempty" jsonschema:"Return multiple options from the same Coupang product page; default false"`
	DisableAffiliate  bool                    `json:"disable_affiliate,omitempty" jsonschema:"Return canonical Coupang URLs only and skip configured affiliate-link generation"`
	Sort              ProductSort             `json:"sort,omitempty" jsonschema:"Result order: relevance,coupang_ranking,sales,latest,price_asc,price_desc,rating,or review_count"`
}

func (r ProductSearchRequest) Validate() error {
	navigationCount := len(r.CategoryTrail) + len(r.FacetSelections)
	if navigationCount > 6 {
		return errors.New("at most 6 sidebar navigation steps are allowed")
	}
	if navigationCount > 0 && (r.Page > 1 || (r.DocumentReadLimit > 0 && navigationCount+1 > r.DocumentReadLimit)) {
		return errors.New("sidebar selections require first-page search and sufficient document allowance")
	}
	seenFacets := map[string]bool{}
	for _, f := range r.FacetSelections {
		if strings.TrimSpace(f.Name) == "" || strings.TrimSpace(f.Label) == "" || !utf8.ValidString(f.Name) || !utf8.ValidString(f.Label) || len(f.Name) > 200 || len(f.Label) > 400 || seenFacets[f.Name] {
			return errors.New("sidebar filters require unique groups and observed labels")
		}
		seenFacets[f.Name] = true
	}
	if len(r.CategoryTrail) > 0 {
		if strings.TrimSpace(r.Query) == "" || r.CategoryID != "" || !seenFacets["카테고리"] {
			return errors.New("category_trail requires a query and one active sidebar category")
		}
		for _, label := range r.CategoryTrail {
			if strings.TrimSpace(label) == "" || !utf8.ValidString(label) || len(label) > 400 {
				return errors.New("category_trail requires bounded observed labels")
			}
		}
	}
	if r.DocumentReadLimit < 0 || r.DocumentReadLimit > max(2, navigationCount+1) {
		return errors.New("document_read_limit exceeds the bounded search and sidebar selection allowance")
	}
	if r.Page < 0 || r.Page > 100 {
		return errors.New("page must be between 1 and 100")
	}
	query := strings.TrimSpace(r.Query)
	if (query == "" && r.CategoryID == "") || !utf8.ValidString(query) || len([]rune(query)) > 200 {
		return errors.New("query or category_id is required; query must not exceed 200 characters")
	}
	if r.CategoryID != "" && !NumericProductIdentifier(r.CategoryID) {
		return errors.New("category_id must be numeric")
	}
	if query != "" && r.CategoryID != "" {
		return errors.New("choose query or category_id, not both")
	}
	if r.Limit < 0 || r.Limit > 20 {
		return errors.New("limit must be between 1 and 20")
	}
	if r.MinPrice < 0 || r.MaxPrice < 0 || (r.MaxPrice > 0 && r.MinPrice > r.MaxPrice) {
		return errors.New("price bounds are invalid")
	}
	if r.MinRating < 0 || r.MinRating > 5 {
		return errors.New("min_rating must be between 0 and 5")
	}
	if r.MinReviewCount < 0 {
		return errors.New("min_review_count must not be negative")
	}
	if r.MinMemoryGB < 0 || r.MinMemoryGB > 1024 || r.MinStorageGB < 0 || r.MinStorageGB > 100_000 {
		return errors.New("computer specification bounds are invalid")
	}
	switch r.Sort {
	case "", ProductSortRelevance, ProductSortCoupangRanking, ProductSortSales, ProductSortLatest,
		ProductSortPriceAsc, ProductSortPriceDesc, ProductSortRating, ProductSortReviewCount:
		return nil
	default:
		return errors.New("unsupported product sort")
	}
}

type ProductInspectRequest struct {
	DocumentReadLimit int    `json:"document_read_limit,omitempty" jsonschema:"Optional allowance from 1 to 3 for the detail document plus explicit quantity/review endpoint attempts. A positive limit disables whole-inspection retries; 0 preserves bounded transport defaults. Excludes browser subresources"`
	ProductID         string `json:"product_id" jsonschema:"Public numeric product identifier returned by products_search"`
	ItemID            string `json:"item_id,omitempty" jsonschema:"Public numeric item identifier returned by products_search"`
	VendorItemID      string `json:"vendor_item_id,omitempty" jsonschema:"Public numeric vendor item identifier returned by products_search"`
	ReviewLimit       int    `json:"review_limit,omitempty" jsonschema:"Maximum sanitized reviews from 0 to 20; default 5"`
	DetailImageLimit  int    `json:"detail_image_limit,omitempty" jsonschema:"Maximum detail images from 0 to 50; default 20"`
	DisableAffiliate  bool   `json:"disable_affiliate,omitempty" jsonschema:"Return the canonical Coupang URL only and skip configured affiliate-link generation"`
}

func (r ProductInspectRequest) Validate() error {
	if r.DocumentReadLimit < 0 || r.DocumentReadLimit > 3 {
		return errors.New("document_read_limit for inspection must be between 0 and 3")
	}
	if !NumericProductIdentifier(r.ProductID) || (r.ItemID != "" && !NumericProductIdentifier(r.ItemID)) || (r.VendorItemID != "" && !NumericProductIdentifier(r.VendorItemID)) {
		return errors.New("product identifiers must be numeric")
	}
	if r.ReviewLimit < 0 || r.ReviewLimit > 20 {
		return errors.New("review_limit must be between 0 and 20")
	}
	if r.DetailImageLimit < 0 || r.DetailImageLimit > 50 {
		return errors.New("detail_image_limit must be between 0 and 50")
	}
	return nil
}

func NumericProductIdentifier(value string) bool {
	if value == "" || len(value) > 24 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

type ProductPrice struct {
	CurrentAmount  int64  `json:"current_amount,omitempty"`
	OriginalAmount int64  `json:"original_amount,omitempty"`
	DiscountRate   int    `json:"discount_rate,omitempty"`
	Currency       string `json:"currency"`
}

type ProductCard struct {
	Reference      ProductReference        `json:"reference"`
	Name           string                  `json:"name"`
	URL            string                  `json:"url"`
	AffiliateURL   string                  `json:"affiliate_url,omitempty"`
	ImageURL       string                  `json:"image_url,omitempty"`
	Price          ProductPrice            `json:"price"`
	Rating         float64                 `json:"rating,omitempty"`
	ReviewCount    int                     `json:"review_count,omitempty"`
	Rocket         bool                    `json:"rocket,omitempty"`
	FreeShipping   bool                    `json:"free_shipping,omitempty"`
	Coupon         bool                    `json:"coupon,omitempty"`
	Sponsored      bool                    `json:"sponsored,omitempty"`
	SearchPosition int                     `json:"search_position,omitempty"`
	RankSource     string                  `json:"rank_source,omitempty"`
	ReviewScope    string                  `json:"review_scope,omitempty"`
	VariantCount   int                     `json:"variant_count"`
	ComputerSpecs  *ComputerSpecifications `json:"computer_specs,omitempty"`
	ObservedFields []string                `json:"observed_fields"`
	FieldEvidence  []ProductFieldEvidence  `json:"field_evidence,omitempty"`
}

type ProductAffiliateStatus string

const (
	ProductAffiliateDisabled      ProductAffiliateStatus = "disabled"
	ProductAffiliateUnconfigured  ProductAffiliateStatus = "unconfigured"
	ProductAffiliateNotApplicable ProductAffiliateStatus = "not_applicable"
	ProductAffiliateUnavailable   ProductAffiliateStatus = "unavailable"
	ProductAffiliatePartial       ProductAffiliateStatus = "partial"
	ProductAffiliateApplied       ProductAffiliateStatus = "applied"
)

type ProductAffiliateDisclosure struct {
	Status                  ProductAffiliateStatus `json:"status"`
	Source                  string                 `json:"source,omitempty"`
	CommissionRecipient     string                 `json:"commission_recipient,omitempty"`
	BuyerPriceEffect        string                 `json:"buyer_price_effect,omitempty"`
	SelfPurchasesEligible   bool                   `json:"self_purchases_eligible"`
	Disclosure              string                 `json:"disclosure,omitempty"`
	PriceVerificationNotice string                 `json:"price_verification_notice,omitempty"`
}

type ComputerSpecifications struct {
	MemoryGB   int    `json:"memory_gb,omitempty"`
	StorageGB  int    `json:"storage_gb,omitempty"`
	CPU        string `json:"cpu,omitempty"`
	GPU        string `json:"gpu,omitempty"`
	OS         string `json:"os,omitempty"`
	Condition  string `json:"condition"`
	Source     string `json:"source"`
	Provenance string `json:"provenance"`
	Method     string `json:"method"`
}

type ProductRankingSummary struct {
	Requested    ProductSort `json:"requested"`
	Applied      ProductSort `json:"applied"`
	Source       string      `json:"source"`
	Scope        string      `json:"scope"`
	SourceNative bool        `json:"source_native"`
	Description  string      `json:"description"`
}

type ProductSearchResult struct {
	SchemaVersion  int                  `json:"schema_version"`
	Query          string               `json:"query"`
	Currency       string               `json:"currency"`
	FetchedAt      time.Time            `json:"fetched_at"`
	Items          []ProductCard        `json:"items"`
	AppliedFilters ProductSearchRequest `json:"applied_filters"`
	// UnavailableFilterFields lists required scalar evidence missing on at least
	// one source card, before filtering, variant collapsing, or display limits.
	// It is not a list of confirmed condition violations or a provenance claim.
	UnavailableFilterFields []string                   `json:"unavailable_filter_fields,omitempty"`
	Coverage                ProductCoverage            `json:"coverage"`
	Ranking                 ProductRankingSummary      `json:"ranking"`
	Affiliate               ProductAffiliateDisclosure `json:"affiliate"`
	Warnings                []string                   `json:"warnings"`
}

type ProductBenefit struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Condition   string `json:"condition,omitempty"`
	Source      string `json:"source"`
}

type ProductDelivery struct {
	Summary      string `json:"summary,omitempty"`
	FreeShipping bool   `json:"free_shipping,omitempty"`
	Rocket       bool   `json:"rocket,omitempty"`
}

type ProductReview struct {
	ObservedFields []string `json:"observed_fields,omitempty"`
	Rating         float64  `json:"rating,omitempty"`
	Content        string   `json:"content,omitempty"`
	CreatedDate    string   `json:"created_date,omitempty"`
	HelpfulCount   int      `json:"helpful_count,omitempty"`
	ImageURLs      []string `json:"image_urls"`
}

type ProductRatingSummary struct {
	Average      float64        `json:"average,omitempty"`
	Count        int            `json:"count,omitempty"`
	Distribution map[string]int `json:"distribution,omitempty"`
}

type ProductCoverage struct {
	AppliedCategoryID   string   `json:"applied_category_id,omitempty" jsonschema:"Verified source category after an explicit sidebar category change. The input category_id remains the starting scope. Selection and a matching native breadcrumb establish this destination, not candidate suitability"`
	BudgetOmittedFields []string `json:"budget_omitted_fields,omitempty" jsonschema:"Fields whose explicit endpoint read was omitted because the document allowance was exhausted; not absent product facts"`
	// SourceNoResults is an explicit source response for the requested search
	// page, never inferred from a filtered, truncated, or rejected local list.
	SourceNoResults bool `json:"source_no_results"`
	// Parser counts are before service filters, variant collapse and display limits.
	SourceItems       int            `json:"source_items,omitempty"`
	RejectedItems     int            `json:"rejected_items,omitempty"`
	DuplicateItems    int            `json:"duplicate_items,omitempty"`
	Facets            []ProductFacet `json:"facets,omitempty"`
	Source            string         `json:"source"`
	ObservedFields    []string       `json:"observed_fields"`
	UnavailableFields []string       `json:"unavailable_fields"`
}

type ProductFacet struct {
	ID            string               `json:"id,omitempty"`
	Source        string               `json:"source,omitempty"`
	MoreAvailable bool                 `json:"more_available,omitempty"`
	Name          string               `json:"name"`
	Options       []ProductFacetOption `json:"options"`
}

type ProductFacetOption struct {
	ID       string `json:"id,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	Label    string `json:"label"`
	Selected bool   `json:"selected"`
}

type ProductFacetSelection struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type ProductInspection struct {
	SchemaVersion      int                        `json:"schema_version"`
	FetchedAt          time.Time                  `json:"fetched_at"`
	Product            ProductCard                `json:"product"`
	Affiliate          ProductAffiliateDisclosure `json:"affiliate"`
	SelectedOptions    []string                   `json:"selected_options"`
	SelectedAttributes []ProductSelectedAttribute `json:"selected_attributes" jsonschema:"Source-native option labels and values bound to the exact item and vendor item. Compound rows remain unsplit; not verified RAM/storage or overall suitability"`
	FieldEvidence      []ProductFieldEvidence     `json:"field_evidence,omitempty"`
	Description        string                     `json:"description,omitempty"`
	Specifications     []string                   `json:"specifications"`
	GalleryImages      []string                   `json:"gallery_images"`
	DetailImages       []string                   `json:"detail_images"`
	Delivery           ProductDelivery            `json:"delivery"`
	Benefits           []ProductBenefit           `json:"benefits"`
	Rating             ProductRatingSummary       `json:"rating"`
	Reviews            []ProductReview            `json:"reviews"`
	Coverage           ProductCoverage            `json:"coverage"`
	Warnings           []string                   `json:"warnings"`
}

type CartAddRequest struct {
	ProductID    string `json:"product_id" jsonschema:"Public numeric product identifier returned by products_search"`
	ItemID       string `json:"item_id,omitempty" jsonschema:"Public numeric item identifier returned by products_search"`
	VendorItemID string `json:"vendor_item_id" jsonschema:"Exact public numeric vendor item identifier returned by products_search"`
	Quantity     int    `json:"quantity,omitempty" jsonschema:"Quantity from 1 to 50; default 1"`
	Confirmed    bool   `json:"confirmed" jsonschema:"Must be true only after the user explicitly asks to change their cart"`
}

func (r CartAddRequest) Validate() error {
	if !NumericProductIdentifier(r.ProductID) || !NumericProductIdentifier(r.VendorItemID) || (r.ItemID != "" && !NumericProductIdentifier(r.ItemID)) {
		return errors.New("cart product identifiers must be numeric")
	}
	if r.Quantity < 0 || r.Quantity > 50 {
		return errors.New("quantity must be between 1 and 50")
	}
	if !r.Confirmed {
		return errors.New("cart addition requires explicit confirmation")
	}
	return nil
}

type CartAddResult struct {
	SchemaVersion int              `json:"schema_version"`
	Attempted     bool             `json:"attempted"`
	Added         bool             `json:"added"`
	Verified      bool             `json:"verified"`
	Product       ProductReference `json:"product"`
	Quantity      int              `json:"quantity"`
	CartURL       string           `json:"cart_url"`
	Source        string           `json:"source"`
	Warnings      []string         `json:"warnings"`
}
