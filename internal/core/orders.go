package core

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidOrderData = errors.New("invalid order data")
var ErrProductCategoryUnavailable = errors.New("product category unavailable")

const CategorySourceProductJSONLDBreadcrumb = "coupang_product_jsonld_breadcrumb_v1"
const CategorySourceProductUnavailable = "coupang_product_category_unavailable_v1"
const CategoryCatalogSchemaVersion = 1

type CommerceKind string

const (
	CommerceKindProductPurchase CommerceKind = "product_purchase"
	CommerceKindMembershipFee   CommerceKind = "membership_fee"
)

type OrderPage struct {
	Orders []Order      `json:"orders"`
	Next   *OrderCursor `json:"next,omitempty"`
}

type OrderCursor struct {
	Year int `json:"year"`
	Page int `json:"page"`
}

type Order struct {
	SourceRef        string      `json:"source_ref"`
	PurchasedAt      string      `json:"purchased_at"`
	PurchasedAtTime  *time.Time  `json:"purchased_at_time,omitempty"`
	TotalAmount      int64       `json:"total_amount"`
	DiscountAmount   int64       `json:"discount_amount"`
	ShippingFee      int64       `json:"shipping_fee"`
	Currency         string      `json:"currency"`
	FullyCanceled    bool        `json:"fully_canceled,omitempty"`
	ReceiptAvailable bool        `json:"receipt_available,omitempty"`
	Items            []OrderItem `json:"items"`
}

type OrderItem struct {
	ProductID         string       `json:"product_id,omitempty"`
	VendorItemID      string       `json:"vendor_item_id,omitempty"`
	Name              string       `json:"name"`
	Quantity          int          `json:"quantity"`
	CancelledQuantity int          `json:"cancelled_quantity,omitempty"`
	ReturnedQuantity  int          `json:"returned_quantity,omitempty"`
	UnitPrice         int64        `json:"unit_price"`
	PaidPrice         int64        `json:"paid_price"`
	SellerName        string       `json:"seller_name,omitempty"`
	BrandName         string       `json:"brand_name,omitempty"`
	ProductType       string       `json:"product_type,omitempty"`
	DivisionType      string       `json:"division_type,omitempty"`
	CommerceKind      CommerceKind `json:"commerce_kind"`
	DeliveryStatus    string       `json:"delivery_status,omitempty"`
	DeliveredAt       *time.Time   `json:"delivered_at,omitempty"`
}

// ClassifyCommerceKind intentionally recognizes only explicit service labels.
// Unknown upstream product labels remain product purchases so an endpoint
// vocabulary change cannot silently remove ordinary goods from user totals.
func ClassifyCommerceKind(item OrderItem) CommerceKind {
	if item.CommerceKind == CommerceKindMembershipFee || item.CommerceKind == CommerceKindProductPurchase {
		return item.CommerceKind
	}
	for _, value := range []string{item.ProductType, item.DivisionType} {
		switch strings.ToUpper(strings.TrimSpace(value)) {
		case "MEMBERSHIP", "WOW_MEMBERSHIP", "MEMBERSHIP_FEE", "SUBSCRIPTION":
			return CommerceKindMembershipFee
		}
	}
	return CommerceKindProductPurchase
}

type OrderFilter struct {
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type UpsertResult struct {
	OrdersSeen int `json:"orders_seen"`
	ItemsSeen  int `json:"items_seen"`
}

type SpendSummary struct {
	From                    string                 `json:"from,omitempty"`
	To                      string                 `json:"to,omitempty"`
	Currency                string                 `json:"currency"`
	OrderCount              int                    `json:"order_count"`
	TotalAmount             int64                  `json:"total_amount"`
	DiscountAmount          int64                  `json:"discount_amount"`
	ShippingFee             int64                  `json:"shipping_fee"`
	FullyCanceledOrderCount int                    `json:"fully_canceled_order_count"`
	FullyCanceledAmount     int64                  `json:"fully_canceled_amount"`
	NonCanceledTotalAmount  int64                  `json:"non_canceled_total_amount"`
	NonCanceledShippingFee  int64                  `json:"non_canceled_shipping_fee"`
	Commerce                CommerceSpendBreakdown `json:"commerce"`
}

// CommerceSpendBreakdown keeps the legacy gross ledger total above intact
// while making product, explicit membership, and unclassified spend visibly
// separate. An order containing any product line is classified as a product
// purchase; a membership fee is counted only when every classified line is a
// membership line.
type CommerceSpendBreakdown struct {
	ProductPurchases CommerceSpendBucket `json:"product_purchases"`
	MembershipFees   CommerceSpendBucket `json:"membership_fees"`
	Unclassified     CommerceSpendBucket `json:"unclassified"`
}

type CommerceSpendBucket struct {
	OrderCount             int   `json:"order_count"`
	GrossAmount            int64 `json:"gross_amount"`
	NonCanceledOrderCount  int   `json:"non_canceled_order_count"`
	NonCanceledTotalAmount int64 `json:"non_canceled_total_amount"`
}

type OrderStats struct {
	From                    string                    `json:"from,omitempty"`
	To                      string                    `json:"to,omitempty"`
	OrderCount              int                       `json:"order_count"`
	FullyCanceledOrderCount int                       `json:"fully_canceled_order_count"`
	ItemLineCount           int                       `json:"item_line_count"`
	OrderedUnits            int                       `json:"ordered_units"`
	CanceledItemLineCount   int                       `json:"canceled_item_line_count"`
	CanceledUnits           int                       `json:"canceled_units"`
	ReturnedItemLineCount   int                       `json:"returned_item_line_count"`
	ReturnedUnits           int                       `json:"returned_units"`
	FullyCanceledOrderRate  float64                   `json:"fully_canceled_order_rate"`
	CanceledUnitRate        float64                   `json:"canceled_unit_rate"`
	ReturnedItemLineRate    float64                   `json:"returned_item_line_rate"`
	ReturnedUnitRate        float64                   `json:"returned_unit_rate"`
	PurchaseHours           []CountBucket             `json:"purchase_hours"`
	PurchaseWeekdays        []CountBucket             `json:"purchase_weekdays"`
	PurchaseMonths          []MonthlyOrderStats       `json:"purchase_months"`
	TopBrands               []CountBucket             `json:"top_brands"`
	DeliveryDuration        DeliveryDurationSummary   `json:"delivery_duration"`
	DeliveryByYear          []DeliveryDurationSummary `json:"delivery_by_year"`
	DeliveryTrend           DeliveryTrendComparison   `json:"delivery_trend"`
}

type CountBucket struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type MonthlyOrderStats struct {
	Month                   string `json:"month"`
	OrderCount              int    `json:"order_count"`
	TotalAmount             int64  `json:"total_amount"`
	NonCanceledTotalAmount  int64  `json:"non_canceled_total_amount"`
	FullyCanceledOrderCount int    `json:"fully_canceled_order_count"`
}

type DeliveryDurationSummary struct {
	Period        string  `json:"period,omitempty"`
	ShipmentCount int     `json:"shipment_count"`
	AverageHours  float64 `json:"average_hours"`
	MedianHours   float64 `json:"median_hours"`
	P90Hours      float64 `json:"p90_hours"`
}

type DeliveryTrendComparison struct {
	BaselinePeriod            string  `json:"baseline_period,omitempty"`
	LatestPeriod              string  `json:"latest_period,omitempty"`
	AverageHoursDelta         float64 `json:"average_hours_delta"`
	AverageHoursPercentChange float64 `json:"average_hours_percent_change"`
	Direction                 string  `json:"direction,omitempty"`
}

type ShoppingInsights struct {
	SchemaVersion               int                       `json:"schema_version"`
	From                        string                    `json:"from,omitempty"`
	To                          string                    `json:"to,omitempty"`
	FirstOrderDate              string                    `json:"first_order_date,omitempty"`
	LastOrderDate               string                    `json:"last_order_date,omitempty"`
	OrderCount                  int                       `json:"order_count"`
	DistinctOrderDays           int                       `json:"distinct_order_days"`
	MultiOrderDays              int                       `json:"multi_order_days"`
	MaxOrdersInOneDay           int                       `json:"max_orders_in_one_day"`
	AverageGapDays              float64                   `json:"average_gap_days"`
	LongestGapDays              int                       `json:"longest_gap_days"`
	ActiveMonthCount            int                       `json:"active_month_count"`
	LongestActiveMonthStreak    int                       `json:"longest_active_month_streak"`
	PeakPurchaseHourKST         CountBucket               `json:"peak_purchase_hour_kst"`
	PeakPurchaseWeekday         CountBucket               `json:"peak_purchase_weekday"`
	BusiestMonth                MonthlyOrderStats         `json:"busiest_month"`
	HighestSpendMonth           MonthlyOrderStats         `json:"highest_spend_month"`
	NightOrderRate              float64                   `json:"night_order_rate"`
	LateEveningOrderRate        float64                   `json:"late_evening_order_rate"`
	WeekendOrderRate            float64                   `json:"weekend_order_rate"`
	NightFullyCanceledOrderRate float64                   `json:"night_fully_canceled_order_rate"`
	OtherFullyCanceledOrderRate float64                   `json:"other_fully_canceled_order_rate"`
	NightReturnedUnitRate       float64                   `json:"night_returned_unit_rate"`
	OtherReturnedUnitRate       float64                   `json:"other_returned_unit_rate"`
	DeliveredWithin24HoursRate  float64                   `json:"delivered_within_24_hours_rate"`
	DeliveredWithin48HoursRate  float64                   `json:"delivered_within_48_hours_rate"`
	TopBrand                    CountBucket               `json:"top_brand"`
	TopBrandShare               float64                   `json:"top_brand_share"`
	RepeatPurchases             RepeatPurchaseInsights    `json:"repeat_purchases"`
	Basket                      BasketInsights            `json:"basket"`
	PurchaseTiming              PurchaseTimingInsights    `json:"purchase_timing"`
	Samples                     InsightSampleSizes        `json:"samples"`
	Profile                     ShoppingProfile           `json:"profile"`
	DeliveryByYear              []DeliveryDurationSummary `json:"delivery_by_year"`
	DeliveryTrend               DeliveryTrendComparison   `json:"delivery_trend"`
	PurchaseHours               []CountBucket             `json:"purchase_hours"`
	PurchaseMonths              []MonthlyOrderStats       `json:"purchase_months"`
	Categories                  CategoryBreakdown         `json:"categories"`
	Definitions                 InsightDefinitions        `json:"definitions"`
}

// ProductInsights contains private, local-only product names and exact dates.
// It is intentionally separate from the shareable ShoppingInsights response.
type ProductInsights struct {
	SchemaVersion              int                       `json:"schema_version"`
	Visibility                 string                    `json:"visibility"`
	From                       string                    `json:"from,omitempty"`
	To                         string                    `json:"to,omitempty"`
	Currency                   string                    `json:"currency"`
	FirstPurchaseDate          string                    `json:"first_purchase_date,omitempty"`
	LastPurchaseDate           string                    `json:"last_purchase_date,omitempty"`
	CalendarMonthCount         int                       `json:"calendar_month_count"`
	ActiveMonthCount           int                       `json:"active_month_count"`
	TotalSpendAmount           int64                     `json:"total_spend_amount"`
	AverageMonthlySpendAmount  int64                     `json:"average_monthly_spend_amount"`
	RetainedItemLineCount      int                       `json:"retained_item_line_count"`
	IdentifiedItemLineCount    int                       `json:"identified_item_line_count"`
	IdentifiedProductCount     int                       `json:"identified_product_count"`
	RetainedUnitCount          int                       `json:"retained_unit_count"`
	ProductIDCoverage          float64                   `json:"product_id_coverage"`
	SpendEligibleItemLineCount int                       `json:"spend_eligible_item_line_count"`
	SpendEligibleItemLineRate  float64                   `json:"spend_eligible_item_line_rate"`
	TopByUnits                 ProductAggregate          `json:"top_by_units"`
	TopByOrders                ProductAggregate          `json:"top_by_orders"`
	TopBySpend                 ProductAggregate          `json:"top_by_spend"`
	HighestPaidUnit            PaidUnitHighlight         `json:"highest_paid_unit"`
	LowestPaidUnit             PaidUnitHighlight         `json:"lowest_paid_unit"`
	HighestSpendDay            SpendDayInsight           `json:"highest_spend_day"`
	LowestSpendDay             SpendDayInsight           `json:"lowest_spend_day"`
	Definitions                ProductInsightDefinitions `json:"definitions"`
}

type ProductAggregate struct {
	Name                  string `json:"name,omitempty"`
	PurchaseCount         int    `json:"purchase_count"`
	UnitCount             int    `json:"unit_count"`
	TotalPaidAmount       int64  `json:"total_paid_amount"`
	AveragePaidUnitAmount int64  `json:"average_paid_unit_amount"`
	FirstPurchased        string `json:"first_purchased,omitempty"`
	LastPurchased         string `json:"last_purchased,omitempty"`
}

type PaidUnitHighlight struct {
	Name           string `json:"name,omitempty"`
	Date           string `json:"date,omitempty"`
	Quantity       int    `json:"quantity"`
	PaidAmount     int64  `json:"paid_amount"`
	PaidUnitAmount int64  `json:"paid_unit_amount"`
}

type SpendDayInsight struct {
	Date                  string              `json:"date,omitempty"`
	TotalAmount           int64               `json:"total_amount"`
	OrderCount            int                 `json:"order_count"`
	RetainedItemLineCount int                 `json:"retained_item_line_count"`
	ProductCount          int                 `json:"product_count"`
	Products              []DayProductSummary `json:"products"`
}

type DayProductSummary struct {
	Name       string `json:"name"`
	UnitCount  int    `json:"unit_count"`
	PaidAmount int64  `json:"paid_amount"`
}

type ProductInsightDefinitions struct {
	Provenance      string `json:"provenance"`
	ProductIdentity string `json:"product_identity"`
	RetainedUnit    string `json:"retained_unit"`
	ProductSpend    string `json:"product_spend"`
	PaidUnitAmount  string `json:"paid_unit_amount"`
	SpendDay        string `json:"spend_day"`
	DayProductLimit int    `json:"day_product_limit"`
}

type CategoryBreakdown struct {
	Method                 string           `json:"method"`
	Grouping               string           `json:"grouping"`
	TotalItemLines         int              `json:"total_item_lines"`
	ClassifiedItemLines    int              `json:"classified_item_lines"`
	ClassifiedItemLineRate float64          `json:"classified_item_line_rate"`
	RetainedUnits          int              `json:"retained_units"`
	Buckets                []CategoryBucket `json:"buckets"`
}

type ProductReference struct {
	ProductID    string `json:"product_id"`
	ItemID       string `json:"item_id,omitempty"`
	VendorItemID string `json:"vendor_item_id,omitempty"`
}

type ProductCategory struct {
	Source string                `json:"source"`
	Path   []ProductCategoryNode `json:"path"`
}

type ProductCategoryNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

type CategoryCatalogRequest struct {
	Query string `json:"query,omitempty" jsonschema:"Optional text matched only against observed Coupang category labels"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum category paths from 1 to 200; default 50"`
}

func (r CategoryCatalogRequest) Validate() error {
	query := strings.TrimSpace(r.Query)
	if !utf8.ValidString(query) || len([]rune(query)) > 100 {
		return errors.New("category query must not exceed 100 characters")
	}
	if r.Limit < 0 || r.Limit > 200 {
		return errors.New("category catalog limit must be between 1 and 200")
	}
	return nil
}

type CategoryCatalog struct {
	SchemaVersion         int                       `json:"schema_version"`
	Visibility            string                    `json:"visibility"`
	Source                string                    `json:"source"`
	Query                 string                    `json:"query,omitempty"`
	MatchMethod           string                    `json:"match_method"`
	Coverage              CategoryCatalogCoverage   `json:"coverage"`
	TotalCategoryCount    int                       `json:"total_category_count"`
	MatchedCategoryCount  int                       `json:"matched_category_count"`
	ReturnedCategoryCount int                       `json:"returned_category_count"`
	Truncated             bool                      `json:"truncated"`
	Categories            []CategoryCatalogEntry    `json:"categories"`
	Definitions           CategoryCatalogDefinition `json:"definitions"`
	Limitations           []string                  `json:"limitations"`
	Provenance            CategoryCatalogProvenance `json:"provenance"`
}

type CategoryCatalogCoverage struct {
	EligibleProductCount     int     `json:"eligible_product_count"`
	ClassifiedProductCount   int     `json:"classified_product_count"`
	UnclassifiedProductCount int     `json:"unclassified_product_count"`
	ClassifiedProductRate    float64 `json:"classified_product_rate"`
}

type CategoryCatalogEntry struct {
	CategoryID                   string                `json:"category_id"`
	Name                         string                `json:"name"`
	Position                     int                   `json:"position"`
	Depth                        int                   `json:"depth"`
	Role                         string                `json:"role"`
	Path                         []ProductCategoryNode `json:"path"`
	ObservedProductCount         int                   `json:"observed_product_count"`
	ObservedLeafProductCount     int                   `json:"observed_leaf_product_count"`
	ObservedAncestorProductCount int                   `json:"observed_ancestor_product_count"`
	MatchKind                    string                `json:"match_kind,omitempty"`
}

type CategoryCatalogDefinition struct {
	ProductUnit   string `json:"product_unit"`
	MatchRule     string `json:"match_rule"`
	CategoryRole  string `json:"category_role"`
	SearchHandoff string `json:"search_handoff"`
}

type CategoryCatalogProvenance struct {
	CategoryIDLabelAndPath string `json:"category_id_label_and_path"`
	ProductCounts          string `json:"product_counts"`
	QueryMatch             string `json:"query_match"`
}

type CategoryEnrichmentRequest struct {
	MaxProducts int `json:"max_products,omitempty"`
}

type CategoryEnrichmentResult struct {
	ProductsProcessed     int  `json:"products_processed"`
	CategoriesStored      int  `json:"categories_stored"`
	CategoriesMissing     int  `json:"categories_missing"`
	CategoriesUnavailable int  `json:"categories_unavailable"`
	RemainingProducts     int  `json:"remaining_products"`
	Complete              bool `json:"complete"`
}

type CategoryBucket struct {
	CategoryID    string  `json:"category_id"`
	Key           string  `json:"key"`
	ItemLineCount int     `json:"item_line_count"`
	UnitCount     int     `json:"unit_count"`
	UnitShare     float64 `json:"unit_share"`
}

type InsightSampleSizes struct {
	TimedOrders              int `json:"timed_orders"`
	NightOrders              int `json:"night_orders"`
	LateEveningOrders        int `json:"late_evening_orders"`
	NightWindowOrders        int `json:"night_window_orders"`
	DaytimeOrders            int `json:"daytime_orders"`
	OtherTimedOrders         int `json:"other_timed_orders"`
	NightOrderedUnits        int `json:"night_ordered_units"`
	OtherOrderedUnits        int `json:"other_ordered_units"`
	DeliveryEvents           int `json:"delivery_events"`
	BrandedRetainedItemLines int `json:"branded_retained_item_lines"`
}

type ShoppingProfile struct {
	SchemaVersion int                   `json:"schema_version"`
	RuleVersion   string                `json:"rule_version"`
	Ready         bool                  `json:"ready"`
	Code          string                `json:"code"`
	Axes          []ShoppingProfileAxis `json:"axes"`
	Badges        []ShoppingBadge       `json:"badges"`
}

type ShoppingProfileAxis struct {
	ID              string  `json:"id"`
	SelectedCode    string  `json:"selected_code"`
	OppositeCode    string  `json:"opposite_code"`
	Metric          string  `json:"metric"`
	Score           float64 `json:"score"`
	Threshold       float64 `json:"threshold"`
	ThresholdBasis  string  `json:"threshold_basis"`
	Numerator       int     `json:"numerator,omitempty"`
	Denominator     int     `json:"denominator,omitempty"`
	SampleSize      int     `json:"sample_size"`
	ObservationDays int     `json:"observation_days,omitempty"`
	Timezone        string  `json:"timezone,omitempty"`
	Provenance      string  `json:"provenance"`
	Ready           bool    `json:"ready"`
}

type ShoppingBadge struct {
	ID    string  `json:"id"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type RepeatPurchaseInsights struct {
	IdentifiedProductCount             int     `json:"identified_product_count"`
	RepeatProductCount                 int     `json:"repeat_product_count"`
	RepeatProductRate                  float64 `json:"repeat_product_rate"`
	PurchaseOccasionCount              int     `json:"purchase_occasion_count"`
	RepeatProductPurchaseOccasionCount int     `json:"repeat_product_purchase_occasion_count"`
	RepeatProductPurchaseOccasionRate  float64 `json:"repeat_product_purchase_occasion_rate"`
	RepeatChoiceCount                  int     `json:"repeat_choice_count"`
	RepeatChoiceRate                   float64 `json:"repeat_choice_rate"`
	RetainedItemLineCount              int     `json:"retained_item_line_count"`
	IdentifiedItemLineCount            int     `json:"identified_item_line_count"`
	ProductIDCoverage                  float64 `json:"product_id_coverage"`
	MostRepeatedProductPurchaseCount   int     `json:"most_repeated_product_purchase_count"`
}

type BasketInsights struct {
	RetainedOrderCount         int     `json:"retained_order_count"`
	RetainedItemLineCount      int     `json:"retained_item_line_count"`
	SingleItemOrderCount       int     `json:"single_item_order_count"`
	SingleItemOrderRate        float64 `json:"single_item_order_rate"`
	AverageItemLines           float64 `json:"average_item_lines"`
	MaxItemLines               int     `json:"max_item_lines"`
	RetainedItemAmount         int64   `json:"retained_item_amount"`
	SingleItemOrderAmount      int64   `json:"single_item_order_amount"`
	SingleItemSpendRate        float64 `json:"single_item_spend_rate"`
	MedianSingleItemOrderValue int64   `json:"median_single_item_order_value"`
	MedianMultiItemOrderValue  int64   `json:"median_multi_item_order_value"`
	CompositionOrderCount      int     `json:"composition_order_count"`
	SingleProductOrderCount    int     `json:"single_product_order_count"`
	SingleProductOrderRate     float64 `json:"single_product_order_rate"`
	AverageDistinctProducts    float64 `json:"average_distinct_products"`
	MaxDistinctProducts        int     `json:"max_distinct_products"`
}

type PurchaseTimingInsights struct {
	Clumpiness        float64 `json:"clumpiness"`
	UniformNullMedian float64 `json:"uniform_null_median"`
	PurchaseDays      int     `json:"purchase_days"`
	ObservationDays   int     `json:"observation_days"`
}

type InsightDefinitions struct {
	NightHoursKST       string `json:"night_hours_kst"`
	LateEveningHoursKST string `json:"late_evening_hours_kst"`
	RateScale           string `json:"rate_scale"`
	RepeatProduct       string `json:"repeat_product"`
	RepeatChoice        string `json:"repeat_choice"`
	BasketComposition   string `json:"basket_composition"`
	PurchaseClumpiness  string `json:"purchase_clumpiness"`
}

type ReorderCandidate struct {
	ProductID       string                 `json:"product_id,omitempty"`
	VendorItemID    string                 `json:"vendor_item_id,omitempty"`
	Name            string                 `json:"name"`
	PurchaseCount   int                    `json:"purchase_count"`
	TotalQuantity   int                    `json:"total_quantity"`
	LastPurchased   string                 `json:"last_purchased"`
	PriceComparison ReorderPriceComparison `json:"price_comparison"`
}

type ReorderPriceComparison struct {
	Status                  string   `json:"status"`
	LastPaidUnitAmountKRW   int64    `json:"last_paid_unit_amount_krw,omitempty"`
	LastPaidAt              string   `json:"last_paid_at,omitempty"`
	LatestObservedAmountKRW int64    `json:"latest_observed_amount_krw,omitempty"`
	ObservedAt              string   `json:"observed_at,omitempty"`
	ObservationAgeHours     int      `json:"observation_age_hours,omitempty"`
	Freshness               string   `json:"freshness,omitempty"`
	DifferenceKRW           int64    `json:"difference_krw,omitempty"`
	DifferencePercent       float64  `json:"difference_percent,omitempty"`
	Direction               string   `json:"direction,omitempty"`
	Provenance              string   `json:"provenance"`
	Limitations             []string `json:"limitations"`
}

type OrderExport struct {
	SchemaVersion int       `json:"schema_version"`
	ExportedAt    time.Time `json:"exported_at"`
	Orders        []Order   `json:"orders"`
}

type PurgeResult struct {
	OrdersDeleted int `json:"orders_deleted"`
	ItemsDeleted  int `json:"items_deleted"`
}

type SyncRequest struct {
	MaxPages int `json:"max_pages,omitempty"`
}

type SyncResult struct {
	Complete       bool         `json:"complete"`
	PagesProcessed int          `json:"pages_processed"`
	OrdersSeen     int          `json:"orders_seen"`
	ItemsSeen      int          `json:"items_seen"`
	OrdersRemoved  int          `json:"orders_removed,omitempty"`
	Next           *OrderCursor `json:"next,omitempty"`
}

type OrderList struct {
	Orders []Order `json:"orders"`
}

type ReorderList struct {
	SchemaVersion int                `json:"schema_version"`
	Visibility    string             `json:"visibility"`
	Candidates    []ReorderCandidate `json:"candidates"`
	Definitions   ReorderDefinitions `json:"definitions"`
}

type ReorderDefinitions struct {
	CandidateIdentity string `json:"candidate_identity"`
	PurchaseEvidence  string `json:"purchase_evidence"`
	PriceComparison   string `json:"price_comparison"`
}

func NewReorderList(candidates []ReorderCandidate) ReorderList {
	if candidates == nil {
		candidates = []ReorderCandidate{}
	}
	return ReorderList{
		SchemaVersion: 1,
		Visibility:    "private_local",
		Candidates:    candidates,
		Definitions: ReorderDefinitions{
			CandidateIdentity: "vendor_item_id_else_product_id_else_exact_name",
			PurchaseEvidence:  "retained product-purchase lines from the normalized structured order model",
			PriceComparison:   "latest locally observed exact-identity price minus the latest eligible paid unit amount; recent means observed less than 24 hours ago, and no live price is fetched by this command",
		},
	}
}

type RecapWriteResult struct {
	Written    bool   `json:"written"`
	Format     string `json:"format"`
	Visibility string `json:"visibility"`
	Bytes      int64  `json:"bytes"`
}
