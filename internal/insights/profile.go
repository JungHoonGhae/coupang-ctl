package insights

import (
	"strconv"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const (
	profileSchemaVersion = 1
	profileRuleVersion   = "shopping_profile_v4"
)

func BuildShoppingProfile(summary core.ShoppingInsights) core.ShoppingProfile {
	nightScore := clamp(summary.NightOrderRate + summary.LateEveningOrderRate)

	axes := []core.ShoppingProfileAxis{
		buildAxis(axisInput{
			id: "rhythm", ready: summary.PurchaseTiming.PurchaseDays >= 20 && summary.PurchaseTiming.ObservationDays >= 180,
			highCode: "B", lowCode: "S", metric: "purchase_timing_clumpiness",
			score: summary.PurchaseTiming.Clumpiness, threshold: summary.PurchaseTiming.UniformNullMedian,
			thresholdBasis: "uniform_same_window_null_median", sampleSize: summary.PurchaseTiming.PurchaseDays,
			observationDays: summary.PurchaseTiming.ObservationDays,
		}),
		buildAxis(axisInput{
			id: "clock", ready: summary.Samples.TimedOrders >= 20 && summary.PurchaseTiming.ObservationDays >= 90,
			highCode: "N", lowCode: "D", metric: "orders_20_to_05_rate",
			score: nightScore, threshold: 0.5, thresholdBasis: "literal_majority",
			numerator: summary.Samples.NightWindowOrders, denominator: summary.Samples.TimedOrders,
			sampleSize: summary.Samples.TimedOrders, observationDays: summary.PurchaseTiming.ObservationDays, timezone: "Asia/Seoul",
		}),
		buildAxis(axisInput{
			id: "choice", ready: summary.RepeatPurchases.PurchaseOccasionCount >= 20 && summary.Basket.RetainedOrderCount >= 10 && summary.PurchaseTiming.ObservationDays >= 180 && summary.RepeatPurchases.ProductIDCoverage >= 0.7,
			highCode: "R", lowCode: "F", metric: "repeat_choice_rate",
			score: summary.RepeatPurchases.RepeatChoiceRate, threshold: 0.5, thresholdBasis: "literal_majority",
			numerator: summary.RepeatPurchases.RepeatChoiceCount, denominator: summary.RepeatPurchases.PurchaseOccasionCount,
			sampleSize: summary.RepeatPurchases.PurchaseOccasionCount, observationDays: summary.PurchaseTiming.ObservationDays,
		}),
		buildAxis(axisInput{
			id: "basket", ready: summary.Basket.CompositionOrderCount >= 10,
			highCode: "O", lowCode: "T", metric: "single_product_order_rate",
			score: summary.Basket.SingleProductOrderRate, threshold: 0.5, thresholdBasis: "literal_majority",
			numerator: summary.Basket.SingleProductOrderCount, denominator: summary.Basket.CompositionOrderCount,
			sampleSize: summary.Basket.CompositionOrderCount, observationDays: summary.PurchaseTiming.ObservationDays,
		}),
	}

	profile := core.ShoppingProfile{SchemaVersion: profileSchemaVersion, RuleVersion: profileRuleVersion, Axes: axes, Badges: buildBadges(summary)}
	var code strings.Builder
	profile.Ready = true
	for _, axis := range axes {
		if !axis.Ready {
			profile.Ready = false
			code.WriteByte('?')
			continue
		}
		code.WriteString(axis.SelectedCode)
	}
	profile.Code = code.String()
	return profile
}

type axisInput struct {
	id, highCode, lowCode, metric string
	ready                         bool
	score, threshold              float64
	thresholdBasis                string
	numerator, denominator        int
	sampleSize, observationDays   int
	timezone                      string
}

func buildAxis(input axisInput) core.ShoppingProfileAxis {
	axis := core.ShoppingProfileAxis{
		ID: input.id, Metric: input.metric, Score: round(input.score, 6), Threshold: round(input.threshold, 6),
		ThresholdBasis: input.thresholdBasis, Numerator: input.numerator, Denominator: input.denominator,
		SampleSize: input.sampleSize, ObservationDays: input.observationDays, Timezone: input.timezone,
		Provenance: "derived_from_normalized_order_history", Ready: input.ready,
	}
	if !input.ready {
		axis.SelectedCode = "?"
		axis.OppositeCode = "?"
		return axis
	}
	if input.score >= input.threshold {
		axis.SelectedCode = input.highCode
		axis.OppositeCode = input.lowCode
	} else {
		axis.SelectedCode = input.lowCode
		axis.OppositeCode = input.highCode
	}
	return axis
}

func buildBadges(summary core.ShoppingInsights) []core.ShoppingBadge {
	badges := []core.ShoppingBadge{}
	if summary.LongestActiveMonthStreak >= 12 {
		badges = append(badges, core.ShoppingBadge{
			ID: "monthly_streak", Value: float64(summary.LongestActiveMonthStreak), Unit: "months",
		})
	}
	if summary.MaxOrdersInOneDay >= 3 {
		badges = append(badges, core.ShoppingBadge{
			ID: "order_combo", Value: float64(summary.MaxOrdersInOneDay), Unit: "orders",
		})
	}
	if summary.Samples.TimedOrders >= 10 {
		if hour, err := strconv.Atoi(summary.PeakPurchaseHourKST.Key); err == nil && hour >= 0 && hour <= 23 {
			badges = append(badges, core.ShoppingBadge{
				ID: "shopping_clock", Value: float64(hour), Unit: "hour_kst",
			})
		}
	}
	if summary.Samples.DeliveryEvents >= 10 && summary.DeliveredWithin24HoursRate >= 0.5 {
		badges = append(badges, core.ShoppingBadge{
			ID: "delivery_speedrun", Value: summary.DeliveredWithin24HoursRate, Unit: "rate",
		})
	}
	if summary.RepeatPurchases.MostRepeatedProductPurchaseCount >= 5 {
		badges = append(badges, core.ShoppingBadge{
			ID: "repeat_regular", Value: float64(summary.RepeatPurchases.MostRepeatedProductPurchaseCount), Unit: "orders",
		})
	}
	if summary.RepeatPurchases.IdentifiedProductCount >= 20 && summary.RepeatPurchases.RepeatProductRate < 0.15 {
		badges = append(badges, core.ShoppingBadge{
			ID: "catalog_explorer", Value: 1 - summary.RepeatPurchases.RepeatProductRate, Unit: "rate",
		})
	}
	return badges
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func round(value float64, places int) float64 {
	factor := 1.0
	for range places {
		factor *= 10
	}
	return float64(int(value*factor+0.5)) / factor
}
