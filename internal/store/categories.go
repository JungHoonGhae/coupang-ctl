package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func (s *SQLite) PendingCategoryProducts(ctx context.Context, limit int) ([]core.ProductReference, error) {
	return s.CategoryProductsForEnrichment(ctx, limit, false)
}

func (s *SQLite) CategoryProductsForEnrichment(ctx context.Context, limit int, recheck bool) ([]core.ProductReference, error) {
	if limit < 1 || limit > 1000 {
		return nil, errors.New("category product limit must be between 1 and 1000")
	}
	query := `WITH products AS (
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
	LIMIT ?`
	if recheck {
		query = `WITH products AS (
			SELECT 'vendor:' || vendor_item_id AS product_key,
				MAX(product_id) AS product_id, vendor_item_id
			FROM order_items
			WHERE COALESCE(product_id, '') != '' AND COALESCE(vendor_item_id, '') != ''
			GROUP BY vendor_item_id
		)
		SELECT products.product_id, products.vendor_item_id
		FROM products LEFT JOIN product_categories cached ON cached.product_key = products.product_key
		ORDER BY CASE WHEN cached.product_key IS NULL THEN 0 ELSE 1 END,
			COALESCE(cached.fetched_at, ''), products.vendor_item_id
		LIMIT ?`
	}
	args := []any{core.CategorySourceProductJSONLDBreadcrumb, limit}
	if recheck {
		args = []any{limit}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *SQLite) CategoryRecheckCandidateCount(ctx context.Context) (int, error) {
	var result int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT vendor_item_id) FROM order_items
		WHERE COALESCE(product_id, '') != '' AND COALESCE(vendor_item_id, '') != ''`).Scan(&result)
	if err != nil {
		return 0, fmt.Errorf("count category recheck products: %w", err)
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
	return s.saveProductCategoryOutcome(ctx, "vendor:"+reference.VendorItemID, category.Source,
		path[0].Name, second, leaf.Name, leaf.ID, string(encodedPath))
}

func (s *SQLite) SaveMissingProductCategory(ctx context.Context, reference core.ProductReference) error {
	if reference.VendorItemID == "" {
		return errors.New("invalid product category")
	}
	return s.saveProductCategoryOutcome(ctx, "vendor:"+reference.VendorItemID,
		core.CategorySourceProductJSONLDBreadcrumbMissing, "", "", "", "", "[]")
}

func (s *SQLite) SaveUnavailableProductCategory(ctx context.Context, reference core.ProductReference) error {
	if reference.VendorItemID == "" {
		return errors.New("invalid product category")
	}
	return s.saveProductCategoryOutcome(ctx, "vendor:"+reference.VendorItemID,
		core.CategorySourceProductUnavailable, "", "", "", "", "[]")
}

func (s *SQLite) saveProductCategoryOutcome(ctx context.Context, productKey, source, top, second, leaf, leafID, breadcrumbJSON string) error {
	observedAt := s.now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin product category save: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO product_categories(
		product_key, source, top_category, second_category, leaf_category,
		leaf_category_id, breadcrumb_json, fetched_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(product_key) DO UPDATE SET source = excluded.source,
		top_category = excluded.top_category, second_category = excluded.second_category,
		leaf_category = excluded.leaf_category, leaf_category_id = excluded.leaf_category_id,
		breadcrumb_json = excluded.breadcrumb_json, fetched_at = excluded.fetched_at`,
		productKey, source, top, second, leaf, leafID, breadcrumbJSON, observedAt); err != nil {
		return fmt.Errorf("save product category: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO product_category_observations(
		product_key, source, breadcrumb_json, observed_at
	) VALUES (?, ?, ?, ?)`, productKey, source, breadcrumbJSON, observedAt); err != nil {
		return fmt.Errorf("record product category observation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit product category save: %w", err)
	}
	return nil
}

func (s *SQLite) CategoryStability(ctx context.Context) (core.CategoryStabilityReport, error) {
	result := core.CategoryStabilityReport{
		SchemaVersion: core.CategoryStabilitySchemaVersion,
		Visibility:    "private_local",
		Source:        core.CategorySourceProductJSONLDBreadcrumb,
		Assessment:    "unavailable_no_observed_breadcrumbs",
		Definitions: core.CategoryStabilityDefinition{
			ProductUnit:              "distinct vendor_item_id values from locally synchronized order products",
			RecheckedProduct:         "a product with at least two valid source-native breadcrumb observations",
			MultiDayRecheckedProduct: "a rechecked product observed on at least two distinct UTC calendar dates",
			StableProduct:            "a rechecked product with one distinct observed breadcrumb path",
			ChangedProduct:           "a rechecked product with more than one distinct observed breadcrumb path",
			ObservationDay:           "distinct UTC calendar date among valid breadcrumb observations",
			Assessment:               "changes_observed; otherwise stable only when at least one exact product has multi-day evidence; insufficient states remain explicit",
		},
		Limitations: []string{
			"observations begin when coupangctl adopts or rechecks a category; no retroactive category history is claimed",
			"missing and unavailable outcomes are retained but excluded from breadcrumb-path stability counts",
			"one private local ledger cannot establish cross-account or population-wide category stability",
		},
		Provenance: core.CategoryStabilityProvenance{PathAndTimestamp: "observed", Counts: "derived", Assessment: "derived"},
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT vendor_item_id) FROM order_items
		WHERE COALESCE(product_id, '') != '' AND COALESCE(vendor_item_id, '') != ''`).Scan(&result.EligibleProductCount); err != nil {
		return core.CategoryStabilityReport{}, fmt.Errorf("count category stability products: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `WITH products AS (
		SELECT DISTINCT 'vendor:' || vendor_item_id AS product_key FROM order_items
		WHERE COALESCE(product_id, '') != '' AND COALESCE(vendor_item_id, '') != ''
	)
	SELECT observations.product_key, observations.breadcrumb_json, observations.observed_at
	FROM product_category_observations observations JOIN products USING(product_key)
	WHERE observations.source = ? AND observations.breadcrumb_json != '[]'
	ORDER BY observations.product_key, observations.observed_at`, core.CategorySourceProductJSONLDBreadcrumb)
	if err != nil {
		return core.CategoryStabilityReport{}, fmt.Errorf("load category stability observations: %w", err)
	}
	defer rows.Close()
	type productEvidence struct {
		observations int
		paths        map[string]struct{}
		days         map[string]struct{}
	}
	products := map[string]*productEvidence{}
	days := map[string]struct{}{}
	var first, last time.Time
	for rows.Next() {
		var productKey, encodedPath, observedAt string
		if err := rows.Scan(&productKey, &encodedPath, &observedAt); err != nil {
			return core.CategoryStabilityReport{}, fmt.Errorf("scan category stability observation: %w", err)
		}
		var path []core.ProductCategoryNode
		observed, parseErr := time.Parse(time.RFC3339Nano, observedAt)
		if json.Unmarshal([]byte(encodedPath), &path) != nil || !validCatalogPath(path) || parseErr != nil {
			continue
		}
		evidence := products[productKey]
		if evidence == nil {
			evidence = &productEvidence{paths: map[string]struct{}{}, days: map[string]struct{}{}}
			products[productKey] = evidence
		}
		evidence.observations++
		evidence.paths[encodedPath] = struct{}{}
		evidence.days[observed.UTC().Format("2006-01-02")] = struct{}{}
		result.ObservationCount++
		days[observed.UTC().Format("2006-01-02")] = struct{}{}
		if first.IsZero() || observed.Before(first) {
			first = observed
		}
		if last.IsZero() || observed.After(last) {
			last = observed
		}
	}
	if err := rows.Err(); err != nil {
		return core.CategoryStabilityReport{}, fmt.Errorf("iterate category stability observations: %w", err)
	}
	result.ObservedProductCount = len(products)
	result.ObservedProductRate = ratio(result.ObservedProductCount, result.EligibleProductCount)
	result.DistinctObservationDayCount = len(days)
	if !first.IsZero() {
		result.FirstObservedAt = first.UTC().Format(time.RFC3339Nano)
		result.LastObservedAt = last.UTC().Format(time.RFC3339Nano)
	}
	for _, evidence := range products {
		if evidence.observations < 2 {
			continue
		}
		result.RecheckedProductCount++
		if len(evidence.days) > 1 {
			result.MultiDayRecheckedProductCount++
		}
		if len(evidence.paths) > 1 {
			result.ChangedProductCount++
		} else {
			result.StableProductCount++
		}
	}
	switch {
	case result.ObservedProductCount == 0:
		result.Assessment = "unavailable_no_observed_breadcrumbs"
	case result.ChangedProductCount > 0:
		result.Assessment = "changes_observed"
	case result.RecheckedProductCount == 0:
		result.Assessment = "insufficient_rechecks"
	case result.MultiDayRecheckedProductCount == 0:
		result.Assessment = "insufficient_distinct_days"
	default:
		result.Assessment = "stable_within_local_observation_window"
	}
	return result, nil
}

func (s *SQLite) CategoryCatalog(ctx context.Context, request core.CategoryCatalogRequest) (core.CategoryCatalog, error) {
	if err := request.Validate(); err != nil {
		return core.CategoryCatalog{}, err
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Limit == 0 {
		request.Limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `WITH products AS (
		SELECT DISTINCT vendor_item_id
		FROM order_items
		WHERE COALESCE(product_id, '') != '' AND COALESCE(vendor_item_id, '') != ''
	)
	SELECT COALESCE(categories.source, ''), COALESCE(categories.breadcrumb_json, '[]')
	FROM products LEFT JOIN product_categories categories
		ON categories.product_key = 'vendor:' || products.vendor_item_id
	ORDER BY products.vendor_item_id`)
	if err != nil {
		return core.CategoryCatalog{}, fmt.Errorf("load category catalog products: %w", err)
	}
	defer rows.Close()

	result := core.CategoryCatalog{
		SchemaVersion: core.CategoryCatalogSchemaVersion,
		Visibility:    "private_local",
		Source:        core.CategorySourceProductJSONLDBreadcrumb,
		Query:         request.Query,
		MatchMethod:   "unfiltered_observed_labels",
		Categories:    []core.CategoryCatalogEntry{},
		Definitions: core.CategoryCatalogDefinition{
			ProductUnit:   "distinct vendor_item_id values from locally synchronized order products",
			MatchRule:     "case-insensitive exact, prefix, then substring matching over the observed category label only",
			CategoryRole:  "always_leaf, sometimes_leaf, or never_leaf according to all local products observed on the exact path prefix",
			SearchHandoff: "pass an observed category_id to products_search; the live search response remains authoritative for current result order and availability",
		},
		Limitations: []string{
			"the catalog includes only source-native breadcrumb categories observed on products in this local order ledger",
			"observed_product_count is local distinct-product coverage, not Coupang sales volume or popularity",
			"upstream category labels and paths may change; a current product search can still return no results",
		},
		Provenance: core.CategoryCatalogProvenance{
			CategoryIDLabelAndPath: "observed",
			ProductCounts:          "derived",
			QueryMatch:             "derived",
		},
	}
	if request.Query != "" {
		result.MatchMethod = "case_insensitive_label_exact_prefix_substring"
	}

	type catalogCandidate struct {
		entry     core.CategoryCatalogEntry
		matchRank int
		key       string
	}
	candidates := map[string]*catalogCandidate{}
	for rows.Next() {
		var source, encodedPath string
		if err := rows.Scan(&source, &encodedPath); err != nil {
			return core.CategoryCatalog{}, fmt.Errorf("scan category catalog product: %w", err)
		}
		result.Coverage.EligibleProductCount++
		if source != core.CategorySourceProductJSONLDBreadcrumb {
			continue
		}
		var path []core.ProductCategoryNode
		if err := json.Unmarshal([]byte(encodedPath), &path); err != nil || !validCatalogPath(path) {
			continue
		}
		result.Coverage.ClassifiedProductCount++
		for index, node := range path {
			matchKind, matchRank := categoryLabelMatch(node.Name, request.Query)
			prefix := append([]core.ProductCategoryNode(nil), path[:index+1]...)
			encodedPrefix, err := json.Marshal(prefix)
			if err != nil {
				return core.CategoryCatalog{}, fmt.Errorf("encode category catalog path: %w", err)
			}
			key := string(encodedPrefix)
			candidate := candidates[key]
			if candidate == nil {
				candidate = &catalogCandidate{
					entry: core.CategoryCatalogEntry{
						CategoryID: node.ID, Name: node.Name, Position: node.Position,
						Depth: index + 1, Path: prefix, MatchKind: matchKind,
					},
					matchRank: matchRank,
					key:       key,
				}
				candidates[key] = candidate
			}
			candidate.entry.ObservedProductCount++
			if index == len(path)-1 {
				candidate.entry.ObservedLeafProductCount++
			} else {
				candidate.entry.ObservedAncestorProductCount++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return core.CategoryCatalog{}, fmt.Errorf("iterate category catalog products: %w", err)
	}

	result.Coverage.UnclassifiedProductCount = result.Coverage.EligibleProductCount - result.Coverage.ClassifiedProductCount
	result.Coverage.ClassifiedProductRate = ratio(result.Coverage.ClassifiedProductCount, result.Coverage.EligibleProductCount)
	result.TotalCategoryCount = len(candidates)
	ordered := make([]catalogCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if request.Query != "" && candidate.matchRank == 0 {
			continue
		}
		ordered = append(ordered, *candidate)
	}
	result.MatchedCategoryCount = len(ordered)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].matchRank != ordered[j].matchRank {
			return ordered[i].matchRank > ordered[j].matchRank
		}
		if ordered[i].entry.ObservedProductCount != ordered[j].entry.ObservedProductCount {
			return ordered[i].entry.ObservedProductCount > ordered[j].entry.ObservedProductCount
		}
		if ordered[i].entry.Name != ordered[j].entry.Name {
			return ordered[i].entry.Name < ordered[j].entry.Name
		}
		if ordered[i].entry.CategoryID != ordered[j].entry.CategoryID {
			return ordered[i].entry.CategoryID < ordered[j].entry.CategoryID
		}
		return ordered[i].key < ordered[j].key
	})
	if len(ordered) > request.Limit {
		ordered = ordered[:request.Limit]
		result.Truncated = true
	}
	for _, candidate := range ordered {
		candidate.entry.Role = categoryCatalogRole(candidate.entry)
		result.Categories = append(result.Categories, candidate.entry)
	}
	result.ReturnedCategoryCount = len(result.Categories)
	return result, nil
}

func categoryCatalogRole(entry core.CategoryCatalogEntry) string {
	switch {
	case entry.ObservedLeafProductCount > 0 && entry.ObservedAncestorProductCount > 0:
		return "sometimes_leaf"
	case entry.ObservedLeafProductCount > 0:
		return "always_leaf"
	default:
		return "never_leaf"
	}
}

func validCatalogPath(path []core.ProductCategoryNode) bool {
	if len(path) == 0 || len(path) > 12 {
		return false
	}
	lastPosition := 0
	for index := range path {
		path[index].ID = strings.TrimSpace(path[index].ID)
		path[index].Name = strings.TrimSpace(path[index].Name)
		if !core.NumericProductIdentifier(path[index].ID) || path[index].Name == "" || len([]rune(path[index].Name)) > 100 || path[index].Position <= lastPosition {
			return false
		}
		lastPosition = path[index].Position
	}
	return true
}

func categoryLabelMatch(label, query string) (string, int) {
	if query == "" {
		return "", 1
	}
	label = strings.ToLower(strings.TrimSpace(label))
	query = strings.ToLower(strings.TrimSpace(query))
	switch {
	case label == query:
		return "exact_label", 3
	case strings.HasPrefix(label, query):
		return "prefix_label", 2
	case strings.Contains(label, query):
		return "substring_label", 1
	default:
		return "", 0
	}
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
