package insights_test

import (
	"testing"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/insights"
)

func TestPurchaseTimingClumpinessSeparatesEvenAndClusteredEvents(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 199)
	even := make([]time.Time, 20)
	clustered := make([]time.Time, 20)
	for index := 0; index < 20; index++ {
		even[index] = start.AddDate(0, 0, index*10)
		clustered[index] = start.AddDate(0, 0, index)
	}
	evenResult := insights.AnalyzePurchaseTiming(even, start, end)
	clusteredResult := insights.AnalyzePurchaseTiming(clustered, start, end)
	if evenResult.PurchaseDays != 20 || evenResult.ObservationDays != 200 || evenResult.UniformNullMedian <= 0 {
		t.Fatalf("unexpected timing sample: %#v", evenResult)
	}
	if !(evenResult.Clumpiness < clusteredResult.Clumpiness) {
		t.Fatalf("even clumpiness %.6f is not below clustered %.6f", evenResult.Clumpiness, clusteredResult.Clumpiness)
	}
}

func TestPurchaseTimingClumpinessIsDeterministic(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dates := []time.Time{start, start.AddDate(0, 0, 5), start.AddDate(0, 0, 20)}
	first := insights.AnalyzePurchaseTiming(dates, start, start.AddDate(0, 0, 29))
	second := insights.AnalyzePurchaseTiming(dates, start, start.AddDate(0, 0, 29))
	if first != second {
		t.Fatalf("timing analysis changed between runs: %#v %#v", first, second)
	}
}
