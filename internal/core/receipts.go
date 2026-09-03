package core

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const ReceiptSchemaVersion = 2

var ErrVendorReceiptNotFound = errors.New("vendor receipt order reference not found")

type ReceiptKind string

const (
	ReceiptKindCash ReceiptKind = "cash"
	ReceiptKindCard ReceiptKind = "card"
)

func (k ReceiptKind) Validate() error {
	switch k {
	case ReceiptKindCash, ReceiptKindCard:
		return nil
	default:
		return errors.New("kind must be cash or card")
	}
}

type ReceiptHistoryRequest struct {
	Kind      ReceiptKind `json:"kind" jsonschema:"Receipt family: cash or card"`
	PageIndex int         `json:"page_index,omitempty" jsonschema:"Zero-based request-history page,minimum=0,maximum=1000"`
	PageSize  int         `json:"page_size,omitempty" jsonschema:"History rows per page,minimum=1,maximum=50"`
}

func (r ReceiptHistoryRequest) Validate() error {
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	if r.PageIndex < 0 || r.PageIndex > 1000 {
		return errors.New("page_index must be between 0 and 1000")
	}
	if r.PageSize < 0 || r.PageSize > 50 {
		return errors.New("page_size must be between 1 and 50")
	}
	return nil
}

type ReceiptSummaryRequest struct {
	Kind     ReceiptKind `json:"kind" jsonschema:"Receipt family: cash or card"`
	From     string      `json:"from" jsonschema:"Inclusive start date in YYYY-MM-DD format"`
	To       string      `json:"to" jsonschema:"Inclusive end date in YYYY-MM-DD format"`
	MaxCards int         `json:"max_cards,omitempty" jsonschema:"Maximum observed card methods to summarize,minimum=1,maximum=30"`
}

func (r ReceiptSummaryRequest) Validate() error {
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	from, fromErr := time.Parse(time.DateOnly, r.From)
	to, toErr := time.Parse(time.DateOnly, r.To)
	if fromErr != nil || toErr != nil || to.Before(from) {
		return errors.New("from and to must be an ordered YYYY-MM-DD range")
	}
	if to.Sub(from) > 366*24*time.Hour {
		return errors.New("receipt summary range cannot exceed 366 days")
	}
	if r.MaxCards < 0 || r.MaxCards > 30 {
		return errors.New("max_cards must be between 1 and 30")
	}
	return nil
}

type ReceiptDownloadRequest struct {
	Kind          ReceiptKind `json:"kind"`
	PageIndex     int         `json:"page_index,omitempty"`
	PageSize      int         `json:"page_size,omitempty"`
	HistoryIndex  int         `json:"history_index" jsonschema:"Zero-based history row index"`
	DownloadIndex int         `json:"download_index,omitempty" jsonschema:"Zero-based file index within the selected history row"`
}

func (r ReceiptDownloadRequest) Validate() error {
	if err := (ReceiptHistoryRequest{Kind: r.Kind, PageIndex: r.PageIndex, PageSize: r.PageSize}).Validate(); err != nil {
		return err
	}
	if r.HistoryIndex < 0 || r.HistoryIndex >= 50 {
		return errors.New("history_index must be between 0 and 49")
	}
	if r.DownloadIndex < 0 || r.DownloadIndex >= 100 {
		return errors.New("download_index must be between 0 and 99")
	}
	return nil
}

type ReceiptRequestStatusSnapshot struct {
	SchemaVersion int                    `json:"schema_version"`
	Visibility    string                 `json:"visibility"`
	FetchedAt     time.Time              `json:"fetched_at"`
	Statuses      []ReceiptRequestStatus `json:"statuses"`
	Definitions   ReceiptDefinitions     `json:"definitions"`
}

type ReceiptRequestStatus struct {
	Kind                    ReceiptKind `json:"kind"`
	Availability            string      `json:"availability"`
	CanRequestNew           bool        `json:"can_request_new"`
	RequestInProgress       *bool       `json:"request_in_progress"`
	RequestInProgressStatus string      `json:"request_in_progress_status"`
	Limitations             []string    `json:"limitations"`
	Provenance              string      `json:"provenance"`
}

type ReceiptHistoryPage struct {
	SchemaVersion int                  `json:"schema_version"`
	Visibility    string               `json:"visibility"`
	FetchedAt     time.Time            `json:"fetched_at"`
	Kind          ReceiptKind          `json:"kind"`
	PageIndex     int                  `json:"page_index"`
	PageSize      int                  `json:"page_size"`
	HasNext       bool                 `json:"has_next"`
	NextPageIndex int                  `json:"next_page_index"`
	HasPrev       bool                 `json:"has_prev"`
	PrevPageIndex int                  `json:"prev_page_index"`
	Items         []ReceiptHistoryItem `json:"items"`
	Definitions   ReceiptDefinitions   `json:"definitions"`
}

type ReceiptHistoryItem struct {
	HistoryIndex         int    `json:"history_index"`
	RequestedAt          string `json:"requested_at,omitempty"`
	From                 string `json:"from,omitempty"`
	To                   string `json:"to,omitempty"`
	TotalCount           int    `json:"total_count"`
	TotalAmountKRW       int64  `json:"total_amount_krw"`
	PaymentMethodDisplay string `json:"payment_method_display,omitempty"`
	Status               string `json:"status"`
	DownloadCount        int    `json:"download_count"`
	Provenance           string `json:"provenance"`
}

type ReceiptSummary struct {
	SchemaVersion  int                    `json:"schema_version"`
	Visibility     string                 `json:"visibility"`
	FetchedAt      time.Time              `json:"fetched_at"`
	Kind           ReceiptKind            `json:"kind"`
	From           string                 `json:"from"`
	To             string                 `json:"to"`
	TotalCount     int                    `json:"total_count"`
	TotalAmountKRW int64                  `json:"total_amount_krw"`
	PaymentMethods []ReceiptPaymentMethod `json:"payment_methods"`
	Installments   ReceiptInstallmentInfo `json:"installments"`
	Definitions    ReceiptDefinitions     `json:"definitions"`
	Warnings       []string               `json:"warnings"`
}

type ReceiptPaymentMethod struct {
	DisplayName    string `json:"display_name"`
	TotalCount     int    `json:"total_count"`
	TotalAmountKRW int64  `json:"total_amount_krw"`
	Provenance     string `json:"provenance"`
}

type ReceiptInstallmentInfo struct {
	Status      string   `json:"status"`
	Limitations []string `json:"limitations"`
}

type ReceiptDefinitions struct {
	Source          string `json:"source"`
	Provenance      string `json:"provenance"`
	RequestStatus   string `json:"request_status"`
	PaymentPrivacy  string `json:"payment_privacy"`
	DownloadPrivacy string `json:"download_privacy"`
}

type ReceiptDownloadMetadata struct {
	SchemaVersion int         `json:"schema_version"`
	Visibility    string      `json:"visibility"`
	Kind          ReceiptKind `json:"kind"`
	HistoryIndex  int         `json:"history_index"`
	DownloadIndex int         `json:"download_index"`
	Filename      string      `json:"filename"`
	ContentType   string      `json:"content_type"`
	Bytes         int         `json:"bytes"`
	OutputPath    string      `json:"output_path,omitempty"`
}

type VendorReceiptRequest struct {
	SourceRef string `json:"source_ref" jsonschema:"Hashed source_ref returned by orders list"`
	MaxPages  int    `json:"max_pages,omitempty" jsonschema:"Maximum order pages searched in memory,minimum=1,maximum=1000"`
}

func (r VendorReceiptRequest) Validate() error {
	decoded, err := hex.DecodeString(r.SourceRef)
	if err != nil || len(decoded) != 32 || r.SourceRef != strings.ToLower(r.SourceRef) {
		return errors.New("source_ref must be a lowercase SHA-256 order reference")
	}
	if r.MaxPages < 0 || r.MaxPages > 1000 {
		return errors.New("max_pages must be between 1 and 1000")
	}
	return nil
}

type VendorReceiptSnapshot struct {
	SchemaVersion int                    `json:"schema_version"`
	Visibility    string                 `json:"visibility"`
	FetchedAt     time.Time              `json:"fetched_at"`
	SourceRef     string                 `json:"source_ref"`
	PagesScanned  int                    `json:"pages_scanned"`
	VendorCount   int                    `json:"vendor_count"`
	Vendors       []VendorReceiptVendor  `json:"vendors"`
	Installments  ReceiptInstallmentInfo `json:"installments"`
	Settlement    ReceiptSettlementInfo  `json:"settlement"`
	Definitions   ReceiptDefinitions     `json:"definitions"`
}

type VendorReceiptVendor struct {
	VendorIndex                         int                     `json:"vendor_index"`
	VendorName                          string                  `json:"vendor_name,omitempty"`
	MainPaymentType                     string                  `json:"main_payment_type,omitempty"`
	MainPaymentTypeName                 string                  `json:"main_payment_type_name,omitempty"`
	MainPaymentTypeDescription          string                  `json:"main_payment_type_description,omitempty"`
	MainPaymentAmountKRW                int64                   `json:"main_payment_amount_krw"`
	PaidByMobile                        bool                    `json:"paid_by_mobile,omitempty"`
	PaidWithCoupangCash                 bool                    `json:"paid_with_coupang_cash,omitempty"`
	ReceiptDisplayAvailable             bool                    `json:"receipt_display_available,omitempty"`
	CashableCashReceiptDisplayAvailable bool                    `json:"cashable_cash_receipt_display_available,omitempty"`
	ProductAmountKRW                    int64                   `json:"product_amount_krw"`
	DeliveryFeeKRW                      int64                   `json:"delivery_fee_krw"`
	IssuedAmountKRW                     int64                   `json:"issued_amount_krw"`
	GiftWrappingAmountKRW               int64                   `json:"gift_wrapping_amount_krw"`
	OverseasProductAmountKRW            int64                   `json:"overseas_product_amount_krw"`
	OverseasDeliveryFeeKRW              int64                   `json:"overseas_delivery_fee_krw"`
	Payment                             ReceiptPaymentBreakdown `json:"payment"`
	Products                            []VendorReceiptProduct  `json:"products"`
}

type VendorReceiptProduct struct {
	ProductIndex          int                     `json:"product_index"`
	Name                  string                  `json:"name,omitempty"`
	VendorItemID          string                  `json:"vendor_item_id,omitempty"`
	Quantity              int                     `json:"quantity"`
	CanceledQuantity      int                     `json:"canceled_quantity"`
	CancelHoldingQuantity int                     `json:"cancel_holding_quantity"`
	UnitPriceKRW          int64                   `json:"unit_price_krw"`
	CombinedUnitPriceKRW  int64                   `json:"combined_unit_price_krw"`
	Payment               ReceiptPaymentBreakdown `json:"payment"`
}

type ReceiptPaymentBreakdown struct {
	OriginalPaymentAmountKRW             int64  `json:"original_payment_amount_krw"`
	OriginalPaymentCanceledAmountKRW     int64  `json:"original_payment_canceled_amount_krw"`
	CoupangCashAmountKRW                 int64  `json:"coupang_cash_amount_krw"`
	CoupangCashCanceledAmountKRW         int64  `json:"coupang_cash_canceled_amount_krw"`
	CashableCoupangCashAmountKRW         int64  `json:"cashable_coupang_cash_amount_krw"`
	CashableCoupangCashCanceledAmountKRW int64  `json:"cashable_coupang_cash_canceled_amount_krw"`
	CouponDiscountAmountKRW              int64  `json:"coupon_discount_amount_krw"`
	CouponDiscountCanceledAmountKRW      int64  `json:"coupon_discount_canceled_amount_krw"`
	CardInstantDiscountAmountKRW         int64  `json:"card_instant_discount_amount_krw"`
	CardInstantDiscountCanceledAmountKRW int64  `json:"card_instant_discount_canceled_amount_krw"`
	InstantDiscountAmountKRW             int64  `json:"instant_discount_amount_krw"`
	InstantDiscountType                  string `json:"instant_discount_type,omitempty"`
}

type ReceiptSettlementInfo struct {
	Status      string   `json:"status"`
	Limitations []string `json:"limitations"`
}
