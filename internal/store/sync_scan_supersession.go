package store

import "context"

// Supersession is independent of source exhaustion. Restarting cannot turn an
// unfinished scan into a completed one or remove its observations.
func (s *SQLite) migrateSyncScanSupersession(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET version=version WHERE version=18`); err != nil {
		return err
	}
	var done bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=18)`).Scan(&done); err != nil {
		return err
	}
	if !done {
		for _, statement := range []string{
			`ALTER TABLE sync_scans ADD COLUMN superseded_at TEXT`,
			`DROP INDEX sync_one_active_scan`,
			`CREATE UNIQUE INDEX sync_one_active_scan ON sync_scans(state) WHERE state='active' AND superseded_at IS NULL`,
			`INSERT INTO schema_migrations(version,applied_at) VALUES(18,strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
