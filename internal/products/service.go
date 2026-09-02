package products

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

const (
	defaultSearchLimit = 10
	defaultReviewLimit = 5
	defaultImageLimit  = 20

	affiliateDisclosure = "이 링크를 통해 구매하면 쿠팡 파트너스 활동의 일환으로 일정액의 수수료를 제공받습니다."
	priceNotice         = "제휴 링크 자체로 구매자에게 별도 수수료가 부과되지는 않습니다. 상품 가격과 할인 혜택은 변동될 수 있으므로 쿠팡의 최종 화면에서 확인하세요."
)

var ErrSourceUnavailable = errors.New("product document source unavailable")

var (
	computerCapacityPattern = regexp.MustCompile(`(?i)([0-9]{1,4})\s*(TB|GB|G)\b`)
	explicitMemoryPattern   = regexp.MustCompile(`(?i)(?:RAM|메모리)\s*[:/-]?\s*([0-9]{1,3})\s*(?:GB|G)\b`)
	explicitStoragePattern  = regexp.MustCompile(`(?i)(?:SSD|NVME)\s*[:/-]?\s*([0-9]{2,4})\s*(?:GB|G)\b`)
	cpuPattern              = regexp.MustCompile(`(?i)(라이젠\s*[3579](?:[-\s]*[0-9]{4}[A-Z]*)?|R[3579](?:[-\s]*[0-9]{4}[A-Z]*)?|(?:인텔\s*)?(?:코어\s*)?(?:ULTRA\s*[3579](?:[-\s]*[0-9]{3}[A-Z]*)?|I[3579](?:(?:[-\s]*[0-9]{1,2}\s*세대\s*[0-9]{4,5}[A-Z]*)|(?:[-\s]*[0-9]{4,5}[A-Z]*))?)|N-?시리즈)`)
	gpuPattern              = regexp.MustCompile(`(?i)(지포스\s*(?:RTX|GTX)\s*[0-9]{3,4}(?:\s*TI)?|(?:RTX|GTX)\s*[0-9]{3,4}(?:\s*TI)?|(?:라데온|RADEON)\s+VEGA\s*[0-9]+)`)
)

type Source interface {
	Search(context.Context, core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error)
	Inspect(context.Context, core.ProductInspectRequest) (core.ProductInspection, error)
	AddToCart(context.Context, core.CartAddRequest) (core.CartAddResult, error)
}

type AffiliateLinker interface {
	Convert(context.Context, []string) (map[string]string, error)
}

func (s *Service) AddToCart(ctx context.Context, request core.CartAddRequest) (core.CartAddResult, error) {
	if request.Quantity == 0 {
		request.Quantity = 1
	}
	if err := request.Validate(); err != nil {
		return core.CartAddResult{}, err
	}
	if s.source == nil {
		return core.CartAddResult{}, ErrSourceUnavailable
	}
	result, err := s.source.AddToCart(ctx, request)
	if err != nil {
		return core.CartAddResult{}, errors.Join(ErrSourceUnavailable, err)
	}
	result.SchemaVersion = core.ProductSchemaVersion
	return result, nil
}

type Service struct {
	source    Source
	affiliate AffiliateLinker
	now       func() time.Time
}

func New(source Source) *Service {
	return &Service{source: source, now: time.Now}
}

func NewWithAffiliate(source Source, affiliate AffiliateLinker) *Service {
	return &Service{source: source, affiliate: affiliate, now: time.Now}
}

func (s *Service) Search(ctx context.Context, request core.ProductSearchRequest) (core.ProductSearchResult, error) {
	request.Query = strings.TrimSpace(request.Query)
	if request.Limit == 0 {
		request.Limit = defaultSearchLimit
	}
	if request.Sort == "" {
		request.Sort = core.ProductSortRelevance
	}
	if err := request.Validate(); err != nil {
		return core.ProductSearchResult{}, err
	}
	if s.source == nil {
		return core.ProductSearchResult{}, ErrSourceUnavailable
	}
	items, coverage, err := s.source.Search(ctx, request)
	if err != nil {
		return core.ProductSearchResult{}, errors.Join(ErrSourceUnavailable, err)
	}
	items = enrichProductCards(items)
	items = filterProducts(items, request)
	sortProducts(items, request.Sort)
	collapsed := 0
	if !request.IncludeVariants {
		items, collapsed = collapseListingVariants(items)
	} else {
		annotateVariantCounts(items)
	}
	if len(items) > request.Limit {
		items = items[:request.Limit]
	}
	warnings := []string{}
	if len(items) == 0 {
		warnings = append(warnings, "no products matched every explicit filter")
	}
	if collapsed > 0 {
		warnings = append(warnings, "multiple options from the same Coupang product page were collapsed; set include_variants=true to inspect every observed option")
	}
	affiliate, affiliateWarnings := s.applyAffiliateLinks(ctx, items, request.DisableAffiliate)
	warnings = append(warnings, affiliateWarnings...)
	return core.ProductSearchResult{
		SchemaVersion: core.ProductSchemaVersion,
		Query:         request.Query, Currency: "KRW", FetchedAt: s.now().UTC(), Items: items,
		AppliedFilters: request, Coverage: coverage, Ranking: rankingSummary(request), Affiliate: affiliate, Warnings: warnings,
	}, nil
}

func (s *Service) Inspect(ctx context.Context, request core.ProductInspectRequest) (core.ProductInspection, error) {
	if request.ReviewLimit == 0 {
		request.ReviewLimit = defaultReviewLimit
	}
	if request.DetailImageLimit == 0 {
		request.DetailImageLimit = defaultImageLimit
	}
	if err := request.Validate(); err != nil {
		return core.ProductInspection{}, err
	}
	if s.source == nil {
		return core.ProductInspection{}, ErrSourceUnavailable
	}
	result, err := s.source.Inspect(ctx, request)
	if err != nil {
		return core.ProductInspection{}, errors.Join(ErrSourceUnavailable, err)
	}
	result.SchemaVersion = core.ProductSchemaVersion
	result.FetchedAt = s.now().UTC()
	computerEvidence := append([]string{result.Product.Name}, result.SelectedOptions...)
	result.Product.ComputerSpecs = parseComputerSpecifications(strings.Join(computerEvidence, " "))
	if len(result.Reviews) > request.ReviewLimit {
		result.Reviews = result.Reviews[:request.ReviewLimit]
	}
	if len(result.DetailImages) > request.DetailImageLimit {
		result.DetailImages = result.DetailImages[:request.DetailImageLimit]
	}
	if result.Benefits == nil {
		result.Benefits = []core.ProductBenefit{}
	}
	if result.Reviews == nil {
		result.Reviews = []core.ProductReview{}
	}
	products := []core.ProductCard{result.Product}
	affiliate, warnings := s.applyAffiliateLinks(ctx, products, request.DisableAffiliate)
	result.Product = products[0]
	result.Affiliate = affiliate
	result.Warnings = append(result.Warnings, warnings...)
	return result, nil
}

func (s *Service) applyAffiliateLinks(ctx context.Context, items []core.ProductCard, disabled bool) (core.ProductAffiliateDisclosure, []string) {
	disclosure := core.ProductAffiliateDisclosure{
		Status:                core.ProductAffiliateDisabled,
		SelfPurchasesEligible: false,
	}
	if disabled {
		return disclosure, nil
	}
	if s.affiliate == nil {
		disclosure.Status = core.ProductAffiliateUnconfigured
		return disclosure, nil
	}
	if len(items) == 0 {
		return configuredAffiliateDisclosure(core.ProductAffiliateNotApplicable), nil
	}
	urls := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.URL == "" {
			continue
		}
		if _, exists := seen[item.URL]; exists {
			continue
		}
		seen[item.URL] = struct{}{}
		urls = append(urls, item.URL)
	}
	if len(urls) == 0 {
		disclosure.Status = core.ProductAffiliateUnavailable
		return disclosure, []string{"affiliate link generation was unavailable because no canonical product URLs were observed"}
	}
	links, err := s.affiliate.Convert(ctx, urls)
	if err != nil {
		disclosure.Status = core.ProductAffiliateUnavailable
		return disclosure, []string{"affiliate link generation was unavailable; canonical product URLs were preserved"}
	}
	applied := 0
	for index := range items {
		if affiliateURL := links[items[index].URL]; affiliateURL != "" {
			items[index].AffiliateURL = affiliateURL
			applied++
		}
	}
	disclosure = configuredAffiliateDisclosure(core.ProductAffiliateApplied)
	if applied == len(items) {
		return disclosure, nil
	}
	disclosure.Status = core.ProductAffiliatePartial
	return disclosure, []string{"affiliate links were generated for only some products; every canonical product URL was preserved"}
}

func configuredAffiliateDisclosure(status core.ProductAffiliateStatus) core.ProductAffiliateDisclosure {
	return core.ProductAffiliateDisclosure{
		Status:                  status,
		Source:                  "coupang_partners_deeplink_api",
		CommissionRecipient:     "configured_coupangctl_operator",
		BuyerPriceEffect:        "no_separate_affiliate_fee",
		SelfPurchasesEligible:   false,
		Disclosure:              affiliateDisclosure,
		PriceVerificationNotice: priceNotice,
	}
}

func filterProducts(items []core.ProductCard, request core.ProductSearchRequest) []core.ProductCard {
	filtered := make([]core.ProductCard, 0, len(items))
	for _, item := range items {
		priceObserved := contains(item.ObservedFields, "price.current_amount")
		ratingObserved := contains(item.ObservedFields, "rating")
		reviewsObserved := contains(item.ObservedFields, "review_count")
		if request.MinPrice > 0 && (!priceObserved || item.Price.CurrentAmount < request.MinPrice) {
			continue
		}
		if request.MaxPrice > 0 && (!priceObserved || item.Price.CurrentAmount > request.MaxPrice) {
			continue
		}
		if request.MinRating > 0 && (!ratingObserved || item.Rating < request.MinRating) {
			continue
		}
		if request.MinReviewCount > 0 && (!reviewsObserved || item.ReviewCount < request.MinReviewCount) {
			continue
		}
		if request.RocketOnly && !item.Rocket {
			continue
		}
		if request.FreeShippingOnly && !item.FreeShipping {
			continue
		}
		if request.ExcludeSponsored && item.Sponsored {
			continue
		}
		if request.MinMemoryGB > 0 && (item.ComputerSpecs == nil || item.ComputerSpecs.MemoryGB < request.MinMemoryGB) {
			continue
		}
		if request.MinStorageGB > 0 && (item.ComputerSpecs == nil || item.ComputerSpecs.StorageGB < request.MinStorageGB) {
			continue
		}
		if request.ExcludeUsed && item.ComputerSpecs != nil {
			switch item.ComputerSpecs.Condition {
			case "used", "refurbished", "display_unit":
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func sortProducts(items []core.ProductCard, order core.ProductSort) {
	if order == core.ProductSortRelevance {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		switch order {
		case core.ProductSortPriceAsc:
			return items[i].Price.CurrentAmount < items[j].Price.CurrentAmount
		case core.ProductSortPriceDesc:
			return items[i].Price.CurrentAmount > items[j].Price.CurrentAmount
		case core.ProductSortRating:
			return items[i].Rating > items[j].Rating
		case core.ProductSortReviewCount:
			return items[i].ReviewCount > items[j].ReviewCount
		default:
			return false
		}
	})
}

func enrichProductCards(items []core.ProductCard) []core.ProductCard {
	result := append([]core.ProductCard(nil), items...)
	for index := range result {
		result[index].ComputerSpecs = parseComputerSpecifications(result[index].Name)
		if result[index].VariantCount < 1 {
			result[index].VariantCount = 1
		}
	}
	return result
}

func parseComputerSpecifications(name string) *core.ComputerSpecifications {
	specs := core.ComputerSpecifications{Condition: "unspecified", Source: "observed_product_title"}
	upper := strings.ToUpper(name)
	if match := explicitMemoryPattern.FindStringSubmatch(name); len(match) == 2 {
		specs.MemoryGB, _ = strconv.Atoi(match[1])
	}
	if match := explicitStoragePattern.FindStringSubmatch(name); len(match) == 2 {
		specs.StorageGB, _ = strconv.Atoi(match[1])
	}
	for _, match := range computerCapacityPattern.FindAllStringSubmatch(name, -1) {
		amount, _ := strconv.Atoi(match[1])
		if strings.EqualFold(match[2], "TB") {
			amount *= 1024
		}
		if amount >= 128 && amount > specs.StorageGB {
			specs.StorageGB = amount
		} else if amount >= 4 && amount <= 128 && (specs.MemoryGB == 0 || amount < specs.MemoryGB) {
			specs.MemoryGB = amount
		}
	}
	for _, match := range cpuPattern.FindAllString(name, -1) {
		match = strings.Join(strings.Fields(match), " ")
		if len([]rune(match)) > len([]rune(specs.CPU)) {
			specs.CPU = match
		}
	}
	if match := gpuPattern.FindString(name); match != "" {
		specs.GPU = strings.Join(strings.Fields(match), " ")
	}
	switch {
	case strings.Contains(upper, "WIN11") || strings.Contains(upper, "WINDOWS 11"):
		specs.OS = "Windows 11"
	case strings.Contains(upper, "FREE DOS") || strings.Contains(upper, "FREEDOS"):
		specs.OS = "Free DOS"
	}
	switch {
	case strings.Contains(name, "리퍼비시") || strings.Contains(name, "리퍼노트북") || strings.Contains(upper, "REFURBISHED"):
		specs.Condition = "refurbished"
	case strings.Contains(name, "중고노트북") || strings.Contains(upper, " USED "):
		specs.Condition = "used"
	case strings.Contains(name, "전시상품") || strings.Contains(name, "전시품") || strings.Contains(name, "전시용") || strings.Contains(upper, "DISPLAY UNIT"):
		specs.Condition = "display_unit"
	}
	if specs.MemoryGB == 0 && specs.StorageGB == 0 && specs.CPU == "" && specs.GPU == "" && specs.OS == "" {
		return nil
	}
	return &specs
}

func collapseListingVariants(items []core.ProductCard) ([]core.ProductCard, int) {
	counts := map[string]int{}
	for _, item := range items {
		counts[listingKey(item)]++
	}
	result := make([]core.ProductCard, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		key := listingKey(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		item.VariantCount = counts[key]
		result = append(result, item)
	}
	return result, len(items) - len(result)
}

func annotateVariantCounts(items []core.ProductCard) {
	counts := map[string]int{}
	for _, item := range items {
		counts[listingKey(item)]++
	}
	for index := range items {
		items[index].VariantCount = counts[listingKey(items[index])]
	}
}

func listingKey(item core.ProductCard) string {
	if item.Reference.ProductID != "" {
		return "product:" + item.Reference.ProductID
	}
	return "vendor:" + item.Reference.VendorItemID
}

func rankingSummary(request core.ProductSearchRequest) core.ProductRankingSummary {
	scope := "search"
	if request.CategoryID != "" {
		scope = "category"
	}
	result := core.ProductRankingSummary{Requested: request.Sort, Applied: request.Sort, Scope: scope}
	switch request.Sort {
	case core.ProductSortRating, core.ProductSortReviewCount:
		result.Source = "local_observed_field_sort"
		result.Description = "sorted locally from observed product-card fields; not a Coupang sales rank"
	case core.ProductSortSales:
		result.Source = "coupang_search_order"
		result.SourceNative = true
		result.Description = "Coupang 판매량순 result order; absolute unit sales are not exposed"
	case core.ProductSortLatest:
		result.Source = "coupang_search_order"
		result.SourceNative = true
		result.Description = "Coupang 최신순 result order"
	case core.ProductSortPriceAsc, core.ProductSortPriceDesc:
		result.Source = "coupang_search_order"
		result.SourceNative = true
		result.Description = "Coupang price-sort order with local deterministic verification"
	default:
		result.Applied = core.ProductSortCoupangRanking
		result.Source = "coupang_search_order"
		result.SourceNative = true
		result.Description = "Coupang ranking combines sales performance, user preference, product information quality, and search relevance"
	}
	return result
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
