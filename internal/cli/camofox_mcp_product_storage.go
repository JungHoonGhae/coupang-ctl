package cli

import (
	"context"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// The existing runtime owner provides on-demand storage through the product
// module's interfaces. It keeps one connection, error recovery and shutdown
// ownership identical for optional evidence and mandatory local tools.
func (r *camofoxMCPRuntime) RecordPriceObservations(ctx context.Context, observations []core.ProductPriceObservation) error {
	ledger, err := r.localLedger(ctx)
	if err != nil {
		return err
	}
	return ledger.RecordPriceObservations(ctx, observations)
}

func (r *camofoxMCPRuntime) ListPriceObservations(ctx context.Context, request core.ProductPriceHistoryRequest) ([]core.ProductPriceObservation, bool, error) {
	ledger, err := r.localLedger(ctx)
	if err != nil {
		return nil, false, err
	}
	return ledger.ListPriceObservations(ctx, request)
}

func (r *camofoxMCPRuntime) PurgePriceObservations(ctx context.Context) (int, error) {
	ledger, err := r.localLedger(ctx)
	if err != nil {
		return 0, err
	}
	return ledger.PurgePriceObservations(ctx)
}

func (r *camofoxMCPRuntime) RecommendationPurchaseContext(ctx context.Context, refs []core.ProductReference) (core.RecommendationPurchaseContext, error) {
	ledger, err := r.localLedger(ctx)
	if err != nil {
		return core.RecommendationPurchaseContext{}, err
	}
	return ledger.RecommendationPurchaseContext(ctx, refs)
}
