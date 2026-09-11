package core

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const OrderDocumentMaxOrders = 5
const OrderDocumentMaxItemsPerOrder = 100

var orderDocumentReferencePattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// OrderPreview is one current source page, never a stored account ledger or a
// full-history result. Its normalized rows remain private to the requesting user.
type OrderPreview struct {
	SchemaVersion           int        `json:"schema_version"`
	Visibility              string     `json:"visibility"`
	Source                  SyncSource `json:"source"`
	Provenance              string     `json:"provenance"`
	CapturedAt              time.Time  `json:"captured_at"`
	Scope                   string     `json:"scope"`
	Orders                  []Order    `json:"orders"`
	OrderCount              int        `json:"order_count"`
	HasNextPage             bool       `json:"has_next_page"`
	HistoryComplete         bool       `json:"history_complete"`
	AccountIdentityVerified bool       `json:"account_identity_verified"`
	Persisted               bool       `json:"persisted"`
	Limitations             []string   `json:"limitations"`
}

// ValidateOrderDocument checks the bounded normalized source-document contract.
// It does not authenticate an account or prove source-wide history coverage.
func ValidateOrderDocument(page OrderPage) error {
	if len(page.Orders) > OrderDocumentMaxOrders {
		return ErrInvalidOrderData
	}
	if page.Next != nil && (page.Next.Year < 2000 || page.Next.Year > 2100 || page.Next.Page < 0 || page.Next.Page > 1000) {
		return ErrInvalidOrderData
	}
	for _, order := range page.Orders {
		if !orderDocumentReferencePattern.MatchString(order.SourceRef) {
			return ErrInvalidOrderData
		}
		purchaseDate, err := time.Parse(time.DateOnly, order.PurchasedAt)
		if err != nil || purchaseDate.Year() < 2000 || purchaseDate.Year() > 2100 {
			return ErrInvalidOrderData
		}
		if order.PurchasedAtTime != nil && (order.PurchasedAtTime.Year() < 2000 || order.PurchasedAtTime.Year() > 2100 || order.PurchasedAtTime.In(time.FixedZone("KST", 9*60*60)).Format(time.DateOnly) != order.PurchasedAt) {
			return ErrInvalidOrderData
		}
		if order.TotalAmount < 0 || order.DiscountAmount < 0 || order.ShippingFee < 0 || order.Currency != "KRW" || len(order.Items) > OrderDocumentMaxItemsPerOrder {
			return ErrInvalidOrderData
		}
		for _, item := range order.Items {
			if item.Quantity < 1 || item.CancelledQuantity < 0 || item.ReturnedQuantity < 0 || item.CancelledQuantity > item.Quantity || item.ReturnedQuantity > item.Quantity || item.UnitPrice < 0 || item.PaidPrice < 0 {
				return ErrInvalidOrderData
			}
			if !validOrderDocumentNumericID(item.ProductID) || !validOrderDocumentNumericID(item.VendorItemID) || item.Name == "" || !validOrderDocumentText(item.Name, 2000) || !validOrderDocumentText(item.SellerName, 1000) || !validOrderDocumentText(item.BrandName, 1000) || !validOrderDocumentText(item.ProductType, 200) || !validOrderDocumentText(item.DivisionType, 200) || !validOrderDocumentDeliveryStatus(item.DeliveryStatus) {
				return ErrInvalidOrderData
			}
			if item.DeliveredAt != nil && (item.DeliveredAt.Year() < 2000 || item.DeliveredAt.Year() > 2100) {
				return ErrInvalidOrderData
			}
			switch item.CommerceKind {
			case CommerceKindProductPurchase, CommerceKindMembershipFee:
			default:
				return ErrInvalidOrderData
			}
		}
	}
	return nil
}

func validOrderDocumentDeliveryStatus(value string) bool {
	switch value {
	case "", "delivered", "in_transit", "cancelled", "returned", "other":
		return true
	default:
		return false
	}
}

func validOrderDocumentNumericID(value string) bool {
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

func validOrderDocumentText(value string, maxBytes int) bool {
	return len(value) <= maxBytes && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
