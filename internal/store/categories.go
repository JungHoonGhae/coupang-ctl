package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func (s *SQLite) PendingCategoryProducts(ctx context.Context, limit int) ([]core.ProductReference, error) {
	if limit < 1 || limit > 1000 {
		return nil, errors.New("category product limit must be between 1 and 1000")
	}
	rows, err := s.db.QueryContext(ctx, `WITH products AS (
		SELECT 'vendor:' || vendor_item_id AS product_key,
			MAX(product_id) AS product_id, vendor_item_id
		FROM order_items
		WHERE COALESCE(product_id, '') != '' AND COALESCE(vendor_item_id, '') != ''
		GROUP BY vendor_item_id
	)
	SELECT products.product_id, products.vendor_item_id
	FROM products LEFT JOIN product_categories cached ON cached.product_key = products.product_key
	WHERE cached.product_key IS NULL
		OR (cached.source = ? AND cached.breadcrumb_json = '[]')
	ORDER BY products.vendor_item_id
	LIMIT ?`, core.CategorySourceProductJSONLDBreadcrumb, limit)
	if err != nil {
		return nil, fmt.Errorf("load uncached category products: %w", err)
	}
	defer rows.Close()
	result := []core.ProductReference{}
	for rows.Next() {
		var reference core.ProductReference
		if err := rows.Scan(&reference.ProductID, &reference.VendorItemID); err != nil {
			return nil, fmt.Errorf("scan uncached category product: %w", err)
		}
		result = append(result, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate uncached category products: %w", err)
	}
	return result, nil
}

func (s *SQLite) RemainingCategoryProducts(ctx context.Context) (int, error) {
	var result int
	err := s.db.QueryRowContext(ctx, `WITH products AS (
		SELECT 'vendor:' || vendor_item_id AS product_key
		FROM order_items
		WHERE COALESCE(product_id, '') != '' AND COALESCE(vendor_item_id, '') != ''
		GROUP BY vendor_item_id
	)
	SELECT COUNT(*) FROM products
	LEFT JOIN product_categories cached ON cached.product_key = products.product_key
	WHERE cached.product_key IS NULL
		OR (cached.source = ? AND cached.breadcrumb_json = '[]')`, core.CategorySourceProductJSONLDBreadcrumb).Scan(&result)
	if err != nil {
		return 0, fmt.Errorf("count uncached category products: %w", err)
	}
	return result, nil
}

func (s *SQLite) SaveProductCategory(ctx context.Context, reference core.ProductReference, category core.ProductCategory) error {
	if reference.VendorItemID == "" || category.Source != core.CategorySourceProductJSONLDBreadcrumb || len(category.Path) == 0 || len(category.Path) > 12 {
		return errors.New("invalid product category")
	}
	path := make([]core.ProductCategoryNode, len(category.Path))
	lastPosition := 0
	for index, node := range category.Path {
		path[index] = node
		path[index].ID = strings.TrimSpace(node.ID)
		path[index].Name = strings.TrimSpace(node.Name)
		if path[index].ID == "" || path[index].Name == "" || len([]rune(path[index].Name)) > 100 || path[index].Position <= lastPosition {
			return errors.New("invalid product category")
		}
		lastPosition = path[index].Position
	}
	encodedPath, err := json.Marshal(path)
	if err != nil {
		return fmt.Errorf("encode product category path: %w", err)
	}
	second := path[0].Name
	if len(path) > 1 {
		second = path[1].Name
	}
	leaf := path[len(path)-1]
	_, err = s.db.ExecContext(ctx, `INSERT INTO product_categories(
		product_key, source, top_category, second_category, leaf_category,
		leaf_category_id, breadcrumb_json, fetched_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	ON CONFLICT(product_key) DO UPDATE SET source = excluded.source,
		top_category = excluded.top_category, second_category = excluded.second_category,
		leaf_category = excluded.leaf_category, leaf_category_id = excluded.leaf_category_id,
		breadcrumb_json = excluded.breadcrumb_json, fetched_at = excluded.fetched_at`,
		"vendor:"+reference.VendorItemID, category.Source, path[0].Name, second, leaf.Name, leaf.ID, string(encodedPath))
	if err != nil {
		return fmt.Errorf("save product category: %w", err)
	}
	return nil
}

func (s *SQLite) SaveMissingProductCategory(ctx context.Context, reference core.ProductReference) error {
	if reference.VendorItemID == "" {
		return errors.New("invalid product category")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO product_categories(
		product_key, source, top_category, second_category, leaf_category,
		leaf_category_id, breadcrumb_json, fetched_at
	) VALUES (?, 'coupang_product_jsonld_breadcrumb_missing_v1', '', '', '', '', '[]', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	ON CONFLICT(product_key) DO UPDATE SET source = excluded.source,
		top_category = '', second_category = '', leaf_category = '', leaf_category_id = '',
		breadcrumb_json = '[]', fetched_at = excluded.fetched_at`,
		"vendor:"+reference.VendorItemID)
	if err != nil {
		return fmt.Errorf("save missing product category: %w", err)
	}
	return nil
}

func (s *SQLite) SaveUnavailableProductCategory(ctx context.Context, reference core.ProductReference) error {
	if reference.VendorItemID == "" {
		return errors.New("invalid product category")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO product_categories(
		product_key, source, top_category, second_category, leaf_category,
		leaf_category_id, breadcrumb_json, fetched_at
	) VALUES (?, ?, '', '', '', '', '[]', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	ON CONFLICT(product_key) DO UPDATE SET source = excluded.source,
		top_category = '', second_category = '', leaf_category = '', leaf_category_id = '',
		breadcrumb_json = '[]', fetched_at = excluded.fetched_at`,
		"vendor:"+reference.VendorItemID, core.CategorySourceProductUnavailable)
	if err != nil {
		return fmt.Errorf("save unavailable product category: %w", err)
	}
	return nil
}

func (s *SQLite) CategoryBreakdown(ctx context.Context, filter core.OrderFilter) (core.CategoryBreakdown, error) {
	filter, err := normalizeFilterForAggregate(filter)
	if err != nil {
		return core.CategoryBreakdown{}, err
	}
	rows, err := s.db.QueryContext(ctx, `WITH retained AS (
		SELECT CASE WHEN COALESCE(i.vendor_item_id, '') != '' THEN 'vendor:' || i.vendor_item_id ELSE '' END AS product_key,
			MAX(i.quantity - i.cancelled_quantity - i.returned_quantity, 0) AS retained_units
		FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
		WHERE o.fully_canceled = 0
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
			AND (? = '' OR o.purchased_at >= ?) AND (? = '' OR o.purchased_at <= ?)
	), joined AS (
		SELECT retained.retained_units,
			COALESCE(categories.leaf_category_id, '') AS category_id,
			COALESCE(categories.leaf_category, '') AS category
		FROM retained LEFT JOIN product_categories categories ON categories.product_key = retained.product_key
		WHERE retained.retained_units > 0
	)
	SELECT category_id, category, COUNT(*), COALESCE(SUM(retained_units), 0)
	FROM joined GROUP BY category_id, category ORDER BY SUM(retained_units) DESC, category`,
		filter.From, filter.From, filter.To, filter.To)
	if err != nil {
		return core.CategoryBreakdown{}, fmt.Errorf("summarize product categories: %w", err)
	}
	defer rows.Close()
	result := core.CategoryBreakdown{Method: core.CategorySourceProductJSONLDBreadcrumb, Grouping: "breadcrumb_leaf"}
	for rows.Next() {
		var categoryID, key string
		var lines, units int
		if err := rows.Scan(&categoryID, &key, &lines, &units); err != nil {
			return core.CategoryBreakdown{}, fmt.Errorf("scan product category summary: %w", err)
		}
		result.TotalItemLines += lines
		result.RetainedUnits += units
		if key == "" {
			continue
		}
		result.ClassifiedItemLines += lines
		result.Buckets = append(result.Buckets, core.CategoryBucket{CategoryID: categoryID, Key: key, ItemLineCount: lines, UnitCount: units})
	}
	if err := rows.Err(); err != nil {
		return core.CategoryBreakdown{}, fmt.Errorf("iterate product category summary: %w", err)
	}
	result.ClassifiedItemLineRate = ratio(result.ClassifiedItemLines, result.TotalItemLines)
	for index := range result.Buckets {
		result.Buckets[index].UnitShare = ratio(result.Buckets[index].UnitCount, result.RetainedUnits)
	}
	return result, nil
}
