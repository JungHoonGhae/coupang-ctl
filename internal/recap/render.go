package recap

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

//go:embed assets/type-roster-v2.svg
var typeRoster []byte

//go:embed assets/Gaegu-Bold.ttf
var doodleFont []byte

//go:embed template.html
var templateSource string

var pageTemplate = template.Must(template.New("recap").Funcs(template.FuncMap{
	"abs":               math.Abs,
	"clock":             formatClock,
	"comma":             formatInteger,
	"pct":               formatPercent,
	"seq":               boundedSequence,
	"weekday":           localizeWeekday,
	"typeTitle":         profileTitle,
	"axisLabel":         axisLabel,
	"badgeLabel":        badgeLabel,
	"badgeDescription":  badgeDescription,
	"axisMeaning":       axisMeaning,
	"axisReceipt":       axisReceipt,
	"axisHigh":          axisHighCode,
	"axisLow":           axisLowCode,
	"axisPosition":      axisPosition,
	"axisThreshold":     axisThresholdPosition,
	"characterColumn":   characterColumn,
	"characterRow":      characterRow,
	"monthRange":        monthRange,
	"deliveryHeight":    deliveryBarHeight,
	"deliverySummary":   deliverySummary,
	"categoryLabel":     categoryLabel,
	"visibleCategories": visibleCategories,
	"categoryWidth":     categoryRelativeWidth,
	"hourSeries":        purchaseHourSeries,
	"hourHeight":        purchaseHourHeight,
	"monthSeries":       purchaseMonthSeries,
	"monthHeight":       purchaseMonthHeight,
	"monthMarker":       purchaseMonthMarker,
}).Parse(templateSource))

type viewData struct {
	Summary    core.ShoppingInsights
	Products   *core.ProductInsights
	RosterData template.URL
	FontData   template.URL
	ShareText  string
}

type Options struct {
	Products *core.ProductInsights
}

func Render(output io.Writer, summary core.ShoppingInsights) error {
	return RenderWithOptions(output, summary, Options{})
}

func RenderWithOptions(output io.Writer, summary core.ShoppingInsights, options Options) error {
	if options.Products != nil && options.Products.Visibility != "private_local" {
		return fmt.Errorf("render shopping recap: product insights must be marked private_local")
	}
	data := viewData{
		Summary:    summary,
		Products:   options.Products,
		RosterData: template.URL("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(typeRoster)),
		FontData:   template.URL("data:font/ttf;base64," + base64.StdEncoding.EncodeToString(doodleFont)),
		ShareText:  publicShareText(summary),
	}
	if err := pageTemplate.Execute(output, data); err != nil {
		return fmt.Errorf("render shopping recap: %w", err)
	}
	return nil
}

func publicShareText(summary core.ShoppingInsights) string {
	if !summary.Profile.Ready {
		return "나의 쇼핑 타입을 분석하는 중. #coupangctl #쇼핑타입"
	}
	axisParts := make([]string, 0, len(summary.Profile.Axes))
	for _, axis := range summary.Profile.Axes {
		axisParts = append(axisParts, axis.SelectedCode+" "+axisLabel(axis.SelectedCode))
	}
	parts := []string{
		fmt.Sprintf("내 쇼핑 기록 타입은 %s, %s.", summary.Profile.Code, profileTitle(summary.Profile.Code)),
		strings.Join(axisParts, " / "),
	}
	if summary.LongestActiveMonthStreak > 0 {
		parts = append(parts, fmt.Sprintf("%d개월 연속 구매 기록", summary.LongestActiveMonthStreak))
	}
	if summary.Samples.DeliveryEvents > 0 {
		parts = append(parts, fmt.Sprintf("배송의 %.1f%%가 24시간 안에 도착", summary.DeliveredWithin24HoursRate*100))
	}
	parts = append(parts, "성격검사가 아닌 내 주문 기록 요약", "#coupangctl #쇼핑리캡")
	return strings.Join(parts, "\n")
}

func profileTitle(code string) string {
	return map[string]string{
		"SDFO": "햇살 한 봉지 참새", "SDFT": "햇살 합배송 카피바라",
		"SDRO": "낮 단골 다람쥐", "SDRT": "낮 비축 거북이",
		"SNFO": "야밤 새것 부엉이", "SNFT": "야밤 합배송 부엉이",
		"SNRO": "야밤 또삼 다람쥐", "SNRT": "야밤 단골 거북이",
		"BDFO": "번개 한 봉지 까치", "BDFT": "우르르 카피바라",
		"BDRO": "번개 또삼 햄스터", "BDRT": "낮 비축 곰",
		"BNFO": "새벽 번개 라쿤", "BNFT": "새벽 우르르 해파리",
		"BNRO": "새벽 또삼 박쥐", "BNRT": "새벽 비축 박쥐",
	}[code]
}

func axisLabel(code string) string {
	return map[string]string{
		"S": "차곡차곡", "B": "몰아서팡", "N": "다 잘 때", "D": "해 떠 있을 때",
		"F": "처음봄", "R": "또삼", "O": "하나만요", "T": "같이 와요",
	}[code]
}

func axisMeaning(id, code string) string {
	meanings := map[string]string{
		"rhythm:S": "주문일이 같은 조건의 균등 패턴보다 고르게 퍼졌어요.",
		"rhythm:B": "주문일이 같은 조건의 균등 패턴보다 한쪽에 뭉쳤어요.",
		"clock:N":  "주문의 과반이 20:00부터 05:59 사이였어요.",
		"clock:D":  "주문의 과반이 06:00부터 19:59 사이였어요.",
		"choice:F": "식별 가능한 선택의 과반이 처음 고른 상품이었어요.",
		"choice:R": "식별 가능한 선택의 과반이 전에 산 상품이었어요.",
		"basket:O": "상품 구성을 확인한 주문의 과반이 한 상품 주문이었어요.",
		"basket:T": "상품 구성을 확인한 주문의 과반이 여러 상품 주문이었어요.",
	}
	return meanings[id+":"+code]
}

func characterColumn(code string) int {
	return characterIndex(code) % 4
}

func characterRow(code string) int {
	return characterIndex(code) / 4
}

func characterIndex(code string) int {
	order := []string{
		"SDFO", "SDFT", "SDRO", "SDRT",
		"SNFO", "SNFT", "SNRO", "SNRT",
		"BDFO", "BDFT", "BDRO", "BDRT",
		"BNFO", "BNFT", "BNRO", "BNRT",
	}
	for index, candidate := range order {
		if candidate == code {
			return index
		}
	}
	return 0
}

func monthRange(first, last string) string {
	start, startErr := time.Parse(time.DateOnly, first)
	end, endErr := time.Parse(time.DateOnly, last)
	if startErr != nil || endErr != nil {
		return "분석 기간 확인 중"
	}
	return fmt.Sprintf("%d.%02d - %d.%02d", start.Year(), int(start.Month()), end.Year(), int(end.Month()))
}

func axisHighCode(id string) string {
	return map[string]string{"rhythm": "B", "clock": "N", "choice": "R", "basket": "O"}[id]
}

func axisLowCode(id string) string {
	return map[string]string{"rhythm": "S", "clock": "D", "choice": "F", "basket": "T"}[id]
}

func axisPosition(axis core.ShoppingProfileAxis) int {
	return boundedPercent(axis.Score)
}

func axisThresholdPosition(axis core.ShoppingProfileAxis) int {
	return boundedPercent(axis.Threshold)
}

func boundedPercent(value float64) int {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	return int(math.Round(value * 100))
}

func axisReceipt(axis core.ShoppingProfileAxis) string {
	switch axis.ID {
	case "rhythm":
		return fmt.Sprintf("구매일 %d일, 관찰 %d일, 균등 모형 중앙값 %.1f%%", axis.SampleSize, axis.ObservationDays, axis.Threshold*100)
	case "clock":
		return fmt.Sprintf("야간 주문 %d건 / 시각 확인 주문 %d건, 한국시간", axis.Numerator, axis.Denominator)
	case "choice":
		return fmt.Sprintf("반복 선택 %d건 / 식별 가능한 선택 %d건", axis.Numerator, axis.Denominator)
	case "basket":
		return fmt.Sprintf("한 상품 주문 %d건 / 구성 확인 주문 %d건", axis.Numerator, axis.Denominator)
	default:
		return fmt.Sprintf("표본 %d건", axis.SampleSize)
	}
}

func deliveryBarHeight(entries []core.DeliveryDurationSummary, hours float64) int {
	maximum := 0.0
	for _, entry := range entries {
		if entry.AverageHours > maximum {
			maximum = entry.AverageHours
		}
	}
	if maximum <= 0 || hours <= 0 {
		return 0
	}
	height := int(math.Round(hours / maximum * 100))
	if height < 8 {
		return 8
	}
	return height
}

func deliverySummary(trend core.DeliveryTrendComparison) string {
	hours := math.Abs(trend.AverageHoursDelta)
	switch trend.Direction {
	case "faster":
		return fmt.Sprintf("첫해보다 평균 %.1f시간 빨라졌어요", hours)
	case "slower":
		return fmt.Sprintf("첫해보다 평균 %.1f시간 느려졌어요", hours)
	case "unchanged", "same":
		return "첫해와 최근 연도의 평균 배송시간이 같아요"
	default:
		return "연도별 배송 기록을 비교하고 있어요"
	}
}

func categoryLabel(key string) string {
	return key
}

func visibleCategories(summary core.CategoryBreakdown) []core.CategoryBucket {
	result := make([]core.CategoryBucket, 0, 6)
	for _, bucket := range summary.Buckets {
		if bucket.UnitCount == 0 {
			continue
		}
		result = append(result, bucket)
		if len(result) == 6 {
			break
		}
	}
	return result
}

func purchaseHourSeries(entries []core.CountBucket) []core.CountBucket {
	counts := make(map[string]int, len(entries))
	for _, entry := range entries {
		counts[entry.Key] = entry.Count
	}
	result := make([]core.CountBucket, 24)
	for hour := 0; hour < 24; hour++ {
		key := fmt.Sprintf("%02d", hour)
		result[hour] = core.CountBucket{Key: key, Count: counts[key]}
	}
	return result
}

func purchaseHourHeight(entries []core.CountBucket, count int) int {
	maximum := 0
	for _, entry := range entries {
		if entry.Count > maximum {
			maximum = entry.Count
		}
	}
	if maximum == 0 || count == 0 {
		return 2
	}
	return 8 + int(math.Round(float64(count)/float64(maximum)*92))
}

func purchaseMonthSeries(summary core.ShoppingInsights) []core.MonthlyOrderStats {
	start, startErr := time.Parse(time.DateOnly, summary.FirstOrderDate)
	end, endErr := time.Parse(time.DateOnly, summary.LastOrderDate)
	if startErr != nil || endErr != nil || end.Before(start) {
		return nil
	}
	byMonth := make(map[string]core.MonthlyOrderStats, len(summary.PurchaseMonths))
	for _, entry := range summary.PurchaseMonths {
		byMonth[entry.Month] = entry
	}
	result := make([]core.MonthlyOrderStats, 0, len(summary.PurchaseMonths))
	current := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	for !current.After(last) {
		key := current.Format("2006-01")
		entry := byMonth[key]
		entry.Month = key
		result = append(result, entry)
		current = current.AddDate(0, 1, 0)
	}
	return result
}

func purchaseMonthHeight(entries []core.MonthlyOrderStats, count int) int {
	maximum := 0
	for _, entry := range entries {
		if entry.OrderCount > maximum {
			maximum = entry.OrderCount
		}
	}
	if maximum == 0 || count == 0 {
		return 2
	}
	return 6 + int(math.Round(float64(count)/float64(maximum)*94))
}

func purchaseMonthMarker(month string) string {
	parsed, err := time.Parse("2006-01", month)
	if err != nil || parsed.Month() != time.January {
		return ""
	}
	return strconv.Itoa(parsed.Year())
}

func categoryRelativeWidth(summary core.CategoryBreakdown, count int) int {
	maximum := 0
	for _, bucket := range visibleCategories(summary) {
		if bucket.UnitCount > maximum {
			maximum = bucket.UnitCount
		}
	}
	if maximum == 0 || count <= 0 {
		return 0
	}
	return int(math.Round(float64(count) / float64(maximum) * 100))
}

func badgeLabel(badge core.ShoppingBadge) string {
	switch badge.ID {
	case "monthly_streak":
		return fmt.Sprintf("%.0f개월 개근", badge.Value)
	case "order_combo":
		return fmt.Sprintf("하루 %.0f콤보", badge.Value)
	case "shopping_clock":
		return fmt.Sprintf("%.0f시 장바구니", badge.Value)
	case "delivery_speedrun":
		return "24시간 스피드런"
	case "repeat_regular":
		return "숨은 단골"
	case "catalog_explorer":
		return "카탈로그 탐험가"
	default:
		return "비밀 업적"
	}
}

func badgeDescription(badge core.ShoppingBadge) string {
	switch badge.ID {
	case "monthly_streak":
		return fmt.Sprintf("%.0f개월 동안 장바구니 서버가 멈춘 적이 없어요.", badge.Value)
	case "order_combo":
		return fmt.Sprintf("하루에 최대 %.0f건을 연속 시전했어요.", badge.Value)
	case "shopping_clock":
		return fmt.Sprintf("%.0f시가 되면 장바구니 봉인이 풀려요.", badge.Value)
	case "delivery_speedrun":
		return fmt.Sprintf("배송 기록의 %.1f%%가 하루 안에 현관을 점령했어요.", badge.Value*100)
	case "repeat_regular":
		return fmt.Sprintf("같은 상품을 최대 %.0f번 다시 소환했어요.", badge.Value)
	case "catalog_explorer":
		return fmt.Sprintf("식별 상품의 %.1f%%를 한 번씩 탐험했어요.", badge.Value*100)
	default:
		return "설명서가 배송 중이에요."
	}
}

func formatClock(hour string) string {
	value, err := strconv.Atoi(hour)
	if err != nil || value < 0 || value > 23 {
		return "기록 부족"
	}
	return fmt.Sprintf("%02d:00", value)
}

func formatInteger(value any) string {
	var text string
	switch typed := value.(type) {
	case int:
		text = strconv.Itoa(typed)
	case int64:
		text = strconv.FormatInt(typed, 10)
	default:
		return "0"
	}
	start := 0
	if strings.HasPrefix(text, "-") {
		start = 1
	}
	for index := len(text) - 3; index > start; index -= 3 {
		text = text[:index] + "," + text[index:]
	}
	return text
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.1f", value*100)
}

func boundedSequence(count int) []int {
	if count < 0 {
		count = 0
	}
	if count > 120 {
		count = 120
	}
	result := make([]int, count)
	for index := range result {
		result[index] = index + 1
	}
	return result
}

func localizeWeekday(value string) string {
	return map[string]string{
		"sun": "일요일", "mon": "월요일", "tue": "화요일", "wed": "수요일",
		"thu": "목요일", "fri": "금요일", "sat": "토요일",
	}[value]
}
