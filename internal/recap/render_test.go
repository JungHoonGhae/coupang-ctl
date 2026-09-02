package recap_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/insights"
	"github.com/JungHoonGhae/coupang-ctl/internal/recap"
)

func TestRenderCreatesStandalonePublicSafeRecap(t *testing.T) {
	summary := syntheticInsights()
	summary.Profile = insights.BuildShoppingProfile(summary)
	var output bytes.Buffer
	if err := recap.Render(&output, summary); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		"<!doctype html>", "BDFO", "번개 한 봉지 까치", "B 몰아서팡", "진짜 내 데이터 맞음",
		"네 글자 해부하기", "로켓, 해마다 어땠나", "카테고리 이름 보기", "구매 달력 심전도",
		"data:image/svg", "data:font/ttf;base64,", "shopping_profile_v4",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered recap does not contain %q", want)
		}
	}
	for _, private := range []string{"Synthetic private brand", "2024-01-05", "2025-12-28"} {
		if strings.Contains(html, private) {
			t.Fatalf("public recap exposed private detail %q", private)
		}
	}
	if strings.ContainsAny(html, "—–") {
		t.Fatal("recap contains a banned dash character")
	}
}

func TestRenderShowsHonestInsufficientDataState(t *testing.T) {
	summary := core.ShoppingInsights{Profile: insights.BuildShoppingProfile(core.ShoppingInsights{})}
	var output bytes.Buffer
	if err := recap.Render(&output, summary); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "조금 더 쇼핑 기록이 필요해요") {
		t.Fatal("missing-data state was not rendered")
	}
}

func TestRenderWithPrivateProductsAddsExplicitProductReceipts(t *testing.T) {
	summary := syntheticInsights()
	summary.Profile = insights.BuildShoppingProfile(summary)
	products := core.ProductInsights{
		Visibility: "private_local", Currency: "KRW", FirstPurchaseDate: "2024-01-05", LastPurchaseDate: "2025-12-28",
		CalendarMonthCount: 24, ActiveMonthCount: 20, TotalSpendAmount: 1234000, AverageMonthlySpendAmount: 51417,
		RetainedItemLineCount: 100, IdentifiedItemLineCount: 90, ProductIDCoverage: 0.9,
		SpendEligibleItemLineCount: 80, SpendEligibleItemLineRate: 0.8,
		TopByUnits:      core.ProductAggregate{Name: "Synthetic private oats", UnitCount: 12, PurchaseCount: 8, TotalPaidAmount: 48000},
		TopByOrders:     core.ProductAggregate{Name: "Synthetic private oats", UnitCount: 12, PurchaseCount: 8, TotalPaidAmount: 48000},
		TopBySpend:      core.ProductAggregate{Name: "Synthetic private monitor", UnitCount: 1, PurchaseCount: 1, TotalPaidAmount: 300000},
		HighestPaidUnit: core.PaidUnitHighlight{Name: "Synthetic private monitor", Date: "2024-06-15", Quantity: 1, PaidAmount: 300000, PaidUnitAmount: 300000},
		LowestPaidUnit:  core.PaidUnitHighlight{Name: "Synthetic private pencil", Date: "2025-11-15", Quantity: 1, PaidAmount: 500, PaidUnitAmount: 500},
		HighestSpendDay: core.SpendDayInsight{Date: "2024-06-15", TotalAmount: 300000, OrderCount: 1, ProductCount: 1, RetainedItemLineCount: 1, Products: []core.DayProductSummary{{Name: "Synthetic private monitor", UnitCount: 1, PaidAmount: 300000}}},
		LowestSpendDay:  core.SpendDayInsight{Date: "2025-11-15", TotalAmount: 500, OrderCount: 1, ProductCount: 1, RetainedItemLineCount: 1, Products: []core.DayProductSummary{{Name: "Synthetic private pencil", UnitCount: 1, PaidAmount: 500}}},
	}
	var publicOutput bytes.Buffer
	if err := recap.Render(&publicOutput, summary); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(publicOutput.String(), "Synthetic private oats") || strings.Contains(publicOutput.String(), "2024-06-15") {
		t.Fatal("public recap exposed private product details")
	}

	var privateOutput bytes.Buffer
	if err := recap.RenderWithOptions(&privateOutput, summary, recap.Options{Products: &products}); err != nil {
		t.Fatal(err)
	}
	html := privateOutput.String()
	for _, want := range []string{
		"PRIVATE PRODUCT RECEIPTS", "이번엔 상품들이 입을 엽니다", "상품명 보기",
		"Synthetic private oats", "Synthetic private monitor", "Synthetic private pencil",
		"2024-06-15", "300,000원", "월평균 51,417원",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("private product recap does not contain %q", want)
		}
	}
}

func syntheticInsights() core.ShoppingInsights {
	return core.ShoppingInsights{
		SchemaVersion:              1,
		FirstOrderDate:             "2024-01-05",
		LastOrderDate:              "2025-12-28",
		OrderCount:                 120,
		DistinctOrderDays:          80,
		MultiOrderDays:             20,
		MaxOrdersInOneDay:          5,
		AverageGapDays:             8.5,
		LongestGapDays:             40,
		ActiveMonthCount:           20,
		LongestActiveMonthStreak:   18,
		PeakPurchaseHourKST:        core.CountBucket{Key: "21", Count: 20},
		PeakPurchaseWeekday:        core.CountBucket{Key: "tue", Count: 25},
		NightOrderRate:             0.15,
		LateEveningOrderRate:       0.25,
		WeekendOrderRate:           0.3,
		DeliveredWithin24HoursRate: 0.65,
		DeliveredWithin48HoursRate: 0.9,
		TopBrand:                   core.CountBucket{Key: "Synthetic private brand", Count: 12},
		TopBrandShare:              0.12,
		DeliveryTrend:              core.DeliveryTrendComparison{BaselinePeriod: "2024", LatestPeriod: "2025", AverageHoursDelta: -5, AverageHoursPercentChange: -0.166667, Direction: "faster"},
		RepeatPurchases: core.RepeatPurchaseInsights{
			IdentifiedProductCount: 90, RepeatProductCount: 10, RepeatProductRate: 0.111111,
			PurchaseOccasionCount: 110, RepeatProductPurchaseOccasionCount: 30,
			RepeatProductPurchaseOccasionRate: 0.272727, RepeatChoiceCount: 25,
			RepeatChoiceRate: 0.227273, RetainedItemLineCount: 100, IdentifiedItemLineCount: 90,
			ProductIDCoverage: 0.9, MostRepeatedProductPurchaseCount: 6,
		},
		Basket: core.BasketInsights{
			RetainedOrderCount: 100, RetainedItemLineCount: 130, SingleItemOrderCount: 80,
			SingleItemOrderRate: 0.8, AverageItemLines: 1.3, MaxItemLines: 5,
			RetainedItemAmount: 1200000, SingleItemOrderAmount: 900000, SingleItemSpendRate: 0.75,
			MedianSingleItemOrderValue: 12000, MedianMultiItemOrderValue: 30000,
			CompositionOrderCount: 90, SingleProductOrderCount: 72, SingleProductOrderRate: 0.8,
			AverageDistinctProducts: 1.25, MaxDistinctProducts: 4,
		},
		PurchaseTiming: core.PurchaseTimingInsights{Clumpiness: 0.4, UniformNullMedian: 0.3, PurchaseDays: 80, ObservationDays: 730},
		Samples: core.InsightSampleSizes{
			TimedOrders: 100, NightOrders: 15, LateEveningOrders: 25, NightWindowOrders: 40,
			DaytimeOrders: 60, OtherTimedOrders: 85, DeliveryEvents: 90, BrandedRetainedItemLines: 100,
		},
		DeliveryByYear: []core.DeliveryDurationSummary{
			{Period: "2024", ShipmentCount: 40, AverageHours: 30, MedianHours: 26, P90Hours: 48},
			{Period: "2025", ShipmentCount: 50, AverageHours: 25, MedianHours: 22, P90Hours: 40},
		},
		PurchaseHours: []core.CountBucket{{Key: "09", Count: 10}, {Key: "21", Count: 20}, {Key: "23", Count: 12}},
		PurchaseMonths: []core.MonthlyOrderStats{
			{Month: "2024-01", OrderCount: 2}, {Month: "2024-06", OrderCount: 6},
			{Month: "2025-05", OrderCount: 10}, {Month: "2025-12", OrderCount: 7},
		},
		Categories: core.CategoryBreakdown{
			Method: "source_native", Grouping: "breadcrumb_leaf", TotalItemLines: 100,
			ClassifiedItemLines: 75, ClassifiedItemLineRate: 0.75, RetainedUnits: 120,
			Buckets: []core.CategoryBucket{
				{CategoryID: "1001", Key: "합성 생활용품", ItemLineCount: 20, UnitCount: 30, UnitShare: 0.25},
				{CategoryID: "1002", Key: "합성 간식", ItemLineCount: 15, UnitCount: 18, UnitShare: 0.15},
			},
		},
	}
}
