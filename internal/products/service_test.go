package products

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type syntheticSource struct {
	items      []core.ProductCard
	inspection core.ProductInspection
	cart       core.CartAddResult
}

type syntheticPriceHistory struct {
	recorded     []core.ProductPriceObservation
	observations []core.ProductPriceObservation
	truncated    bool
	err          error
}

func (s *syntheticPriceHistory) RecordPriceObservations(_ context.Context, observations []core.ProductPriceObservation) error {
	s.recorded = append(s.recorded, observations...)
	return s.err
}

func (s *syntheticPriceHistory) ListPriceObservations(context.Context, core.ProductPriceHistoryRequest) ([]core.ProductPriceObservation, bool, error) {
	return append([]core.ProductPriceObservation(nil), s.observations...), s.truncated, s.err
}

func (s *syntheticPriceHistory) PurgePriceObservations(context.Context) (int, error) {
	deleted := len(s.observations)
	s.observations = nil
	return deleted, s.err
}

func (s syntheticSource) Search(context.Context, core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	return s.items, core.ProductCoverage{Source: "synthetic"}, nil
}

func (s syntheticSource) Inspect(context.Context, core.ProductInspectRequest) (core.ProductInspection, error) {
	return s.inspection, nil
}

func (s syntheticSource) AddToCart(_ context.Context, request core.CartAddRequest) (core.CartAddResult, error) {
	result := s.cart
	result.Quantity = request.Quantity
	result.Product = core.ProductReference{ProductID: request.ProductID, ItemID: request.ItemID, VendorItemID: request.VendorItemID}
	return result, nil
}

func TestSearchAppliesNaturalLanguageDerivedFiltersWithoutTreatingUnknownAsMatch(t *testing.T) {
	service := New(syntheticSource{items: []core.ProductCard{
		{Name: "known match", Price: core.ProductPrice{CurrentAmount: 79000}, Rating: 4.8, ReviewCount: 200, Rocket: true, ObservedFields: []string{"price.current_amount", "rating", "review_count"}},
		{Name: "unknown price", Rating: 5, ReviewCount: 500, Rocket: true, ObservedFields: []string{"rating", "review_count"}},
		{Name: "over budget", Price: core.ProductPrice{CurrentAmount: 120000}, Rating: 4.9, ReviewCount: 800, Rocket: true, ObservedFields: []string{"price.current_amount", "rating", "review_count"}},
	}})
	service.now = func() time.Time { return time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC) }

	result, err := service.Search(context.Background(), core.ProductSearchRequest{
		Query: "맥북용 허브 중 후기 좋고 10만원 아래 로켓배송", MaxPrice: 100000,
		MinRating: 4.5, MinReviewCount: 100, RocketOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "known match" {
		t.Fatalf("unexpected filtered products: %#v", result.Items)
	}
	if result.SchemaVersion != core.ProductSchemaVersion || result.AppliedFilters.Limit != 10 || result.AppliedFilters.Sort != core.ProductSortRelevance {
		t.Fatalf("unexpected response metadata: %#v", result)
	}
}

func TestSearchRecordsOnlyReturnedObservedCurrentPrices(t *testing.T) {
	history := &syntheticPriceHistory{}
	service := NewWithAffiliateAndPrices(syntheticSource{items: []core.ProductCard{
		{Reference: core.ProductReference{ProductID: "101", VendorItemID: "201"}, Name: "Synthetic observed", URL: "https://www.coupang.com/vp/products/101", Price: core.ProductPrice{CurrentAmount: 42000}, ObservedFields: []string{"price.current_amount"}},
		{Reference: core.ProductReference{ProductID: "102", VendorItemID: "202"}, Name: "Synthetic unknown", URL: "https://www.coupang.com/vp/products/102", Price: core.ProductPrice{CurrentAmount: 51000}},
	}}, nil, history)
	service.now = func() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }

	result, err := service.Search(context.Background(), core.ProductSearchRequest{Query: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || len(history.recorded) != 1 || history.recorded[0].CurrentAmount != 42000 || history.recorded[0].Source != "coupang_product_search" {
		t.Fatalf("unexpected price recording: result=%#v recorded=%#v", result.Items, history.recorded)
	}
}

func TestPriceHistoryKeepsVariantsSeparateAndDerivesPerSeriesTrend(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	history := &syntheticPriceHistory{observations: []core.ProductPriceObservation{
		{Reference: core.ProductReference{ProductID: "101", VendorItemID: "201"}, Name: "Synthetic A", CurrentAmount: 42000, Currency: "KRW", ObservedAt: base, Source: "coupang_product_search", Provenance: "observed"},
		{Reference: core.ProductReference{ProductID: "101", VendorItemID: "202"}, Name: "Synthetic B", CurrentAmount: 51000, Currency: "KRW", ObservedAt: base.Add(time.Hour), Source: "coupang_product_search", Provenance: "observed"},
		{Reference: core.ProductReference{ProductID: "101", VendorItemID: "201"}, Name: "Synthetic A", CurrentAmount: 39000, Currency: "KRW", ObservedAt: base.Add(2 * time.Hour), Source: "coupang_product_inspection", Provenance: "observed"},
	}}
	result, err := NewWithAffiliateAndPrices(syntheticSource{}, nil, history).PriceHistory(context.Background(), core.ProductPriceHistoryRequest{ProductID: "101"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Visibility != "private_local" || result.SeriesCount != 2 || result.ObservationCount != 3 || len(result.Warnings) != 1 {
		t.Fatalf("unexpected price history envelope: %#v", result)
	}
	var exact *core.ProductPriceSeries
	for index := range result.Series {
		if result.Series[index].Reference.VendorItemID == "201" {
			exact = &result.Series[index]
		}
	}
	if exact == nil || exact.Trend.Direction != "lower" || exact.Trend.ChangeFromFirstReturnedKRW != -3000 || exact.Trend.ChangeFromFirstReturnedPercent != -7.14 || exact.Trend.Provenance == "observed" {
		t.Fatalf("unexpected exact-option trend: %#v", exact)
	}
}

func TestSearchNormalizesComputerSpecsAndCollapsesListingVariants(t *testing.T) {
	service := New(syntheticSource{items: []core.ProductCard{
		{Reference: core.ProductReference{ProductID: "101", VendorItemID: "201"}, Name: "Synthetic laptop, 256GB, 16GB, Free DOS", Price: core.ProductPrice{CurrentAmount: 700000}, Rating: 4.8, ReviewCount: 500, SearchPosition: 1, ObservedFields: []string{"price.current_amount", "rating", "review_count"}},
		{Reference: core.ProductReference{ProductID: "101", VendorItemID: "202"}, Name: "Synthetic laptop, 512GB, 16GB, WIN11 Home", Price: core.ProductPrice{CurrentAmount: 800000}, Rating: 4.8, ReviewCount: 500, SearchPosition: 2, ObservedFields: []string{"price.current_amount", "rating", "review_count"}},
		{Reference: core.ProductReference{ProductID: "102", VendorItemID: "203"}, Name: "Synthetic 리퍼비시 laptop, 512GB, 16GB, WIN11 Home", Price: core.ProductPrice{CurrentAmount: 400000}, Rating: 5, ReviewCount: 900, SearchPosition: 3, ObservedFields: []string{"price.current_amount", "rating", "review_count"}},
	}})

	result, err := service.Search(context.Background(), core.ProductSearchRequest{
		Query: "16GB 512GB laptop", MinMemoryGB: 16, MinStorageGB: 512,
		ExcludeUsed: true, Sort: core.ProductSortCoupangRanking,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Reference.VendorItemID != "202" {
		t.Fatalf("unexpected diverse result: %#v", result.Items)
	}
	if result.Items[0].ComputerSpecs.MemoryGB != 16 || result.Items[0].ComputerSpecs.StorageGB != 512 || result.Items[0].VariantCount != 1 {
		t.Fatalf("computer specs or variant evidence missing: %#v", result.Items[0])
	}
	if result.Ranking.Applied != core.ProductSortCoupangRanking || result.Ranking.Source != "coupang_search_order" {
		t.Fatalf("native ranking evidence missing: %#v", result.Ranking)
	}
}

func TestSearchCanReturnAllExplicitVariants(t *testing.T) {
	service := New(syntheticSource{items: []core.ProductCard{
		{Reference: core.ProductReference{ProductID: "101", VendorItemID: "201"}, Name: "Synthetic option A"},
		{Reference: core.ProductReference{ProductID: "101", VendorItemID: "202"}, Name: "Synthetic option B"},
	}})
	result, err := service.Search(context.Background(), core.ProductSearchRequest{Query: "synthetic", IncludeVariants: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("explicit variants were collapsed: %#v", result.Items)
	}
}

func TestComputerSpecificationParserRecognizesCommonKoreanListingSpellings(t *testing.T) {
	tests := []struct {
		name string
		cpu  string
		gpu  string
	}{
		{name: "Synthetic desktop R5 5500GT, 라데온 Vega 7, 16GB, 512GB", cpu: "R5 5500GT", gpu: "라데온 Vega 7"},
		{name: "Synthetic desktop [AMD] 라이젠5-9600X RTX 5060 16GB 512GB", cpu: "라이젠5-9600X", gpu: "RTX 5060"},
		{name: "Synthetic notebook 인텔 코어Ultra5 16GB 512GB", cpu: "인텔 코어Ultra5"},
		{name: "Synthetic notebook Ultra5 16GB 512GB", cpu: "Ultra5"},
		{name: "Synthetic 코어i5 인텔 14세대 RTX 5060 코어i5-14400F 16GB 512GB", cpu: "코어i5-14400F", gpu: "RTX 5060"},
		{name: "Synthetic 라이젠5 라이젠 7000 시리즈 RTX 5060 라이젠5-7500F 16GB 512GB", cpu: "라이젠5-7500F", gpu: "RTX 5060"},
		{name: "Synthetic 인텔 코어i5-14세대 14400F RTX 5060 16GB 512GB", cpu: "인텔 코어i5-14세대 14400F", gpu: "RTX 5060"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			specs := parseComputerSpecifications(test.name)
			if specs == nil || specs.CPU != test.cpu || specs.GPU != test.gpu || specs.MemoryGB != 16 || specs.StorageGB != 512 {
				t.Fatalf("unexpected parsed specifications: %#v", specs)
			}
		})
	}
}

func TestInspectDerivesComputerSpecificationsFromObservedSelectedOptions(t *testing.T) {
	service := New(syntheticSource{inspection: core.ProductInspection{
		Product:         core.ProductCard{Name: "Synthetic gaming desktop"},
		SelectedOptions: []string{"16GB, 512GB, R5 5500GT, 라데온 Vega 7, WIN11 Home"},
	}})
	result, err := service.Inspect(context.Background(), core.ProductInspectRequest{ProductID: "101"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Product.ComputerSpecs == nil || result.Product.ComputerSpecs.CPU != "R5 5500GT" || result.Product.ComputerSpecs.GPU != "라데온 Vega 7" || result.Product.ComputerSpecs.OS != "Windows 11" {
		t.Fatalf("selected-option specifications were not preserved: %#v", result.Product.ComputerSpecs)
	}
}

func TestCartRequiresExplicitConfirmationAndDefaultsQuantity(t *testing.T) {
	service := New(syntheticSource{cart: core.CartAddResult{Attempted: true, Added: true, Verified: true}})
	request := core.CartAddRequest{ProductID: "101", VendorItemID: "201"}
	if _, err := service.AddToCart(context.Background(), request); err == nil {
		t.Fatal("unconfirmed cart mutation was accepted")
	}
	request.Confirmed = true
	result, err := service.AddToCart(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Quantity != 1 || result.SchemaVersion != core.ProductSchemaVersion {
		t.Fatalf("unexpected cart response: %#v", result)
	}
}

type syntheticAffiliateLinker struct {
	links map[string]string
	err   error
	calls int
}

func (s *syntheticAffiliateLinker) Convert(_ context.Context, _ []string) (map[string]string, error) {
	s.calls++
	return s.links, s.err
}

func TestSearchAddsAffiliateLinkWithoutReplacingCanonicalURL(t *testing.T) {
	canonical := "https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301"
	linker := &syntheticAffiliateLinker{links: map[string]string{canonical: "https://link.coupang.com/a/synthetic"}}
	service := NewWithAffiliate(syntheticSource{items: []core.ProductCard{{
		Reference: core.ProductReference{ProductID: "101", ItemID: "201", VendorItemID: "301"},
		Name:      "Synthetic product", URL: canonical,
	}}}, linker)

	result, err := service.Search(context.Background(), core.ProductSearchRequest{Query: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[0].URL != canonical || result.Items[0].AffiliateURL != "https://link.coupang.com/a/synthetic" {
		t.Fatalf("canonical and affiliate URLs were not preserved separately: %#v", result.Items[0])
	}
	if result.Affiliate.Status != "applied" || result.Affiliate.BuyerPriceEffect != "no_separate_affiliate_fee" || result.Affiliate.SelfPurchasesEligible {
		t.Fatalf("affiliate disclosure is incomplete: %#v", result.Affiliate)
	}
	if result.Affiliate.Disclosure == "" || result.Affiliate.PriceVerificationNotice == "" {
		t.Fatalf("user-facing affiliate notices are missing: %#v", result.Affiliate)
	}
	if !strings.Contains(result.Affiliate.Disclosure, "제공받습니다") || strings.Contains(result.Affiliate.Disclosure, "수 있습니다") {
		t.Fatalf("affiliate disclosure is conditional or ambiguous: %q", result.Affiliate.Disclosure)
	}
}

func TestAffiliateOptOutSkipsLinkGeneration(t *testing.T) {
	linker := &syntheticAffiliateLinker{}
	service := NewWithAffiliate(syntheticSource{items: []core.ProductCard{{Name: "Synthetic product", URL: "https://www.coupang.com/vp/products/101"}}}, linker)
	result, err := service.Search(context.Background(), core.ProductSearchRequest{Query: "synthetic", DisableAffiliate: true})
	if err != nil {
		t.Fatal(err)
	}
	if linker.calls != 0 || result.Affiliate.Status != "disabled" || result.Items[0].AffiliateURL != "" {
		t.Fatalf("affiliate opt-out was ignored: %#v", result)
	}
}

func TestAffiliateConfigurationAndEmptyResultsAreDistinguished(t *testing.T) {
	unconfigured, err := New(syntheticSource{}).Search(context.Background(), core.ProductSearchRequest{Query: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if unconfigured.Affiliate.Status != "unconfigured" {
		t.Fatalf("missing credentials were not distinguished from opt-out: %#v", unconfigured.Affiliate)
	}

	linker := &syntheticAffiliateLinker{}
	configured, err := NewWithAffiliate(syntheticSource{}, linker).Search(context.Background(), core.ProductSearchRequest{Query: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if linker.calls != 0 || configured.Affiliate.Status != "not_applicable" {
		t.Fatalf("empty results caused an unnecessary affiliate call: %#v", configured)
	}
}

func TestAffiliateFailurePreservesCanonicalLinks(t *testing.T) {
	canonical := "https://www.coupang.com/vp/products/101"
	linker := &syntheticAffiliateLinker{err: errors.New("synthetic upstream failure")}
	service := NewWithAffiliate(syntheticSource{inspection: core.ProductInspection{Product: core.ProductCard{Name: "Synthetic product", URL: canonical}}}, linker)
	result, err := service.Inspect(context.Background(), core.ProductInspectRequest{ProductID: "101"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Product.URL != canonical || result.Product.AffiliateURL != "" || result.Affiliate.Status != "unavailable" {
		t.Fatalf("canonical fallback failed: %#v", result)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "affiliate link generation was unavailable; canonical product URLs were preserved" {
		t.Fatalf("unsafe or missing warning: %#v", result.Warnings)
	}
}
