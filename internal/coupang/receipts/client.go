package receipts

import (
	"context"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	receiptworkflow "github.com/JungHoonGhae/coupang-ctl/internal/receipts"
)

type DocumentSource interface {
	FetchReceiptStatus(context.Context) ([]byte, error)
	FetchReceiptHistory(context.Context, core.ReceiptHistoryRequest) ([]byte, error)
	FetchReceiptSummary(context.Context, core.ReceiptSummaryRequest) ([]byte, error)
	FetchReceiptDownload(context.Context, core.ReceiptDownloadRequest) ([]byte, error)
	FetchVendorReceipt(context.Context, core.VendorReceiptRequest) ([]byte, error)
}

type Client struct {
	source DocumentSource
}

func New(source DocumentSource) *Client {
	return &Client{source: source}
}

func (c *Client) Status(ctx context.Context) (core.ReceiptRequestStatusSnapshot, error) {
	document, err := c.source.FetchReceiptStatus(ctx)
	if err != nil {
		return core.ReceiptRequestStatusSnapshot{}, err
	}
	return ParseStatusDocument(document)
}

func (c *Client) History(ctx context.Context, request core.ReceiptHistoryRequest) (core.ReceiptHistoryPage, error) {
	document, err := c.source.FetchReceiptHistory(ctx, request)
	if err != nil {
		return core.ReceiptHistoryPage{}, err
	}
	return ParseHistoryDocument(document, request)
}

func (c *Client) Summary(ctx context.Context, request core.ReceiptSummaryRequest) (core.ReceiptSummary, error) {
	document, err := c.source.FetchReceiptSummary(ctx, request)
	if err != nil {
		return core.ReceiptSummary{}, err
	}
	return ParseSummaryDocument(document, request)
}

func (c *Client) Download(ctx context.Context, request core.ReceiptDownloadRequest) (receiptworkflow.Download, error) {
	document, err := c.source.FetchReceiptDownload(ctx, request)
	if err != nil {
		return receiptworkflow.Download{}, err
	}
	return ParseDownloadDocument(document)
}

func (c *Client) Vendor(ctx context.Context, request core.VendorReceiptRequest) (core.VendorReceiptSnapshot, error) {
	document, err := c.source.FetchVendorReceipt(ctx, request)
	if err != nil {
		return core.VendorReceiptSnapshot{}, err
	}
	return ParseVendorDocument(document, request)
}
