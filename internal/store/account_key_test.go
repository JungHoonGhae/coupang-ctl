package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func privateAccountTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPersistentAccountKeyReusesNamespaceAndPreservesLegacy(t *testing.T) {
	ctx := context.Background()
	root := privateAccountTestRoot(t)
	legacy, err := Open(ctx, filepath.Join(root, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-legacy", PurchasedAt: "2026-08-01", Currency: "KRW"}}}); err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	a, first, err := OpenCoupangAccountLedger(ctx, root, []byte("synthetic-subject"))
	if err != nil {
		t.Fatal(err)
	}
	assertCount(t, a.db, `SELECT COUNT(*) FROM orders`, 0)
	if _, err := a.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-current", PurchasedAt: "2026-08-01", Currency: "KRW"}}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(root, accountKeyFilename)
	before, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(before)
	b, second, err := OpenCoupangAccountLedger(ctx, root, []byte("synthetic-subject"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if first != second {
		t.Fatal("same installation and subject selected another namespace")
	}
	assertCount(t, b.db, `SELECT COUNT(*) FROM orders`, 1)
	assertCount(t, legacy.db, `SELECT COUNT(*) FROM orders`, 1)
	after, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(after)
	if !bytes.Equal(before, after) {
		t.Fatal("opening another ledger replaced the namespace key")
	}
	if info, err := os.Stat(keyPath); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("namespace key permissions are not private")
	}
}

func TestMissingAccountKeyRequiresRecoveryAndOriginalKeyRestoresNamespace(t *testing.T) {
	ctx := context.Background()
	root := privateAccountTestRoot(t)
	ledger, original, err := OpenCoupangAccountLedger(ctx, root, []byte("synthetic-subject"))
	if err != nil {
		t.Fatal(err)
	}
	ledger.Close()
	path := filepath.Join(root, accountKeyFilename)
	backup, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(backup)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenCoupangAccountLedger(ctx, root, []byte("synthetic-subject")); !errors.Is(err, ErrAccountKeyRecoveryRequired) {
		t.Fatal("missing key silently regenerated over existing account state")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("recovery failure created a replacement key")
	}
	if err := os.WriteFile(path, backup, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, restored, err := OpenCoupangAccountLedger(ctx, root, []byte("synthetic-subject"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if restored != original {
		t.Fatal("original key did not restore the namespace")
	}
}

func TestMalformedAccountKeyIsNeverOverwritten(t *testing.T) {
	for _, kind := range []string{"truncated", "oversized", "version", "checksum"} {
		t.Run(kind, func(t *testing.T) {
			root := privateAccountTestRoot(t)
			if _, err := localAccountKey(context.Background(), root); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, accountKeyFilename)
			record, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(record)
			switch kind {
			case "truncated":
				record = record[:7]
			case "oversized":
				record = append(record, 0)
			case "version":
				record[0] ^= 1
			case "checksum":
				record[len(accountKeyMagic)] ^= 1
			}
			if err := os.WriteFile(path, record, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := OpenCoupangAccountLedger(context.Background(), root, []byte("synthetic-subject")); !errors.Is(err, ErrAccountKeyRecoveryRequired) {
				t.Fatal("damaged key was accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(after)
			if !bytes.Equal(record, after) {
				t.Fatal("damaged key was silently replaced")
			}
		})
	}
}

func TestConcurrentAccountKeySetupReusesOneNamespace(t *testing.T) {
	root := privateAccountTestRoot(t)
	start := make(chan struct{})
	type result struct {
		namespace AccountNamespace
		err       error
	}
	results := make(chan result, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ledger, namespace, err := OpenCoupangAccountLedger(context.Background(), root, []byte("synthetic-concurrent-subject"))
			if err == nil {
				err = ledger.Close()
			}
			results <- result{namespace, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var expected AccountNamespace
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if expected.reference == "" {
			expected = result.namespace
		}
		if result.namespace != expected {
			t.Fatal("concurrent setup used different namespace keys")
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 2 {
		t.Fatal("key initialization left temporary files")
	}
}

func TestAccountKeySetupRejectsUnsafeInputWithoutWriting(t *testing.T) {
	root := privateAccountTestRoot(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := OpenCoupangAccountLedger(ctx, root, []byte("synthetic")); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled setup accepted")
	}
	if _, _, err := OpenCoupangAccountLedger(context.Background(), root, nil); !errors.Is(err, ErrAccountNamespaceInvalid) {
		t.Fatal("missing identity accepted")
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatal("invalid setup wrote state")
	}
	outside := filepath.Join(t.TempDir(), "synthetic-key")
	if err := os.WriteFile(outside, []byte("synthetic-not-a-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, accountKeyFilename)); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, _, err := OpenCoupangAccountLedger(context.Background(), root, []byte("synthetic")); !errors.Is(err, ErrAccountKeyUnavailable) {
		t.Fatal("symlink key accepted")
	}
}

func TestAccountKeyRejectsPublicDirectoryAndFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission check")
	}
	root := privateAccountTestRoot(t)
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := localAccountKey(context.Background(), root); !errors.Is(err, ErrAccountKeyUnavailable) {
		t.Fatal("public key directory accepted")
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatal("unsafe directory received key material")
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := localAccountKey(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, accountKeyFilename), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := localAccountKey(context.Background(), root); !errors.Is(err, ErrAccountKeyUnavailable) {
		t.Fatal("publicly readable key accepted")
	}
}

func TestAccountKeyHelperProcess(t *testing.T) {
	if os.Getenv("COUPANGCTL_TEST_ACCOUNT_KEY_HELPER") != "1" {
		return
	}
	root := os.Getenv("COUPANGCTL_TEST_ACCOUNT_KEY_ROOT")
	if !filepath.IsAbs(root) {
		os.Exit(2)
	}
	ledger, namespace, err := OpenCoupangAccountLedger(context.Background(), root, []byte("synthetic-process-subject"))
	if err != nil {
		os.Exit(3)
	}
	if err := ledger.Close(); err != nil {
		os.Exit(4)
	}
	// Only this synthetic account's opaque reference is captured by the parent.
	if _, err := os.Stdout.WriteString(namespace.Reference()); err != nil {
		os.Exit(5)
	}
	os.Exit(0)
}

func TestSeparateProcessesSharePersistentAccountKey(t *testing.T) {
	root := privateAccountTestRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	type outcome struct {
		reference string
		err       error
	}
	results := make(chan outcome, 4)
	for i := 0; i < 4; i++ {
		go func() {
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAccountKeyHelperProcess$")
			command.Env = append(os.Environ(), "COUPANGCTL_TEST_ACCOUNT_KEY_HELPER=1", "COUPANGCTL_TEST_ACCOUNT_KEY_ROOT="+root)
			output, err := command.Output()
			results <- outcome{string(output), err}
		}()
	}
	var expected string
	for i := 0; i < 4; i++ {
		result := <-results
		if result.err != nil {
			t.Error("separate process could not open the account ledger")
			continue
		}
		if _, err := ParseAccountNamespace(result.reference); err != nil {
			t.Error("helper did not return a namespace")
			continue
		}
		if expected == "" {
			expected = result.reference
		}
		if result.reference != expected {
			t.Error("separate processes selected different namespaces")
		}
	}
}
