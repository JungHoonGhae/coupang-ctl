package browser

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const OrdinaryBridgeSchemaVersion = 1
const ordinaryBridgeMaxFrameBytes = 256 << 10
const ordinaryBridgeMaxOrders = 5
const ordinaryBridgeMaxItemsPerOrder = 100
const ordinaryBridgeRequestIDBytes = 24

var ordinaryBridgeSourceReferencePattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type OrdinaryBridgeOperation string
type OrdinaryBridgeStatus string

const (
	OrdinaryBridgeReadOrders OrdinaryBridgeOperation = "read_order_document"
)

const (
	OrdinaryBridgeOK                     OrdinaryBridgeStatus = "ok"
	OrdinaryBridgeAccessDenied           OrdinaryBridgeStatus = "access_denied"
	OrdinaryBridgeAuthenticationRequired OrdinaryBridgeStatus = "authentication_required"
	OrdinaryBridgeStructuredDataMissing  OrdinaryBridgeStatus = "structured_data_missing"
	OrdinaryBridgeUnavailable            OrdinaryBridgeStatus = "ordinary_browser_unavailable"
)

var ErrOrdinaryBrowserUnavailable = errors.New("ordinary browser unavailable")
var ErrOrdinaryBridgeProtocol = errors.New("invalid ordinary browser bridge response")

type OrdinaryBridgeRequest struct {
	SchemaVersion int                     `json:"schema_version"`
	RequestID     string                  `json:"request_id"`
	Operation     OrdinaryBridgeOperation `json:"operation"`
	Cursor        *core.OrderCursor       `json:"cursor,omitempty"`
}

func (r OrdinaryBridgeRequest) Validate() error {
	if r.SchemaVersion != OrdinaryBridgeSchemaVersion {
		return errors.New("unsupported ordinary browser bridge request schema")
	}
	if !validOrdinaryBridgeRequestID(r.RequestID) {
		return errors.New("invalid ordinary browser bridge request id")
	}
	if r.Operation != OrdinaryBridgeReadOrders {
		return errors.New("unsupported ordinary browser bridge operation")
	}
	if r.Cursor != nil && (r.Cursor.Year < 2000 || r.Cursor.Year > 2100 || r.Cursor.Page < 0 || r.Cursor.Page > 1000) {
		return errors.New("invalid ordinary browser bridge order cursor")
	}
	return nil
}

type OrdinaryBridgeResponse struct {
	SchemaVersion int                  `json:"schema_version"`
	RequestID     string               `json:"request_id"`
	Status        OrdinaryBridgeStatus `json:"status"`
	Page          *core.OrderPage      `json:"page,omitempty"`
}

func (r OrdinaryBridgeResponse) Validate(requestID string) error {
	if r.SchemaVersion != OrdinaryBridgeSchemaVersion || !validOrdinaryBridgeRequestID(requestID) || r.RequestID != requestID {
		return ErrOrdinaryBridgeProtocol
	}
	if r.Status == OrdinaryBridgeOK {
		if r.Page == nil || validateOrdinaryBridgePage(*r.Page) != nil {
			return ErrOrdinaryBridgeProtocol
		}
		encoded, err := json.Marshal(r)
		if err != nil || len(encoded) > ordinaryBridgeMaxFrameBytes {
			return ErrOrdinaryBridgeProtocol
		}
		return nil
	}
	if r.Page != nil {
		return ErrOrdinaryBridgeProtocol
	}
	switch r.Status {
	case OrdinaryBridgeAccessDenied, OrdinaryBridgeAuthenticationRequired, OrdinaryBridgeStructuredDataMissing, OrdinaryBridgeUnavailable:
		return nil
	default:
		return ErrOrdinaryBridgeProtocol
	}
}

func decodeOrdinaryBridgeResponse(response OrdinaryBridgeResponse, requestID string) (*core.OrderPage, error) {
	if err := response.Validate(requestID); err != nil {
		return nil, err
	}
	switch response.Status {
	case OrdinaryBridgeOK:
		page := *response.Page
		page.Orders = append([]core.Order(nil), response.Page.Orders...)
		return &page, nil
	case OrdinaryBridgeAccessDenied:
		return nil, ErrBrowserAccessDenied
	case OrdinaryBridgeAuthenticationRequired:
		return nil, ErrAuthenticationRequired
	case OrdinaryBridgeStructuredDataMissing:
		return nil, ErrStructuredOrderDataMissing
	case OrdinaryBridgeUnavailable:
		return nil, ErrOrdinaryBrowserUnavailable
	default:
		return nil, fmt.Errorf("%w: unsupported status", ErrOrdinaryBridgeProtocol)
	}
}

func decodeOrdinaryBridgeRequestFrame(payload []byte) (OrdinaryBridgeRequest, error) {
	var request OrdinaryBridgeRequest
	if err := decodeOrdinaryBridgeJSON(payload, &request); err != nil || request.Validate() != nil {
		return OrdinaryBridgeRequest{}, ErrOrdinaryBridgeProtocol
	}
	return request, nil
}

func decodeOrdinaryBridgeResponseFrame(payload []byte, requestID string) (OrdinaryBridgeResponse, error) {
	var response OrdinaryBridgeResponse
	if err := decodeOrdinaryBridgeJSON(payload, &response); err != nil || response.Validate(requestID) != nil {
		return OrdinaryBridgeResponse{}, ErrOrdinaryBridgeProtocol
	}
	return response, nil
}

func decodeOrdinaryBridgeJSON(payload []byte, destination any) error {
	if len(payload) == 0 || len(payload) > ordinaryBridgeMaxFrameBytes {
		return ErrOrdinaryBridgeProtocol
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return ErrOrdinaryBridgeProtocol
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrOrdinaryBridgeProtocol
	}
	return nil
}

func newOrdinaryBridgeRequestID(reader io.Reader) (string, error) {
	entropy := make([]byte, ordinaryBridgeRequestIDBytes)
	if _, err := io.ReadFull(reader, entropy); err != nil {
		return "", errors.New("generate ordinary browser request id")
	}
	return base64.RawURLEncoding.EncodeToString(entropy), nil
}

func validateOrdinaryBridgePage(page core.OrderPage) error {
	if len(page.Orders) > ordinaryBridgeMaxOrders {
		return ErrOrdinaryBridgeProtocol
	}
	if page.Next != nil && (page.Next.Year < 2000 || page.Next.Year > 2100 || page.Next.Page < 0 || page.Next.Page > 1000) {
		return ErrOrdinaryBridgeProtocol
	}
	for _, order := range page.Orders {
		if !ordinaryBridgeSourceReferencePattern.MatchString(order.SourceRef) {
			return ErrOrdinaryBridgeProtocol
		}
		purchaseDate, err := time.Parse(time.DateOnly, order.PurchasedAt)
		if err != nil || purchaseDate.Year() < 2000 || purchaseDate.Year() > 2100 {
			return ErrOrdinaryBridgeProtocol
		}
		if order.PurchasedAtTime != nil && (order.PurchasedAtTime.Year() < 2000 || order.PurchasedAtTime.Year() > 2100 || order.PurchasedAtTime.In(time.FixedZone("KST", 9*60*60)).Format(time.DateOnly) != order.PurchasedAt) {
			return ErrOrdinaryBridgeProtocol
		}
		if order.TotalAmount < 0 || order.DiscountAmount < 0 || order.ShippingFee < 0 || order.Currency != "KRW" || len(order.Items) > ordinaryBridgeMaxItemsPerOrder {
			return ErrOrdinaryBridgeProtocol
		}
		for _, item := range order.Items {
			if item.Quantity < 1 || item.CancelledQuantity < 0 || item.ReturnedQuantity < 0 || item.CancelledQuantity > item.Quantity || item.ReturnedQuantity > item.Quantity || item.UnitPrice < 0 || item.PaidPrice < 0 {
				return ErrOrdinaryBridgeProtocol
			}
			if !validOrdinaryBridgeNumericID(item.ProductID) || !validOrdinaryBridgeNumericID(item.VendorItemID) || item.Name == "" || !validOrdinaryBridgeText(item.Name, 2000) || !validOrdinaryBridgeText(item.SellerName, 1000) || !validOrdinaryBridgeText(item.BrandName, 1000) || !validOrdinaryBridgeText(item.ProductType, 200) || !validOrdinaryBridgeText(item.DivisionType, 200) || !validOrdinaryBridgeDeliveryStatus(item.DeliveryStatus) {
				return ErrOrdinaryBridgeProtocol
			}
			if item.DeliveredAt != nil && (item.DeliveredAt.Year() < 2000 || item.DeliveredAt.Year() > 2100) {
				return ErrOrdinaryBridgeProtocol
			}
			switch item.CommerceKind {
			case core.CommerceKindProductPurchase, core.CommerceKindMembershipFee:
			default:
				return ErrOrdinaryBridgeProtocol
			}
		}
	}
	return nil
}

func validOrdinaryBridgeDeliveryStatus(value string) bool {
	switch value {
	case "", "delivered", "in_transit", "cancelled", "returned", "other":
		return true
	default:
		return false
	}
}

func validOrdinaryBridgeNumericID(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 24 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validOrdinaryBridgeText(value string, maxBytes int) bool {
	return len(value) <= maxBytes && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func validOrdinaryBridgeRequestID(value string) bool {
	if len(value) < 32 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}
