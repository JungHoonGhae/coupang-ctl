package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func syntheticNamespace(t *testing.T, subject string) AccountNamespace {
	t.Helper()
	n, err := DeriveCoupangAccountNamespace(bytes.Repeat([]byte{17}, 32), []byte(subject))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAccountNamespaceIsStableKeyedAndDoesNotExposeSubject(t *testing.T) {
	a := syntheticNamespace(t, "synthetic-account-a")
	b := syntheticNamespace(t, "synthetic-account-b")
	if a != syntheticNamespace(t, "synthetic-account-a") || a == b {
		t.Fatal("namespace stability/isolation failed")
	}
	other, err := DeriveCoupangAccountNamespace(bytes.Repeat([]byte{18}, 32), []byte("synthetic-account-a"))
	if err != nil || other == a {
		t.Fatal("namespace was not bound to the local key")
	}
	if bytes.Contains([]byte(a.Reference()), []byte("synthetic-account-a")) {
		t.Fatal("namespace exposed its input")
	}
	parsed, err := ParseAccountNamespace(a.Reference())
	if err != nil || parsed != a {
		t.Fatal("persisted namespace cannot be restored")
	}
	for _, ref := range []string{"", "../legacy", "acct_v1_../../ledger", a.Reference() + "/", "acct_v1_" + string(bytes.Repeat([]byte{'G'}, 64))} {
		if _, err := ParseAccountNamespace(ref); !errors.Is(err, ErrAccountNamespaceInvalid) {
			t.Fatal("invalid namespace accepted")
		}
	}
	if _, err := DeriveCoupangAccountNamespace(nil, []byte("synthetic")); !errors.Is(err, ErrAccountNamespaceInvalid) {
		t.Fatal("missing key accepted")
	}
	if _, err := DeriveCoupangAccountNamespace(bytes.Repeat([]byte{17}, 32), nil); !errors.Is(err, ErrAccountNamespaceInvalid) {
		t.Fatal("missing subject accepted")
	}
}

func TestAccountLedgersIsolateOrdersCursorsRunsAndPurgeFromLegacy(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	legacy, err := Open(ctx, filepath.Join(root, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	makePage := func(amount int64, next int) core.OrderPage {
		return core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-shared-ref", PurchasedAt: "2026-08-01", Currency: "KRW", TotalAmount: amount}}, Next: &core.OrderCursor{Year: 2026, Page: next}}
	}
	if _, err := legacy.UpsertOrderPage(ctx, makePage(999, 1)); err != nil {
		t.Fatal(err)
	}
	a, err := OpenAccountLedger(ctx, root, syntheticNamespace(t, "synthetic-account-a"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := OpenAccountLedger(ctx, root, syntheticNamespace(t, "synthetic-account-b"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for i, ledger := range []*SQLite{a, b} {
		assertCount(t, ledger.db, `SELECT COUNT(*) FROM orders`, 0)
		run, err := ledger.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.ApplySyncPage(ctx, run, nil, makePage(int64(100*(i+1)), 2+i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = OpenAccountLedger(ctx, root, syntheticNamespace(t, "synthetic-account-a"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for i, ledger := range []*SQLite{a, b} {
		summary, err := ledger.Spend(ctx, core.OrderFilter{})
		if err != nil || summary.OrderCount != 1 || summary.TotalAmount != int64(100*(i+1)) {
			t.Fatal("account totals mixed")
		}
		cursor, err := ledger.LoadSyncCursor(ctx)
		if err != nil || cursor == nil || cursor.Page != 2+i {
			t.Fatal("account checkpoint mixed")
		}
		assertCount(t, ledger.db, `SELECT COUNT(*) FROM sync_runs`, 1)
		assertCount(t, ledger.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 1)
	}
	if _, err := a.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	assertCount(t, a.db, `SELECT COUNT(*) FROM orders`, 0)
	assertCount(t, b.db, `SELECT COUNT(*) FROM orders`, 1)
	assertCount(t, b.db, `SELECT COUNT(*) FROM sync_run_observed_orders`, 1)
	summary, err := legacy.Spend(ctx, core.OrderFilter{})
	if err != nil || summary.OrderCount != 1 || summary.TotalAmount != 999 {
		t.Fatal("legacy data was adopted or changed")
	}
}

func TestAccountLedgerRejectsUnboundAndMismatchedFilesWithoutMigration(t *testing.T) {
	for _, bound := range []bool{false, true} {
		ctx := context.Background()
		root := t.TempDir()
		wanted := syntheticNamespace(t, "synthetic-wanted")
		path := filepath.Join(root, "accounts", wanted.Reference(), "ledger.sqlite3")
		original, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if bound {
			if err := original.bindAccountNamespace(ctx, syntheticNamespace(t, "synthetic-other"), true); err != nil {
				t.Fatal(err)
			}
		}
		if err := original.Close(); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = OpenAccountLedger(ctx, root, wanted)
		want := ErrAccountLedgerUnbound
		if bound {
			want = ErrAccountLedgerMismatch
		}
		if !errors.Is(err, want) {
			t.Fatal("untrusted ledger binding accepted")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("rejected ledger was modified")
		}
	}
}

func TestConcurrentAccountLedgerInitializationPublishesOnlyBoundDatabase(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	namespace := syntheticNamespace(t, "synthetic-concurrent")
	start := make(chan struct{})
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			s, err := OpenAccountLedger(ctx, root, namespace)
			if err == nil {
				err = s.Close()
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "accounts", namespace.Reference()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "ledger.sqlite3" {
		t.Fatal("initializer left temporary databases")
	}
}

func TestAccountLedgerRejectsSymlinkAndCanceledInitialization(t *testing.T) {
	root := t.TempDir()
	namespace := syntheticNamespace(t, "synthetic-links")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OpenAccountLedger(ctx, root, namespace); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled initialization proceeded")
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatal("canceled initialization wrote state")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "accounts")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := OpenAccountLedger(context.Background(), root, namespace); err == nil {
		t.Fatal("symlinked account root accepted")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatal("account open wrote outside root")
	}
}
