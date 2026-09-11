package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

var ErrAccountKeyUnavailable = errors.New("local account namespace key is unavailable or unsafe")
var ErrAccountKeyRecoveryRequired = errors.New("restore the original local account namespace key before opening account history")

const accountKeyFilename = "account-namespace.key"
const accountKeyMagic = "CPACTK01"
const accountKeyRecordSize = len(accountKeyMagic) + 32 + sha256.Size

// OpenCoupangAccountLedger is the composition seam for an adapter that has
// already verified a stable source-native subject. It reuses one installation's
// local key across processes. It does not authenticate the subject or silently
// select an account, and never falls back to the unverified legacy database.
func OpenCoupangAccountLedger(ctx context.Context, stateDir string, verifiedSourceSubject []byte) (*SQLite, AccountNamespace, error) {
	// Reject malformed identity before creating any key or database state.
	if len(verifiedSourceSubject) == 0 || len(verifiedSourceSubject) > 4096 || !filepath.IsAbs(stateDir) {
		return nil, AccountNamespace{}, ErrAccountNamespaceInvalid
	}
	key, err := localAccountKey(ctx, stateDir)
	if err != nil {
		return nil, AccountNamespace{}, err
	}
	defer clear(key[:])
	namespace, err := DeriveCoupangAccountNamespace(key[:], verifiedSourceSubject)
	if err != nil {
		return nil, AccountNamespace{}, err
	}
	ledger, err := OpenAccountLedger(ctx, stateDir, namespace)
	if err != nil {
		return nil, AccountNamespace{}, err
	}
	return ledger, namespace, nil
}

func localAccountKey(ctx context.Context, stateDir string) ([32]byte, error) {
	var empty [32]byte
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return empty, ErrAccountKeyUnavailable
	}
	info, err := os.Lstat(stateDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return empty, ErrAccountKeyUnavailable
	}
	path := filepath.Join(stateDir, accountKeyFilename)
	key, err := readAccountKey(path)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return empty, err
	}
	// Missing key with any prior account state is recovery, not first setup.
	// Generating a new key would silently strand the old namespaces.
	entries, err := os.ReadDir(filepath.Join(stateDir, "accounts"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return empty, ErrAccountKeyUnavailable
	}
	if len(entries) != 0 {
		// Another process may have published the key and opened an account
		// between our first read and the directory check.
		if key, err := readAccountKey(path); err == nil {
			return key, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return empty, err
		}
		return empty, ErrAccountKeyRecoveryRequired
	}
	if err := publishAccountKey(ctx, stateDir, path); err != nil {
		return empty, err
	}
	return readAccountKey(path)
}

func readAccountKey(path string) ([32]byte, error) {
	var key [32]byte
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return key, os.ErrNotExist
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return key, ErrAccountKeyUnavailable
	}
	file, err := os.Open(path)
	if err != nil {
		return key, ErrAccountKeyUnavailable
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return key, ErrAccountKeyUnavailable
	}
	record, err := io.ReadAll(io.LimitReader(file, int64(accountKeyRecordSize+1)))
	defer clear(record)
	if err != nil || len(record) != accountKeyRecordSize || !bytes.Equal(record[:len(accountKeyMagic)], []byte(accountKeyMagic)) {
		return key, ErrAccountKeyRecoveryRequired
	}
	digest := sha256.Sum256(record[:len(accountKeyMagic)+32])
	if subtle.ConstantTimeCompare(digest[:], record[len(accountKeyMagic)+32:]) != 1 {
		return key, ErrAccountKeyRecoveryRequired
	}
	copy(key[:], record[len(accountKeyMagic):len(accountKeyMagic)+32])
	return key, nil
}

// The checksum detects accidental damage, not tampering by a process with
// access to this private directory. The key is local secret state, not a public
// artifact, source identifier, or credential sent to Coupang.
func publishAccountKey(ctx context.Context, stateDir, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	record := make([]byte, accountKeyRecordSize)
	defer clear(record)
	copy(record, accountKeyMagic)
	if _, err := rand.Read(record[len(accountKeyMagic) : len(accountKeyMagic)+32]); err != nil {
		return ErrAccountKeyUnavailable
	}
	digest := sha256.Sum256(record[:len(accountKeyMagic)+32])
	copy(record[len(accountKeyMagic)+32:], digest[:])
	file, err := os.CreateTemp(stateDir, ".account-key-*.tmp")
	if err != nil {
		return ErrAccountKeyUnavailable
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(record); err != nil {
		file.Close()
		return ErrAccountKeyUnavailable
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return ErrAccountKeyUnavailable
	}
	if err := file.Close(); err != nil {
		return ErrAccountKeyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Link publishes the complete record only if no concurrent writer won.
	// Existing keys, including malformed files, are never overwritten.
	if err := os.Link(file.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
		return ErrAccountKeyUnavailable
	}
	return nil
}
