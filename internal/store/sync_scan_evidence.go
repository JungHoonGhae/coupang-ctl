package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// The caller holds the same read transaction used for latest-attempt evidence
// and, where requested, aggregate results. No account or process liveness is
// inferred here, and no old scan is silently substituted for an untracked run.
func syncScanEvidence(ctx context.Context, reader syncCursorReader, runID int64) (*core.SyncScanEvidence, error) {
	var result core.SyncScanEvidence
	var state string
	var year, page sql.NullInt64
	var retained int
	err := reader.QueryRowContext(ctx, `SELECT s.state,s.started_at,COALESCE(s.ended_at,''),COALESCE(s.superseded_at,''),
		s.starts_from_beginning,s.next_year,s.next_page,
		(SELECT COUNT(*) FROM sync_scan_attempts a WHERE a.scan_id=s.id),
		(SELECT COUNT(*) FROM sync_scan_pages p WHERE p.scan_id=s.id),
		(SELECT COUNT(DISTINCT o.source_ref) FROM sync_run_observed_orders o
		 JOIN sync_scan_attempts a ON a.run_id=o.run_id JOIN orders r ON r.source_ref=o.source_ref WHERE a.scan_id=s.id),
		(SELECT COUNT(*) FROM orders)
		FROM sync_scans s JOIN sync_scan_attempts current ON current.scan_id=s.id WHERE current.run_id=?`, runID).Scan(
		&state, &result.StartedAt, &result.EndedAt, &result.SupersededAt, &result.StartsFromBeginning, &year, &page,
		&result.Attempts, &result.PagesProcessed, &result.RetainedOrdersObserved, &retained,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("cannot read cumulative sync scan evidence")
	}
	if year.Valid != page.Valid || retained < result.RetainedOrdersObserved {
		return nil, errors.New("invalid cumulative sync scan evidence")
	}
	if year.Valid {
		if year.Int64 < 2000 || year.Int64 > 2100 || page.Int64 < 0 || page.Int64 > 1000 {
			return nil, errors.New("invalid scan checkpoint")
		}
		result.Next = &core.OrderCursor{Year: int(year.Int64), Page: int(page.Int64)}
	}
	switch state {
	case "active":
		result.State = core.SyncScanActive
		if result.SupersededAt != "" {
			result.State = core.SyncScanSuperseded
		}
	case "exhausted":
		if result.Next != nil || result.EndedAt == "" || result.SupersededAt != "" {
			return nil, errors.New("invalid exhausted scan evidence")
		}
		result.State = core.SyncScanCursorExhausted
	default:
		return nil, errors.New("invalid sync scan state")
	}
	result.RetainedOrdersNotObserved = retained - result.RetainedOrdersObserved
	return &result, nil
}
