package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrAccountNamespaceInvalid = errors.New("invalid local account namespace")
var ErrAccountLedgerUnbound = errors.New("existing ledger has no verified namespace binding")
var ErrAccountLedgerMismatch = errors.New("ledger namespace does not match the requested account")
var ErrAccountLedgerUnavailable = errors.New("account ledger binding could not be read")

// AccountNamespace is a local pseudonym, not authentication or source-identity
// proof. The source adapter must verify identity before selecting a namespace.
// No raw source identifier or local secret is retained in this value.
type AccountNamespace struct{ reference string }

func (n AccountNamespace) Reference() string { return n.reference }

// DeriveCoupangAccountNamespace requires a stable source-native subject and a
// persistent, local-only 32-byte key. Callers must not substitute a profile,
// display name, session token, or cookie for the subject. This function only
// derives a namespace; it does not verify that the caller supplied such proof.
// The domain is source-wide so changing browser adapters does not split accounts.
func DeriveCoupangAccountNamespace(localKey, sourceSubject []byte) (AccountNamespace, error) {
	if len(localKey) != 32 || len(sourceSubject) == 0 || len(sourceSubject) > 4096 {
		return AccountNamespace{}, ErrAccountNamespaceInvalid
	}
	mac := hmac.New(sha256.New, localKey)
	mac.Write([]byte("coupangctl/account-namespace/coupang/v1\x00"))
	mac.Write(sourceSubject)
	return AccountNamespace{reference: "acct_v1_" + hex.EncodeToString(mac.Sum(nil))}, nil
}

// ParseAccountNamespace restores a previously stored local reference. Knowing
// a reference is not permission to read or import an account's data.
func ParseAccountNamespace(reference string) (AccountNamespace, error) {
	if len(reference) != len("acct_v1_")+sha256.Size*2 || !strings.HasPrefix(reference, "acct_v1_") {
		return AccountNamespace{}, ErrAccountNamespaceInvalid
	}
	for _, c := range reference[len("acct_v1_"):] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return AccountNamespace{}, ErrAccountNamespaceInvalid
		}
	}
	return AccountNamespace{reference: reference}, nil
}

// OpenAccountLedger isolates every existing SQLite operation (including cursor,
// runs, observations, statistics and purge) by database, avoiding unscoped SQL
// accidentally crossing accounts. It never imports or opens the legacy ledger
// at stateDir/coupangctl.sqlite3. Public price storage remains a separate choice
// for the composition root. This API does not select a currently logged-in user.
func OpenAccountLedger(ctx context.Context, stateDir string, namespace AccountNamespace) (*SQLite, error) {
	if _, err := ParseAccountNamespace(namespace.reference); err != nil || !filepath.IsAbs(stateDir) {
		return nil, ErrAccountNamespaceInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	root := filepath.Join(stateDir, "accounts")
	dir := filepath.Join(root, namespace.reference)
	for _, path := range []string{root, dir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, errors.New("cannot create private account directory")
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("account directory must be a real directory")
		}
	}
	path := filepath.Join(dir, "ledger.sqlite3")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := initializeAccountLedger(ctx, dir, path, namespace); err != nil {
			return nil, err
		}
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("account ledger must be a regular file")
	}
	// Validate existing files read-only before Open can migrate them. A copied
	// legacy or another-account DB must never be silently adopted or modified.
	readOnlyURL := &url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", readOnlyURL.String())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return nil, ErrAccountLedgerUnavailable
	}
	var hasBinding bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='account_ledger_binding')`).Scan(&hasBinding); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrAccountLedgerUnavailable
	}
	if !hasBinding {
		return nil, ErrAccountLedgerUnbound
	}
	var reference string
	err = db.QueryRowContext(ctx, `SELECT namespace FROM account_ledger_binding WHERE id=1`).Scan(&reference)
	db.Close()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAccountLedgerUnbound
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrAccountLedgerUnavailable
	}
	if reference != namespace.reference {
		return nil, ErrAccountLedgerMismatch
	}
	return Open(ctx, path)
}

// Publish a completely initialized, closed SQLite file without overwriting a
// concurrent winner. A crash before publication cannot strand an unbound final
// ledger; each initializer owns only its temporary files.
func initializeAccountLedger(ctx context.Context, dir, path string, namespace AccountNamespace) error {
	file, err := os.CreateTemp(dir, ".initializing-*.sqlite3")
	if err != nil {
		return errors.New("cannot prepare account ledger")
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	defer os.Remove(temporary + "-wal")
	defer os.Remove(temporary + "-shm")
	defer os.Remove(temporary + "-journal")
	if err := file.Close(); err != nil {
		return errors.New("cannot close account ledger initializer")
	}
	ledger, err := Open(ctx, temporary)
	if err != nil {
		return err
	}
	if err := ledger.bindAccountNamespace(ctx, namespace, true); err != nil {
		ledger.Close()
		return err
	}
	if err := ledger.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Link(temporary, path); err != nil && !errors.Is(err, os.ErrExist) {
		return errors.New("cannot publish account ledger")
	}
	return nil
}

func (s *SQLite) bindAccountNamespace(ctx context.Context, namespace AccountNamespace, created bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if created {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE account_ledger_binding (
			id INTEGER PRIMARY KEY CHECK(id = 1), namespace TEXT NOT NULL UNIQUE
		)`); err != nil {
			return errors.New("cannot initialize account ledger binding")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_ledger_binding(id,namespace) VALUES (1,?)`, namespace.reference); err != nil {
			return errors.New("cannot record account ledger binding")
		}
	}
	var reference string
	if err := tx.QueryRowContext(ctx, `SELECT namespace FROM account_ledger_binding WHERE id=1`).Scan(&reference); err != nil {
		return ErrAccountLedgerUnbound
	}
	if reference != namespace.reference {
		return ErrAccountLedgerMismatch
	}
	return tx.Commit()
}
