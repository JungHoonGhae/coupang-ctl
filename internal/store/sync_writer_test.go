package store

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func openWriterLedger(t *testing.T, path string) *SQLite {
	t.Helper()
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSyncWriterScopeAndRelease(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	a := openWriterLedger(t, path)
	b := openWriterLedger(t, path)
	other := openWriterLedger(t, filepath.Join(t.TempDir(), "other.sqlite3"))
	release, err := a.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { release() })
	if unexpected, err := b.AcquireSyncWriter(ctx); !errors.Is(err, core.ErrSyncInProgress) {
		if unexpected != nil {
			unexpected()
		}
		t.Fatal("same ledger accepted a concurrent writer")
	}
	otherRelease, err := other.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal("independent ledger was blocked", err)
	}
	if err := otherRelease(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := release(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	next, err := b.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal("released writer could not resume", err)
	}
	defer next()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(canonical + ".sync-writer.lock")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatal("lock must be empty and private")
	}
}

func TestSyncWriterCanceledBeforeAcquisition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	s := openWriterLedger(t, path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if release, err := s.AcquireSyncWriter(ctx); !errors.Is(err, context.Canceled) || release != nil {
		t.Fatal("canceled acquisition was not rejected")
	}
	if _, err := os.Stat(path + ".sync-writer.lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled acquisition touched the lock")
	}
}

func TestSyncWriterSymlinkAliasesShareLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	a := openWriterLedger(t, path)
	alias := filepath.Join(t.TempDir(), "alias.sqlite3")
	if err := os.Symlink(path, alias); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("Windows symlink privilege unavailable")
		}
		t.Fatal(err)
	}
	// Exercise lock identity without opening a second SQLite WAL through an alias.
	b := &SQLite{path: alias}
	release, err := a.AcquireSyncWriter(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if extra, err := b.AcquireSyncWriter(context.Background()); !errors.Is(err, core.ErrSyncInProgress) {
		if extra != nil {
			extra()
		}
		t.Fatal("symlink alias bypassed writer exclusion")
	}
}

func TestSyncWriterRejectsUnsafeLockFiles(t *testing.T) {
	for _, kind := range []string{"directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "synthetic-private.sqlite3")
			s := openWriterLedger(t, path)
			lockPath := path + ".sync-writer.lock"
			var err error
			if kind == "directory" {
				err = os.Mkdir(lockPath, 0o700)
			} else {
				err = os.Symlink(path, lockPath)
			}
			if err != nil {
				if runtime.GOOS == "windows" && kind == "symlink" {
					t.Skip("Windows symlink privilege unavailable")
				}
				t.Fatal(err)
			}
			release, err := s.AcquireSyncWriter(context.Background())
			if release != nil {
				release()
				t.Fatal("unsafe lock accepted")
			}
			if err == nil || strings.Contains(err.Error(), "synthetic-private") || strings.Contains(err.Error(), filepath.Dir(path)) {
				t.Fatal("unsafe lock failure was missing or exposed a local path")
			}
			if err := s.Ping(context.Background()); err != nil {
				t.Fatal("ledger damaged", err)
			}
		})
	}
}

// The child exits without releasing the writer or closing SQLite. The parent
// proves actual process ownership and OS cleanup, not just two Go mutex users.
func TestSyncWriterProcessHelper(t *testing.T) {
	path := os.Getenv("COUPANGCTL_TEST_SYNC_WRITER_PATH")
	if path == "" {
		return
	}
	s, err := Open(context.Background(), path)
	if err != nil {
		os.Exit(2)
	}
	if _, err := s.AcquireSyncWriter(context.Background()); err != nil {
		os.Exit(3)
	}
	fmt.Fprintln(os.Stdout, "writer-ready")
	var signal [1]byte
	if _, err := io.ReadFull(os.Stdin, signal[:]); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

func TestSyncWriterReleasedAfterProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
	s := openWriterLedger(t, path)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSyncWriterProcessHelper$")
	cmd.Env = append(os.Environ(), "COUPANGCTL_TEST_SYNC_WRITER_PATH="+path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "writer-ready\n" {
		t.Fatal("child did not establish writer ownership")
	}
	if release, err := s.AcquireSyncWriter(ctx); !errors.Is(err, core.ErrSyncInProgress) {
		if release != nil {
			release()
		}
		t.Fatal("child process did not exclude another writer")
	}
	if _, err := stdin.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal("child exit failed", err)
	}
	waited = true
	release, err := s.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal("process exit stranded writer lock", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}
