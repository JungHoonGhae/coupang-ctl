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

func TestServiceAddsPrivateVersionedEnvelopeAndDefaults(t *testing.T) {
	service := New(syntheticSource{})
	service.now = func() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }

	history, err := service.History(context.Background(), core.ReceiptHistoryRequest{Kind: core.ReceiptKindCard})
	if err != nil {
		t.Fatal(err)
	}
	if history.SchemaVersion != 1 || history.Visibility != "private_local" || history.PageSize != 5 || history.Items == nil || history.Definitions.Provenance != "observed" {
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
