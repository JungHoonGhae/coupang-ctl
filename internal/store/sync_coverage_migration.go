package store

import "context"

func (s *SQLite) migrateSyncCoverage(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Take the writer lock before reading schema/version to support concurrent
	// CLI/MCP openers without a read-to-write lock upgrade.
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET version=version WHERE version=16`); err != nil {
		return err
	}
	var done bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=16)`).Scan(&done); err != nil {
		return err
	}
	if !done {
		for _, statement := range []string{
			`ALTER TABLE sync_runs ADD COLUMN cursor_exhausted INTEGER CHECK(cursor_exhausted IN (0,1))`,
			`ALTER TABLE sync_runs ADD COLUMN coverage_status TEXT NOT NULL DEFAULT 'unknown_legacy'`,
			`INSERT INTO schema_migrations(version,applied_at) VALUES(16,strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
