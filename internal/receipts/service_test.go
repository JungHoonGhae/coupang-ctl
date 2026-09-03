package receipts

import (
	"context"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type syntheticSource struct{}

func (syntheticSource) Status(context.Context) (core.ReceiptRequestStatusSnapshot, error) {
	return core.ReceiptRequestStatusSnapshot{Statuses: []core.ReceiptRequestStatus{{Kind: core.ReceiptKindCash, CanRequestNew: true}}}, nil
}

func (syntheticSource) History(_ context.Context, request core.ReceiptHistoryRequest) (core.ReceiptHistoryPage, error) {
	return core.ReceiptHistoryPage{PageIndex: request.PageIndex, PageSize: request.PageSize}, nil
}

func (syntheticSource) Summary(_ context.Context, request core.ReceiptSummaryRequest) (core.ReceiptSummary, error) {
	return core.ReceiptSummary{From: request.From, To: request.To, PaymentMethods: nil}, nil
}

func (syntheticSource) Download(context.Context, core.ReceiptDownloadRequest) (Download, error) {
	return Download{Metadata: core.ReceiptDownloadMetadata{Filename: "receipt.pdf", ContentType: "application/pdf"}, Content: []byte("synthetic")}, nil
}

func (syntheticSource) Vendor(_ context.Context, request core.VendorReceiptRequest) (core.VendorReceiptSnapshot, error) {
	return core.VendorReceiptSnapshot{
		SourceRef: request.SourceRef, PagesScanned: 2,
		Vendors: []core.VendorReceiptVendor{{VendorIndex: 0, MainPaymentTypeName: "Synthetic card"}},
	}, nil
}

type overviewSource struct{ syntheticSource }

func (overviewSource) Summary(_ context.Context, request core.ReceiptSummaryRequest) (core.ReceiptSummary, error) {
	result := core.ReceiptSummary{Kind: request.Kind, From: request.From, To: request.To}
	switch request.From + "/" + string(request.Kind) {
	case "2024-12-01/card":
		result.TotalCount, result.TotalAmountKRW = 2, 20000
		result.PaymentMethods = []core.ReceiptPaymentMethod{{DisplayName: "Synthetic A", TotalCount: 2, TotalAmountKRW: 20000}}
	case "2025-01-01/card":
		result.TotalCount, result.TotalAmountKRW = 3, 45000
		result.PaymentMethods = []core.ReceiptPaymentMethod{
			{DisplayName: "Synthetic A", TotalCount: 1, TotalAmountKRW: 10000},
			{DisplayName: "Synthetic B", TotalCount: 2, TotalAmountKRW: 35000},
		}
	case "2026-01-01/card":
		result.TotalCount, result.TotalAmountKRW = 1, 5000
		result.PaymentMethods = []core.ReceiptPaymentMethod{{DisplayName: "Synthetic B", TotalCount: 1, TotalAmountKRW: 5000}}
	case "2024-12-01/cash":
		result.TotalCount, result.TotalAmountKRW = 1, 3000
	case "2025-01-01/cash":
		result.TotalCount, result.TotalAmountKRW = 2, 7000
	}
	return result, nil
}

func TestOverviewAggregatesObservedReceiptSummariesAcrossCalendarYears(t *testing.T) {
	service := New(overviewSource{})
	service.now = func() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }

	got, err := service.Overview(context.Background(), core.ReceiptOverviewRequest{
		From: "2024-12-01", To: "2026-01-15", MaxCards: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != core.ReceiptSchemaVersion || got.Visibility != "private_local" || len(got.Periods) != 3 || len(got.Totals) != 2 {
		t.Fatalf("unexpected overview envelope: %#v", got)
	}
	if got.Periods[0].From != "2024-12-01" || got.Periods[0].To != "2024-12-31" || got.Periods[2].From != "2026-01-01" || got.Periods[2].To != "2026-01-15" {
		t.Fatalf("unexpected overview periods: %#v", got.Periods)
	}
	card := got.Totals[1]
	if card.Kind != core.ReceiptKindCard || card.TotalCount != 6 || card.TotalAmountKRW != 70000 || len(card.PaymentMethods) != 2 {
		t.Fatalf("unexpected card total: %#v", card)
	}
	if card.PaymentMethods[0].DisplayName != "Synthetic B" || card.PaymentMethods[0].TotalAmountKRW != 40000 || card.PaymentMethods[1].TotalAmountKRW != 30000 {
		t.Fatalf("unexpected payment method ranking: %#v", card.PaymentMethods)
	}
	if got.Installments.Status != "unavailable" || got.Definitions.Interpretation == "" {
		t.Fatalf("missing evidence boundary: %#v", got)
	}
}

func TestServiceAddsPrivateVersionedEnvelopeAndDefaults(t *testing.T) {
	service := New(syntheticSource{})
	service.now = func() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }

	history, err := service.History(context.Background(), core.ReceiptHistoryRequest{Kind: core.ReceiptKindCard})
	if err != nil {
		t.Fatal(err)
	}
	if history.SchemaVersion != core.ReceiptSchemaVersion || history.Visibility != "private_local" || history.PageSize != 5 || history.Items == nil || history.Definitions.Provenance != "observed" {
		t.Fatalf("unexpected history envelope: %#v", history)
	}

	summary, err := service.Summary(context.Background(), core.ReceiptSummaryRequest{Kind: core.ReceiptKindCard, From: "2026-01-01", To: "2026-09-03"})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Installments.Status != "unavailable" || len(summary.Installments.Limitations) != 1 || summary.PaymentMethods == nil {
		t.Fatalf("unexpected summary evidence boundary: %#v", summary)
	}

	download, err := service.Download(context.Background(), core.ReceiptDownloadRequest{Kind: core.ReceiptKindCash, HistoryIndex: 0})
	if err != nil {
		t.Fatal(err)
	}
	if download.Metadata.Bytes != len(download.Content) || download.Metadata.Visibility != "private_local" {
		t.Fatalf("unexpected download metadata: %#v", download.Metadata)
	}

	sourceRef := core.OrderSourceReference("synthetic-order")
	vendor, err := service.Vendor(context.Background(), core.VendorReceiptRequest{SourceRef: sourceRef})
	if err != nil {
		t.Fatal(err)
	}
	if vendor.Visibility != "private_local" || vendor.SourceRef != sourceRef || vendor.VendorCount != 1 || vendor.Installments.Status != "unavailable" || vendor.Settlement.Status != "source_components_observed" {
		t.Fatalf("unexpected vendor receipt envelope: %#v", vendor)
	}
}

func TestSummaryRejectsRangesOverOneYear(t *testing.T) {
	_, err := New(syntheticSource{}).Summary(context.Background(), core.ReceiptSummaryRequest{
		Kind: core.ReceiptKindCash, From: "2024-01-01", To: "2026-01-01",
	})
	if err == nil {
		t.Fatal("expected oversized summary range to be rejected")
	}
}
