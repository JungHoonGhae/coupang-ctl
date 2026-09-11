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

func TestRetiredAsideRejectsOtherBrowserModesAndSecretsBeforeAcquisition(t *testing.T) {
	for _, args := range [][]string{
		{"auth", "status", "--aside", "--apple-events"},
		{"login", "--aside", "--phone"},
		{"login", "--aside", "--link", "--qr-output", "private.png"},
		{"login", "--aside", "--qr-output", "private.png"},
		{"products", "cart-add", "--aside"},
	} {
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), args, &out, &stderr, "test"); err == nil {
			t.Fatalf("accepted %v", args)
		}
		if out.Len() != 0 || stderr.Len() != 0 {
			t.Fatal("unexpected output or prompt")
		}
	}
}

func TestCamofoxLoginChoice(t *testing.T) {
	for _, args := range [][]string{nil, {"--manual"}, {"--qr"}, {"--link"}, {"--qr", "--link"}} {
		var output bytes.Buffer
		r, err := camofoxLoginRequest(args, &output)
		wantLink := false
		for _, arg := range args {
			wantLink = wantLink || arg == "--link"
		}
		if err != nil || (r.PresentQRLink != nil) != wantLink || output.Len() != 0 {
			t.Fatal("login choice not preserved")
		}
	}
}

func TestCamofoxSMSAndOTPRejectedWithoutRuntimeOrPrivateInput(t *testing.T) {
	state := filepath.Join(t.TempDir(), "unused-state")
	t.Setenv("COUPANGCTL_STATE_DIR", state)
	for _, args := range [][]string{{"login", "--phone"}, {"login", "--sms"}, {"login", "--otp"}, {"auth", "ensure", "--phone"}} {
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), args, &out, &stderr, "test"); !errors.Is(err, browser.ErrCamofoxUnsupported) {
			t.Fatalf("unsupported authentication choice did not fail before runtime: %v", err)
		}
		if out.Len() != 0 || stderr.Len() != 0 {
			t.Fatal("unsupported authentication produced a prompt")
		}
	}
	if _, err := os.Stat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unsupported authentication created state")
	}
	var help bytes.Buffer
	if err := Run(context.Background(), []string{"login", "--help"}, &help, &bytes.Buffer{}, "test"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help.String(), "--phone") || !strings.Contains(help.String(), "--manual") || !strings.Contains(help.String(), "--qr --link") {
		t.Fatal("help does not match supported login choices")
	}
}
