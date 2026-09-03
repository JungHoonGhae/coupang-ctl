package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestOrdinaryBridgeRequestAllowsOnlyBoundedOrderCursor(t *testing.T) {
	requestID := strings.Repeat("a", 32)
	for _, request := range []OrdinaryBridgeRequest{
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Operation: OrdinaryBridgeReadOrders},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Operation: OrdinaryBridgeReadOrders, Cursor: &core.OrderCursor{Year: 2026, Page: 7}},
	} {
		if err := request.Validate(); err != nil {
			t.Fatalf("valid request rejected: %v", err)
		}
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "http") || strings.Contains(string(encoded), "cookie") {
			t.Fatalf("request exposed a generic navigation or credential surface: %s", encoded)
		}
	}

	invalid := []OrdinaryBridgeRequest{
		{},
		{SchemaVersion: 2, RequestID: requestID, Operation: OrdinaryBridgeReadOrders},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: "short", Operation: OrdinaryBridgeReadOrders},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Operation: "fetch_url"},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Operation: OrdinaryBridgeReadOrders, Cursor: &core.OrderCursor{Year: 1999, Page: 0}},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Operation: OrdinaryBridgeReadOrders, Cursor: &core.OrderCursor{Year: 2026, Page: 1001}},
	}
	for index, request := range invalid {
		if err := request.Validate(); err == nil {
			t.Fatalf("invalid request %d accepted: %#v", index, request)
		}
	}
}

func TestOrdinaryBridgeDecodersRejectUnknownAndTrailingFields(t *testing.T) {
	requestID := strings.Repeat("f", 32)
	validRequest := []byte(`{"schema_version":1,"request_id":"` + requestID + `","operation":"read_order_document"}`)
	if _, err := decodeOrdinaryBridgeRequestFrame(validRequest); err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{
		[]byte(`{"schema_version":1,"request_id":"` + requestID + `","operation":"read_order_document","url":"https://example.test"}`),
		append(append([]byte(nil), validRequest...), []byte(` {}`)...),
	} {
		if _, err := decodeOrdinaryBridgeRequestFrame(payload); !errors.Is(err, ErrOrdinaryBridgeProtocol) {
			t.Fatalf("request decode error = %v, want ErrOrdinaryBridgeProtocol", err)
		}
	}

	validResponse := []byte(`{"schema_version":1,"request_id":"` + requestID + `","status":"ok","page":{"orders":[]}}`)
	if _, err := decodeOrdinaryBridgeResponseFrame(validResponse, requestID); err != nil {
		t.Fatal(err)
	}
	unknownResponse := []byte(`{"schema_version":1,"request_id":"` + requestID + `","status":"ok","page":{"orders":[]},"raw":"private"}`)
	if _, err := decodeOrdinaryBridgeResponseFrame(unknownResponse, requestID); !errors.Is(err, ErrOrdinaryBridgeProtocol) {
		t.Fatalf("response decode error = %v, want ErrOrdinaryBridgeProtocol", err)
	}
}

func TestOrdinaryBridgeRequestIDUsesFixedEntropy(t *testing.T) {
	requestID, err := newOrdinaryBridgeRequestID(bytes.NewReader(bytes.Repeat([]byte{0xab}, ordinaryBridgeRequestIDBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if len(requestID) != 32 || !validOrdinaryBridgeRequestID(requestID) {
		t.Fatalf("request id shape = %q", requestID)
	}
	if _, err := newOrdinaryBridgeRequestID(io.LimitReader(bytes.NewReader([]byte{1}), 1)); err == nil {
		t.Fatal("short entropy source accepted")
	}
}

func TestOrdinaryBridgeResponseEnforcesDocumentAndErrorShapes(t *testing.T) {
	requestID := strings.Repeat("b", 32)
	valid := []OrdinaryBridgeResponse{
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: OrdinaryBridgeOK, Page: &core.OrderPage{Orders: []core.Order{}}},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: OrdinaryBridgeAccessDenied},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: OrdinaryBridgeAuthenticationRequired},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: OrdinaryBridgeStructuredDataMissing},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: OrdinaryBridgeUnavailable},
	}
	for _, response := range valid {
		if err := response.Validate(requestID); err != nil {
			t.Fatalf("valid response rejected: %v", err)
		}
	}

	invalid := []OrdinaryBridgeResponse{
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: strings.Repeat("c", 32), Status: OrdinaryBridgeOK, Page: &core.OrderPage{Orders: []core.Order{}}},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: OrdinaryBridgeOK},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: OrdinaryBridgeOK, Page: &core.OrderPage{Orders: []core.Order{{SourceRef: "raw-source-id"}}}},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: OrdinaryBridgeAccessDenied, Page: &core.OrderPage{Orders: []core.Order{}}},
		{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: "upstream said private text"},
	}
	for index, response := range invalid {
		if err := response.Validate(requestID); err == nil {
			t.Fatalf("invalid response %d accepted: %#v", index, response)
		}
	}
}

func TestOrdinaryBridgeResponseMapsOnlyTypedErrors(t *testing.T) {
	requestID := strings.Repeat("d", 32)
	for status, want := range map[OrdinaryBridgeStatus]error{
		OrdinaryBridgeAccessDenied:           ErrBrowserAccessDenied,
		OrdinaryBridgeAuthenticationRequired: ErrAuthenticationRequired,
		OrdinaryBridgeStructuredDataMissing:  ErrStructuredOrderDataMissing,
		OrdinaryBridgeUnavailable:            ErrOrdinaryBrowserUnavailable,
	} {
		_, err := decodeOrdinaryBridgeResponse(OrdinaryBridgeResponse{SchemaVersion: OrdinaryBridgeSchemaVersion, RequestID: requestID, Status: status}, requestID)
		if !errors.Is(err, want) {
			t.Fatalf("status %q mapped to %v, want %v", status, err, want)
		}
	}

	page, err := decodeOrdinaryBridgeResponse(OrdinaryBridgeResponse{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		RequestID:     requestID,
		Status:        OrdinaryBridgeOK,
		Page:          &core.OrderPage{Orders: []core.Order{}},
	}, requestID)
	if err != nil || page == nil || len(page.Orders) != 0 {
		t.Fatalf("successful response = %#v, %v", page, err)
	}
}

func TestOrdinaryBridgeResponseAcceptsBoundedNormalizedOrderPage(t *testing.T) {
	requestID := strings.Repeat("e", 32)
	response := OrdinaryBridgeResponse{
		SchemaVersion: OrdinaryBridgeSchemaVersion,
		RequestID:     requestID,
		Status:        OrdinaryBridgeOK,
		Page: &core.OrderPage{
			Orders: []core.Order{{
				SourceRef:   core.OrderSourceReference("synthetic-order"),
				PurchasedAt: "2026-09-03",
				TotalAmount: 25900,
				Currency:    "KRW",
				Items: []core.OrderItem{{
					ProductID: "101", VendorItemID: "202", Name: "Synthetic item", Quantity: 1,
					UnitPrice: 25900, PaidPrice: 25900, CommerceKind: core.CommerceKindProductPurchase,
				}},
			}},
			Next: &core.OrderCursor{Year: 2026, Page: 1},
		},
	}
	if err := response.Validate(requestID); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "synthetic-order") {
		t.Fatal("raw order identifier crossed the ordinary-browser protocol")
	}
}
