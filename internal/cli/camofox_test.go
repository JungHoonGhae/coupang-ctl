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

func TestCamofoxNoConfigurationOrConflictingModeNeverFallsBack(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"products", "search", "--camofox", "--query", "synthetic"}, &stdout, &stderr, "test")
	if !errors.Is(err, browser.ErrCamofoxUnavailable) || stdout.Len() != 0 {
		t.Fatal("unconfigured browser did not fail closed")
	}
	err = Run(context.Background(), []string{"products", "search", "--camofox", "--aside"}, &stdout, &stderr, "test")
	if err == nil || errors.Is(err, browser.ErrCamofoxUnavailable) {
		t.Fatal("conflicting modes reached setup")
	}
	for _, failure := range []error{browser.ErrBrowserAccessDenied, browser.ErrAuthenticationRequired, browser.ErrSearchFacetUnavailable} {
		var out bytes.Buffer
		WriteCommandError(&out, []string{"products", "search"}, &camofoxReadError{cause: failure})
		if strings.Contains(out.String(), "--headed") || strings.Contains(out.String(), "--current-browser") || strings.Contains(out.String(), "internal_error") {
			t.Fatal("wrong recovery path")
		}
	}
}

func TestCamofoxRuntimeErrorsNeverExposeUpstreamDetails(t *testing.T) {
	var out bytes.Buffer
	WriteError(&out, &browser.CamofoxError{Reason: "synthetic-private-value"})
	if strings.Contains(out.String(), "synthetic-private-value") || !strings.Contains(out.String(), "camofox_request_failed") {
		t.Fatal("runtime reason was not constrained to public error codes")
	}
}

func TestCamofoxCancellationIsNotAnInternalError(t *testing.T) {
	var out bytes.Buffer
	WriteCommandError(&out, []string{"login", "--qr", "--link"}, &camofoxReadError{cause: context.Canceled})
	if !strings.Contains(out.String(), "operation_cancelled") || strings.Contains(out.String(), "internal_error") {
		t.Fatal("cancellation not classified")
	}
}

func TestCamofoxFullSelectionCoversCLIAndMCPWithoutLegacyFallback(t *testing.T) {
	dir := t.TempDir()
	config := `{"schema_version":1,"node":"/node","server":"/server.js","engine_dir":"/engine","user_id":"synthetic","default_browser":true}`
	if err := os.WriteFile(filepath.Join(dir, "camofox.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COUPANGCTL_STATE_DIR", dir)
	var out, stderr bytes.Buffer
	err := Run(context.Background(), []string{"account", "snapshot"}, &out, &stderr, "test")
	if !errors.Is(err, browser.ErrCamofoxUnsupported) || out.Len() != 0 || stderr.Len() != 0 {
		t.Fatal("unsupported default operation fell through")
	}
	out.Reset()
	if err := Run(context.Background(), []string{"login", "--help"}, &out, &stderr, "test"); err != nil || !strings.Contains(out.String(), "Camofox") {
		t.Fatal("default login help not Camofox")
	}
	out.Reset()
	// All executable paths above are deliberately nonexistent. Local ledger
	// reads must still work and must not try to launch a browser.
	if err := Run(context.Background(), []string{"orders", "stats"}, &out, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\"order_count\": 0") {
		t.Fatal("wrong isolated ledger")
	}
}
