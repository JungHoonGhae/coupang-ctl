package insights_test

import (
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/insights"
)

func TestBuildShoppingProfileProducesDeterministicFourAxisTypeAndBadges(t *testing.T) {
	input := core.ShoppingInsights{
		FirstOrderDate:             "2024-01-05",
		LastOrderDate:              "2025-12-28",
		OrderCount:                 120,
		ActiveMonthCount:           20,
		LongestActiveMonthStreak:   18,
		MaxOrdersInOneDay:          5,
		NightOrderRate:             0.15,
		LateEveningOrderRate:       0.25,
		PeakPurchaseHourKST:        core.CountBucket{Key: "21", Count: 20},
		DeliveredWithin24HoursRate: 0.65,
		RepeatPurchases: core.RepeatPurchaseInsights{
			IdentifiedProductCount:             90,
			RepeatProductCount:                 10,
			RepeatProductRate:                  0.111111,
			PurchaseOccasionCount:              110,
			RepeatProductPurchaseOccasionCount: 30,
			RepeatProductPurchaseOccasionRate:  0.272727,
			RepeatChoiceCount:                  25,
			RepeatChoiceRate:                   0.227273,
			RetainedItemLineCount:              100,
			IdentifiedItemLineCount:            90,
			ProductIDCoverage:                  0.9,
			MostRepeatedProductPurchaseCount:   6,
		},
		Basket:         core.BasketInsights{RetainedOrderCount: 100, CompositionOrderCount: 90, SingleProductOrderCount: 72, SingleProductOrderRate: 0.8},
		PurchaseTiming: core.PurchaseTimingInsights{Clumpiness: 0.4, UniformNullMedian: 0.3, PurchaseDays: 80, ObservationDays: 730},
		Samples:        core.InsightSampleSizes{TimedOrders: 100, NightWindowOrders: 40, DeliveryEvents: 90},
	}

	profile := insights.BuildShoppingProfile(input)
	if !profile.Ready || profile.Code != "BDFO" || profile.RuleVersion != "shopping_profile_v4" {
		t.Fatalf("unexpected shopping profile: %#v", profile)
	}
	if len(profile.Axes) != 4 {
		t.Fatalf("axis count = %d, want 4", len(profile.Axes))
	}
	if !hasBadge(profile.Badges, "monthly_streak") || !hasBadge(profile.Badges, "order_combo") || !hasBadge(profile.Badges, "repeat_regular") {
		t.Fatalf("expected streak, combo, and repeat badges: %#v", profile.Badges)
	}
}

func TestBuildShoppingProfileMarksMissingAxesInsteadOfInventingAType(t *testing.T) {
	profile := insights.BuildShoppingProfile(core.ShoppingInsights{})
	if profile.Ready || profile.Code != "????" {
		t.Fatalf("missing evidence produced a type: %#v", profile)
	}
	for _, axis := range profile.Axes {
		if axis.Ready {
			t.Fatalf("axis unexpectedly ready: %#v", axis)
		}
	}
}

func TestFourProfileAxesUsePublishedThresholds(t *testing.T) {
	base := core.ShoppingInsights{
		FirstOrderDate: "2026-01-01", LastOrderDate: "2026-12-31", OrderCount: 12,
		NightOrderRate: 0.2, LateEveningOrderRate: 0.3,
		PurchaseTiming:  core.PurchaseTimingInsights{Clumpiness: 0.2, UniformNullMedian: 0.2, PurchaseDays: 20, ObservationDays: 365},
		RepeatPurchases: core.RepeatPurchaseInsights{PurchaseOccasionCount: 20, RepeatChoiceCount: 10, RepeatChoiceRate: 0.5, ProductIDCoverage: 0.7},
		Basket:          core.BasketInsights{RetainedOrderCount: 10, CompositionOrderCount: 10, SingleProductOrderCount: 5, SingleProductOrderRate: 0.5},
		Samples:         core.InsightSampleSizes{TimedOrders: 20, NightWindowOrders: 10},
	}
	if got := insights.BuildShoppingProfile(base).Code; got != "BNRO" {
		t.Fatalf("threshold-inclusive profile code = %q, want BNRO", got)
	}

	below := base
	below.PurchaseTiming.Clumpiness = 0.199
	below.LateEveningOrderRate = 0.299
	below.RepeatPurchases.RepeatChoiceRate = 0.499
	below.Basket.SingleProductOrderRate = 0.499
	if got := insights.BuildShoppingProfile(below).Code; got != "SDFT" {
		t.Fatalf("below-threshold profile code = %q, want SDFT", got)
	}
}

func hasBadge(badges []core.ShoppingBadge, id string) bool {
	for _, badge := range badges {
		if badge.ID == id {
			return true
		}
	}
	return false
}
