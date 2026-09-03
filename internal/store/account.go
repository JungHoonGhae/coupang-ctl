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
	result := core.MembershipCostEvidence{
		Status:     "partial_history",
		Source:     "normalized_order_ledger_explicit_membership_metadata",
		Provenance: "derived",
		Limitations: []string{
			"membership charges absent from the source order history cannot be recovered",
			"refund settlements outside the normalized order cancellation state are not deducted",
		},
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MIN(purchased_at), ''), COALESCE(MAX(purchased_at), '')
		FROM orders`).Scan(&result.FirstObservedOrderDate, &result.LastObservedOrderDate); err != nil {
		return core.MembershipCostEvidence{}, fmt.Errorf("read membership-cost order coverage: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `WITH membership_orders AS (
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

	var completedAt, status string
	var historyComplete int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(completed_at, ''), status, history_complete
		FROM sync_runs ORDER BY id DESC LIMIT 1`).Scan(&completedAt, &status, &historyComplete)
	if err != nil && err != sql.ErrNoRows {
		return core.MembershipCostEvidence{}, fmt.Errorf("read membership-cost sync coverage: %w", err)
	}
	var checkpointCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_checkpoint`).Scan(&checkpointCount); err != nil {
		return core.MembershipCostEvidence{}, fmt.Errorf("read membership-cost checkpoint coverage: %w", err)
	}
	result.CompleteHistorySync = err == nil && status == "completed" && historyComplete != 0 && checkpointCount == 0
	if result.CompleteHistorySync {
		result.Status = "complete_available_history"
		result.LastCompleteHistorySyncAt = completedAt
	} else if result.FirstObservedOrderDate == "" {
		result.Status = "no_order_history"
	}
	return result, nil
}
