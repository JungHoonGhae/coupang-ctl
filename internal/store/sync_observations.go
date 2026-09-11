package store

import (
	"context"
	"fmt"
)

// Attempt observations are not verified-account or complete-scan evidence.
// Do not backfill old orders: their acquisition attempt cannot be reconstructed.
func (s *SQLite) migrateSyncObservations(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		// Acquire the write lock before inspecting/creating schema. Otherwise
		// concurrent reopeners can both hold read locks and fail to upgrade.
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at)
		 VALUES (15, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
		`CREATE TABLE IF NOT EXISTS sync_run_observed_orders (
			run_id INTEGER NOT NULL REFERENCES sync_runs(id) ON DELETE CASCADE,
			source_ref TEXT NOT NULL REFERENCES orders(source_ref) ON DELETE CASCADE,
			PRIMARY KEY(run_id, source_ref)
		)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate sync observations: %w", err)
		}
	}
	return tx.Commit()
}
