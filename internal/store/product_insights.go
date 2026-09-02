package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

const spendDayProductLimit = 5

type rankedProduct struct {
	key                    string
	aggregate              core.ProductAggregate
	spendEligibleUnitCount int
}

func (s *SQLite) ProductInsights(ctx context.Context, filter core.OrderFilter) (core.ProductInsights, error) {
	filter, err := normalizeFilterForAggregate(filter)
	if err != nil {
		return core.ProductInsights{}, err
	}
	result := core.ProductInsights{
		SchemaVersion: 1,
		Visibility:    "private_local",
		From:          filter.From,
		To:            filter.To,
		Currency:      "KRW",
		Definitions: core.ProductInsightDefinitions{
			Provenance:      "derived_from_normalized_structured_order_model",
			ProductIdentity: "vendor_item_id_else_product_id",
			RetainedUnit:    "quantity_minus_cancelled_quantity_minus_returned_quantity_on_non_fully_cancelled_non_cancelled_non_returned_item_lines",
			ProductSpend:    "paid_price_on_identified_item_lines_with_zero_cancelled_and_returned_quantity",
			PaidUnitAmount:  "rounded_paid_price_divided_by_quantity_on_product_spend_eligible_item_lines",
			SpendDay:        "sum_of_non_fully_cancelled_product_order_total_amount_grouped_by_purchase_date",
			DayProductLimit: spendDayProductLimit,
		},
	}
	if err := s.productSpendWindow(ctx, filter, &result); err != nil {
		return core.ProductInsights{}, err
	}
	if err := s.productCoverage(ctx, filter, &result); err != nil {
		return core.ProductInsights{}, err
	}
	products, err := s.productAggregates(ctx, filter)
	if err != nil {
		return core.ProductInsights{}, err
	}
	if len(products) > 0 {
		result.TopByUnits = rankedProductLeader(products, compareProductUnits).aggregate
		result.TopByOrders = rankedProductLeader(products, compareProductOrders).aggregate
		spendLeader := rankedProductLeader(products, compareProductSpend)
		if spendLeader.aggregate.TotalPaidAmount > 0 {
			result.TopBySpend = spendLeader.aggregate
		}
	}
	result.HighestPaidUnit, err = s.paidUnitHighlight(ctx, filter, true)
	if err != nil {
		return core.ProductInsights{}, err
	}
	result.LowestPaidUnit, err = s.paidUnitHighlight(ctx, filter, false)
	if err != nil {
		return core.ProductInsights{}, err
	}
	result.HighestSpendDay, err = s.spendDayInsight(ctx, filter, true)
	if err != nil {
		return core.ProductInsights{}, err
	}
	result.LowestSpendDay, err = s.spendDayInsight(ctx, filter, false)
	if err != nil {
		return core.ProductInsights{}, err
	}
	return result, nil
}

func (s *SQLite) productSpendWindow(ctx context.Context, filter core.OrderFilter, result *core.ProductInsights) error {
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MIN(purchased_at), ''), COALESCE(MAX(purchased_at), ''),
		COUNT(DISTINCT substr(purchased_at, 1, 7)), COALESCE(SUM(total_amount), 0)
		FROM orders WHERE fully_canceled = 0
			AND EXISTS (SELECT 1 FROM order_items i WHERE i.order_ref = orders.source_ref AND i.commerce_kind = 'product_purchase')
			AND (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)`,
		filter.From, filter.From, filter.To, filter.To).Scan(
		&result.FirstPurchaseDate, &result.LastPurchaseDate, &result.ActiveMonthCount, &result.TotalSpendAmount)
	if err != nil {
		return fmt.Errorf("summarize product insight spend window: %w", err)
	}
	result.CalendarMonthCount = inclusiveMonthCount(result.FirstPurchaseDate, result.LastPurchaseDate)
	if result.CalendarMonthCount > 0 {
		result.AverageMonthlySpendAmount = roundedQuotient(result.TotalSpendAmount, int64(result.CalendarMonthCount))
	}
	return nil
}

func (s *SQLite) productCoverage(ctx context.Context, filter core.OrderFilter, result *core.ProductInsights) error {
	err := s.db.QueryRowContext(ctx, `WITH retained AS (
		SELECT CASE
			WHEN COALESCE(i.vendor_item_id, '') != '' THEN 'vendor:' || i.vendor_item_id
			WHEN COALESCE(i.product_id, '') != '' THEN 'product:' || i.product_id
			ELSE NULL
		END AS product_key,
		MAX(i.quantity - i.cancelled_quantity - i.returned_quantity, 0) AS retained_units,
		i.cancelled_quantity, i.returned_quantity
		FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
		WHERE o.fully_canceled = 0 AND i.commerce_kind = 'product_purchase'
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
	), eligible AS (
		SELECT * FROM retained WHERE retained_units > 0
	)
	SELECT COUNT(*), COALESCE(SUM(retained_units), 0),
		COALESCE(SUM(CASE WHEN product_key IS NOT NULL THEN 1 ELSE 0 END), 0),
		COUNT(DISTINCT product_key),
		COALESCE(SUM(CASE WHEN product_key IS NOT NULL AND cancelled_quantity = 0 AND returned_quantity = 0 THEN 1 ELSE 0 END), 0)
	FROM eligible`, filter.From, filter.From, filter.To, filter.To).Scan(
		&result.RetainedItemLineCount, &result.RetainedUnitCount,
		&result.IdentifiedItemLineCount, &result.IdentifiedProductCount,
		&result.SpendEligibleItemLineCount)
	if err != nil {
		return fmt.Errorf("summarize product insight coverage: %w", err)
	}
	result.ProductIDCoverage = ratio(result.IdentifiedItemLineCount, result.RetainedItemLineCount)
	result.SpendEligibleItemLineRate = ratio(result.SpendEligibleItemLineCount, result.RetainedItemLineCount)
	return nil
}

func (s *SQLite) productAggregates(ctx context.Context, filter core.OrderFilter) ([]rankedProduct, error) {
	rows, err := s.db.QueryContext(ctx, `WITH retained AS (
		SELECT i.id, i.order_ref, i.name, i.quantity, i.paid_price,
			i.cancelled_quantity, i.returned_quantity, o.purchased_at,
			CASE
				WHEN COALESCE(i.vendor_item_id, '') != '' THEN 'vendor:' || i.vendor_item_id
				WHEN COALESCE(i.product_id, '') != '' THEN 'product:' || i.product_id
				ELSE NULL
			END AS product_key,
			MAX(i.quantity - i.cancelled_quantity - i.returned_quantity, 0) AS retained_units
		FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
		WHERE o.fully_canceled = 0 AND i.commerce_kind = 'product_purchase'
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
	), eligible AS (
		SELECT * FROM retained WHERE retained_units > 0 AND product_key IS NOT NULL
	), names AS (
		SELECT product_key, name, ROW_NUMBER() OVER (
			PARTITION BY product_key ORDER BY purchased_at DESC, id DESC
		) AS position FROM eligible
	), aggregates AS (
		SELECT product_key, COUNT(DISTINCT order_ref) AS purchase_count,
			COALESCE(SUM(retained_units), 0) AS unit_count,
			COALESCE(SUM(CASE WHEN cancelled_quantity = 0 AND returned_quantity = 0 THEN paid_price ELSE 0 END), 0) AS paid_amount,
			COALESCE(SUM(CASE WHEN cancelled_quantity = 0 AND returned_quantity = 0 THEN quantity ELSE 0 END), 0) AS spend_units,
			MIN(purchased_at) AS first_purchased, MAX(purchased_at) AS last_purchased
		FROM eligible GROUP BY product_key
	)
	SELECT aggregates.product_key, names.name, aggregates.purchase_count, aggregates.unit_count,
		aggregates.paid_amount, aggregates.spend_units, aggregates.first_purchased, aggregates.last_purchased
	FROM aggregates JOIN names ON names.product_key = aggregates.product_key AND names.position = 1
	ORDER BY aggregates.product_key`, filter.From, filter.From, filter.To, filter.To)
	if err != nil {
		return nil, fmt.Errorf("summarize identified products: %w", err)
	}
	defer rows.Close()
	products := []rankedProduct{}
	for rows.Next() {
		var product rankedProduct
		if err := rows.Scan(&product.key, &product.aggregate.Name, &product.aggregate.PurchaseCount,
			&product.aggregate.UnitCount, &product.aggregate.TotalPaidAmount, &product.spendEligibleUnitCount,
			&product.aggregate.FirstPurchased, &product.aggregate.LastPurchased); err != nil {
			return nil, fmt.Errorf("scan identified product summary: %w", err)
		}
		if product.spendEligibleUnitCount > 0 {
			product.aggregate.AveragePaidUnitAmount = roundedQuotient(
				product.aggregate.TotalPaidAmount, int64(product.spendEligibleUnitCount),
			)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate identified product summaries: %w", err)
	}
	return products, nil
}

func (s *SQLite) paidUnitHighlight(ctx context.Context, filter core.OrderFilter, highest bool) (core.PaidUnitHighlight, error) {
	direction := "ASC"
	if highest {
		direction = "DESC"
	}
	query := `SELECT i.name, o.purchased_at, i.quantity, i.paid_price,
		CAST(ROUND(1.0 * i.paid_price / i.quantity) AS INTEGER)
	FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
	WHERE o.fully_canceled = 0 AND i.quantity > 0 AND i.paid_price > 0
		AND i.commerce_kind = 'product_purchase'
		AND i.cancelled_quantity = 0 AND i.returned_quantity = 0
		AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
		AND (COALESCE(i.vendor_item_id, '') != '' OR COALESCE(i.product_id, '') != '')
		AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
	ORDER BY (1.0 * i.paid_price / i.quantity) ` + direction + `, o.purchased_at, i.id LIMIT 1`
	var result core.PaidUnitHighlight
	err := s.db.QueryRowContext(ctx, query, filter.From, filter.From, filter.To, filter.To).Scan(
		&result.Name, &result.Date, &result.Quantity, &result.PaidAmount, &result.PaidUnitAmount)
	if err == sql.ErrNoRows {
		return result, nil
	}
	if err != nil {
		return core.PaidUnitHighlight{}, fmt.Errorf("summarize paid-unit extreme: %w", err)
	}
	return result, nil
}

func (s *SQLite) spendDayInsight(ctx context.Context, filter core.OrderFilter, highest bool) (core.SpendDayInsight, error) {
	direction := "ASC"
	if highest {
		direction = "DESC"
	}
	query := `SELECT purchased_at, COALESCE(SUM(total_amount), 0), COUNT(*)
		FROM orders WHERE fully_canceled = 0
			AND EXISTS (SELECT 1 FROM order_items i WHERE i.order_ref = orders.source_ref AND i.commerce_kind = 'product_purchase')
			AND (? = '' OR purchased_at >= ?) AND (? = '' OR purchased_at <= ?)
		GROUP BY purchased_at HAVING SUM(total_amount) > 0
		ORDER BY SUM(total_amount) ` + direction + `, purchased_at LIMIT 1`
	result := core.SpendDayInsight{Products: []core.DayProductSummary{}}
	err := s.db.QueryRowContext(ctx, query, filter.From, filter.From, filter.To, filter.To).Scan(
		&result.Date, &result.TotalAmount, &result.OrderCount)
	if err == sql.ErrNoRows {
		return result, nil
	}
	if err != nil {
		return core.SpendDayInsight{}, fmt.Errorf("summarize spend-day extreme: %w", err)
	}
	result.Products, result.ProductCount, result.RetainedItemLineCount, err = s.spendDayProducts(ctx, result.Date)
	if err != nil {
		return core.SpendDayInsight{}, err
	}
	return result, nil
}

func (s *SQLite) spendDayProducts(ctx context.Context, date string) ([]core.DayProductSummary, int, int, error) {
	rows, err := s.db.QueryContext(ctx, `WITH retained AS (
		SELECT i.id, i.name, i.quantity, i.paid_price, i.cancelled_quantity, i.returned_quantity,
			CASE
				WHEN COALESCE(i.vendor_item_id, '') != '' THEN 'vendor:' || i.vendor_item_id
				WHEN COALESCE(i.product_id, '') != '' THEN 'product:' || i.product_id
				ELSE 'line:' || i.id
			END AS product_key,
			MAX(i.quantity - i.cancelled_quantity - i.returned_quantity, 0) AS retained_units
		FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
		WHERE o.fully_canceled = 0 AND o.purchased_at = ?
			AND i.commerce_kind = 'product_purchase'
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
	), eligible AS (
		SELECT * FROM retained WHERE retained_units > 0
	), names AS (
		SELECT product_key, name, ROW_NUMBER() OVER (
			PARTITION BY product_key ORDER BY id DESC
		) AS position FROM eligible
	), grouped AS (
		SELECT product_key, SUM(retained_units) AS unit_count,
			COALESCE(SUM(CASE WHEN cancelled_quantity = 0 AND returned_quantity = 0 THEN paid_price ELSE 0 END), 0) AS paid_amount,
			COUNT(*) AS item_line_count
		FROM eligible GROUP BY product_key
	), presented AS (
		SELECT names.name, grouped.unit_count, grouped.paid_amount, grouped.item_line_count
		FROM grouped JOIN names ON names.product_key = grouped.product_key AND names.position = 1
	)
	SELECT name, unit_count, paid_amount, COUNT(*) OVER (), SUM(item_line_count) OVER ()
	FROM presented ORDER BY paid_amount DESC, unit_count DESC, name LIMIT ?`, date, spendDayProductLimit)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("list spend-day products: %w", err)
	}
	defer rows.Close()
	products := []core.DayProductSummary{}
	productCount, itemLineCount := 0, 0
	for rows.Next() {
		var product core.DayProductSummary
		if err := rows.Scan(&product.Name, &product.UnitCount, &product.PaidAmount, &productCount, &itemLineCount); err != nil {
			return nil, 0, 0, fmt.Errorf("scan spend-day product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, fmt.Errorf("iterate spend-day products: %w", err)
	}
	return products, productCount, itemLineCount, nil
}

func rankedProductLeader(products []rankedProduct, compare func(left, right rankedProduct) bool) rankedProduct {
	ranked := append([]rankedProduct(nil), products...)
	sort.SliceStable(ranked, func(left, right int) bool { return compare(ranked[left], ranked[right]) })
	return ranked[0]
}

func compareProductUnits(left, right rankedProduct) bool {
	if left.aggregate.UnitCount != right.aggregate.UnitCount {
		return left.aggregate.UnitCount > right.aggregate.UnitCount
	}
	if left.aggregate.PurchaseCount != right.aggregate.PurchaseCount {
		return left.aggregate.PurchaseCount > right.aggregate.PurchaseCount
	}
	if left.aggregate.TotalPaidAmount != right.aggregate.TotalPaidAmount {
		return left.aggregate.TotalPaidAmount > right.aggregate.TotalPaidAmount
	}
	return left.key < right.key
}

func compareProductOrders(left, right rankedProduct) bool {
	if left.aggregate.PurchaseCount != right.aggregate.PurchaseCount {
		return left.aggregate.PurchaseCount > right.aggregate.PurchaseCount
	}
	if left.aggregate.UnitCount != right.aggregate.UnitCount {
		return left.aggregate.UnitCount > right.aggregate.UnitCount
	}
	if left.aggregate.TotalPaidAmount != right.aggregate.TotalPaidAmount {
		return left.aggregate.TotalPaidAmount > right.aggregate.TotalPaidAmount
	}
	return left.key < right.key
}

func compareProductSpend(left, right rankedProduct) bool {
	if left.aggregate.TotalPaidAmount != right.aggregate.TotalPaidAmount {
		return left.aggregate.TotalPaidAmount > right.aggregate.TotalPaidAmount
	}
	if left.aggregate.UnitCount != right.aggregate.UnitCount {
		return left.aggregate.UnitCount > right.aggregate.UnitCount
	}
	if left.aggregate.PurchaseCount != right.aggregate.PurchaseCount {
		return left.aggregate.PurchaseCount > right.aggregate.PurchaseCount
	}
	return left.key < right.key
}

func inclusiveMonthCount(first, last string) int {
	start, startErr := time.Parse(time.DateOnly, first)
	end, endErr := time.Parse(time.DateOnly, last)
	if startErr != nil || endErr != nil || end.Before(start) {
		return 0
	}
	return (end.Year()-start.Year())*12 + int(end.Month()-start.Month()) + 1
}

func roundedQuotient(numerator, denominator int64) int64 {
	if denominator <= 0 {
		return 0
	}
	return (numerator + denominator/2) / denominator
}
