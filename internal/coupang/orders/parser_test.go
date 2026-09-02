package orders_test

import (
	"errors"
	"testing"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
	coupangorders "github.com/JungHoonGhae/oss-coupangctl/internal/coupang/orders"
)

func TestParseOrderDocumentNormalizesEmbeddedData(t *testing.T) {
	document := []byte(`<!doctype html><html><body>
<script id="__NEXT_DATA__" type="application/json">{
  "props":{"pageProps":{"domains":{"desktopOrder":{
    "orderList":[{
      "orderId":"synthetic-upstream-order-a",
      "orderDate":"2026-08-29",
      "totalPrice":25900,
      "discountAmount":3100,
      "shippingFee":0,
      "shipments":[{
        "status":"DELIVERED",
        "deliveredAt":"2026-08-30T05:30:00Z",
        "orderItems":[{
          "productId":101,
          "vendorItemId":202,
          "productName":"Synthetic refill pack",
          "quantity":2,
          "unitPrice":14500,
          "paidPrice":25900,
          "sellerName":"Synthetic seller"
        }]
      }]
    }],
    "orderPagination":{"hasNext":true,"nextYear":2026,"nextPageIndex":2}
  }}}}
}</script></body></html>`)

	page, err := coupangorders.ParseOrderDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 1 {
		t.Fatalf("orders = %d, want 1", len(page.Orders))
	}
	order := page.Orders[0]
	if order.SourceRef == "" || order.SourceRef == "synthetic-upstream-order-a" {
		t.Fatalf("source reference was not irreversibly normalized: %q", order.SourceRef)
	}
	if order.PurchasedAt != "2026-08-29" || order.TotalAmount != 25900 || order.DiscountAmount != 3100 {
		t.Fatalf("unexpected order: %#v", order)
	}
	if len(order.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(order.Items))
	}
	item := order.Items[0]
	if item.ProductID != "101" || item.VendorItemID != "202" || item.Name != "Synthetic refill pack" {
		t.Fatalf("unexpected item: %#v", item)
	}
	if item.Quantity != 2 || item.UnitPrice != 14500 || item.PaidPrice != 25900 {
		t.Fatalf("unexpected item amounts: %#v", item)
	}
	if item.DeliveryStatus != "delivered" || item.DeliveredAt == nil || !item.DeliveredAt.Equal(time.Date(2026, 8, 30, 5, 30, 0, 0, time.UTC)) {
		t.Fatalf("unexpected delivery state: %#v", item)
	}
	if page.Next == nil || page.Next.Year != 2026 || page.Next.Page != 2 {
		t.Fatalf("unexpected cursor: %#v", page.Next)
	}
}

func TestParseOrderDocumentAcceptsNextDataJSONDirectly(t *testing.T) {
	document := []byte(`{"props":{"pageProps":{"domains":{"desktopOrder":{"orderList":[],"orderPagination":{"hasNext":false}}}}}}`)
	page, err := coupangorders.ParseOrderDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 0 || page.Next != nil {
		t.Fatalf("unexpected page: %#v", page)
	}
}

func TestParseOrderDocumentAcceptsStructuredOrderModel(t *testing.T) {
	document := []byte(`{
  "orderList":[{
    "orderId":123456789,
    "orderedAt":1787929200000,
    "totalProductPrice":25900.0,
    "orderCurrencyType":"KRW",
    "deliveryGroupList":[{"productList":[{
      "productId":101,
      "vendorItemId":202,
      "productName":"Synthetic refill pack",
      "quantity":1,
      "unitPrice":25900,
      "discountedUnitPrice":25900
    }]}]
  }],
  "hasNext":true,
  "nextYear":2025,
  "nextPageIndex":1
}`)

	page, err := coupangorders.ParseOrderDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 1 || page.Orders[0].TotalAmount != 25900 {
		t.Fatalf("unexpected structured model: %#v", page)
	}
	if page.Next == nil || page.Next.Year != 2025 || page.Next.Page != 1 {
		t.Fatalf("unexpected structured model cursor: %#v", page.Next)
	}
}

func TestParseOrderDocumentRejectsFractionalKRWAmount(t *testing.T) {
	document := []byte(`{
  "orderList":[{
    "orderId":123456789,
    "orderedAt":1787929200000,
    "totalProductPrice":25900.5,
    "deliveryGroupList":[]
  }],
  "hasNext":false
}`)

	_, err := coupangorders.ParseOrderDocument(document)
	if !errors.Is(err, core.ErrInvalidOrderData) {
		t.Fatalf("error = %v, want ErrInvalidOrderData", err)
	}
}

func TestParseOrderDocumentNormalizesCurrentDesktopOrderShape(t *testing.T) {
	document := []byte(`{"props":{"pageProps":{"domains":{"desktopOrder":{
  "orderList":[{
    "orderId":123456789,
    "orderedAt":1787929200000,
    "totalProductPrice":25900,
    "baseDeliveryPrice":0,
    "orderCurrencyType":"KRW",
    "paymentReceiptInfo":{"paymentReceiptVisible":true},
    "bundleReceiptList":[{"vendorItems":[{
      "vendorItemId":202,
      "vendorItemName":"Synthetic receipt copy",
      "quantity":2,
      "unitPrice":12950
    }]}],
    "deliveryGroupList":[{
      "deliveredDate":1788015600000,
      "groupStatus":{"status":"DELIVERED"},
      "vendor":{"vendorName":"Synthetic seller"},
      "productList":[{
        "productId":101,
        "vendorItemId":202,
        "productName":"Synthetic refill pack",
        "vendorItemName":"Synthetic refill pack option",
        "brandInfo":{"brandName":"Synthetic brand"},
        "productType":"SYNTHETIC_STANDARD",
        "divisionType":"SYNTHETIC_RETAIL",
        "quantity":2,
        "unitPrice":14500,
        "discountedUnitPrice":12950
      }]
    }]
  }],
  "orderPagination":{"hasNext":false}
}}}}}`)

	page, err := coupangorders.ParseOrderDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 1 {
		t.Fatalf("orders = %d, want 1", len(page.Orders))
	}
	order := page.Orders[0]
	if order.SourceRef == "" || order.SourceRef == "123456789" {
		t.Fatalf("source reference was not normalized")
	}
	if order.PurchasedAt != "2026-08-29" || order.TotalAmount != 25900 || order.Currency != "KRW" || !order.ReceiptAvailable {
		t.Fatalf("current order shape was not normalized: %#v", order)
	}
	wantPurchasedAt := time.UnixMilli(1787929200000).UTC()
	if order.PurchasedAtTime == nil || !order.PurchasedAtTime.Equal(wantPurchasedAt) {
		t.Fatalf("purchased_at_time = %v, want %v", order.PurchasedAtTime, wantPurchasedAt)
	}
	if len(order.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(order.Items))
	}
	item := order.Items[0]
	if item.PaidPrice != 25900 || item.UnitPrice != 14500 || item.DeliveryStatus != "delivered" || item.BrandName != "Synthetic brand" || item.ProductType != "SYNTHETIC_STANDARD" || item.DivisionType != "SYNTHETIC_RETAIL" || item.CommerceKind != core.CommerceKindProductPurchase {
		t.Fatalf("current item shape was not normalized: %#v", item)
	}
	wantDelivered := time.UnixMilli(1788015600000).UTC()
	if item.DeliveredAt == nil || !item.DeliveredAt.Equal(wantDelivered) {
		t.Fatalf("delivered_at = %v, want %v", item.DeliveredAt, wantDelivered)
	}
}

func TestParseOrderDocumentNormalizesCancellationAndReturnQuantities(t *testing.T) {
	document := []byte(`{
  "orderList":[{
    "orderId":123456789,
    "orderedAt":1787929200000,
    "totalProductPrice":30000,
    "allCanceled":true,
    "deliveryGroupList":[{
      "groupStatus":{"status":"CANCELLED"},
      "productList":[{
        "productId":101,
        "vendorItemId":202,
        "productName":"Synthetic cancelled item",
        "quantity":2,
        "cancelQuantity":2,
        "returnReceiptQuantity":1,
        "unitPrice":10000,
        "discountedUnitPrice":10000
      }]
    }]
  }],
  "hasNext":false
}`)

	page, err := coupangorders.ParseOrderDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 1 || !page.Orders[0].FullyCanceled || len(page.Orders[0].Items) != 1 {
		t.Fatalf("unexpected cancelled order: %#v", page)
	}
	item := page.Orders[0].Items[0]
	if item.CancelledQuantity != 2 || item.ReturnedQuantity != 1 {
		t.Fatalf("unexpected adjusted quantities: %#v", item)
	}
}

func TestParseOrderDocumentRejectsMissingOrderDomain(t *testing.T) {
	_, err := coupangorders.ParseOrderDocument([]byte(`<html><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{}}}</script></html>`))
	if !errors.Is(err, coupangorders.ErrOrderDataMissing) {
		t.Fatalf("error = %v, want ErrOrderDataMissing", err)
	}
}

func TestParseOrderDocumentRejectsInvalidOrderWithoutLeakingPayload(t *testing.T) {
	privateMarker := "synthetic-private-marker"
	document := []byte(`{"props":{"pageProps":{"domains":{"desktopOrder":{"orderList":[{"orderId":"` + privateMarker + `","orderDate":"not-a-date","totalPrice":10}]}}}}}`)
	_, err := coupangorders.ParseOrderDocument(document)
	if !errors.Is(err, core.ErrInvalidOrderData) {
		t.Fatalf("error = %v, want ErrInvalidOrderData", err)
	}
	if err != nil && contains(err.Error(), privateMarker) {
		t.Fatalf("error leaked source payload: %v", err)
	}
}

func contains(value, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}
