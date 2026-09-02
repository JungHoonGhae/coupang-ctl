package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	behavior "github.com/JungHoonGhae/coupang-ctl/internal/insights"
)

const (
	defaultOrderLimit = 100
	maxOrderLimit     = 1000
)

func (s *SQLite) UpsertOrderPage(ctx context.Context, page core.OrderPage) (core.UpsertResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.UpsertResult{}, fmt.Errorf("begin order update: %w", err)
	}
	defer tx.Rollback()

	result := core.UpsertResult{OrdersSeen: len(page.Orders)}
	for _, order := range page.Orders {
		if err := validateOrder(order); err != nil {
			return core.UpsertResult{}, err
		}
		var purchasedAtTime any
		if order.PurchasedAtTime != nil {
			purchasedAtTime = order.PurchasedAtTime.UTC().Format(time.RFC3339Nano)
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO orders(
			source_ref, purchased_at, purchased_at_time, total_amount, discount_amount, shipping_fee, currency,
			fully_canceled, receipt_available, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_ref) DO UPDATE SET
			purchased_at = excluded.purchased_at,
			purchased_at_time = excluded.purchased_at_time,
			total_amount = excluded.total_amount,
			discount_amount = excluded.discount_amount,
			shipping_fee = excluded.shipping_fee,
			currency = excluded.currency,
			fully_canceled = excluded.fully_canceled,
			receipt_available = excluded.receipt_available,
			updated_at = excluded.updated_at`,
			order.SourceRef, order.PurchasedAt, purchasedAtTime, order.TotalAmount, order.DiscountAmount,
			order.ShippingFee, order.Currency, order.FullyCanceled, order.ReceiptAvailable,
			time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return core.UpsertResult{}, fmt.Errorf("upsert normalized order: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM order_items WHERE order_ref = ?", order.SourceRef); err != nil {
			return core.UpsertResult{}, fmt.Errorf("replace normalized order items: %w", err)
		}
		for position, item := range order.Items {
			result.ItemsSeen++
			item.CommerceKind = core.ClassifyCommerceKind(item)
			var deliveredAt any
			if item.DeliveredAt != nil {
				deliveredAt = item.DeliveredAt.UTC().Format(time.RFC3339Nano)
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO order_items(
				order_ref, position, product_id, vendor_item_id, name, quantity, cancelled_quantity, returned_quantity,
				unit_price, paid_price, seller_name, brand_name, product_type, division_type, commerce_kind, delivery_status, delivered_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				order.SourceRef, position, nullIfEmpty(item.ProductID), nullIfEmpty(item.VendorItemID),
				item.Name, item.Quantity, item.CancelledQuantity, item.ReturnedQuantity,
				item.UnitPrice, item.PaidPrice, nullIfEmpty(item.SellerName), nullIfEmpty(item.BrandName),
				nullIfEmpty(item.ProductType), nullIfEmpty(item.DivisionType), item.CommerceKind,
				nullIfEmpty(item.DeliveryStatus), deliveredAt)
			if err != nil {
				return core.UpsertResult{}, fmt.Errorf("insert normalized order item: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return core.UpsertResult{}, fmt.Errorf("commit order update: %w", err)
	}
	return result, nil
}

// ReconcileOrders removes normalized rows that were not observed during one
// complete history walk. Callers must not use it for partial or resumed syncs.
func (s *SQLite) ReconcileOrders(ctx context.Context, sourceRefs []string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin order reconciliation: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS sync_seen_orders (
		source_ref TEXT PRIMARY KEY
	)`); err != nil {
		return 0, fmt.Errorf("prepare order reconciliation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sync_seen_orders"); err != nil {
		return 0, fmt.Errorf("reset order reconciliation: %w", err)
	}
	for _, sourceRef := range sourceRefs {
		if sourceRef == "" {
			return 0, fmt.Errorf("%w: invalid reconciliation reference", core.ErrInvalidOrderData)
		}
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO sync_seen_orders(source_ref) VALUES (?)", sourceRef); err != nil {
			return 0, fmt.Errorf("record order reconciliation reference: %w", err)
		}
	}
	var removed int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM orders
		WHERE NOT EXISTS (SELECT 1 FROM sync_seen_orders seen WHERE seen.source_ref = orders.source_ref)`).Scan(&removed); err != nil {
		return 0, fmt.Errorf("count stale normalized orders: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM orders
		WHERE NOT EXISTS (SELECT 1 FROM sync_seen_orders seen WHERE seen.source_ref = orders.source_ref)`); err != nil {
		return 0, fmt.Errorf("remove stale normalized orders: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit order reconciliation: %w", err)
	}
	return removed, nil
}

func (s *SQLite) ListOrders(ctx context.Context, filter core.OrderFilter) ([]core.Order, error) {
	filter, err := normalizeFilter(filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT source_ref, purchased_at, purchased_at_time, total_amount,
		discount_amount, shipping_fee, currency, fully_canceled, receipt_available
		FROM orders
		WHERE (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)
		ORDER BY purchased_at DESC, source_ref
		LIMIT ?`, filter.From, filter.From, filter.To, filter.To, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list normalized orders: %w", err)
	}
	defer rows.Close()

	var orders []core.Order
	for rows.Next() {
		var order core.Order
		var purchasedAtTime sql.NullString
		var fullyCanceled, receiptAvailable int
		if err := rows.Scan(&order.SourceRef, &order.PurchasedAt, &purchasedAtTime, &order.TotalAmount,
			&order.DiscountAmount, &order.ShippingFee, &order.Currency, &fullyCanceled, &receiptAvailable); err != nil {
			return nil, fmt.Errorf("scan normalized order: %w", err)
		}
		order.FullyCanceled = fullyCanceled != 0
		order.ReceiptAvailable = receiptAvailable != 0
		if purchasedAtTime.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, purchasedAtTime.String)
			if err != nil {
				return nil, fmt.Errorf("decode normalized purchase timestamp: %w", err)
			}
			order.PurchasedAtTime = &parsed
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate normalized orders: %w", err)
	}
	for index := range orders {
		items, err := s.listItems(ctx, orders[index].SourceRef)
		if err != nil {
			return nil, err
		}
		orders[index].Items = items
	}
	if orders == nil {
		orders = []core.Order{}
	}
	return orders, nil
}

func (s *SQLite) Spend(ctx context.Context, filter core.OrderFilter) (core.SpendSummary, error) {
	filter, err := normalizeFilterForAggregate(filter)
	if err != nil {
		return core.SpendSummary{}, err
	}
	result := core.SpendSummary{From: filter.From, To: filter.To, Currency: "KRW"}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(total_amount), 0),
		COALESCE(SUM(discount_amount), 0), COALESCE(SUM(shipping_fee), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 1 THEN total_amount ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 0 THEN total_amount ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 0 THEN shipping_fee ELSE 0 END), 0)
		FROM orders WHERE (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)`,
		filter.From, filter.From, filter.To, filter.To).Scan(
		&result.OrderCount, &result.TotalAmount, &result.DiscountAmount, &result.ShippingFee,
		&result.FullyCanceledOrderCount, &result.FullyCanceledAmount,
		&result.NonCanceledTotalAmount, &result.NonCanceledShippingFee)
	if err != nil {
		return core.SpendSummary{}, fmt.Errorf("summarize normalized orders: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `WITH classified AS (
		SELECT orders.*,
			CASE
				WHEN EXISTS (SELECT 1 FROM order_items i WHERE i.order_ref = orders.source_ref AND i.commerce_kind = 'product_purchase') THEN 'product_purchase'
				WHEN EXISTS (SELECT 1 FROM order_items i WHERE i.order_ref = orders.source_ref AND i.commerce_kind = 'membership_fee') THEN 'membership_fee'
				ELSE 'unclassified'
			END AS commerce_kind
		FROM orders WHERE (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)
	)
	SELECT commerce_kind, COUNT(*), COALESCE(SUM(total_amount), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 0 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 0 THEN total_amount ELSE 0 END), 0)
	FROM classified GROUP BY commerce_kind`, filter.From, filter.From, filter.To, filter.To)
	if err != nil {
		return core.SpendSummary{}, fmt.Errorf("summarize commerce spend: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var bucket core.CommerceSpendBucket
		if err := rows.Scan(&kind, &bucket.OrderCount, &bucket.GrossAmount,
			&bucket.NonCanceledOrderCount, &bucket.NonCanceledTotalAmount); err != nil {
			return core.SpendSummary{}, fmt.Errorf("scan commerce spend: %w", err)
		}
		switch core.CommerceKind(kind) {
		case core.CommerceKindProductPurchase:
			result.Commerce.ProductPurchases = bucket
		case core.CommerceKindMembershipFee:
			result.Commerce.MembershipFees = bucket
		default:
			result.Commerce.Unclassified = bucket
		}
	}
	if err := rows.Err(); err != nil {
		return core.SpendSummary{}, fmt.Errorf("iterate commerce spend: %w", err)
	}
	return result, nil
}

func (s *SQLite) Stats(ctx context.Context, filter core.OrderFilter) (core.OrderStats, error) {
	filter, err := normalizeFilterForAggregate(filter)
	if err != nil {
		return core.OrderStats{}, err
	}
	result := core.OrderStats{From: filter.From, To: filter.To}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN fully_canceled = 1 THEN 1 ELSE 0 END), 0)
		FROM orders WHERE (NOT EXISTS (
			SELECT 1 FROM order_items any_item WHERE any_item.order_ref = orders.source_ref
		) OR EXISTS (
			SELECT 1 FROM order_items product_item WHERE product_item.order_ref = orders.source_ref
				AND product_item.commerce_kind = 'product_purchase'
		)) AND (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)`,
		filter.From, filter.From, filter.To, filter.To).Scan(
		&result.OrderCount, &result.FullyCanceledOrderCount)
	if err != nil {
		return core.OrderStats{}, fmt.Errorf("summarize normalized order states: %w", err)
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(i.quantity), 0),
		COALESCE(SUM(CASE WHEN COALESCE(i.delivery_status, '') = 'cancelled'
			OR (i.cancelled_quantity > 0 AND i.returned_quantity = 0) THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN COALESCE(i.delivery_status, '') = 'cancelled'
			OR (i.cancelled_quantity > 0 AND i.returned_quantity = 0)
			THEN CASE WHEN i.cancelled_quantity > 0 THEN i.cancelled_quantity ELSE i.quantity END ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN COALESCE(i.delivery_status, '') = 'returned'
			OR i.returned_quantity > 0 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN COALESCE(i.delivery_status, '') = 'returned'
			OR i.returned_quantity > 0
			THEN CASE WHEN i.returned_quantity > 0 THEN i.returned_quantity ELSE i.quantity END ELSE 0 END), 0)
		FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
		WHERE i.commerce_kind = 'product_purchase'
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)`,
		filter.From, filter.From, filter.To, filter.To).Scan(
		&result.ItemLineCount, &result.OrderedUnits,
		&result.CanceledItemLineCount, &result.CanceledUnits,
		&result.ReturnedItemLineCount, &result.ReturnedUnits)
	if err != nil {
		return core.OrderStats{}, fmt.Errorf("summarize normalized order items: %w", err)
	}
	result.FullyCanceledOrderRate = ratio(result.FullyCanceledOrderCount, result.OrderCount)
	result.CanceledUnitRate = ratio(result.CanceledUnits, result.OrderedUnits)
	result.ReturnedItemLineRate = ratio(result.ReturnedItemLineCount, result.ItemLineCount)
	result.ReturnedUnitRate = ratio(result.ReturnedUnits, result.OrderedUnits)
	result.PurchaseHours, err = s.purchaseHourBuckets(ctx, filter)
	if err != nil {
		return core.OrderStats{}, err
	}
	result.PurchaseWeekdays, err = s.purchaseWeekdayBuckets(ctx, filter)
	if err != nil {
		return core.OrderStats{}, err
	}
	result.PurchaseMonths, err = s.purchaseMonthStats(ctx, filter)
	if err != nil {
		return core.OrderStats{}, err
	}
	result.TopBrands, err = s.topBrandBuckets(ctx, filter)
	if err != nil {
		return core.OrderStats{}, err
	}
	result.DeliveryDuration, result.DeliveryByYear, err = s.deliveryDurationStats(ctx, filter)
	if err != nil {
		return core.OrderStats{}, err
	}
	result.DeliveryTrend = compareDeliveryTrend(result.DeliveryByYear)
	return result, nil
}

func (s *SQLite) Insights(ctx context.Context, filter core.OrderFilter) (core.ShoppingInsights, error) {
	filter, err := normalizeFilterForAggregate(filter)
	if err != nil {
		return core.ShoppingInsights{}, err
	}
	stats, err := s.Stats(ctx, filter)
	if err != nil {
		return core.ShoppingInsights{}, err
	}
	result := core.ShoppingInsights{
		SchemaVersion: 1, From: filter.From, To: filter.To, OrderCount: stats.OrderCount,
		DeliveryByYear: stats.DeliveryByYear, DeliveryTrend: stats.DeliveryTrend,
		PurchaseHours: stats.PurchaseHours, PurchaseMonths: retainedPurchaseMonths(stats.PurchaseMonths),
		Definitions: core.InsightDefinitions{
			NightHoursKST: "00-05", LateEveningHoursKST: "20-23", RateScale: "0_to_1",
			RepeatProduct:      "same_vendor_item_id_else_product_id_in_2_or_more_distinct_retained_orders",
			RepeatChoice:       "repeat_order_product_events_after_each_identified_products_first_retained_order",
			BasketComposition:  "unique_stable_product_ids_per_fully_identified_retained_order",
			PurchaseClumpiness: "normalized_gap_entropy_against_uniform_same_window_null_median",
		},
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MIN(purchased_at), ''), COALESCE(MAX(purchased_at), ''), COUNT(DISTINCT purchased_at), COUNT(*)
		FROM orders WHERE fully_canceled = 0 AND (NOT EXISTS (
			SELECT 1 FROM order_items any_item WHERE any_item.order_ref = orders.source_ref
		) OR EXISTS (
			SELECT 1 FROM order_items product_item WHERE product_item.order_ref = orders.source_ref
				AND product_item.commerce_kind = 'product_purchase'
				AND COALESCE(product_item.delivery_status, '') NOT IN ('cancelled', 'returned')
		))
			AND (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)`,
		filter.From, filter.From, filter.To, filter.To).Scan(
		&result.FirstOrderDate, &result.LastOrderDate, &result.DistinctOrderDays, &result.OrderCount); err != nil {
		return core.ShoppingInsights{}, fmt.Errorf("summarize shopping insight dates: %w", err)
	}
	dates, dailyCounts, err := s.orderDaySeries(ctx, filter)
	if err != nil {
		return core.ShoppingInsights{}, err
	}
	for index, count := range dailyCounts {
		if count > 1 {
			result.MultiOrderDays++
		}
		if count > result.MaxOrdersInOneDay {
			result.MaxOrdersInOneDay = count
		}
		if index > 0 {
			gap := int(dates[index].Sub(dates[index-1]).Hours() / 24)
			result.AverageGapDays += float64(gap)
			if gap > result.LongestGapDays {
				result.LongestGapDays = gap
			}
		}
	}
	if len(dates) > 1 {
		result.AverageGapDays = roundDecimal(result.AverageGapDays/float64(len(dates)-1), 2)
	}
	windowStart, startErr := time.Parse(time.DateOnly, result.FirstOrderDate)
	windowEnd, endErr := time.Parse(time.DateOnly, result.LastOrderDate)
	if filter.From != "" {
		windowStart, startErr = time.Parse(time.DateOnly, filter.From)
	}
	if filter.To != "" {
		windowEnd, endErr = time.Parse(time.DateOnly, filter.To)
	}
	if startErr == nil && endErr == nil {
		result.PurchaseTiming = behavior.AnalyzePurchaseTiming(dates, windowStart, windowEnd)
	}
	result.PeakPurchaseHourKST = peakBucket(stats.PurchaseHours)
	result.PeakPurchaseWeekday = peakBucket(stats.PurchaseWeekdays)
	result.ActiveMonthCount = len(result.PurchaseMonths)
	result.LongestActiveMonthStreak = longestMonthStreak(result.PurchaseMonths)
	result.BusiestMonth, result.HighestSpendMonth = peakMonths(result.PurchaseMonths)
	if len(stats.TopBrands) > 0 {
		result.TopBrand = stats.TopBrands[0]
		var brandedLines int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
			WHERE i.brand_name IS NOT NULL AND i.brand_name != '' AND o.fully_canceled = 0
				AND i.commerce_kind = 'product_purchase'
				AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
				AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)`,
			filter.From, filter.From, filter.To, filter.To).Scan(&brandedLines); err != nil {
			return core.ShoppingInsights{}, fmt.Errorf("summarize branded purchases: %w", err)
		}
		result.Samples.BrandedRetainedItemLines = brandedLines
		result.TopBrandShare = ratio(result.TopBrand.Count, brandedLines)
	}
	var timedOrders, nightOrders, lateEveningOrders, weekendOrders int
	var allNightOrders, nightCanceled, allOtherOrders, otherCanceled int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN fully_canceled = 0 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 0 AND CAST(strftime('%H', purchased_at_time, '+9 hours') AS INTEGER) BETWEEN 0 AND 5 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 0 AND CAST(strftime('%H', purchased_at_time, '+9 hours') AS INTEGER) BETWEEN 20 AND 23 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 0 AND CAST(strftime('%w', purchased_at_time, '+9 hours') AS INTEGER) IN (0, 6) THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN CAST(strftime('%H', purchased_at_time, '+9 hours') AS INTEGER) BETWEEN 0 AND 5 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN CAST(strftime('%H', purchased_at_time, '+9 hours') AS INTEGER) BETWEEN 0 AND 5 AND fully_canceled = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN CAST(strftime('%H', purchased_at_time, '+9 hours') AS INTEGER) NOT BETWEEN 0 AND 5 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN CAST(strftime('%H', purchased_at_time, '+9 hours') AS INTEGER) NOT BETWEEN 0 AND 5 AND fully_canceled = 1 THEN 1 ELSE 0 END), 0)
		FROM orders WHERE purchased_at_time IS NOT NULL AND (NOT EXISTS (
			SELECT 1 FROM order_items any_item WHERE any_item.order_ref = orders.source_ref
		) OR EXISTS (
			SELECT 1 FROM order_items product_item WHERE product_item.order_ref = orders.source_ref
				AND product_item.commerce_kind = 'product_purchase'
		))
			AND (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)`,
		filter.From, filter.From, filter.To, filter.To).Scan(
		&timedOrders, &nightOrders, &lateEveningOrders, &weekendOrders, &allNightOrders,
		&nightCanceled, &allOtherOrders, &otherCanceled); err != nil {
		return core.ShoppingInsights{}, fmt.Errorf("summarize purchase-time insights: %w", err)
	}
	result.NightOrderRate = ratio(nightOrders, timedOrders)
	result.LateEveningOrderRate = ratio(lateEveningOrders, timedOrders)
	result.WeekendOrderRate = ratio(weekendOrders, timedOrders)
	result.NightFullyCanceledOrderRate = ratio(nightCanceled, allNightOrders)
	result.OtherFullyCanceledOrderRate = ratio(otherCanceled, allOtherOrders)
	result.Samples.TimedOrders = timedOrders
	result.Samples.NightOrders = nightOrders
	result.Samples.LateEveningOrders = lateEveningOrders
	result.Samples.NightWindowOrders = nightOrders + lateEveningOrders
	result.Samples.DaytimeOrders = timedOrders - result.Samples.NightWindowOrders
	result.Samples.OtherTimedOrders = timedOrders - nightOrders
	var nightUnits, nightReturned, otherUnits, otherReturned int
	if err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN CAST(strftime('%H', o.purchased_at_time, '+9 hours') AS INTEGER) BETWEEN 0 AND 5 THEN i.quantity ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN CAST(strftime('%H', o.purchased_at_time, '+9 hours') AS INTEGER) BETWEEN 0 AND 5
			THEN CASE WHEN i.returned_quantity > 0 THEN i.returned_quantity WHEN i.delivery_status = 'returned' THEN i.quantity ELSE 0 END ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN CAST(strftime('%H', o.purchased_at_time, '+9 hours') AS INTEGER) NOT BETWEEN 0 AND 5 THEN i.quantity ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN CAST(strftime('%H', o.purchased_at_time, '+9 hours') AS INTEGER) NOT BETWEEN 0 AND 5
			THEN CASE WHEN i.returned_quantity > 0 THEN i.returned_quantity WHEN i.delivery_status = 'returned' THEN i.quantity ELSE 0 END ELSE 0 END), 0)
		FROM orders o JOIN order_items i ON i.order_ref = o.source_ref
		WHERE o.purchased_at_time IS NOT NULL AND i.commerce_kind = 'product_purchase'
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)`,
		filter.From, filter.From, filter.To, filter.To).Scan(
		&nightUnits, &nightReturned, &otherUnits, &otherReturned); err != nil {
		return core.ShoppingInsights{}, fmt.Errorf("summarize return-time insights: %w", err)
	}
	result.NightReturnedUnitRate = ratio(nightReturned, nightUnits)
	result.OtherReturnedUnitRate = ratio(otherReturned, otherUnits)
	result.Samples.NightOrderedUnits = nightUnits
	result.Samples.OtherOrderedUnits = otherUnits
	var shipments, within24, within48 int
	if err := s.db.QueryRowContext(ctx, `WITH events AS (
		SELECT DISTINCT o.source_ref, o.purchased_at_time, i.delivered_at
		FROM orders o JOIN order_items i ON i.order_ref = o.source_ref
		WHERE o.purchased_at_time IS NOT NULL AND i.delivered_at IS NOT NULL AND o.fully_canceled = 0
			AND i.commerce_kind = 'product_purchase'
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
	), durations AS (
		SELECT 24.0 * (julianday(delivered_at) - julianday(purchased_at_time)) AS hours FROM events
	)
	SELECT COUNT(*), COALESCE(SUM(CASE WHEN hours <= 24 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN hours <= 48 THEN 1 ELSE 0 END), 0)
	FROM durations WHERE hours >= 0`, filter.From, filter.From, filter.To, filter.To).Scan(
		&shipments, &within24, &within48); err != nil {
		return core.ShoppingInsights{}, fmt.Errorf("summarize delivery-speed insights: %w", err)
	}
	result.DeliveredWithin24HoursRate = ratio(within24, shipments)
	result.DeliveredWithin48HoursRate = ratio(within48, shipments)
	result.Samples.DeliveryEvents = shipments
	result.RepeatPurchases, err = s.repeatPurchaseInsights(ctx, filter)
	if err != nil {
		return core.ShoppingInsights{}, err
	}
	result.Basket, err = s.basketInsights(ctx, filter)
	if err != nil {
		return core.ShoppingInsights{}, err
	}
	return result, nil
}

func (s *SQLite) repeatPurchaseInsights(ctx context.Context, filter core.OrderFilter) (core.RepeatPurchaseInsights, error) {
	var result core.RepeatPurchaseInsights
	err := s.db.QueryRowContext(ctx, `WITH retained AS (
		SELECT CASE
			WHEN COALESCE(i.vendor_item_id, '') != '' THEN 'vendor:' || i.vendor_item_id
			WHEN COALESCE(i.product_id, '') != '' THEN 'product:' || i.product_id
			ELSE NULL
		END AS product_key, i.order_ref
		FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
		WHERE o.fully_canceled = 0 AND i.commerce_kind = 'product_purchase'
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
	), occasions AS (
		SELECT DISTINCT product_key, order_ref FROM retained WHERE product_key IS NOT NULL
	), products AS (
		SELECT product_key, COUNT(*) AS purchase_count FROM occasions GROUP BY product_key
	)
	SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN purchase_count >= 2 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(purchase_count), 0),
		COALESCE(SUM(CASE WHEN purchase_count >= 2 THEN purchase_count ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN purchase_count >= 2 THEN purchase_count - 1 ELSE 0 END), 0),
		COALESCE(MAX(purchase_count), 0),
		(SELECT COUNT(*) FROM retained),
		(SELECT COUNT(*) FROM retained WHERE product_key IS NOT NULL)
	FROM products`, filter.From, filter.From, filter.To, filter.To).Scan(
		&result.IdentifiedProductCount, &result.RepeatProductCount,
		&result.PurchaseOccasionCount, &result.RepeatProductPurchaseOccasionCount,
		&result.RepeatChoiceCount, &result.MostRepeatedProductPurchaseCount,
		&result.RetainedItemLineCount, &result.IdentifiedItemLineCount)
	if err != nil {
		return core.RepeatPurchaseInsights{}, fmt.Errorf("summarize repeat purchases: %w", err)
	}
	result.RepeatProductRate = ratio(result.RepeatProductCount, result.IdentifiedProductCount)
	result.RepeatProductPurchaseOccasionRate = ratio(
		result.RepeatProductPurchaseOccasionCount, result.PurchaseOccasionCount,
	)
	result.RepeatChoiceRate = ratio(result.RepeatChoiceCount, result.PurchaseOccasionCount)
	result.ProductIDCoverage = ratio(result.IdentifiedItemLineCount, result.RetainedItemLineCount)
	return result, nil
}

func (s *SQLite) basketInsights(ctx context.Context, filter core.OrderFilter) (core.BasketInsights, error) {
	var result core.BasketInsights
	var average float64
	err := s.db.QueryRowContext(ctx, `WITH baskets AS (
		SELECT o.source_ref, COUNT(*) AS retained_lines, COALESCE(SUM(i.paid_price), 0) AS retained_value
		FROM orders o JOIN order_items i ON i.order_ref = o.source_ref
		WHERE o.fully_canceled = 0 AND i.commerce_kind = 'product_purchase'
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
		GROUP BY o.source_ref
	)
	SELECT COUNT(*), COALESCE(SUM(retained_lines), 0),
		COALESCE(SUM(CASE WHEN retained_lines = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(retained_lines), 0), COALESCE(MAX(retained_lines), 0),
		COALESCE(SUM(retained_value), 0),
		COALESCE(SUM(CASE WHEN retained_lines = 1 THEN retained_value ELSE 0 END), 0)
	FROM baskets`, filter.From, filter.From, filter.To, filter.To).Scan(
		&result.RetainedOrderCount, &result.RetainedItemLineCount,
		&result.SingleItemOrderCount, &average, &result.MaxItemLines,
		&result.RetainedItemAmount, &result.SingleItemOrderAmount)
	if err != nil {
		return core.BasketInsights{}, fmt.Errorf("summarize retained baskets: %w", err)
	}
	result.SingleItemOrderRate = ratio(result.SingleItemOrderCount, result.RetainedOrderCount)
	result.SingleItemSpendRate = ratioInt64(result.SingleItemOrderAmount, result.RetainedItemAmount)
	result.AverageItemLines = roundDecimal(average, 2)
	err = s.db.QueryRowContext(ctx, `WITH baskets AS (
		SELECT o.source_ref, COUNT(*) AS retained_lines, COALESCE(SUM(i.paid_price), 0) AS retained_value
		FROM orders o JOIN order_items i ON i.order_ref = o.source_ref
		WHERE o.fully_canceled = 0 AND i.commerce_kind = 'product_purchase'
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
		GROUP BY o.source_ref
	), ranked AS (
		SELECT CASE WHEN retained_lines = 1 THEN 'single' ELSE 'multi' END AS cohort,
			retained_value, ROW_NUMBER() OVER (
				PARTITION BY CASE WHEN retained_lines = 1 THEN 'single' ELSE 'multi' END ORDER BY retained_value
			) AS row_number, COUNT(*) OVER (
				PARTITION BY CASE WHEN retained_lines = 1 THEN 'single' ELSE 'multi' END
			) AS sample_count
		FROM baskets
	), medians AS (
		SELECT cohort, CAST(ROUND(AVG(retained_value)) AS INTEGER) AS median_value
		FROM ranked WHERE row_number IN ((sample_count + 1) / 2, (sample_count + 2) / 2)
		GROUP BY cohort
	)
	SELECT COALESCE(MAX(CASE WHEN cohort = 'single' THEN median_value END), 0),
		COALESCE(MAX(CASE WHEN cohort = 'multi' THEN median_value END), 0)
	FROM medians`, filter.From, filter.From, filter.To, filter.To).Scan(
		&result.MedianSingleItemOrderValue, &result.MedianMultiItemOrderValue)
	if err != nil {
		return core.BasketInsights{}, fmt.Errorf("summarize retained basket medians: %w", err)
	}
	var compositionAverage float64
	err = s.db.QueryRowContext(ctx, `WITH retained AS (
		SELECT i.order_ref, CASE
			WHEN COALESCE(i.vendor_item_id, '') != '' THEN 'vendor:' || i.vendor_item_id
			WHEN COALESCE(i.product_id, '') != '' THEN 'product:' || i.product_id
			ELSE NULL
		END AS product_key
		FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
		WHERE o.fully_canceled = 0 AND i.commerce_kind = 'product_purchase'
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
	), baskets AS (
		SELECT order_ref, COUNT(*) AS retained_lines, COUNT(product_key) AS identified_lines,
			COUNT(DISTINCT product_key) AS distinct_products
		FROM retained GROUP BY order_ref
	), valid AS (
		SELECT distinct_products FROM baskets
		WHERE retained_lines = identified_lines AND distinct_products > 0
	)
	SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN distinct_products = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(distinct_products), 0), COALESCE(MAX(distinct_products), 0)
	FROM valid`, filter.From, filter.From, filter.To, filter.To).Scan(
		&result.CompositionOrderCount, &result.SingleProductOrderCount,
		&compositionAverage, &result.MaxDistinctProducts)
	if err != nil {
		return core.BasketInsights{}, fmt.Errorf("summarize basket composition: %w", err)
	}
	result.SingleProductOrderRate = ratio(result.SingleProductOrderCount, result.CompositionOrderCount)
	result.AverageDistinctProducts = roundDecimal(compositionAverage, 2)
	return result, nil
}

func (s *SQLite) orderDaySeries(ctx context.Context, filter core.OrderFilter) ([]time.Time, []int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT purchased_at, COUNT(*) FROM orders
		WHERE fully_canceled = 0 AND (NOT EXISTS (
			SELECT 1 FROM order_items any_item WHERE any_item.order_ref = orders.source_ref
		) OR EXISTS (
			SELECT 1 FROM order_items product_item WHERE product_item.order_ref = orders.source_ref
				AND product_item.commerce_kind = 'product_purchase'
				AND COALESCE(product_item.delivery_status, '') NOT IN ('cancelled', 'returned')
		))
			AND (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)
		GROUP BY purchased_at ORDER BY purchased_at`, filter.From, filter.From, filter.To, filter.To)
	if err != nil {
		return nil, nil, fmt.Errorf("list order-day series: %w", err)
	}
	defer rows.Close()
	dates := []time.Time{}
	counts := []int{}
	for rows.Next() {
		var text string
		var count int
		if err := rows.Scan(&text, &count); err != nil {
			return nil, nil, fmt.Errorf("scan order-day series: %w", err)
		}
		parsed, err := time.Parse(time.DateOnly, text)
		if err != nil {
			return nil, nil, fmt.Errorf("decode order-day series")
		}
		dates = append(dates, parsed)
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate order-day series: %w", err)
	}
	return dates, counts, nil
}

func peakBucket(entries []core.CountBucket) core.CountBucket {
	var result core.CountBucket
	for _, entry := range entries {
		if entry.Count > result.Count || (entry.Count == result.Count && (result.Key == "" || entry.Key < result.Key)) {
			result = entry
		}
	}
	return result
}

func retainedPurchaseMonths(entries []core.MonthlyOrderStats) []core.MonthlyOrderStats {
	result := make([]core.MonthlyOrderStats, 0, len(entries))
	for _, entry := range entries {
		retainedOrders := entry.OrderCount - entry.FullyCanceledOrderCount
		if retainedOrders <= 0 {
			continue
		}
		result = append(result, core.MonthlyOrderStats{
			Month:                  entry.Month,
			OrderCount:             retainedOrders,
			TotalAmount:            entry.NonCanceledTotalAmount,
			NonCanceledTotalAmount: entry.NonCanceledTotalAmount,
		})
	}
	return result
}

func peakMonths(entries []core.MonthlyOrderStats) (core.MonthlyOrderStats, core.MonthlyOrderStats) {
	var busiest, highestSpend core.MonthlyOrderStats
	for _, entry := range entries {
		if entry.OrderCount > busiest.OrderCount || (entry.OrderCount == busiest.OrderCount && (busiest.Month == "" || entry.Month < busiest.Month)) {
			busiest = entry
		}
		if entry.NonCanceledTotalAmount > highestSpend.NonCanceledTotalAmount ||
			(entry.NonCanceledTotalAmount == highestSpend.NonCanceledTotalAmount && (highestSpend.Month == "" || entry.Month < highestSpend.Month)) {
			highestSpend = entry
		}
	}
	return busiest, highestSpend
}

func longestMonthStreak(entries []core.MonthlyOrderStats) int {
	longest, current, previous := 0, 0, -2
	for _, entry := range entries {
		parsed, err := time.Parse("2006-01", entry.Month)
		if err != nil {
			continue
		}
		index := parsed.Year()*12 + int(parsed.Month())
		if index == previous+1 {
			current++
		} else {
			current = 1
		}
		if current > longest {
			longest = current
		}
		previous = index
	}
	return longest
}

func compareDeliveryTrend(trend []core.DeliveryDurationSummary) core.DeliveryTrendComparison {
	if len(trend) < 2 {
		return core.DeliveryTrendComparison{}
	}
	baseline := trend[0]
	latest := trend[len(trend)-1]
	result := core.DeliveryTrendComparison{
		BaselinePeriod:    baseline.Period,
		LatestPeriod:      latest.Period,
		AverageHoursDelta: roundDecimal(latest.AverageHours-baseline.AverageHours, 2),
	}
	if baseline.AverageHours != 0 {
		result.AverageHoursPercentChange = roundDecimal(result.AverageHoursDelta/baseline.AverageHours, 6)
	}
	switch {
	case result.AverageHoursDelta > 0:
		result.Direction = "slower"
	case result.AverageHoursDelta < 0:
		result.Direction = "faster"
	default:
		result.Direction = "unchanged"
	}
	return result
}

func (s *SQLite) purchaseHourBuckets(ctx context.Context, filter core.OrderFilter) ([]core.CountBucket, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT strftime('%H', purchased_at_time, '+9 hours'), COUNT(*)
		FROM orders WHERE purchased_at_time IS NOT NULL AND fully_canceled = 0 AND (NOT EXISTS (
			SELECT 1 FROM order_items any_item WHERE any_item.order_ref = orders.source_ref
		) OR EXISTS (
			SELECT 1 FROM order_items product_item WHERE product_item.order_ref = orders.source_ref
				AND product_item.commerce_kind = 'product_purchase'
				AND COALESCE(product_item.delivery_status, '') NOT IN ('cancelled', 'returned')
		))
			AND (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)
		GROUP BY 1 ORDER BY 1`, filter.From, filter.From, filter.To, filter.To)
	if err != nil {
		return nil, fmt.Errorf("summarize purchase hours: %w", err)
	}
	return scanCountBuckets(rows, nil)
}

func (s *SQLite) purchaseWeekdayBuckets(ctx context.Context, filter core.OrderFilter) ([]core.CountBucket, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT strftime('%w', purchased_at_time, '+9 hours'), COUNT(*)
		FROM orders WHERE purchased_at_time IS NOT NULL AND fully_canceled = 0 AND (NOT EXISTS (
			SELECT 1 FROM order_items any_item WHERE any_item.order_ref = orders.source_ref
		) OR EXISTS (
			SELECT 1 FROM order_items product_item WHERE product_item.order_ref = orders.source_ref
				AND product_item.commerce_kind = 'product_purchase'
				AND COALESCE(product_item.delivery_status, '') NOT IN ('cancelled', 'returned')
		))
			AND (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)
		GROUP BY 1 ORDER BY 1`, filter.From, filter.From, filter.To, filter.To)
	if err != nil {
		return nil, fmt.Errorf("summarize purchase weekdays: %w", err)
	}
	weekday := map[string]string{"0": "sun", "1": "mon", "2": "tue", "3": "wed", "4": "thu", "5": "fri", "6": "sat"}
	return scanCountBuckets(rows, weekday)
}

func (s *SQLite) purchaseMonthStats(ctx context.Context, filter core.OrderFilter) ([]core.MonthlyOrderStats, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT substr(purchased_at, 1, 7), COUNT(*),
		COALESCE(SUM(total_amount), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 0 THEN total_amount ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 1 THEN 1 ELSE 0 END), 0)
		FROM orders WHERE (NOT EXISTS (
			SELECT 1 FROM order_items any_item WHERE any_item.order_ref = orders.source_ref
		) OR EXISTS (
			SELECT 1 FROM order_items product_item WHERE product_item.order_ref = orders.source_ref
				AND product_item.commerce_kind = 'product_purchase'
				AND COALESCE(product_item.delivery_status, '') NOT IN ('cancelled', 'returned')
		)) AND (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)
		GROUP BY 1 ORDER BY 1`, filter.From, filter.From, filter.To, filter.To)
	if err != nil {
		return nil, fmt.Errorf("summarize purchase months: %w", err)
	}
	defer rows.Close()
	result := []core.MonthlyOrderStats{}
	for rows.Next() {
		var entry core.MonthlyOrderStats
		if err := rows.Scan(&entry.Month, &entry.OrderCount, &entry.TotalAmount,
			&entry.NonCanceledTotalAmount, &entry.FullyCanceledOrderCount); err != nil {
			return nil, fmt.Errorf("scan purchase months: %w", err)
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase months: %w", err)
	}
	return result, nil
}

func (s *SQLite) topBrandBuckets(ctx context.Context, filter core.OrderFilter) ([]core.CountBucket, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.brand_name, COUNT(*)
		FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
		WHERE i.brand_name IS NOT NULL AND i.brand_name != '' AND o.fully_canceled = 0
			AND i.commerce_kind = 'product_purchase'
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
		GROUP BY i.brand_name ORDER BY COUNT(*) DESC, i.brand_name LIMIT 20`,
		filter.From, filter.From, filter.To, filter.To)
	if err != nil {
		return nil, fmt.Errorf("summarize purchase brands: %w", err)
	}
	return scanCountBuckets(rows, nil)
}

func scanCountBuckets(rows *sql.Rows, labels map[string]string) ([]core.CountBucket, error) {
	defer rows.Close()
	result := []core.CountBucket{}
	for rows.Next() {
		var entry core.CountBucket
		if err := rows.Scan(&entry.Key, &entry.Count); err != nil {
			return nil, fmt.Errorf("scan count buckets: %w", err)
		}
		if label := labels[entry.Key]; label != "" {
			entry.Key = label
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate count buckets: %w", err)
	}
	return result, nil
}

func (s *SQLite) deliveryDurationStats(ctx context.Context, filter core.OrderFilter) (core.DeliveryDurationSummary, []core.DeliveryDurationSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT substr(o.purchased_at, 1, 4), o.purchased_at_time, event.delivered_at
		FROM (SELECT DISTINCT order_ref, delivered_at FROM order_items WHERE delivered_at IS NOT NULL AND commerce_kind = 'product_purchase') event
		JOIN orders o ON o.source_ref = event.order_ref
		WHERE o.purchased_at_time IS NOT NULL AND o.fully_canceled = 0
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
		ORDER BY o.purchased_at_time`, filter.From, filter.From, filter.To, filter.To)
	if err != nil {
		return core.DeliveryDurationSummary{}, nil, fmt.Errorf("summarize delivery duration: %w", err)
	}
	defer rows.Close()
	all := []float64{}
	byYear := map[string][]float64{}
	for rows.Next() {
		var year, purchasedText, deliveredText string
		if err := rows.Scan(&year, &purchasedText, &deliveredText); err != nil {
			return core.DeliveryDurationSummary{}, nil, fmt.Errorf("scan delivery duration: %w", err)
		}
		purchased, purchaseErr := time.Parse(time.RFC3339Nano, purchasedText)
		delivered, deliveryErr := time.Parse(time.RFC3339Nano, deliveredText)
		if purchaseErr != nil || deliveryErr != nil {
			return core.DeliveryDurationSummary{}, nil, fmt.Errorf("decode delivery duration timestamp")
		}
		hours := delivered.Sub(purchased).Hours()
		if hours < 0 {
			continue
		}
		all = append(all, hours)
		byYear[year] = append(byYear[year], hours)
	}
	if err := rows.Err(); err != nil {
		return core.DeliveryDurationSummary{}, nil, fmt.Errorf("iterate delivery duration: %w", err)
	}
	years := make([]string, 0, len(byYear))
	for year := range byYear {
		years = append(years, year)
	}
	sort.Strings(years)
	trend := make([]core.DeliveryDurationSummary, 0, len(years))
	for _, year := range years {
		trend = append(trend, durationSummary(year, byYear[year]))
	}
	return durationSummary("", all), trend, nil
}

func durationSummary(period string, values []float64) core.DeliveryDurationSummary {
	result := core.DeliveryDurationSummary{Period: period, ShipmentCount: len(values)}
	if len(values) == 0 {
		return result
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	var total float64
	for _, value := range sorted {
		total += value
	}
	result.AverageHours = roundDecimal(total/float64(len(sorted)), 2)
	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		result.MedianHours = roundDecimal((sorted[middle-1]+sorted[middle])/2, 2)
	} else {
		result.MedianHours = roundDecimal(sorted[middle], 2)
	}
	result.P90Hours = roundDecimal(sorted[int(math.Ceil(float64(len(sorted))*0.9))-1], 2)
	return result
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return roundDecimal(float64(numerator)/float64(denominator), 6)
}

func ratioInt64(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return roundDecimal(float64(numerator)/float64(denominator), 6)
}

func roundDecimal(value float64, places int) float64 {
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}

func (s *SQLite) ReorderCandidates(ctx context.Context, filter core.OrderFilter) ([]core.ReorderCandidate, error) {
	filter, err := normalizeFilter(filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT COALESCE(i.product_id, ''),
		COALESCE(i.vendor_item_id, ''), i.name, COUNT(DISTINCT i.order_ref),
		SUM(i.quantity), MAX(o.purchased_at)
		FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
		WHERE (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
			AND i.commerce_kind = 'product_purchase'
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
		GROUP BY COALESCE(i.vendor_item_id, i.product_id, i.name), i.name
		ORDER BY MAX(o.purchased_at) DESC, SUM(i.quantity) DESC
		LIMIT ?`, filter.From, filter.From, filter.To, filter.To, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list reorder candidates: %w", err)
	}
	defer rows.Close()
	var result []core.ReorderCandidate
	for rows.Next() {
		var candidate core.ReorderCandidate
		if err := rows.Scan(&candidate.ProductID, &candidate.VendorItemID, &candidate.Name,
			&candidate.PurchaseCount, &candidate.TotalQuantity, &candidate.LastPurchased); err != nil {
			return nil, fmt.Errorf("scan reorder candidate: %w", err)
		}
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reorder candidates: %w", err)
	}
	if result == nil {
		result = []core.ReorderCandidate{}
	}
	return result, nil
}

func (s *SQLite) Export(ctx context.Context, filter core.OrderFilter, exportedAt time.Time) (core.OrderExport, error) {
	orders, err := s.ListOrders(ctx, filter)
	if err != nil {
		return core.OrderExport{}, err
	}
	return core.OrderExport{SchemaVersion: 1, ExportedAt: exportedAt.UTC(), Orders: orders}, nil
}

func (s *SQLite) Purge(ctx context.Context) (core.PurgeResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.PurgeResult{}, fmt.Errorf("begin purge: %w", err)
	}
	defer tx.Rollback()
	var result core.PurgeResult
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM order_items").Scan(&result.ItemsDeleted); err != nil {
		return core.PurgeResult{}, fmt.Errorf("count normalized order items: %w", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM orders").Scan(&result.OrdersDeleted); err != nil {
		return core.PurgeResult{}, fmt.Errorf("count normalized orders: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM orders"); err != nil {
		return core.PurgeResult{}, fmt.Errorf("purge normalized orders: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return core.PurgeResult{}, fmt.Errorf("commit purge: %w", err)
	}
	return result, nil
}

func (s *SQLite) listItems(ctx context.Context, sourceRef string) ([]core.OrderItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT COALESCE(product_id, ''), COALESCE(vendor_item_id, ''),
		name, quantity, cancelled_quantity, returned_quantity, unit_price, paid_price, COALESCE(seller_name, ''),
		COALESCE(brand_name, ''), COALESCE(product_type, ''), COALESCE(division_type, ''),
		commerce_kind, COALESCE(delivery_status, ''), delivered_at
		FROM order_items WHERE order_ref = ? ORDER BY position`, sourceRef)
	if err != nil {
		return nil, fmt.Errorf("list normalized order items: %w", err)
	}
	defer rows.Close()
	var items []core.OrderItem
	for rows.Next() {
		var item core.OrderItem
		var deliveredAt sql.NullString
		if err := rows.Scan(&item.ProductID, &item.VendorItemID, &item.Name, &item.Quantity,
			&item.CancelledQuantity, &item.ReturnedQuantity, &item.UnitPrice, &item.PaidPrice,
			&item.SellerName, &item.BrandName, &item.ProductType, &item.DivisionType,
			&item.CommerceKind, &item.DeliveryStatus, &deliveredAt); err != nil {
			return nil, fmt.Errorf("scan normalized order item: %w", err)
		}
		if deliveredAt.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, deliveredAt.String)
			if err != nil {
				return nil, fmt.Errorf("decode normalized delivery timestamp: %w", err)
			}
			item.DeliveredAt = &parsed
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate normalized order items: %w", err)
	}
	if items == nil {
		items = []core.OrderItem{}
	}
	return items, nil
}

func normalizeFilter(filter core.OrderFilter) (core.OrderFilter, error) {
	if filter.Limit == 0 {
		filter.Limit = defaultOrderLimit
	}
	if filter.Limit < 1 || filter.Limit > maxOrderLimit {
		return core.OrderFilter{}, errors.New("invalid order filter: limit must be between 1 and 1000")
	}
	return validateFilterDates(filter)
}

func normalizeFilterForAggregate(filter core.OrderFilter) (core.OrderFilter, error) {
	filter.Limit = 0
	return validateFilterDates(filter)
}

func validateFilterDates(filter core.OrderFilter) (core.OrderFilter, error) {
	var from, to time.Time
	var err error
	if filter.From != "" {
		from, err = time.Parse(time.DateOnly, filter.From)
		if err != nil {
			return core.OrderFilter{}, errors.New("invalid order filter: from must use YYYY-MM-DD")
		}
	}
	if filter.To != "" {
		to, err = time.Parse(time.DateOnly, filter.To)
		if err != nil {
			return core.OrderFilter{}, errors.New("invalid order filter: to must use YYYY-MM-DD")
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return core.OrderFilter{}, errors.New("invalid order filter: from must not be after to")
	}
	return filter, nil
}

func validateOrder(order core.Order) error {
	if order.SourceRef == "" || order.Currency != "KRW" {
		return fmt.Errorf("%w: invalid normalized order identity", core.ErrInvalidOrderData)
	}
	if _, err := time.Parse(time.DateOnly, order.PurchasedAt); err != nil {
		return fmt.Errorf("%w: invalid normalized purchase date", core.ErrInvalidOrderData)
	}
	if order.PurchasedAtTime != nil {
		if order.PurchasedAtTime.Year() < 2000 || order.PurchasedAtTime.Year() > 2100 ||
			order.PurchasedAtTime.In(time.FixedZone("KST", 9*60*60)).Format(time.DateOnly) != order.PurchasedAt {
			return fmt.Errorf("%w: invalid normalized purchase timestamp", core.ErrInvalidOrderData)
		}
	}
	if order.TotalAmount < 0 || order.DiscountAmount < 0 || order.ShippingFee < 0 {
		return fmt.Errorf("%w: invalid normalized order amount", core.ErrInvalidOrderData)
	}
	for _, item := range order.Items {
		if item.Name == "" || item.Quantity < 1 || item.UnitPrice < 0 || item.PaidPrice < 0 ||
			item.CancelledQuantity < 0 || item.ReturnedQuantity < 0 ||
			item.CancelledQuantity > item.Quantity || item.ReturnedQuantity > item.Quantity {
			return fmt.Errorf("%w: invalid normalized order item", core.ErrInvalidOrderData)
		}
	}
	return nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
