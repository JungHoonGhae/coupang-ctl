package receipts

import (
	"context"
	"errors"
	"sort"
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
	Vendor(context.Context, core.VendorReceiptRequest) (core.VendorReceiptSnapshot, error)
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

func (s *Service) Overview(ctx context.Context, request core.ReceiptOverviewRequest) (core.ReceiptOverview, error) {
	if request.MaxCards == 0 {
		request.MaxCards = defaultMaxCards
	}
	if err := request.Validate(); err != nil {
		return core.ReceiptOverview{}, err
	}
	if s.source == nil {
		return core.ReceiptOverview{}, ErrSourceUnavailable
	}
	periods, err := receiptOverviewPeriods(request.From, request.To)
	if err != nil {
		return core.ReceiptOverview{}, err
	}
	result := core.ReceiptOverview{
		SchemaVersion: core.ReceiptSchemaVersion,
		Visibility:    "private_local",
		FetchedAt:     s.now().UTC(),
		From:          request.From,
		To:            request.To,
		Periods:       make([]core.ReceiptOverviewPeriod, 0, len(periods)),
		Totals: []core.ReceiptOverviewKindTotal{
			newReceiptOverviewKindTotal(core.ReceiptKindCash),
			newReceiptOverviewKindTotal(core.ReceiptKindCard),
		},
		Installments: core.ReceiptInstallmentInfo{
			Status: "unavailable",
			Limitations: []string{
				"the verified receipt summaries expose payment-method totals but no installment-month field",
			},
		},
		Definitions: core.ReceiptOverviewDefinitions{
			Source:         "coupang_payment_receipt_summary_reads",
			Provenance:     "derived_from_observed_receipt_summaries",
			Windowing:      "the inclusive requested range is split into non-overlapping calendar-year reads",
			Interpretation: "cash and card totals are separate receipt-source totals and are not relabeled as order spend or summed into a lifetime-spend headline",
		},
		Warnings: []string{},
	}
	warnings := map[string]struct{}{}
	for _, period := range periods {
		periodResult := core.ReceiptOverviewPeriod{From: period.From, To: period.To, Totals: make([]core.ReceiptOverviewKindTotal, 0, 2)}
		for kindIndex, kind := range []core.ReceiptKind{core.ReceiptKindCash, core.ReceiptKindCard} {
			summary, readErr := s.source.Summary(ctx, core.ReceiptSummaryRequest{
				Kind: kind, From: period.From, To: period.To, MaxCards: request.MaxCards,
			})
			if readErr != nil {
				return core.ReceiptOverview{}, errors.Join(ErrSourceUnavailable, readErr)
			}
			periodTotal := receiptOverviewTotal(summary, kind)
			periodResult.Totals = append(periodResult.Totals, periodTotal)
			mergeReceiptOverviewTotal(&result.Totals[kindIndex], periodTotal)
			for _, warning := range summary.Warnings {
				if warning != "" {
					warnings[warning] = struct{}{}
				}
			}
		}
		result.Periods = append(result.Periods, periodResult)
	}
	for index := range result.Totals {
		sortReceiptPaymentMethods(result.Totals[index].PaymentMethods)
	}
	for warning := range warnings {
		result.Warnings = append(result.Warnings, warning)
	}
	sort.Strings(result.Warnings)
	return result, nil
}

func receiptOverviewPeriods(fromText, toText string) ([]core.ReceiptOverviewPeriod, error) {
	from, err := time.Parse(time.DateOnly, fromText)
	if err != nil {
		return nil, err
	}
	to, err := time.Parse(time.DateOnly, toText)
	if err != nil {
		return nil, err
	}
	periods := []core.ReceiptOverviewPeriod{}
	for start := from; !start.After(to); {
		end := time.Date(start.Year(), time.December, 31, 0, 0, 0, 0, time.UTC)
		if end.After(to) {
			end = to
		}
		periods = append(periods, core.ReceiptOverviewPeriod{From: start.Format(time.DateOnly), To: end.Format(time.DateOnly)})
		start = end.AddDate(0, 0, 1)
	}
	return periods, nil
}

func newReceiptOverviewKindTotal(kind core.ReceiptKind) core.ReceiptOverviewKindTotal {
	return core.ReceiptOverviewKindTotal{
		Kind: kind, PaymentMethods: []core.ReceiptPaymentMethod{},
		Provenance: "derived_from_observed_receipt_summaries",
	}
}

func receiptOverviewTotal(summary core.ReceiptSummary, kind core.ReceiptKind) core.ReceiptOverviewKindTotal {
	result := newReceiptOverviewKindTotal(kind)
	result.TotalCount = summary.TotalCount
	result.TotalAmountKRW = summary.TotalAmountKRW
	result.PaymentMethods = append(result.PaymentMethods, summary.PaymentMethods...)
	sortReceiptPaymentMethods(result.PaymentMethods)
	return result
}

func mergeReceiptOverviewTotal(target *core.ReceiptOverviewKindTotal, source core.ReceiptOverviewKindTotal) {
	target.TotalCount += source.TotalCount
	target.TotalAmountKRW += source.TotalAmountKRW
	byName := make(map[string]int, len(target.PaymentMethods))
	for index, method := range target.PaymentMethods {
		byName[method.DisplayName] = index
	}
	for _, method := range source.PaymentMethods {
		if index, ok := byName[method.DisplayName]; ok {
			target.PaymentMethods[index].TotalCount += method.TotalCount
			target.PaymentMethods[index].TotalAmountKRW += method.TotalAmountKRW
			continue
		}
		method.Provenance = "derived_from_observed_receipt_summaries"
		byName[method.DisplayName] = len(target.PaymentMethods)
		target.PaymentMethods = append(target.PaymentMethods, method)
	}
}

func sortReceiptPaymentMethods(methods []core.ReceiptPaymentMethod) {
	sort.Slice(methods, func(i, j int) bool {
		if methods[i].TotalAmountKRW == methods[j].TotalAmountKRW {
			return methods[i].DisplayName < methods[j].DisplayName
		}
		return methods[i].TotalAmountKRW > methods[j].TotalAmountKRW
	})
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

func (s *Service) Vendor(ctx context.Context, request core.VendorReceiptRequest) (core.VendorReceiptSnapshot, error) {
	if request.MaxPages == 0 {
		request.MaxPages = 1000
	}
	if err := request.Validate(); err != nil {
		return core.VendorReceiptSnapshot{}, err
	}
	if s.source == nil {
		return core.VendorReceiptSnapshot{}, ErrSourceUnavailable
	}
	result, err := s.source.Vendor(ctx, request)
	if err != nil {
		return core.VendorReceiptSnapshot{}, errors.Join(ErrSourceUnavailable, err)
	}
	result.SchemaVersion = core.ReceiptSchemaVersion
	result.Visibility = "private_local"
	result.FetchedAt = s.now().UTC()
	result.SourceRef = request.SourceRef
	result.VendorCount = len(result.Vendors)
	result.Definitions = definitions()
	result.Installments = core.ReceiptInstallmentInfo{
		Status: "unavailable",
		Limitations: []string{
			"the verified vendor-receipt response exposes payment types and cancellation components but no installment-month field",
		},
	}
	result.Settlement = core.ReceiptSettlementInfo{
		Status: "source_components_observed",
		Limitations: []string{
			"source cancellation amount fields are exposed without relabeling them as a completed refund settlement",
			"exact post-refund net spend remains unavailable until status and amount semantics agree across canceled and returned samples",
		},
	}
	if result.Vendors == nil {
		result.Vendors = []core.VendorReceiptVendor{}
	}
	return result, nil
}

func definitions() core.ReceiptDefinitions {
	return core.ReceiptDefinitions{
		Source:          "coupang_payment_receipt_read_endpoints",
		Provenance:      "observed",
		RequestStatus:   "source true maps to possible and false maps to impossible; request_in_progress stays null because impossibility does not prove its cause",
		PaymentPrivacy:  "card identifiers and card numbers are discarded before the typed response",
		DownloadPrivacy: "download URLs are used in memory and never returned or logged",
	}
}
