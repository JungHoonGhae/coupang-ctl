package receipts

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	receiptworkflow "github.com/JungHoonGhae/coupang-ctl/internal/receipts"
)

const maxReceiptDocumentBytes = 8 << 20

var (
	ErrReceiptDataMissing = errors.New("structured receipt data missing")
	safeFilenamePattern   = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

type responseEnvelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type statusDocument struct {
	Cash responseEnvelope[bool] `json:"cash"`
	Card responseEnvelope[bool] `json:"card"`
}

type historyData struct {
	PageIndex     int              `json:"pageIndex"`
	PageSize      int              `json:"pageSize"`
	HasNext       bool             `json:"hasNext"`
	NextPageIndex int              `json:"nextPageIndex"`
	HasPrev       bool             `json:"hasPrev"`
	PrevPageIndex int              `json:"prevPageIndex"`
	List          []historyPayload `json:"list"`
}

type historyPayload struct {
	RequestDate     string            `json:"requestDate"`
	From            string            `json:"from"`
	To              string            `json:"to"`
	TotalCount      int               `json:"totalCount"`
	TotalAmount     int64             `json:"totalAmount"`
	DisplayCardName string            `json:"displayCardName"`
	RequestStatus   string            `json:"requestStatus"`
	DownloadURLList []downloadPayload `json:"downloadUrlList"`
}

type downloadPayload struct {
	StartIndex  int    `json:"startIndex"`
	EndIndex    int    `json:"endIndex"`
	DownloadURL string `json:"downloadUrl"`
}

type receiptSummaryPayload struct {
	From            string              `json:"from"`
	To              string              `json:"to"`
	TotalCount      int                 `json:"totalCount"`
	TotalAmount     int64               `json:"totalAmount"`
	DisplayCardName string              `json:"displayCardName"`
	CardList        []cardMethodPayload `json:"cardList"`
}

type cardMethodPayload struct {
	CardID            string `json:"cardId"`
	CardNumber        string `json:"cardNumber"`
	DisplayCardName   string `json:"displayCardName"`
	DisplayCardNumber string `json:"displayCardNumber"`
}

type summaryDocument struct {
	Kind    core.ReceiptKind                          `json:"kind"`
	Summary responseEnvelope[receiptSummaryPayload]   `json:"summary"`
	PerCard []responseEnvelope[receiptSummaryPayload] `json:"per_card"`
}

type downloadDocument struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Bytes       int    `json:"bytes"`
	Base64      string `json:"base64"`
}

type vendorDocument struct {
	Found        bool                   `json:"found"`
	SourceRef    string                 `json:"source_ref"`
	PagesScanned int                    `json:"pages_scanned"`
	Vendors      []vendorReceiptPayload `json:"vendors"`
}

type vendorReceiptPayload struct {
	MainPayType                      string                 `json:"mainPayType"`
	MainPayTypeName                  string                 `json:"mainPayTypeName"`
	MainPayTypeDescription           string                 `json:"mainPayTypeDescription"`
	MainPayTypePrice                 int64                  `json:"mainPayTypePrice"`
	MainPayedByMobile                bool                   `json:"mainPayedByMobile"`
	PayedWithCoupangCash             bool                   `json:"payedWithCoupangCash"`
	NeedToDisplayReceipt             bool                   `json:"needToDisplayReceipt"`
	NeedToDisplayCashableCashReceipt bool                   `json:"needToDisplayCashableCashReceipt"`
	TotalProductPrice                int64                  `json:"totalProductPrice"`
	TotalDeliveryFee                 int64                  `json:"totalDeliveryFee"`
	TotalIssuedPrice                 int64                  `json:"totalIssuedPrice"`
	TotalGiftWrappingPrice           int64                  `json:"totalGiftWrappingPrice"`
	TotalOverseasProductPrice        int64                  `json:"totalOverseasProductPrice"`
	OverseasDeliveryFee              int64                  `json:"overSeasDeliveryFee"`
	RepresentedVendorName            string                 `json:"representedVendorName"`
	PaymentDetails                   paymentDetailsPayload  `json:"paymentDetails"`
	ProductList                      []vendorProductPayload `json:"productList"`
}

type vendorProductPayload struct {
	VendorItemID          int64                 `json:"vendorItemId"`
	VendorItemName        string                `json:"vendorItemName"`
	Quantity              int                   `json:"quantity"`
	CanceledQuantity      int                   `json:"canceledQuantity"`
	CancelHoldingQuantity int                   `json:"cancelHoldingQuantity"`
	UnitPrice             int64                 `json:"unitPrice"`
	CombinedUnitPrice     int64                 `json:"combinedUnitPrice"`
	PaymentDetails        paymentDetailsPayload `json:"productPaymentDetails"`
}

type paymentDetailsPayload struct {
	OriginalPaymentPrice                 int64                  `json:"originalPaymentPrice"`
	OriginalPaymentCancelPrice           int64                  `json:"originalPaymentCancelPrice"`
	CoupangCashPrice                     int64                  `json:"coupangCashPrice"`
	CoupangCashCancelPrice               int64                  `json:"coupangCashCancelPrice"`
	CashableCoupangCashPrice             int64                  `json:"cashableCoupangCashPrice"`
	CashableCoupangCashCancelPrice       int64                  `json:"cashableCoupangCashCancelPrice"`
	DiscountCouponPrice                  int64                  `json:"discountCouponPrice"`
	DiscountCouponCancelPrice            int64                  `json:"discountCouponCancelPrice"`
	CreditCardInstantDiscountPrice       int64                  `json:"creditCardInstantDiscountPrice"`
	CreditCardInstantDiscountCancelPrice int64                  `json:"creditCardInstantDiscountCancelPrice"`
	InstantDiscount                      instantDiscountPayload `json:"instantDiscount"`
}

type instantDiscountPayload struct {
	DiscountAmount int64  `json:"discountAmount"`
	DiscountType   string `json:"discountType"`
}

func ParseStatusDocument(document []byte) (core.ReceiptRequestStatusSnapshot, error) {
	var raw statusDocument
	if decode(document, &raw) != nil || !raw.Cash.Success || !raw.Card.Success {
		return core.ReceiptRequestStatusSnapshot{}, ErrReceiptDataMissing
	}
	return core.ReceiptRequestStatusSnapshot{Statuses: []core.ReceiptRequestStatus{
		{Kind: core.ReceiptKindCash, RequestInProgress: raw.Cash.Data, CanRequestNew: !raw.Cash.Data, Provenance: "observed"},
		{Kind: core.ReceiptKindCard, RequestInProgress: raw.Card.Data, CanRequestNew: !raw.Card.Data, Provenance: "observed"},
	}}, nil
}

func ParseHistoryDocument(document []byte, request core.ReceiptHistoryRequest) (core.ReceiptHistoryPage, error) {
	var raw responseEnvelope[historyData]
	if decode(document, &raw) != nil || !raw.Success || raw.Data.PageIndex < 0 || raw.Data.PageSize < 0 || len(raw.Data.List) > 50 {
		return core.ReceiptHistoryPage{}, ErrReceiptDataMissing
	}
	items := make([]core.ReceiptHistoryItem, 0, len(raw.Data.List))
	for index, item := range raw.Data.List {
		if item.TotalCount < 0 || item.TotalAmount < 0 || len(item.DownloadURLList) > 100 {
			return core.ReceiptHistoryPage{}, ErrReceiptDataMissing
		}
		items = append(items, core.ReceiptHistoryItem{
			HistoryIndex: index, RequestedAt: cleanText(item.RequestDate, 40),
			From: cleanText(item.From, 20), To: cleanText(item.To, 20), TotalCount: item.TotalCount,
			TotalAmountKRW: item.TotalAmount, PaymentMethodDisplay: cleanText(item.DisplayCardName, 120),
			Status: cleanText(item.RequestStatus, 40), DownloadCount: len(item.DownloadURLList), Provenance: "observed",
		})
	}
	return core.ReceiptHistoryPage{
		Kind: request.Kind, PageIndex: raw.Data.PageIndex, PageSize: raw.Data.PageSize,
		HasNext: raw.Data.HasNext, NextPageIndex: raw.Data.NextPageIndex,
		HasPrev: raw.Data.HasPrev, PrevPageIndex: raw.Data.PrevPageIndex, Items: items,
	}, nil
}

func ParseSummaryDocument(document []byte, request core.ReceiptSummaryRequest) (core.ReceiptSummary, error) {
	var raw summaryDocument
	if decode(document, &raw) != nil || raw.Kind != request.Kind || !raw.Summary.Success || raw.Summary.Data.TotalCount < 0 || raw.Summary.Data.TotalAmount < 0 {
		return core.ReceiptSummary{}, ErrReceiptDataMissing
	}
	byName := map[string]core.ReceiptPaymentMethod{}
	for _, response := range raw.PerCard {
		item := response.Data
		name := cleanText(item.DisplayCardName, 120)
		if !response.Success || name == "" || item.TotalCount < 0 || item.TotalAmount < 0 {
			continue
		}
		method := byName[name]
		method.DisplayName = name
		method.TotalCount += item.TotalCount
		method.TotalAmountKRW += item.TotalAmount
		method.Provenance = "derived_from_observed_receipt_summaries"
		byName[name] = method
	}
	methods := make([]core.ReceiptPaymentMethod, 0, len(byName))
	for _, method := range byName {
		methods = append(methods, method)
	}
	sort.Slice(methods, func(i, j int) bool {
		if methods[i].TotalAmountKRW == methods[j].TotalAmountKRW {
			return methods[i].DisplayName < methods[j].DisplayName
		}
		return methods[i].TotalAmountKRW > methods[j].TotalAmountKRW
	})
	warnings := []string{}
	if sourceFrom := normalizedReceiptDate(raw.Summary.Data.From); sourceFrom != "" && sourceFrom != request.From {
		warnings = append(warnings, "the source-reported start date differed from the requested range")
	}
	if sourceTo := normalizedReceiptDate(raw.Summary.Data.To); sourceTo != "" && sourceTo != request.To {
		warnings = append(warnings, "the source-reported end date differed from the requested range")
	}
	return core.ReceiptSummary{
		Kind: request.Kind, From: request.From, To: request.To,
		TotalCount: raw.Summary.Data.TotalCount, TotalAmountKRW: raw.Summary.Data.TotalAmount,
		PaymentMethods: methods, Warnings: warnings,
	}, nil
}

func normalizedReceiptDate(value string) string {
	value = strings.ReplaceAll(cleanText(value, 20), ".", "-")
	if _, err := time.Parse(time.DateOnly, value); err != nil {
		return ""
	}
	return value
}

func ParseDownloadDocument(document []byte) (receiptworkflow.Download, error) {
	var raw downloadDocument
	if decode(document, &raw) != nil || raw.Bytes < 1 || raw.Bytes > maxReceiptDocumentBytes || len(raw.Base64) > base64.StdEncoding.EncodedLen(maxReceiptDocumentBytes) {
		return receiptworkflow.Download{}, ErrReceiptDataMissing
	}
	content, err := base64.StdEncoding.DecodeString(raw.Base64)
	if err != nil || len(content) != raw.Bytes || len(content) > maxReceiptDocumentBytes {
		return receiptworkflow.Download{}, ErrReceiptDataMissing
	}
	contentType := strings.TrimSpace(strings.Split(raw.ContentType, ";")[0])
	switch contentType {
	case "application/pdf", "text/html", "image/png", "image/jpeg", "application/octet-stream":
	default:
		return receiptworkflow.Download{}, ErrReceiptDataMissing
	}
	filename := safeFilename(raw.Filename, contentType)
	return receiptworkflow.Download{
		Metadata: core.ReceiptDownloadMetadata{Filename: filename, ContentType: contentType, Bytes: len(content)},
		Content:  content,
	}, nil
}

func ParseVendorDocument(document []byte, request core.VendorReceiptRequest) (core.VendorReceiptSnapshot, error) {
	var raw vendorDocument
	if decode(document, &raw) != nil || raw.SourceRef != request.SourceRef || raw.PagesScanned < 1 || raw.PagesScanned > 1000 || len(raw.Vendors) > 200 {
		return core.VendorReceiptSnapshot{}, ErrReceiptDataMissing
	}
	if !raw.Found {
		return core.VendorReceiptSnapshot{}, core.ErrVendorReceiptNotFound
	}
	result := core.VendorReceiptSnapshot{
		SourceRef: request.SourceRef, PagesScanned: raw.PagesScanned,
		Vendors: make([]core.VendorReceiptVendor, 0, len(raw.Vendors)),
	}
	for vendorIndex, vendor := range raw.Vendors {
		if len(vendor.ProductList) > 1000 || !validVendorAmounts(vendor) || !validPaymentDetails(vendor.PaymentDetails) {
			return core.VendorReceiptSnapshot{}, ErrReceiptDataMissing
		}
		normalized := core.VendorReceiptVendor{
			VendorIndex: vendorIndex, VendorName: cleanText(vendor.RepresentedVendorName, 200),
			MainPaymentType: cleanText(vendor.MainPayType, 80), MainPaymentTypeName: cleanText(vendor.MainPayTypeName, 120),
			MainPaymentTypeDescription: cleanText(vendor.MainPayTypeDescription, 200), MainPaymentAmountKRW: vendor.MainPayTypePrice,
			PaidByMobile: vendor.MainPayedByMobile, PaidWithCoupangCash: vendor.PayedWithCoupangCash,
			ReceiptDisplayAvailable: vendor.NeedToDisplayReceipt, CashableCashReceiptDisplayAvailable: vendor.NeedToDisplayCashableCashReceipt,
			ProductAmountKRW: vendor.TotalProductPrice, DeliveryFeeKRW: vendor.TotalDeliveryFee, IssuedAmountKRW: vendor.TotalIssuedPrice,
			GiftWrappingAmountKRW: vendor.TotalGiftWrappingPrice, OverseasProductAmountKRW: vendor.TotalOverseasProductPrice,
			OverseasDeliveryFeeKRW: vendor.OverseasDeliveryFee, Payment: normalizePaymentDetails(vendor.PaymentDetails),
			Products: make([]core.VendorReceiptProduct, 0, len(vendor.ProductList)),
		}
		for productIndex, product := range vendor.ProductList {
			if product.Quantity < 0 || product.CanceledQuantity < 0 || product.CancelHoldingQuantity < 0 || product.UnitPrice < 0 || product.CombinedUnitPrice < 0 || !validPaymentDetails(product.PaymentDetails) {
				return core.VendorReceiptSnapshot{}, ErrReceiptDataMissing
			}
			vendorItemID := ""
			if product.VendorItemID > 0 {
				vendorItemID = strconv.FormatInt(product.VendorItemID, 10)
			}
			normalized.Products = append(normalized.Products, core.VendorReceiptProduct{
				ProductIndex: productIndex, Name: cleanText(product.VendorItemName, 300), VendorItemID: vendorItemID,
				Quantity: product.Quantity, CanceledQuantity: product.CanceledQuantity, CancelHoldingQuantity: product.CancelHoldingQuantity,
				UnitPriceKRW: product.UnitPrice, CombinedUnitPriceKRW: product.CombinedUnitPrice,
				Payment: normalizePaymentDetails(product.PaymentDetails),
			})
		}
		result.Vendors = append(result.Vendors, normalized)
	}
	result.VendorCount = len(result.Vendors)
	return result, nil
}

func validVendorAmounts(value vendorReceiptPayload) bool {
	return value.MainPayTypePrice >= 0 && value.TotalProductPrice >= 0 && value.TotalDeliveryFee >= 0 && value.TotalIssuedPrice >= 0 &&
		value.TotalGiftWrappingPrice >= 0 && value.TotalOverseasProductPrice >= 0 && value.OverseasDeliveryFee >= 0
}

func validPaymentDetails(value paymentDetailsPayload) bool {
	return value.OriginalPaymentPrice >= 0 && value.OriginalPaymentCancelPrice >= 0 &&
		value.CoupangCashPrice >= 0 && value.CoupangCashCancelPrice >= 0 &&
		value.CashableCoupangCashPrice >= 0 && value.CashableCoupangCashCancelPrice >= 0 &&
		value.DiscountCouponPrice >= 0 && value.DiscountCouponCancelPrice >= 0 &&
		value.CreditCardInstantDiscountPrice >= 0 && value.CreditCardInstantDiscountCancelPrice >= 0 &&
		value.InstantDiscount.DiscountAmount >= 0
}

func normalizePaymentDetails(value paymentDetailsPayload) core.ReceiptPaymentBreakdown {
	return core.ReceiptPaymentBreakdown{
		OriginalPaymentAmountKRW: value.OriginalPaymentPrice, OriginalPaymentCanceledAmountKRW: value.OriginalPaymentCancelPrice,
		CoupangCashAmountKRW: value.CoupangCashPrice, CoupangCashCanceledAmountKRW: value.CoupangCashCancelPrice,
		CashableCoupangCashAmountKRW: value.CashableCoupangCashPrice, CashableCoupangCashCanceledAmountKRW: value.CashableCoupangCashCancelPrice,
		CouponDiscountAmountKRW: value.DiscountCouponPrice, CouponDiscountCanceledAmountKRW: value.DiscountCouponCancelPrice,
		CardInstantDiscountAmountKRW: value.CreditCardInstantDiscountPrice, CardInstantDiscountCanceledAmountKRW: value.CreditCardInstantDiscountCancelPrice,
		InstantDiscountAmountKRW: value.InstantDiscount.DiscountAmount, InstantDiscountType: cleanText(value.InstantDiscount.DiscountType, 80),
	}
}

func decode(document []byte, target any) error {
	if len(document) == 0 || len(document) > maxReceiptDocumentBytes || !json.Valid(document) {
		return ErrReceiptDataMissing
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	if err := decoder.Decode(target); err != nil {
		return ErrReceiptDataMissing
	}
	return nil
}

func cleanText(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" || !utf8.ValidString(value) {
		return ""
	}
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

func safeFilename(value, contentType string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = safeFilenamePattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		value = "coupang-receipt"
	}
	if len(value) > 120 {
		value = value[:120]
	}
	if filepath.Ext(value) == "" {
		if extensions, _ := mime.ExtensionsByType(contentType); len(extensions) > 0 {
			value += extensions[0]
		}
	}
	return value
}
