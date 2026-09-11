package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
)

type orderPreviewSource struct {
	page  core.OrderPage
	err   error
	calls int
}

func (s *orderPreviewSource) FetchPage(_ context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	s.calls++
	if cursor != nil {
		return core.OrderPage{}, errors.New("unexpected cursor")
	}
	return s.page, s.err
}

func TestOrderPreviewCLIHelpAndArgumentsBeforeStateAccess(t *testing.T) {
	state := filepath.Join(t.TempDir(), "absent")
	t.Setenv("COUPANGCTL_STATE_DIR", state)
	for _, tail := range [][]string{{"--help"}, {"-h"}, {"--limit", "3"}, {"extra"}, {}} {
		args := append([]string{"orders", "preview"}, tail...)
		var out, stderr bytes.Buffer
		err := Run(context.Background(), args, &out, &stderr, "test")
		switch {
		case isFlagHelp(tail):
			if err != nil || !strings.Contains(out.String(), "No ledger") {
				t.Fatal("preview help requires setup")
			}
		case len(tail) == 0:
			if !errors.Is(err, browser.ErrCamofoxUnavailable) || out.Len() != 0 {
				t.Fatal("preview skipped source setup")
			}
		default:
			WriteCommandError(&stderr, args, err)
			if err == nil || out.Len() != 0 || !strings.Contains(stderr.String(), "invalid_command") {
				t.Fatal("invalid preview arguments lack usage error")
			}
		}
		if _, err := os.Stat(state); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("help, invalid arguments or missing setup created state")
		}
	}
}

func TestOrderPreviewCLIReturnsPrivatePageWithoutLedger(t *testing.T) {
	source := &orderPreviewSource{page: core.OrderPage{Orders: []core.Order{{SourceRef: core.OrderSourceReference("synthetic-preview"), PurchasedAt: "2026-09-01", Currency: "KRW", TotalAmount: 1000}}}}
	var out bytes.Buffer
	service := orders.NewWithPageSourceAndSyncSource(nil, source, core.SyncSourceCamofox)
	if err := runOrderPreview(context.Background(), &out, service); err != nil {
		t.Fatal(err)
	}
	var result core.OrderPreview
	if json.Unmarshal(out.Bytes(), &result) != nil || source.calls != 1 || result.OrderCount != 1 || result.Scope != "source_entry_page" || result.Visibility != "private_local" || result.Persisted || result.HistoryComplete || result.AccountIdentityVerified {
		t.Fatal("preview lost page scope or accessed ledger")
	}
	for _, failure := range []error{core.ErrPartialOrderData, core.ErrAuthenticationRequired, core.ErrBrowserAccessDenied, context.DeadlineExceeded} {
		out.Reset()
		source.err = errors.Join(failure, errors.New("synthetic-private-detail"))
		err := runOrderPreview(context.Background(), &out, service)
		if !errors.Is(err, failure) || out.Len() != 0 {
			t.Fatal("failed preview emitted rows")
		}
		var stderr bytes.Buffer
		WriteCommandError(&stderr, []string{"orders", "preview"}, &camofoxReadError{cause: err})
		if strings.Contains(stderr.String(), "synthetic-private") || strings.Contains(stderr.String(), "sync-status") {
			t.Fatal("preview error leaked data or advised a ledger operation")
		}
	}
}

func TestOrderPreviewCompositionDoesNotOpenExistingLedger(t *testing.T) {
	state := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", state)
	// Deliberately non-executable installation; no external program can start.
	cfg := browser.CamofoxConfig{SchemaVersion: 1, Node: filepath.Join(state, "absent-node"), Server: filepath.Join(state, "absent-server"), EngineDir: filepath.Join(state, "absent-engine"), UserID: "synthetic"}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(state, "camofox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(state, "camofox-coupangctl.sqlite3")
	want := []byte("synthetic legacy bytes, not a SQLite database")
	if err := os.WriteFile(db, want, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"orders", "preview"}, &out, &stderr, "test"); !errors.Is(err, browser.ErrCamofoxUnavailable) || out.Len() != 0 {
		t.Fatal("CLI touched ledger before source")
	}
	r := &camofoxMCPRuntime{}
	defer r.close()
	provider, err := r.factories().OrderPreview(context.Background(), "orders_preview")
	if err != nil || r.ledger != nil {
		t.Fatal("preview factory opened ledger")
	}
	if _, err = provider.Preview(context.Background()); !errors.Is(err, browser.ErrCamofoxUnavailable) || r.ledger != nil {
		t.Fatal("MCP preview touched ledger before source")
	}
	got, err := os.ReadFile(db)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("preview changed existing ledger")
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(db + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("preview created ledger sidecars")
		}
	}
}
