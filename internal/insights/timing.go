package insights

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

const timingNullSimulations = 512

// AnalyzePurchaseTiming measures how unevenly unique retained purchase days
// are distributed inside one explicit observation window. The null median is
// generated deterministically from uniformly distributed event days with the
// same sample size and window length. It is not a population comparison.
func AnalyzePurchaseTiming(dates []time.Time, start, end time.Time) core.PurchaseTimingInsights {
	result := core.PurchaseTimingInsights{}
	start = dateOnly(start)
	end = dateOnly(end)
	if end.Before(start) {
		return result
	}
	observationDays := int(end.Sub(start).Hours()/24) + 1
	positions := uniqueDayPositions(dates, start, end)
	result.PurchaseDays = len(positions)
	result.ObservationDays = observationDays
	if len(positions) < 2 || observationDays < 2 || len(positions) > observationDays {
		return result
	}
	result.Clumpiness = round(clumpiness(positions, observationDays), 6)
	result.UniformNullMedian = round(uniformNullMedian(len(positions), observationDays), 6)
	return result
}

func uniqueDayPositions(dates []time.Time, start, end time.Time) []int {
	seen := make(map[int]struct{}, len(dates))
	for _, value := range dates {
		value = dateOnly(value)
		if value.Before(start) || value.After(end) {
			continue
		}
		seen[int(value.Sub(start).Hours()/24)] = struct{}{}
	}
	positions := make([]int, 0, len(seen))
	for position := range seen {
		positions = append(positions, position)
	}
	sort.Ints(positions)
	return positions
}

func clumpiness(positions []int, observationDays int) float64 {
	span := observationDays - 1
	if len(positions) < 2 || span <= 0 {
		return 0
	}
	intervals := make([]int, 0, len(positions)+1)
	intervals = append(intervals, positions[0])
	for index := 1; index < len(positions); index++ {
		intervals = append(intervals, positions[index]-positions[index-1])
	}
	intervals = append(intervals, span-positions[len(positions)-1])
	var entropy float64
	for _, interval := range intervals {
		if interval <= 0 {
			continue
		}
		share := float64(interval) / float64(span)
		entropy -= share * math.Log(share)
	}
	maximumEntropy := math.Log(float64(len(intervals)))
	if maximumEntropy == 0 {
		return 0
	}
	value := 1 - entropy/maximumEntropy
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func uniformNullMedian(eventDays, observationDays int) float64 {
	if eventDays < 2 || observationDays < eventDays {
		return 0
	}
	random := rand.New(rand.NewSource(int64(observationDays*100_003 + eventDays)))
	values := make([]float64, timingNullSimulations)
	for index := range values {
		permutation := random.Perm(observationDays)
		positions := append([]int(nil), permutation[:eventDays]...)
		sort.Ints(positions)
		values[index] = clumpiness(positions, observationDays)
	}
	sort.Float64s(values)
	return (values[len(values)/2-1] + values[len(values)/2]) / 2
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
