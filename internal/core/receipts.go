package core

import (
	"errors"
	"time"
)

const ReceiptSchemaVersion = 1

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
	Kind              ReceiptKind `json:"kind"`
	RequestInProgress bool        `json:"request_in_progress"`
	CanRequestNew     bool        `json:"can_request_new"`
	Provenance        string      `json:"provenance"`
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
