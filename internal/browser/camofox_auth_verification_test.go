package browser

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestCamofoxVerifiesAuthenticationWithoutParsingOrders(t *testing.T) {
	calls := 0
	c := &Camofox{run: func(_ context.Context, script, mode string, present core.QRLinkPresenter) ([]byte, error) {
		calls++
		if mode != "orders" || present != nil {
			t.Fatal("authentication check changed the quiet runtime or requested QR material")
		}
		if !strings.Contains(script, "/ssr/api/member/auth") || strings.Contains(script, "orderList") {
			t.Error("authentication still depends on order parsing instead of source authentication evidence")
		}
		return []byte(`{"status":"ok","authenticated":true}`), nil
	}}
	if err := c.Verify(context.Background()); err != nil || calls != 1 {
		t.Fatalf("authenticated session without order data rejected: %v; calls=%d", err, calls)
	}
}

func TestCamofoxAuthenticationReplyIsStrictAndKeepsFailuresDistinct(t *testing.T) {
	for _, tc := range []struct {
		body string
		want error
	}{
		{`{"status":"ok","authenticated":true}`, nil},
		{`{"status":"ok"}`, ErrDocumentProtocol},
		{`{"status":"ok","authenticated":false}`, ErrDocumentProtocol},
		{`{"status":"ok","authenticated":"true"}`, ErrDocumentProtocol},
		{`{"status":"ok","authenticated":true,"page":{"orders":[]}}`, ErrDocumentProtocol},
		{`{"status":"ok","authenticated":true} {}`, ErrDocumentProtocol},
		{`{"status":"access_denied","authenticated":true}`, ErrDocumentProtocol},
		{`{"status":"authentication_required"}`, core.ErrAuthenticationRequired},
		{`{"status":"access_denied"}`, core.ErrBrowserAccessDenied},
		{`{"status":"authentication_data_missing"}`, core.ErrAuthenticationStatusUnavailable},
		{`{"status":"loading"}`, core.ErrAuthenticationStatusUnavailable},
		{`{"status":"unexpected"}`, core.ErrAuthenticationStatusUnavailable},
		{strings.Repeat(" ", 4097), ErrDocumentProtocol},
	} {
		c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
			return []byte(tc.body), nil
		}}
		if err := c.Verify(context.Background()); !errors.Is(err, tc.want) {
			t.Errorf("verification error %v, want %v", err, tc.want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &Camofox{run: func(context.Context, string, string, core.QRLinkPresenter) ([]byte, error) {
		t.Fatal("cancelled check invoked runtime")
		return nil, nil
	}}
	if err := c.Verify(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
}

func TestCamofoxUnknownAuthenticationNeverOpensLogin(t *testing.T) {
	calls := 0
	c := &Camofox{run: func(_ context.Context, _ string, mode string, _ core.QRLinkPresenter) ([]byte, error) {
		calls++
		if mode != "orders" {
			t.Fatal("unknown status opened interactive login")
		}
		return []byte(`{"status":"authentication_data_missing"}`), nil
	}}
	if err := c.Login(context.Background(), core.LoginRequest{Mode: core.LoginModeQR}); !errors.Is(err, core.ErrAuthenticationStatusUnavailable) || calls != 1 {
		t.Fatal("unknown authentication treated as expiration")
	}
}
