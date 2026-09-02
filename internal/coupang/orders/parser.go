package orders

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

const maxDocumentBytes = 8 << 20

var ErrOrderDataMissing = errors.New("order data missing")

func ParseOrderDocument(document []byte) (core.OrderPage, error) {
	if len(document) == 0 || len(document) > maxDocumentBytes {
		return core.OrderPage{}, ErrOrderDataMissing
	}
	payload, err := nextDataPayload(document)
	if err != nil {
		return core.OrderPage{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return core.OrderPage{}, fmt.Errorf("%w: malformed structured document", core.ErrInvalidOrderData)
	}
	domain, ok := nestedMap(root, "props", "pageProps", "domains", "desktopOrder")
	if !ok {
		if _, modelOK := root["orderList"]; !modelOK {
			return core.OrderPage{}, ErrOrderDataMissing
		}
		domain = root
	}
	rawOrders, ok := valueSlice(domain["orderList"])
	if !ok {
		return core.OrderPage{}, fmt.Errorf("%w: order list has an unsupported shape", core.ErrInvalidOrderData)
	}

	page := core.OrderPage{Orders: make([]core.Order, 0, len(rawOrders))}
	for _, raw := range rawOrders {
		object, ok := raw.(map[string]any)
		if !ok {
			return core.OrderPage{}, fmt.Errorf("%w: order entry has an unsupported shape", core.ErrInvalidOrderData)
		}
		order, err := normalizeOrder(object)
		if err != nil {
			return core.OrderPage{}, err
		}
		page.Orders = append(page.Orders, order)
	}
	pagination := domain
	if embedded, ok := domain["orderPagination"].(map[string]any); ok {
		pagination = embedded
	}
	if boolValue(pagination, "hasNext") {
		year, yearOK := integerValue(pagination, "nextYear")
		nextPage, pageOK := integerValue(pagination, "nextPageIndex", "nextPage")
		if !yearOK || !pageOK || year < 2000 || nextPage < 0 {
			return core.OrderPage{}, fmt.Errorf("%w: invalid next-page cursor", core.ErrInvalidOrderData)
		}
		page.Next = &core.OrderCursor{Year: year, Page: nextPage}
	}
	return page, nil
}

func nextDataPayload(document []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(document)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed, nil
	}
	remaining := document
	for {
		start := bytes.Index(remaining, []byte("<script"))
		if start < 0 {
			return nil, ErrOrderDataMissing
		}
		remaining = remaining[start:]
		tagEnd := bytes.IndexByte(remaining, '>')
		if tagEnd < 0 {
			return nil, ErrOrderDataMissing
		}
		tag := remaining[:tagEnd+1]
		body := remaining[tagEnd+1:]
		end := bytes.Index(body, []byte("</script>"))
		if end < 0 {
			return nil, ErrOrderDataMissing
		}
		if bytes.Contains(tag, []byte(`id="__NEXT_DATA__"`)) || bytes.Contains(tag, []byte(`id='__NEXT_DATA__'`)) {
			return bytes.TrimSpace(body[:end]), nil
		}
		remaining = body[end+len("</script>"):]
	}
}

func normalizeOrder(raw map[string]any) (core.Order, error) {
	sourceID := scalarString(raw, "orderId", "orderID", "orderNumber")
	date, purchasedAtTime, ok := normalizeOrderMoment(raw, "orderDate", "orderedAt", "purchasedAt")
	if sourceID == "" || !ok {
		return core.Order{}, fmt.Errorf("%w: required order fields are missing", core.ErrInvalidOrderData)
	}
	total, ok := amountValue(raw, "totalPrice", "orderTotalPrice", "totalAmount", "paidAmount", "totalProductPrice")
	if !ok || total < 0 {
		return core.Order{}, fmt.Errorf("%w: invalid order total", core.ErrInvalidOrderData)
	}
	discount, _ := amountValue(raw, "discountAmount", "totalDiscountPrice")
	shipping, _ := amountValue(raw, "shippingFee", "deliveryFee", "baseDeliveryPrice")
	items, err := collectOrderItems(raw)
	if err != nil {
		return core.Order{}, err
	}
	currency := stringValue(raw, "orderCurrencyType", "currency")
	if currency == "" {
		currency = "KRW"
	}
	return core.Order{
		SourceRef:        sourceReference(sourceID),
		PurchasedAt:      date,
		PurchasedAtTime:  purchasedAtTime,
		TotalAmount:      total,
		DiscountAmount:   discount,
		ShippingFee:      shipping,
		Currency:         currency,
		FullyCanceled:    boolValue(raw, "allCanceled"),
		ReceiptAvailable: receiptAvailable(raw),
		Items:            items,
	}, nil
}

func receiptAvailable(raw map[string]any) bool {
	if payment, ok := raw["paymentReceiptInfo"].(map[string]any); ok {
		return boolValue(payment, "paymentReceiptVisible")
	}
	return false
}

type deliveryContext struct {
	status      string
	deliveredAt *time.Time
	sellerName  string
}

func collectOrderItems(raw map[string]any) ([]core.OrderItem, error) {
	var items []core.OrderItem
	for _, key := range []string{"deliveryGroupList", "shipments", "deliveries", "shipmentList", "orderItems", "items"} {
		container, ok := raw[key]
		if !ok || container == nil {
			continue
		}
		if err := walkForItems(container, deliveryContext{}, &items); err != nil {
			return nil, err
		}
		break
	}
	return items, nil
}

func walkForItems(value any, inherited deliveryContext, items *[]core.OrderItem) error {
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			if err := walkForItems(child, inherited, items); err != nil {
				return err
			}
		}
	case map[string]any:
		current := inherited
		if status := stringValue(typed, "deliveryStatus", "shipmentStatus", "status"); status != "" {
			current.status = normalizeDeliveryStatus(status)
		} else if groupStatus, ok := typed["groupStatus"].(map[string]any); ok {
			current.status = normalizeDeliveryStatus(stringValue(groupStatus, "status"))
		}
		if delivered, ok := firstValue(typed, "deliveredAt", "deliveredDate", "deliveryCompletedAt"); ok {
			if parsed, ok := parseTimeValue(delivered); ok {
				current.deliveredAt = &parsed
			}
		}
		if vendor, ok := typed["vendor"].(map[string]any); ok {
			current.sellerName = stringValue(vendor, "vendorName", "sellerName")
		}
		if looksLikeItem(typed) {
			item, err := normalizeItem(typed, current)
			if err != nil {
				return err
			}
			*items = append(*items, item)
			return nil
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := walkForItems(typed[key], current, items); err != nil {
				return err
			}
		}
	}
	return nil
}

func looksLikeItem(raw map[string]any) bool {
	name := stringValue(raw, "productName", "itemName", "vendorItemName")
	quantity, quantityOK := integerValue(raw, "quantity", "orderQuantity")
	return name != "" && quantityOK && quantity > 0
}

func normalizeItem(raw map[string]any, delivery deliveryContext) (core.OrderItem, error) {
	quantity, _ := integerValue(raw, "quantity", "orderQuantity")
	cancelledQuantity, _ := integerValue(raw, "cancelQuantity", "cancelledQuantity")
	returnedQuantity, _ := integerValue(raw, "returnReceiptQuantity", "returnedQuantity")
	unitPrice, _ := amountValue(raw, "unitPrice", "salesPrice", "listPrice")
	paidPrice, paidOK := amountValue(raw, "paidPrice", "discountedPrice", "orderPrice")
	if !paidOK {
		if discountedUnitPrice, ok := amountValue(raw, "discountedUnitPrice", "combinedUnitPrice"); ok {
			paidPrice = discountedUnitPrice * int64(quantity)
			paidOK = true
		}
	}
	if !paidOK {
		paidPrice = unitPrice * int64(quantity)
	}
	if unitPrice < 0 || paidPrice < 0 || cancelledQuantity < 0 || returnedQuantity < 0 || cancelledQuantity > quantity || returnedQuantity > quantity {
		return core.OrderItem{}, fmt.Errorf("%w: invalid item amount", core.ErrInvalidOrderData)
	}
	brandName := ""
	if brand, ok := raw["brandInfo"].(map[string]any); ok {
		brandName = stringValue(brand, "brandName", "officialBrandName")
	}
	item := core.OrderItem{
		ProductID:         scalarString(raw, "productId", "productID"),
		VendorItemID:      scalarString(raw, "vendorItemId", "vendorItemID"),
		Name:              stringValue(raw, "productName", "itemName", "vendorItemName"),
		Quantity:          quantity,
		CancelledQuantity: cancelledQuantity,
		ReturnedQuantity:  returnedQuantity,
		UnitPrice:         unitPrice,
		PaidPrice:         paidPrice,
		SellerName:        firstNonEmpty(stringValue(raw, "sellerName", "vendorName"), delivery.sellerName),
		BrandName:         brandName,
		ProductType:       stringValue(raw, "productType"),
		DivisionType:      stringValue(raw, "divisionType"),
		DeliveryStatus:    delivery.status,
		DeliveredAt:       delivery.deliveredAt,
	}
	item.CommerceKind = core.ClassifyCommerceKind(item)
	return item, nil
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

func valueSlice(value any) ([]any, bool) {
	result, ok := value.([]any)
	return result, ok
}

func firstValue(raw map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := raw[key]; ok && value != nil {
			return value, true
		}
	}
	return nil, false
}

func stringValue(raw map[string]any, keys ...string) string {
	value, ok := firstValue(raw, keys...)
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func scalarString(raw map[string]any, keys ...string) string {
	value, ok := firstValue(raw, keys...)
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func integerValue(raw map[string]any, keys ...string) (int, bool) {
	amount, ok := amountValue(raw, keys...)
	if !ok {
		return 0, false
	}
	return int(amount), true
}

func amountValue(raw map[string]any, keys ...string) (int64, bool) {
	value, ok := firstValue(raw, keys...)
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case json.Number:
		return integralJSONNumber(typed)
	case float64:
		return int64(typed), typed == float64(int64(typed))
	case string:
		cleaned := strings.NewReplacer(",", "", "₩", "", "원", "", " ", "").Replace(typed)
		parsed, err := strconv.ParseInt(cleaned, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func integralJSONNumber(value json.Number) (int64, bool) {
	if parsed, err := value.Int64(); err == nil {
		return parsed, true
	}
	rational, ok := new(big.Rat).SetString(value.String())
	if !ok || !rational.IsInt() || !rational.Num().IsInt64() {
		return 0, false
	}
	return rational.Num().Int64(), true
}

func boolValue(raw map[string]any, key string) bool {
	value, _ := raw[key].(bool)
	return value
}

func normalizeDate(value string) (string, bool) {
	parsed, ok := parseTime(value)
	if !ok {
		return "", false
	}
	return parsed.Format(time.DateOnly), true
}

func normalizeOrderMoment(raw map[string]any, keys ...string) (string, *time.Time, bool) {
	value, ok := firstValue(raw, keys...)
	if !ok {
		return "", nil, false
	}
	if text, ok := value.(string); ok {
		parsed, parsedOK := parseTime(text)
		if !parsedOK {
			return "", nil, false
		}
		date := parsed.In(time.FixedZone("KST", 9*60*60)).Format(time.DateOnly)
		trimmed := strings.TrimSpace(text)
		if len(trimmed) == len(time.DateOnly) || len(trimmed) == len("2006.01.02") || len(trimmed) == len("2006/01/02") {
			return date, nil, true
		}
		return date, &parsed, true
	}
	parsed, ok := parseTimeValue(value)
	if !ok {
		return "", nil, false
	}
	return parsed.In(time.FixedZone("KST", 9*60*60)).Format(time.DateOnly), &parsed, true
}

func parseTimeValue(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case string:
		return parseTime(typed)
	case json.Number:
		epoch, ok := integralJSONNumber(typed)
		if !ok {
			return time.Time{}, false
		}
		return parseEpoch(epoch)
	case float64:
		if typed != float64(int64(typed)) {
			return time.Time{}, false
		}
		return parseEpoch(int64(typed))
	default:
		return time.Time{}, false
	}
}

func parseEpoch(value int64) (time.Time, bool) {
	var parsed time.Time
	if value >= 1_000_000_000_000 {
		parsed = time.UnixMilli(value).UTC()
	} else {
		parsed = time.Unix(value, 0).UTC()
	}
	if parsed.Year() < 2000 || parsed.Year() > 2100 {
		return time.Time{}, false
	}
	return parsed, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parseTime(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, time.DateOnly, "2006.01.02", "2006. 01. 02", "2006/01/02"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func normalizeDeliveryStatus(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(normalized, "deliver") && strings.Contains(normalized, "complete"),
		strings.Contains(normalized, "delivered"), strings.Contains(normalized, "배송완료"):
		return "delivered"
	case strings.Contains(normalized, "shipping"), strings.Contains(normalized, "transit"), strings.Contains(normalized, "배송중"):
		return "in_transit"
	case strings.Contains(normalized, "cancel"), strings.Contains(normalized, "취소"):
		return "cancelled"
	case strings.Contains(normalized, "return"), strings.Contains(normalized, "반품"):
		return "returned"
	case normalized == "":
		return ""
	default:
		return "other"
	}
}

func sourceReference(sourceID string) string {
	digest := sha256.Sum256([]byte("coupangctl:order:" + sourceID))
	return hex.EncodeToString(digest[:])
}
