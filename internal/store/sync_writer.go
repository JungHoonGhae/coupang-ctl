package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
)

// AcquireSyncWriter serializes cooperative sync attempts across connections
// and processes, before reading a checkpoint or contacting a source. The OS
// releases the advisory lock on process exit; no persisted PID or timeout can
// accidentally evict a live writer. It does not lock SQLite readers, prove
// account identity, or fence arbitrary direct SQL/import/purge writers.
func (s *SQLite) AcquireSyncWriter(ctx context.Context) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(s.path)
	if err != nil {
		return nil, errors.New("cannot resolve sync ledger")
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return nil, errors.New("cannot resolve sync ledger")
	}
	unlock, err := acquireSyncWriterFile(canonical + ".sync-writer.lock")
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, unlock())
	}
	var once sync.Once
	var releaseErr error
	return func() error { once.Do(func() { releaseErr = unlock() }); return releaseErr }, nil
}
