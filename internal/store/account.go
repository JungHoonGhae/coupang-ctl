package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// MembershipCosts derives a bounded cost view from normalized orders. An
// order qualifies only when it has at least one item and every item carries
// the explicit membership_fee classification.
func (s *SQLite) MembershipCosts(ctx context.Context) (core.MembershipCostEvidence, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return core.MembershipCostEvidence{}, err
	}
	defer tx.Rollback()
	result := core.MembershipCostEvidence{
		Status:     "partial_history",
		Source:     "normalized_order_ledger_explicit_membership_metadata",
		Provenance: "derived",
		Limitations: []string{
			"membership charges absent from the source order history cannot be recovered",
			"refund settlements outside the normalized order cancellation state are not deducted",
			"counts describe retained local history, not verified complete coverage of the currently signed-in account",
		},
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(purchased_at), ''), COALESCE(MAX(purchased_at), '')
		FROM orders`).Scan(&result.FirstObservedOrderDate, &result.LastObservedOrderDate); err != nil {
		return core.MembershipCostEvidence{}, fmt.Errorf("read membership-cost order coverage: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `WITH membership_orders AS (
		SELECT o.source_ref, o.purchased_at, o.total_amount, o.fully_canceled
		FROM orders o
		WHERE EXISTS (
			SELECT 1 FROM order_items i
			WHERE i.order_ref = o.source_ref AND i.commerce_kind = 'membership_fee'
		) AND NOT EXISTS (
			SELECT 1 FROM order_items i
			WHERE i.order_ref = o.source_ref AND i.commerce_kind <> 'membership_fee'
		)
	)
	SELECT COUNT(*), COALESCE(SUM(total_amount), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 0 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN fully_canceled = 0 THEN total_amount ELSE 0 END), 0),
		COALESCE(MIN(purchased_at), ''), COALESCE(MAX(purchased_at), '')
	FROM membership_orders`).Scan(
		&result.ObservedPaymentCount, &result.ObservedGrossAmountKRW,
		&result.ObservedNonCanceledPaymentCount, &result.ObservedPaidAmountKRW,
		&result.FirstObservedPaymentDate, &result.LastObservedPaymentDate,
	); err != nil {
		return core.MembershipCostEvidence{}, fmt.Errorf("summarize normalized membership costs: %w", err)
	}

	status, err := latestSyncStatus(ctx, tx)
	if err != nil {
		return core.MembershipCostEvidence{}, err
	}
	result.CompleteHistorySync = status.HistoryComplete
	if result.CompleteHistorySync {
		result.Status = "complete_available_history"
		result.LastCompleteHistorySyncAt = status.CompletedAt
	} else if result.FirstObservedOrderDate == "" {
		result.Status = "no_order_history"
	}
	if err := tx.Commit(); err != nil {
		return core.MembershipCostEvidence{}, err
	}
	return result, nil
}
