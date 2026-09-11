package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
)

func TestUnconfiguredReadsNeverSelectLegacyBrowser(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Also prevent an old implementation from starting a real app.
	for _, args := range [][]string{{"products", "search", "--query", "synthetic"}, {"auth", "verify"}} {
		var out, stderr bytes.Buffer
		err := Run(ctx, args, &out, &stderr, "test")
		if !errors.Is(err, browser.ErrCamofoxUnavailable) || out.Len() != 0 {
			t.Errorf("%s did not require Camofox: %v", args[0], err)
		}
	}
	// Starting MCP is not itself a source read. Cancellation still stops it
	// before stdio setup, and missing browser setup is handled per tool call.
	var out, stderr bytes.Buffer
	if err := Run(ctx, []string{"mcp"}, &out, &stderr, "test"); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("cancelled MCP bootstrap reached runtime setup")
	}
}

func TestLegacyBrowserEntryPointsAreRetiredBeforeSideEffects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", dir)
	t.Setenv("PATH", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, args := range [][]string{
		{"products", "search", "--aside"}, {"products", "search", "--headed=true"},
		{"orders", "sync", "--ordinary-browser"}, {"auth", "status", "--apple-events"},
		{"current-browser", "status"}, {"browser-bridge", "install"},
		{"shopping-connection", "status"}, {"chrome-extension://synthetic/"},
	} {
		var out, stderr bytes.Buffer
		err := Run(ctx, args, &out, &stderr, "test")
		if err == nil || !strings.Contains(err.Error(), "retired") || out.Len() != 0 {
			t.Errorf("legacy route not retired: %s", args[0])
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("retired command changed application state")
	}
}

func TestLocalAnalyticsDoNotRequireBrowserConfiguration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", dir)
	t.Setenv("PATH", t.TempDir())
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"orders", "stats"}, &out, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"order_count": 0`) {
		t.Fatal("missing empty local summary")
	}
	if _, err := os.Stat(filepath.Join(dir, "camofox-coupangctl.sqlite3")); err != nil {
		t.Fatal("analytics selected the wrong ledger")
	}
}
