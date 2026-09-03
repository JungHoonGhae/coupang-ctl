package receipts

import (
	"context"
	"errors"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const (
	defaultHistoryPageSize = 5
	defaultMaxCards        = 20
)

var ErrSourceUnavailable = errors.New("receipt source unavailable")

type Download struct {
	Metadata core.ReceiptDownloadMetadata
	Content  []byte
}

type Source interface {
	Status(context.Context) (core.ReceiptRequestStatusSnapshot, error)
	History(context.Context, core.ReceiptHistoryRequest) (core.ReceiptHistoryPage, error)
	Summary(context.Context, core.ReceiptSummaryRequest) (core.ReceiptSummary, error)
	Download(context.Context, core.ReceiptDownloadRequest) (Download, error)
}

type Service struct {
	source Source
	now    func() time.Time
}

func New(source Source) *Service {
	return &Service{source: source, now: time.Now}
}

func (s *Service) Status(ctx context.Context) (core.ReceiptRequestStatusSnapshot, error) {
	if s.source == nil {
		return core.ReceiptRequestStatusSnapshot{}, ErrSourceUnavailable
	}
	result, err := s.source.Status(ctx)
	if err != nil {
		return core.ReceiptRequestStatusSnapshot{}, errors.Join(ErrSourceUnavailable, err)
	}
	result.SchemaVersion = core.ReceiptSchemaVersion
	result.Visibility = "private_local"
	result.FetchedAt = s.now().UTC()
	result.Definitions = definitions()
	if result.Statuses == nil {
		result.Statuses = []core.ReceiptRequestStatus{}
	}
	return result, nil
}

func (s *Service) History(ctx context.Context, request core.ReceiptHistoryRequest) (core.ReceiptHistoryPage, error) {
	if request.PageSize == 0 {
		request.PageSize = defaultHistoryPageSize
	}
	if err := request.Validate(); err != nil {
		return core.ReceiptHistoryPage{}, err
	}
	if s.source == nil {
		return core.ReceiptHistoryPage{}, ErrSourceUnavailable
	}
	result, err := s.source.History(ctx, request)
	if err != nil {
		return core.ReceiptHistoryPage{}, errors.Join(ErrSourceUnavailable, err)
	}
	result.SchemaVersion = core.ReceiptSchemaVersion
	result.Visibility = "private_local"
	result.FetchedAt = s.now().UTC()
	result.Kind = request.Kind
	result.Definitions = definitions()
	if result.Items == nil {
		result.Items = []core.ReceiptHistoryItem{}
	}
	return result, nil
}

func (s *Service) Summary(ctx context.Context, request core.ReceiptSummaryRequest) (core.ReceiptSummary, error) {
	if request.MaxCards == 0 {
		request.MaxCards = defaultMaxCards
	}
	if err := request.Validate(); err != nil {
		return core.ReceiptSummary{}, err
	}
	if s.source == nil {
		return core.ReceiptSummary{}, ErrSourceUnavailable
	}
	result, err := s.source.Summary(ctx, request)
	if err != nil {
		return core.ReceiptSummary{}, errors.Join(ErrSourceUnavailable, err)
	}
	result.SchemaVersion = core.ReceiptSchemaVersion
	result.Visibility = "private_local"
	result.FetchedAt = s.now().UTC()
	result.Kind = request.Kind
	result.Definitions = definitions()
	if result.PaymentMethods == nil {
		result.PaymentMethods = []core.ReceiptPaymentMethod{}
	}
	if result.Warnings == nil {
		result.Warnings = []string{}
	}
	result.Installments = core.ReceiptInstallmentInfo{
		Status: "unavailable",
		Limitations: []string{
			"the verified sales-slip summary exposes payment-method totals but no installment-month field",
		},
	}
	return result, nil
}

func (s *Service) Download(ctx context.Context, request core.ReceiptDownloadRequest) (Download, error) {
	if request.PageSize == 0 {
		request.PageSize = defaultHistoryPageSize
	}
	if err := request.Validate(); err != nil {
		return Download{}, err
	}
	if s.source == nil {
		return Download{}, ErrSourceUnavailable
	}
	result, err := s.source.Download(ctx, request)
	if err != nil {
		return Download{}, errors.Join(ErrSourceUnavailable, err)
	}
	result.Metadata.SchemaVersion = core.ReceiptSchemaVersion
	result.Metadata.Visibility = "private_local"
	result.Metadata.Kind = request.Kind
	result.Metadata.HistoryIndex = request.HistoryIndex
	result.Metadata.DownloadIndex = request.DownloadIndex
	result.Metadata.Bytes = len(result.Content)
	return result, nil
}

func definitions() core.ReceiptDefinitions {
	return core.ReceiptDefinitions{
		Source:          "coupang_payment_receipt_read_endpoints",
		Provenance:      "observed",
		PaymentPrivacy:  "card identifiers and card numbers are discarded before the typed response",
		DownloadPrivacy: "download URLs are used in memory and never returned or logged",
	}
}
