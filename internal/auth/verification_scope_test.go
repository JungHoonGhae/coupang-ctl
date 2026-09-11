package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestAuthenticationScopeIsPresentOnlyForVerifiedSessions(t *testing.T) {
	for _, tc := range []struct {
		present bool
		verify  error
		want    core.AuthState
	}{
		{false, nil, core.AuthNotConfigured}, {true, nil, core.AuthVerified},
		{true, core.ErrAuthenticationRequired, core.AuthUnverified}, {true, core.ErrBrowserAccessDenied, core.AuthAccessBlocked},
	} {
		b := &fakeBrowser{status: BrowserStatus{ProfilePresent: tc.present}, verify: tc.verify}
		s := NewService(b)
		status, err := s.Status(context.Background())
		if err != nil || status.State != tc.want {
			t.Fatal("incorrect auth state")
		}
		data, _ := json.Marshal(status)
		var wire map[string]any
		_ = json.Unmarshal(data, &wire)
		scope, present := wire["verification_scope"]
		if present != (tc.want == core.AuthVerified) || present && scope != core.AuthVerificationSession {
			t.Fatal("authentication scope claimed without verification")
		}
		if tc.want == core.AuthVerified {
			verified, err := s.Verify(context.Background())
			if err != nil || verified.VerificationScope != core.AuthVerificationSession {
				t.Fatal("verify lost scope")
			}
			login, err := s.Login(context.Background(), core.LoginRequest{Mode: core.LoginModeQR})
			if err != nil || login.VerificationScope != core.AuthVerificationSession {
				t.Fatal("login lost scope")
			}
			recovered, err := s.Recover(context.Background(), core.AuthRecoveryRequest{})
			if err != nil || recovered.VerificationScope != core.AuthVerificationSession || recovered.VisibleBrowserOpened {
				t.Fatal("quiet recovery lost scope")
			}
		}
	}
}

func TestUnknownAuthenticationStopsRecoveryBeforeInteractiveLogin(t *testing.T) {
	b := &fakeBrowser{status: BrowserStatus{ProfilePresent: true}, verify: core.ErrAuthenticationStatusUnavailable}
	_, err := NewService(b).Recover(context.Background(), core.AuthRecoveryRequest{Confirmed: true})
	if !errors.Is(err, core.ErrAuthenticationStatusUnavailable) || b.loginRan {
		t.Fatal("unknown authentication was treated as expired")
	}
}
