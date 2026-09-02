// order-shape prints keys, JSON types, and array lengths only. It never emits
// order values, identifiers, cookies, or other customer data.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangorders "github.com/JungHoonGhae/coupang-ctl/internal/coupang/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/platform"
)

func main() {
	ctx := context.Background()
	paths, err := platform.DefaultPaths()
	if err != nil {
		fail()
	}
	source := browser.NewNative(paths.ProfileDir)
	defer source.Close()
	document, err := source.Fetch(ctx, &core.OrderCursor{Year: 2026, Page: 1})
	if err != nil {
		fail()
	}
	var root map[string]any
	if json.Unmarshal(document, &root) != nil {
		fail()
	}
	domain, ok := nestedMap(root, "props", "pageProps", "domains", "desktopOrder")
	if !ok {
		if _, modelOK := root["orderList"]; !modelOK {
			fail()
		}
		domain = root
	}
	shape := map[string]any{
		"root":   describe(root, 2),
		"domain": describe(domain, 2),
	}
	if _, parseErr := coupangorders.ParseOrderDocument(document); parseErr != nil {
		shape["normalization_error"] = parseErr.Error()
	} else {
		shape["normalization"] = "ok"
	}
	if query, ok := root["query"].(map[string]any); ok {
		shape["pagination_query"] = safePaginationValues(query)
	}
	pagination := domain
	if embedded, ok := domain["orderPagination"].(map[string]any); ok {
		pagination = embedded
	}
	if pagination != nil {
		shape["pagination_result"] = safePaginationValues(pagination)
		if value, ok := pagination["hasNext"].(bool); ok {
			shape["pagination_result"].(map[string]any)["hasNext"] = value
		}
	}
	if orders, ok := domain["orderList"].([]any); ok && len(orders) > 0 {
		shape["order_total_summary"] = summarizeOrderTotals(orders)
		shape["per_order_normalization"] = summarizeNormalization(orders)
		shape["order"] = describe(orders[0], 2)
		if order, ok := orders[0].(map[string]any); ok {
			if groups, ok := order["deliveryGroupList"].([]any); ok && len(groups) > 0 {
				if group, ok := groups[0].(map[string]any); ok {
					if products, ok := group["productList"].([]any); ok && len(products) > 0 {
						shape["delivery_product"] = describe(products[0], 6)
					}
				}
			}
			if receipts, ok := order["bundleReceiptList"].([]any); ok && len(receipts) > 0 {
				if receipt, ok := receipts[0].(map[string]any); ok {
					if products, ok := receipt["vendorItems"].([]any); ok && len(products) > 0 {
						shape["receipt_product"] = describe(products[0], 6)
					}
				}
			}
		}
	}
	encoded, err := json.MarshalIndent(shape, "", "  ")
	if err != nil {
		fail()
	}
	fmt.Println(string(encoded))
}

func summarizeNormalization(orders []any) []map[string]any {
	results := make([]map[string]any, 0, len(orders))
	for index, order := range orders {
		document, err := json.Marshal(map[string]any{"orderList": []any{order}, "hasNext": false})
		if err != nil {
			results = append(results, map[string]any{"index": index, "status": "marshal_error"})
			continue
		}
		if _, err := coupangorders.ParseOrderDocument(document); err != nil {
			results = append(results, map[string]any{"index": index, "status": err.Error()})
		} else {
			results = append(results, map[string]any{"index": index, "status": "ok"})
		}
	}
	return results
}

func summarizeOrderTotals(orders []any) map[string]int {
	summary := map[string]int{"orders": len(orders)}
	for _, raw := range orders {
		order, ok := raw.(map[string]any)
		if !ok {
			summary["non_object"]++
			continue
		}
		if canceled, _ := order["allCanceled"].(bool); canceled {
			summary["all_canceled"]++
		}
		total, present := order["totalProductPrice"]
		if !present || total == nil {
			summary["total_missing"]++
			continue
		}
		number, ok := total.(float64)
		if !ok {
			summary["total_non_number"]++
			continue
		}
		switch {
		case number < 0:
			summary["total_negative"]++
		case number == 0:
			summary["total_zero"]++
		default:
			summary["total_positive"]++
		}
	}
	return summary
}

func safePaginationValues(raw map[string]any) map[string]any {
	result := map[string]any{}
	for _, key := range []string{"page", "pageIndex", "year", "periodYear", "nextPageIndex", "nextYear", "prevPageIndex", "prevYear"} {
		if value, ok := raw[key]; ok {
			switch value.(type) {
			case string, float64, json.Number:
				result[key] = value
			}
		}
	}
	return result
}

func nestedMap(root map[string]any, path ...string) (map[string]any, bool) {
	current := root
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func describe(value any, depth int) any {
	if depth <= 0 {
		return jsonType(value)
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make(map[string]any, len(keys))
		for _, key := range keys {
			result[key] = describe(typed[key], depth-1)
		}
		return result
	case []any:
		result := map[string]any{"type": "array", "length": len(typed)}
		if len(typed) > 0 {
			result["item"] = describe(typed[0], depth-1)
		}
		return result
	default:
		return jsonType(value)
	}
}

func jsonType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case float64, json.Number:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

func fail() {
	fmt.Fprintln(os.Stderr, "order shape probe failed")
	os.Exit(1)
}
