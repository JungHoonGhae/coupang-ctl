package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type fakeBrowser struct {
	status   BrowserStatus
	inspect  error
	login    error
	loginRan bool
	loginReq core.LoginRequest
	verify   error
	verified bool
}

func (f *fakeBrowser) Inspect(context.Context) (BrowserStatus, error) {
	return f.status, f.inspect
}

func (f *fakeBrowser) Login(_ context.Context, request core.LoginRequest) error {
	f.loginRan = true
	f.loginReq = request
	return f.login
}

func (f *fakeBrowser) Verify(context.Context) error {
	f.verified = true
	return f.verify
}

func TestStatusLiveChecksAndRefreshesStoredBrowserSession(t *testing.T) {
	fixed := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	browser := &fakeBrowser{status: BrowserStatus{Name: "Chrome", ProfilePresent: true}}
	service := NewService(browser)
	service.now = func() time.Time { return fixed }

	got, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.State != core.AuthVerified {
		t.Fatalf("state = %q, want %q", got.State, core.AuthVerified)
	}
	if !got.ProfilePresent || got.Browser != "Chrome" {
		t.Fatalf("unexpected status: %#v", got)
	}
	if !got.CheckedAt.Equal(fixed.UTC()) {
		t.Fatalf("checked_at = %s, want %s", got.CheckedAt, fixed.UTC())
	}
	if got.NextAction != "the read-only browser session is available" {
		t.Fatalf("next_action = %q", got.NextAction)
	}
	if !browser.verified {
		t.Fatal("status did not perform the protected session check")
	}
}

func TestStatusReportsExpiredStoredSessionWithoutClaimingSuccess(t *testing.T) {
	browser := &fakeBrowser{
		status: BrowserStatus{Name: "Chrome", ProfilePresent: true},
		verify: core.ErrAuthenticationRequired,
	}
	service := NewService(browser)
	got, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.State != core.AuthUnverified || !got.ProfilePresent {
		t.Fatalf("unexpected status: %#v", got)
	}
	if got.NextAction != "run `coupangctl login` to renew the expired session" {
		t.Fatalf("next_action = %q", got.NextAction)
	}
}

func TestStatusReportsBackgroundAccessBlockWithoutOpeningLogin(t *testing.T) {
	browser := &fakeBrowser{
		status: BrowserStatus{Name: "Chrome", ProfilePresent: true},
		verify: core.ErrBrowserAccessDenied,
	}
	service := NewService(browser)
	got, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.State != core.AuthAccessBlocked || !got.ProfilePresent {
		t.Fatalf("unexpected status: %#v", got)
	}
	if got.NextAction != "retry later, or explicitly run `coupangctl auth verify --headed` when an interactive check is acceptable" {
		t.Fatalf("next_action = %q", got.NextAction)
	}
}

func TestVerifyReportsAuthenticatedOnlyAfterReadOnlyCheck(t *testing.T) {
	browser := &fakeBrowser{status: BrowserStatus{Name: "Chrome", ProfilePresent: true}}
	service := NewService(browser)
	got, err := service.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.State != core.AuthVerified || !got.ProfilePresent {
		t.Fatalf("unexpected verified status: %#v", got)
	}
}

func TestStatusReportsMissingProfile(t *testing.T) {
	service := NewService(&fakeBrowser{status: BrowserStatus{Name: "Chrome"}})
	got, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.State != core.AuthNotConfigured || got.ProfilePresent {
		t.Fatalf("unexpected status: %#v", got)
	}
}

func TestRecoverSkipsVisibleLoginWhenBackgroundSessionIsAlreadyReady(t *testing.T) {
	browser := &fakeBrowser{status: BrowserStatus{Name: "Chrome", ProfilePresent: true}}
	service := NewService(browser)

	got, err := service.Recover(context.Background(), core.AuthRecoveryRequest{Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.BeforeState != core.AuthVerified || got.State != core.AuthVerified {
		t.Fatalf("unexpected recovery states: %#v", got)
	}
	if got.VisibleBrowserOpened || browser.loginRan {
		t.Fatalf("recovery opened an unnecessary visible login: result=%#v browser=%#v", got, browser)
	}
}

func TestRecoverOpensQRLoginOnlyForConfirmedExpiredSession(t *testing.T) {
	browser := &fakeBrowser{
		status: BrowserStatus{Name: "Chrome", ProfilePresent: true},
		verify: core.ErrAuthenticationRequired,
	}
	service := NewService(browser)

	if _, err := service.Recover(context.Background(), core.AuthRecoveryRequest{}); !errors.Is(err, core.ErrInteractiveConfirmationRequired) {
		t.Fatalf("unconfirmed recovery error = %v, want %v", err, core.ErrInteractiveConfirmationRequired)
	}
	if browser.loginRan {
		t.Fatal("unconfirmed recovery opened a visible login")
	}

	got, err := service.Recover(context.Background(), core.AuthRecoveryRequest{Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.BeforeState != core.AuthUnverified || got.State != core.AuthVerified || !got.VisibleBrowserOpened {
		t.Fatalf("unexpected recovery result: %#v", got)
	}
	if !browser.loginRan || browser.loginReq.Mode != core.LoginModeQR {
		t.Fatalf("recovery login request = %#v", browser.loginReq)
	}
}

func TestRecoverDoesNotMisreadAccessBlockAsExpiredLogin(t *testing.T) {
	browser := &fakeBrowser{
		status: BrowserStatus{Name: "Chrome", ProfilePresent: true},
		verify: core.ErrBrowserAccessDenied,
	}
	service := NewService(browser)

	got, err := service.Recover(context.Background(), core.AuthRecoveryRequest{Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.BeforeState != core.AuthAccessBlocked || got.State != core.AuthAccessBlocked {
		t.Fatalf("unexpected recovery states: %#v", got)
	}
	if got.VisibleBrowserOpened || browser.loginRan {
		t.Fatalf("access block incorrectly opened login: result=%#v browser=%#v", got, browser)
	}
}

func TestLoginDelegatesToBrowser(t *testing.T) {
	browser := &fakeBrowser{}
	service := NewService(browser)
	request := core.LoginRequest{Mode: core.LoginModeQR}
	got, err := service.Login(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !browser.loginRan {
		t.Fatal("browser login was not called")
	}
	if browser.loginReq.Mode != request.Mode || browser.loginReq.QROutputPath != request.QROutputPath {
		t.Fatalf("login request = %#v, want %#v", browser.loginReq, request)
	}
	if got.State != core.AuthVerified {
		t.Fatalf("state = %q, want %q", got.State, core.AuthVerified)
	}
	if got.NextAction != "the protected read-only browser session is available" {
		t.Fatalf("next_action = %q", got.NextAction)
	}
	if got.Mode != core.LoginModeQR {
		t.Fatalf("mode = %q, want %q", got.Mode, core.LoginModeQR)
	}
}

func TestAutomatedPhoneLoginReturnsVerifiedAfterBrowserCheck(t *testing.T) {
	service := NewService(&fakeBrowser{})
	got, err := service.Login(context.Background(), validPhoneLoginRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.State != core.AuthVerified {
		t.Fatalf("state = %q, want %q", got.State, core.AuthVerified)
	}
}

func validPhoneLoginRequest() core.LoginRequest {
	return core.LoginRequest{
		Mode:  core.LoginModePhone,
		Phone: "01000000000",
		ReadOTP: func(context.Context) (string, error) {
			return "000000", nil
		},
	}
}

func TestBrowserErrorsAreReturned(t *testing.T) {
	want := errors.New("browser unavailable")
	service := NewService(&fakeBrowser{inspect: want, login: want})
	if _, err := service.Status(context.Background()); !errors.Is(err, want) {
		t.Fatalf("status error = %v, want %v", err, want)
	}
	if _, err := service.Login(context.Background(), core.LoginRequest{Mode: core.LoginModeQR}); !errors.Is(err, want) {
		t.Fatalf("login error = %v, want %v", err, want)
	}
}

func TestLoginRejectsUnknownModeBeforeOpeningBrowser(t *testing.T) {
	browser := &fakeBrowser{}
	service := NewService(browser)
	if _, err := service.Login(context.Background(), core.LoginRequest{Mode: "password"}); !errors.Is(err, core.ErrInvalidLoginMode) {
		t.Fatalf("login error = %v, want %v", err, core.ErrInvalidLoginMode)
	}
	if browser.loginRan {
		t.Fatal("browser was opened for an invalid login mode")
	}
}

func TestLoginRejectsQROutputForPhoneMode(t *testing.T) {
	browser := &fakeBrowser{}
	service := NewService(browser)
	request := core.LoginRequest{Mode: core.LoginModePhone, QROutputPath: "/synthetic/qr.png"}
	if _, err := service.Login(context.Background(), request); !errors.Is(err, core.ErrInvalidLoginRequest) {
		t.Fatalf("login error = %v, want %v", err, core.ErrInvalidLoginRequest)
	}
	if browser.loginRan {
		t.Fatal("browser was opened for an invalid QR output request")
	}
}
