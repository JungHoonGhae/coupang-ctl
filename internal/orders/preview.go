package orders

import (
	"context"
	"errors"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// Preview deliberately uses only pageSource and never consults the ledger,
// saved cursor, sync lease or previous account data. Each invocation starts at
// the source entry page; it does not combine pages from unverified accounts.
func (s *Service) Preview(ctx context.Context) (core.OrderPreview, error) {
	if err := ctx.Err(); err != nil {
		return core.OrderPreview{}, err
	}
	if s.pageSource == nil || !s.syncSource.ValidForAcquisition() {
		return core.OrderPreview{}, ErrDocumentSource
	}
	ctx, cancel := context.WithTimeout(ctx, syncPageTimeout)
	defer cancel()
	page, err := s.pageSource.FetchPage(ctx, nil)
	if err != nil {
		return core.OrderPreview{}, errors.Join(ErrDocumentSource, err)
	}
	if err := ctx.Err(); err != nil {
		return core.OrderPreview{}, err
	}
	if page.Orders == nil || core.ValidateOrderDocument(page) != nil {
		return core.OrderPreview{}, core.ErrInvalidOrderData
	}
	seen := make(map[string]bool, len(page.Orders))
	for _, order := range page.Orders {
		if seen[order.SourceRef] {
			return core.OrderPreview{}, core.ErrInvalidOrderData
		}
		seen[order.SourceRef] = true
	}
	return core.OrderPreview{
		SchemaVersion: 1, Visibility: "private_local", Source: s.syncSource,
		Provenance: core.SyncProvenanceObservedStructuredOrderDocument,
		CapturedAt: s.now().UTC(), Scope: "source_entry_page", Orders: page.Orders,
		OrderCount: len(page.Orders), HasNextPage: page.Next != nil,
		HistoryComplete: false, AccountIdentityVerified: false, Persisted: false,
		Limitations: []string{
			"Only the source entry page was read; page order is preserved and no full-history or requested-period coverage is established.",
			"An empty page is not evidence that this account has never ordered; has_next_page describes this response only.",
			"Stable account identity is unverified; this response is not associated or merged with any retained local ledger.",
			"No order ledger, checkpoint, price history or account data was read or written; the dedicated browser may persist its own session state.",
		},
	}, nil
}
