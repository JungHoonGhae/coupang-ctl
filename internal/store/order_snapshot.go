package store

import (
	"context"
	"database/sql"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// All aggregate helpers share this reader. It cannot acquire a second DB
// connection or accidentally escape the transaction to start another snapshot.
type orderAggregateReader struct {
	db interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}
}

func (s *SQLite) Stats(ctx context.Context, filter core.OrderFilter) (core.OrderStats, error) {
	result, evidence, err := readOrderSnapshot(ctx, s, func(reader orderAggregateReader) (core.OrderStats, error) { return reader.Stats(ctx, filter) })
	if err != nil {
		return core.OrderStats{}, err
	}
	result.SchemaVersion = core.OrderStatsSchemaVersion
	result.Evidence = evidence
	return result, nil
}

func (s *SQLite) Spend(ctx context.Context, filter core.OrderFilter) (core.SpendSummary, error) {
	result, evidence, err := readOrderSnapshot(ctx, s, func(reader orderAggregateReader) (core.SpendSummary, error) { return reader.Spend(ctx, filter) })
	if err != nil {
		return core.SpendSummary{}, err
	}
	result.SchemaVersion = core.OrderAggregateSchemaVersion
	result.Evidence = evidence
	result.Evidence.Limitations = append(result.Evidence.Limitations, "non-canceled order totals are not verified net spending after refunds or settlement")
	return result, nil
}

func (s *SQLite) Insights(ctx context.Context, filter core.OrderFilter) (core.ShoppingInsights, error) {
	result, err := s.ShoppingAnalysis(ctx, filter, false)
	return result.Insights, err
}

func (s *SQLite) ProductInsights(ctx context.Context, filter core.OrderFilter) (core.ProductInsights, error) {
	result, evidence, err := readOrderSnapshot(ctx, s, func(reader orderAggregateReader) (core.ProductInsights, error) {
		return reader.ProductInsights(ctx, filter)
	})
	if err != nil {
		return core.ProductInsights{}, err
	}
	result.SchemaVersion = core.OrderAggregateSchemaVersion
	result.Evidence = evidence
	return result, nil
}

func (s *SQLite) ShoppingAnalysis(ctx context.Context, filter core.OrderFilter, includeProducts bool) (core.ShoppingAnalysis, error) {
	result, evidence, err := readOrderSnapshot(ctx, s, func(reader orderAggregateReader) (core.ShoppingAnalysis, error) {
		var result core.ShoppingAnalysis
		var err error
		result.Insights, err = reader.Insights(ctx, filter)
		if err != nil {
			return result, err
		}
		result.Insights.Categories, err = reader.CategoryBreakdown(ctx, filter)
		if err != nil {
			return result, err
		}
		if includeProducts {
			products, err := reader.ProductInsights(ctx, filter)
			if err != nil {
				return result, err
			}
			result.Products = &products
		}
		return result, nil
	})
	if err != nil {
		return core.ShoppingAnalysis{}, err
	}
	result.Insights.SchemaVersion = core.ShoppingInsightsSchemaVersion
	result.Insights.Evidence = evidence
	if result.Products != nil {
		result.Products.SchemaVersion = core.OrderAggregateSchemaVersion
		result.Products.Evidence = evidence
	}
	return result, nil
}

func readOrderSnapshot[T any](ctx context.Context, s *SQLite, read func(orderAggregateReader) (T, error)) (T, core.OrderReadEvidence, error) {
	var zero T
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return zero, core.OrderReadEvidence{}, err
	}
	defer tx.Rollback()
	// The first read establishes SQLite's snapshot before any aggregate query.
	status, err := latestSyncStatus(ctx, tx)
	if err != nil {
		return zero, core.OrderReadEvidence{}, err
	}
	evidence := core.OrderReadEvidence{
		Visibility: "private_local", Dataset: "retained_local_history", SnapshotCapturedAt: s.now().UTC(), LatestAttempt: status,
		Limitations: []string{
			"local aggregate evidence does not establish the currently signed-in source account",
			"ledger-wide sync status does not prove completeness of the requested date range",
			"stored records may include orders not observed in the latest scan; absence is not deletion",
			"snapshot capture time is local read time, not upstream data freshness",
		},
	}
	result, err := read(orderAggregateReader{db: tx})
	if err != nil {
		return zero, core.OrderReadEvidence{}, err
	}
	if err := tx.Commit(); err != nil {
		return zero, core.OrderReadEvidence{}, err
	}
	return result, evidence, nil
}
