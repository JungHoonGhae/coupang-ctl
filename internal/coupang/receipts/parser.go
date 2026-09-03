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
