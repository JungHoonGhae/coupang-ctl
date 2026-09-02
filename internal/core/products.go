package core

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const ProductSchemaVersion = 3

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
	Query            string      `json:"query" jsonschema:"Natural-language product query,for example: 맥북용 허브 중 후기 좋고 10만원 아래"`
	CategoryID       string      `json:"category_id,omitempty" jsonschema:"Source-native numeric Coupang category ID; may be used instead of query"`
	Limit            int         `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 20"`
	MinPrice         int64       `json:"min_price,omitempty" jsonschema:"Minimum current price in KRW"`
	MaxPrice         int64       `json:"max_price,omitempty" jsonschema:"Maximum current price in KRW"`
	MinRating        float64     `json:"min_rating,omitempty" jsonschema:"Minimum rating from 0 to 5"`
	MinReviewCount   int         `json:"min_review_count,omitempty" jsonschema:"Minimum review count"`
	RocketOnly       bool        `json:"rocket_only,omitempty" jsonschema:"Only include items explicitly marked for Rocket delivery"`
	FreeShippingOnly bool        `json:"free_shipping_only,omitempty" jsonschema:"Only include items explicitly marked as free shipping"`
	ExcludeSponsored bool        `json:"exclude_sponsored,omitempty" jsonschema:"Exclude items explicitly marked as sponsored"`
	MinMemoryGB      int         `json:"min_memory_gb,omitempty" jsonschema:"Minimum explicitly observed computer memory in GB"`
	MinStorageGB     int         `json:"min_storage_gb,omitempty" jsonschema:"Minimum explicitly observed computer storage in GB"`
	ExcludeUsed      bool        `json:"exclude_used,omitempty" jsonschema:"Exclude titles explicitly marked used,refurbished,or display-unit"`
	IncludeVariants  bool        `json:"include_variants,omitempty" jsonschema:"Return multiple options from the same Coupang product page; default false"`
	DisableAffiliate bool        `json:"disable_affiliate,omitempty" jsonschema:"Return canonical Coupang URLs only and skip configured affiliate-link generation"`
	Sort             ProductSort `json:"sort,omitempty" jsonschema:"Result order: relevance,coupang_ranking,sales,latest,price_asc,price_desc,rating,or review_count"`
}

func (r ProductSearchRequest) Validate() error {
	query := strings.TrimSpace(r.Query)
	if (query == "" && r.CategoryID == "") || !utf8.ValidString(query) || len([]rune(query)) > 200 {
		return errors.New("query or category_id is required; query must not exceed 200 characters")
	}
	if r.CategoryID != "" && !NumericProductIdentifier(r.CategoryID) {
		return errors.New("category_id must be numeric")
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
	ProductID        string `json:"product_id" jsonschema:"Public numeric product identifier returned by products_search"`
	ItemID           string `json:"item_id,omitempty" jsonschema:"Public numeric item identifier returned by products_search"`
	VendorItemID     string `json:"vendor_item_id,omitempty" jsonschema:"Public numeric vendor item identifier returned by products_search"`
	ReviewLimit      int    `json:"review_limit,omitempty" jsonschema:"Maximum sanitized reviews from 0 to 20; default 5"`
	DetailImageLimit int    `json:"detail_image_limit,omitempty" jsonschema:"Maximum detail images from 0 to 50; default 20"`
	DisableAffiliate bool   `json:"disable_affiliate,omitempty" jsonschema:"Return the canonical Coupang URL only and skip configured affiliate-link generation"`
}

func (r ProductInspectRequest) Validate() error {
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
	Rocket         bool                    `json:"rocket"`
	FreeShipping   bool                    `json:"free_shipping"`
	Coupon         bool                    `json:"coupon"`
	Sponsored      bool                    `json:"sponsored"`
	SearchPosition int                     `json:"search_position,omitempty"`
	RankSource     string                  `json:"rank_source,omitempty"`
	ReviewScope    string                  `json:"review_scope,omitempty"`
	VariantCount   int                     `json:"variant_count"`
	ComputerSpecs  *ComputerSpecifications `json:"computer_specs,omitempty"`
	ObservedFields []string                `json:"observed_fields"`
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
	MemoryGB  int    `json:"memory_gb,omitempty"`
	StorageGB int    `json:"storage_gb,omitempty"`
	CPU       string `json:"cpu,omitempty"`
	GPU       string `json:"gpu,omitempty"`
	OS        string `json:"os,omitempty"`
	Condition string `json:"condition"`
	Source    string `json:"source"`
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
	SchemaVersion  int                        `json:"schema_version"`
	Query          string                     `json:"query"`
	Currency       string                     `json:"currency"`
	FetchedAt      time.Time                  `json:"fetched_at"`
	Items          []ProductCard              `json:"items"`
	AppliedFilters ProductSearchRequest       `json:"applied_filters"`
	Coverage       ProductCoverage            `json:"coverage"`
	Ranking        ProductRankingSummary      `json:"ranking"`
	Affiliate      ProductAffiliateDisclosure `json:"affiliate"`
	Warnings       []string                   `json:"warnings"`
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
	FreeShipping bool   `json:"free_shipping"`
	Rocket       bool   `json:"rocket"`
}

type ProductReview struct {
	Rating       float64  `json:"rating,omitempty"`
	Content      string   `json:"content,omitempty"`
	CreatedDate  string   `json:"created_date,omitempty"`
	HelpfulCount int      `json:"helpful_count,omitempty"`
	ImageURLs    []string `json:"image_urls"`
}

type ProductRatingSummary struct {
	Average      float64        `json:"average,omitempty"`
	Count        int            `json:"count,omitempty"`
	Distribution map[string]int `json:"distribution,omitempty"`
}

type ProductCoverage struct {
	Source            string   `json:"source"`
	ObservedFields    []string `json:"observed_fields"`
	UnavailableFields []string `json:"unavailable_fields"`
}

type ProductInspection struct {
	SchemaVersion   int                        `json:"schema_version"`
	FetchedAt       time.Time                  `json:"fetched_at"`
	Product         ProductCard                `json:"product"`
	Affiliate       ProductAffiliateDisclosure `json:"affiliate"`
	SelectedOptions []string                   `json:"selected_options"`
	Description     string                     `json:"description,omitempty"`
	Specifications  []string                   `json:"specifications"`
	GalleryImages   []string                   `json:"gallery_images"`
	DetailImages    []string                   `json:"detail_images"`
	Delivery        ProductDelivery            `json:"delivery"`
	Benefits        []ProductBenefit           `json:"benefits"`
	Rating          ProductRatingSummary       `json:"rating"`
	Reviews         []ProductReview            `json:"reviews"`
	Coverage        ProductCoverage            `json:"coverage"`
	Warnings        []string                   `json:"warnings"`
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
