package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenMigratesMetadataOnlySchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state", "coupangctl.sqlite3")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	assertCount(t, store.db, "SELECT COUNT(*) FROM schema_migrations", 11)
	assertCount(t, store.db, "SELECT COUNT(*) FROM pragma_table_info('sync_runs')", 8)
}

func TestOpenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "coupangctl.sqlite3")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	assertCount(t, second.db, "SELECT COUNT(*) FROM schema_migrations", 11)
}

func TestOpenSecuresDatabaseAndSidecarFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not a Windows access-control boundary")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state", "coupangctl.sqlite3")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, "INSERT INTO sync_runs(started_at, status) VALUES ('2026-09-02T00:00:00Z', 'running')"); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode for %s = %o, want 600", filepath.Base(candidate), info.Mode().Perm())
		}
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}
